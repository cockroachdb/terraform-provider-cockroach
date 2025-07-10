/*
Copyright 2023 The Cockroach Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

var metricExportDatadogConfigAttributes = map[string]schema.Attribute{
	"id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Cluster ID.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	},
	"site": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The Datadog region to export to.",
	},
	"api_key": schema.StringAttribute{
		Required:            true,
		Sensitive:           true,
		MarkdownDescription: "A Datadog API key.",
	},
	"status": schema.StringAttribute{
		Computed:    true,
		Description: "Encodes the possible states that a metric export configuration can be in as it is created, deployed, and disabled.",
	},
	"user_message": schema.StringAttribute{
		Computed:    true,
		Description: "Elaborates on the metric export status and hints at how to fix issues that may have occurred during asynchronous operations.",
	},
}

type metricExportDatadogConfigResource struct {
	provider *provider
}

func (r *metricExportDatadogConfigResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "DataDog metric export configuration for a cluster.",
		Attributes:  metricExportDatadogConfigAttributes,
	}
}

func (r *metricExportDatadogConfigResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_metric_export_datadog_config"
}

func (r *metricExportDatadogConfigResource) Configure(
	_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if r.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError("Internal provider error",
			fmt.Sprintf("Error in Configure: expected %T but got %T", provider{}, req.ProviderData))
	}
}

// Create
func (r *metricExportDatadogConfigResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan ClusterDatadogMetricExportConfig
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ID.ValueString()
	// Check cluster
	traceAPICall("GetCluster")
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
			"Datadog metric export services are only available for dedicated clusters",
		)
		return
	}

	site, err := client.NewDatadogSiteTypeFromValue(plan.Site.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error preparing Datadog metric export config",
			fmt.Sprintf("Invalid Datadog site: %s", err),
		)
		return
	}

	apiObj := &client.DatadogMetricExportInfo{}
	err = retry.RetryContext(ctx, clusterUpdateTimeout, retryEnableDatadogMetricExport(
		ctx, clusterUpdateTimeout, r.provider.service, clusterID, cluster,
		client.NewEnableDatadogMetricExportRequest(plan.ApiKey.ValueString(), *site),
		apiObj,
	))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling Datadog metric export", err.Error(),
		)
		return
	}

	err = retry.RetryContext(ctx, clusterUpdateTimeout,
		waitForDatdogMetricExportReadyFunc(ctx, clusterID, r.provider.service, apiObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling Datadog metric export",
			fmt.Sprintf("Could not enable Datadog metric export: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state ClusterDatadogMetricExportConfig
	// The API returns a truncated API key, so use the plan value.
	state.ApiKey = plan.ApiKey
	loadDatadogMetricExportIntoTerraformState(apiObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func retryEnableDatadogMetricExport(
	ctx context.Context,
	timeout time.Duration,
	cl client.Service,
	clusterID string,
	cluster *client.Cluster,
	apiRequest *client.EnableDatadogMetricExportRequest,
	apiObj *client.DatadogMetricExportInfo,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("EnableDatadogMetricExport")
		apiResp, httpResp, err := cl.EnableDatadogMetricExport(ctx, clusterID, apiRequest)
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
				fmt.Errorf("could not enable Datadog metric export: %v", apiErrMsg),
			)
		}
		*apiObj = *apiResp
		return nil
	}
}

func loadDatadogMetricExportIntoTerraformState(
	apiObj *client.DatadogMetricExportInfo, state *ClusterDatadogMetricExportConfig,
) {
	state.ID = types.StringValue(apiObj.GetClusterId())
	state.Site = types.StringValue(string(apiObj.GetSite()))
	state.Status = types.StringValue(string(apiObj.GetStatus()))
	state.UserMessage = types.StringValue(apiObj.GetUserMessage())
}

func waitForDatdogMetricExportReadyFunc(
	ctx context.Context,
	clusterID string,
	cl client.Service,
	datadogMetricExportInfo *client.DatadogMetricExportInfo,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("GetDatadogMetricExportInfo")
		apiObj, httpResp, err := cl.GetDatadogMetricExportInfo(ctx, clusterID)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return retry.NonRetryableError(fmt.Errorf(
					"error getting Datadog metric export info: %s", formatAPIErrorMessage(err)))
			} else {
				return retry.RetryableError(fmt.Errorf(
					"encountered a server error while reading Datadog metric export status - trying again"))
			}
		}

		*datadogMetricExportInfo = *apiObj
		switch datadogMetricExportInfo.GetStatus() {
		case client.METRICEXPORTSTATUSTYPE_ERROR:
			errMsg := "an error occurred during Datadog metric export config update"
			if datadogMetricExportInfo.GetUserMessage() != "" {
				errMsg = fmt.Sprintf("%s: %s", errMsg, datadogMetricExportInfo.GetUserMessage())
			}
			return retry.NonRetryableError(errors.New(errMsg))
		case client.METRICEXPORTSTATUSTYPE_ENABLING, client.METRICEXPORTSTATUSTYPE_DISABLING:
			return retry.RetryableError(errors.New("the Datadog metric export is not ready yet"))
		default:
			return nil
		}
	}
}

// Read
func (r *metricExportDatadogConfigResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state ClusterDatadogMetricExportConfig
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || state.ID.IsNull() {
		return
	}

	clusterID := state.ID.ValueString()
	traceAPICall("GetDatadogMetricExportInfo")
	apiObj, httpResp, err := r.provider.service.GetDatadogMetricExportInfo(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Datadog metric export config not found",
				fmt.Sprintf(
					"The Datadog metric export config with cluster ID %s is not found. Removing from state.",
					clusterID,
				))
			resp.State.RemoveResource(ctx)
			return
		} else {
			// Check cluster existence.
			traceAPICall("GetCluster")
			cluster, clusterHttpResp, clusterErr := r.provider.service.GetCluster(ctx, clusterID)
			if clusterErr != nil {
				if clusterHttpResp != nil && clusterHttpResp.StatusCode == http.StatusNotFound {
					resp.Diagnostics.AddWarning(
						"Cluster not found",
						fmt.Sprintf("The Datadog metric export config's cluster with ID %s is not found. Removing from state.",
							clusterID,
						))
					resp.State.RemoveResource(ctx)
					return
				} else {
					resp.Diagnostics.AddError(
						"Error getting cluster",
						fmt.Sprintf("Unexpected error retrieving the Datadog metric export config's cluster: %s",
							formatAPIErrorMessage(clusterErr),
						))
				}
			}

			if cluster.State == client.CLUSTERSTATETYPE_DELETED {
				resp.Diagnostics.AddWarning(
					"Cluster deleted",
					fmt.Sprintf("The Datadog metric export config's cluster with ID %s was deleted. Removing from state.",
						clusterID,
					))
				resp.State.RemoveResource(ctx)
				return
			}

			resp.Diagnostics.AddError(
				"Error getting Datadog metric export info",
				fmt.Sprintf("Unexpected error retrieving Datadog metric export info: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	loadDatadogMetricExportIntoTerraformState(apiObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update
func (r *metricExportDatadogConfigResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	// Get plan values
	var plan ClusterDatadogMetricExportConfig
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state ClusterDatadogMetricExportConfig
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	site, err := client.NewDatadogSiteTypeFromValue(plan.Site.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error preparing Datadog metric export config",
			fmt.Sprintf("Invalid Datadog site: %s", err),
		)
		return
	}

	clusterID := plan.ID.ValueString()
	cluster := &client.Cluster{}
	apiObj := &client.DatadogMetricExportInfo{}
	err = retry.RetryContext(ctx, clusterUpdateTimeout, retryEnableDatadogMetricExport(
		ctx, clusterUpdateTimeout, r.provider.service, clusterID, cluster,
		client.NewEnableDatadogMetricExportRequest(plan.ApiKey.ValueString(), *site),
		apiObj,
	))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling Datadog metric export", err.Error(),
		)
		return
	}

	err = retry.RetryContext(ctx, clusterUpdateTimeout,
		waitForDatdogMetricExportReadyFunc(ctx, clusterID, r.provider.service, apiObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling Datadog metric export",
			fmt.Sprintf("Could not enable Datadog metric export: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// The API returns a truncated API key, so use the plan value.
	state.ApiKey = plan.ApiKey
	loadDatadogMetricExportIntoTerraformState(apiObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Delete
func (r *metricExportDatadogConfigResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state ClusterDatadogMetricExportConfig
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ID.ValueString()
	cluster := &client.Cluster{}
	retryFunc := func() retry.RetryFunc {
		return func() *retry.RetryError {
			traceAPICall("DeleteDatadogMetricExport")
			_, httpResp, err := r.provider.service.DeleteDatadogMetricExport(ctx, clusterID)
			if err != nil {
				apiErrMsg := formatAPIErrorMessage(err)
				if httpResp != nil {
					if httpResp.StatusCode == http.StatusNotFound {
						// Datadog metric export config or cluster is already gone. Swallow the error.
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
					fmt.Errorf("could not delete Datadog metric export config: %v", apiErrMsg))
			}
			return nil
		}
	}

	err := retry.RetryContext(ctx, clusterUpdateTimeout, retryFunc())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Datadog metric export config", err.Error(),
		)
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

// Import state
func (r *metricExportDatadogConfigResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func NewMetricExportDatadogConfigResource() resource.Resource {
	return &metricExportDatadogConfigResource{}
}
