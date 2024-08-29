package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v2/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

const (
	metricExportEnableTimeout  = time.Minute      // This request just triggers an async job that rolls out the integration
	metricExportDisableTimeout = time.Minute      // Same as above. Different job, but the HTTP request behaves the same.
	metricExportStableTimeout  = 10 * time.Minute // This might take a while. Stability here means the integration is up and running
)

var metricExportPrometheusConfigAttributes = map[string]schema.Attribute{
	"id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Cluster ID.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	},

	"status": schema.StringAttribute{
		Computed:    true,
		Description: "The current state of the metric export configuration.  Possible values are: " + fmt.Sprintf("%#q", client.AllowedMetricExportStatusTypeEnumValues),
	},
	"targets": schema.MapAttribute{
		Computed:    true,
		ElementType: types.StringType,
		Description: "Represents prometheus scrape endpoint for each region. You can fetch endpoints either by executing" +
			" `terraform state show {resource}` or by enabling terraform logs.",
	},
}

type metricExportPrometheusConfigResource struct {
	provider *provider
}

func (r *metricExportPrometheusConfigResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "Prometheus metric export configuration for a cluster. This is only available for dedicated clusters with AWS and GCP cloud providers.",
		Attributes:  metricExportPrometheusConfigAttributes,
	}
}

func (r *metricExportPrometheusConfigResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_metric_export_prometheus_config"
}

func (r *metricExportPrometheusConfigResource) Configure(
	_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if r.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError("Internal provider error",
			fmt.Sprintf("Error in Configuration: expected %T but got %T", provider{}, req.ProviderData))
	}
}

func (r *metricExportPrometheusConfigResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan ClusterPrometheusMetricExportConfig
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ID.ValueString()
	// Check cluster
	cluster, _, err := r.provider.service.GetCluster(ctx, clusterID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting cluster",
			fmt.Sprintf("Could not retrieve cluster info: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	if cluster.Config.Serverless != nil {
		resp.Diagnostics.AddError(
			"Incompatible cluster type",
			"Prometheus metric export services are only available for dedicated clusters with AWS and GCP cloud providers",
		)
		return
	}

	apiObj := &client.PrometheusMetricExportInfo{}
	err = retry.RetryContext(ctx, metricExportEnableTimeout, retryEnablePrometheusMetricExport(
		ctx, r.provider.service, clusterID, cluster,
		apiObj,
	))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling Prometheus metric export", err.Error(),
		)
		return
	}

	err = retry.RetryContext(ctx, metricExportStableTimeout,
		waitForPrometheusMetricExportStableFunc(ctx, clusterID, r.provider.service, apiObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling Prometheus metric export",
			fmt.Sprintf("Could not enable Prometheus metric export: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// print enable prometheus metric export as part of tflogs to fetch scrape endpoints.
	// we are skipping error handling here as we are using it for logging & relying on standard libraries.
	rawResponse, _ := json.MarshalIndent(apiObj, "", "  ")
	tflog.Info(ctx, fmt.Sprintf("enable prometheus metric export response: %s", string(rawResponse)))

	var state ClusterPrometheusMetricExportConfig
	var targets types.Map
	targets, diags = types.MapValueFrom(ctx, types.StringType, apiObj.GetTargets())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError(
			"Error in parsing enable prometheus metric export",
			fmt.Sprintf(
				"There is an error in parsing enable prometheus metric export. response: %v",
				apiObj,
			))
		return
	}
	loadPrometheusMetricExportIntoTerraformState(types.StringValue(apiObj.GetClusterId()), types.StringValue(string(apiObj.GetStatus())), targets, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func retryEnablePrometheusMetricExport(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	cluster *client.Cluster,
	apiObj *client.PrometheusMetricExportInfo,
) retry.RetryFunc {
	return func() *retry.RetryError {
		apiResp, httpResp, err := cl.EnablePrometheusMetricExport(ctx, clusterID)
		if err != nil {
			apiErrMsg := formatAPIErrorMessage(err)
			if (httpResp != nil && httpResp.StatusCode == http.StatusServiceUnavailable) ||
				strings.Contains(apiErrMsg, "lock") {
				// Wait for cluster to be ready.
				clusterErr := retry.RetryContext(ctx, clusterUpdateTimeout,
					waitForClusterReadyFunc(ctx, clusterID, cl, cluster))
				if clusterErr != nil {
					return retry.NonRetryableError(
						fmt.Errorf("error checking cluster availability: %s", clusterErr.Error()))
				}
				return retry.RetryableError(fmt.Errorf("cluster was not ready - trying again"))
			}

			return retry.NonRetryableError(
				fmt.Errorf("could not enable Prometheus metric export: %v", apiErrMsg),
			)
		}
		*apiObj = *apiResp
		return nil
	}
}

func loadPrometheusMetricExportIntoTerraformState(
	ID types.String,
	status types.String,
	targets types.Map,
	state *ClusterPrometheusMetricExportConfig,
) {
	state.ID = ID
	state.Status = status
	state.Targets = targets
}

func waitForPrometheusMetricExportStableFunc(
	ctx context.Context,
	clusterID string,
	cl client.Service,
	prometheusMetricExportInfo *client.PrometheusMetricExportInfo,
) retry.RetryFunc {
	return func() *retry.RetryError {
		apiObj, httpResp, err := cl.GetPrometheusMetricExportInfo(ctx, clusterID)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return retry.NonRetryableError(fmt.Errorf(
					"error getting Prometheus metric export info: %s", formatAPIErrorMessage(err)))
			} else {
				// if this is a server error, we should retry
				tflog.Error(ctx, "encountered a server error while refreshing Prometheus metric export status. Retrying...")
				return retry.RetryableError(fmt.Errorf(
					"encountered a server error while refreshing Prometheus metric export status",
				))
			}
		}

		*prometheusMetricExportInfo = *apiObj
		switch prometheusMetricExportInfo.GetStatus() {
		case client.METRICEXPORTSTATUSTYPE_ERROR:
			errMsg := "an error occurred during Prometheus metric export config update"
			if prometheusMetricExportInfo.GetUserMessage() != "" {
				errMsg = fmt.Sprintf("%s: %s", errMsg, prometheusMetricExportInfo.GetUserMessage())
			}
			return retry.NonRetryableError(fmt.Errorf(errMsg))
		case client.METRICEXPORTSTATUSTYPE_ENABLING, client.METRICEXPORTSTATUSTYPE_DISABLING:
			return retry.RetryableError(fmt.Errorf("the Prometheus metric export is not ready yet"))
		default:
			return nil
		}
	}
}

func (r *metricExportPrometheusConfigResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state ClusterPrometheusMetricExportConfig
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || state.ID.IsNull() {
		return
	}

	clusterID := state.ID.ValueString()
	apiObj, httpResp, err := r.provider.service.GetPrometheusMetricExportInfo(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Prometheus metric export config not found",
				fmt.Sprintf(
					"The Prometheus metric export config's cluster with ID %s is not found. Removing from state.",
					clusterID,
				))
			resp.State.RemoveResource(ctx)
			return
		} else {
			resp.Diagnostics.AddError(
				"Error getting Prometheus metric export info",
				fmt.Sprintf("Unexpected error retrieving Prometheus metric export info: %s", formatAPIErrorMessage(err)))
			return

		}
	}
	if apiObj.GetStatus() == client.METRICEXPORTSTATUSTYPE_NOT_DEPLOYED {
		resp.Diagnostics.AddWarning(
			"Prometheus metric export config not found",
			fmt.Sprintf(
				"The Prometheus metric export config's cluster with ID %s is not found. Removing from state.",
				clusterID,
			))
		resp.State.RemoveResource(ctx)
		return
	}

	var targets types.Map
	targets, diags = types.MapValueFrom(ctx, types.StringType, apiObj.GetTargets())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError(
			"Error in parsing get prometheus metric export info",
			fmt.Sprintf(
				"There is an error in parsing get prometheus metric export details. response: %v",
				apiObj,
			))
		return
	}
	loadPrometheusMetricExportIntoTerraformState(types.StringValue(apiObj.GetClusterId()),
		types.StringValue(string(apiObj.GetStatus())), targets, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update resource is immutable so no action is necessary.
func (r *metricExportPrometheusConfigResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
}

func (r *metricExportPrometheusConfigResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state ClusterPrometheusMetricExportConfig
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ID.ValueString()
	cluster := &client.Cluster{}
	deletePromMetricExport := func() retry.RetryFunc {
		return func() *retry.RetryError {
			apiObj, httpResp, err := r.provider.service.DeletePrometheusMetricExport(ctx, clusterID)
			if err != nil {
				// This code block will handle below error scenarios and perform retries accordingly:
				// 1. cluster does not exist.
				// 2. control plane services are unavailable.
				// 3. there is an issue in client in request preparation.

				apiErrMsg := formatAPIErrorMessage(err)
				if httpResp != nil {
					if httpResp.StatusCode == http.StatusNotFound {
						// Prometheus metric export config or cluster is already gone. Swallow the error.
						return nil
					}

					if httpResp.StatusCode == http.StatusServiceUnavailable ||
						strings.Contains(apiErrMsg, "lock") {
						// Wait for cluster to be ready.
						clusterErr := retry.RetryContext(ctx, clusterUpdateTimeout,
							waitForClusterReadyFunc(ctx, clusterID, r.provider.service, cluster))
						if clusterErr != nil {
							return retry.NonRetryableError(
								fmt.Errorf("Error checking cluster availability: %s", clusterErr.Error()))
						}
						return retry.RetryableError(fmt.Errorf("cluster was not ready - trying again"))
					}
				}

				return retry.NonRetryableError(
					fmt.Errorf("could not delete Prometheus metric export config: %v", apiErrMsg))
			}

			if apiObj.GetStatus() == client.METRICEXPORTSTATUSTYPE_DISABLING {
				// We are passing empty PrometheusMetricExportInfo object as we don't need it in stage management.
				err = retry.RetryContext(ctx, metricExportStableTimeout,
					waitForPrometheusMetricExportStableFunc(ctx, clusterID, r.provider.service, client.NewPrometheusMetricExportInfoWithDefaults()))
				if err != nil {
					resp.Diagnostics.AddError(
						"Error in delete Prometheus metric export",
						fmt.Sprintf("Could not delete Prometheus metric export: %s", formatAPIErrorMessage(err)),
					)
				}
			}

			return nil
		}
	}

	err := retry.RetryContext(ctx, metricExportDisableTimeout, deletePromMetricExport())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Prometheus metric export config", err.Error(),
		)
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r *metricExportPrometheusConfigResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func NewMetricExportPrometheusConfigResource() resource.Resource {
	return &metricExportPrometheusConfigResource{}
}
