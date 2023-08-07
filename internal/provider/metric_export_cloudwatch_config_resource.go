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
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	sdk_resource "github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

var metricExportCloudWathConfigAttributes = map[string]schema.Attribute{
	"id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Cluster ID.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	},
	"role_arn": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The IAM role used to upload metric segments to the target AWS account.",
	},
	"target_region": schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "The specific AWS region that the metrics will be exported to.",
		Computed:            true,
	},
	"log_group_name": schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "The customized AWS CloudWatch log group name.",
		Computed:            true,
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

type metricExportCloudWatchConfigResource struct {
	provider *provider
}

func (r *metricExportCloudWatchConfigResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "Amazon CloudWatch metric export configuration for a cluster.",
		Attributes:  metricExportCloudWathConfigAttributes,
	}
}

func (r *metricExportCloudWatchConfigResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_metric_export_cloudwatch_config"
}

func (r *metricExportCloudWatchConfigResource) Configure(
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
func (r *metricExportCloudWatchConfigResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan ClusterCloudWatchMetricExportConfig
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
			"CloudWatch metric export services are only available for dedicated clusters",
		)
		return
	}

	if cluster.GetCloudProvider() != client.CLOUDPROVIDERTYPE_AWS {
		resp.Diagnostics.AddError(
			"Incompatible cluster type",
			"CloudWatch metric export services are only available for AWS clusters",
		)
		return
	}

	apiRequest := client.NewEnableCloudWatchMetricExportRequest(plan.RoleArn.ValueString())
	if !plan.TargetRegion.IsNull() && !plan.TargetRegion.IsUnknown() {
		apiRequest.SetTargetRegion(plan.TargetRegion.ValueString())
	}
	if !plan.LogGroupName.IsNull() && !plan.LogGroupName.IsUnknown() {
		apiRequest.SetLogGroupName(plan.LogGroupName.ValueString())
	}

	apiObj := &client.CloudWatchMetricExportInfo{}
	err = sdk_resource.RetryContext(ctx, clusterUpdateTimeout, retryEnableCloudWatchMetricExport(
		ctx, clusterUpdateTimeout, r.provider.service,
		clusterID, cluster, apiRequest, apiObj,
	))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling CloudWatch metric export", err.Error(),
		)
		return
	}

	err = sdk_resource.RetryContext(ctx, clusterUpdateTimeout,
		waitForCloudWatchMetricExportReadyFunc(ctx, plan.ID.ValueString(), r.provider.service, apiObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling CloudWatch metric export",
			fmt.Sprintf("Could not enable CloudWatch metric export: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state ClusterCloudWatchMetricExportConfig
	loadCloudWatchMetricExportIntoTerraformState(apiObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func retryEnableCloudWatchMetricExport(
	ctx context.Context,
	timeout time.Duration,
	cl client.Service,
	clusterID string,
	cluster *client.Cluster,
	apiRequest *client.EnableCloudWatchMetricExportRequest,
	apiObj *client.CloudWatchMetricExportInfo,
) sdk_resource.RetryFunc {
	return func() *sdk_resource.RetryError {
		apiResp, httpResp, err := cl.EnableCloudWatchMetricExport(ctx, clusterID, apiRequest)
		if err != nil {
			apiErrMsg := formatAPIErrorMessage(err)
			if (httpResp != nil && httpResp.StatusCode == http.StatusServiceUnavailable) ||
				strings.Contains(apiErrMsg, "lock") {
				// Wait for cluster to be ready.
				clusterErr := sdk_resource.RetryContext(ctx, clusterUpdateTimeout,
					waitForClusterReadyFunc(ctx, clusterID, cl, cluster))
				if clusterErr != nil {
					return sdk_resource.NonRetryableError(
						fmt.Errorf("error checking cluster availability: %s", clusterErr.Error()))
				}
				return sdk_resource.RetryableError(fmt.Errorf("cluster was not ready - trying again"))
			}

			return sdk_resource.NonRetryableError(
				fmt.Errorf("could not enable CloudWatch metric export: %v", apiErrMsg),
			)
		}
		*apiObj = *apiResp
		return nil
	}
}

func loadCloudWatchMetricExportIntoTerraformState(
	apiObj *client.CloudWatchMetricExportInfo, state *ClusterCloudWatchMetricExportConfig,
) {
	state.ID = types.StringValue(apiObj.GetClusterId())
	state.RoleArn = types.StringValue(apiObj.GetRoleArn())

	if apiObj.TargetRegion == nil {
		state.TargetRegion = types.StringNull()
	} else {
		state.TargetRegion = types.StringValue(apiObj.GetTargetRegion())
	}

	if apiObj.LogGroupName == nil {
		state.LogGroupName = types.StringNull()
	} else {
		state.LogGroupName = types.StringValue(apiObj.GetLogGroupName())
	}

	state.Status = types.StringValue(string(apiObj.GetStatus()))
	state.UserMessage = types.StringValue(apiObj.GetUserMessage())
}

func waitForCloudWatchMetricExportReadyFunc(
	ctx context.Context,
	clusterID string,
	cl client.Service,
	cloudWatchMetricExportInfo *client.CloudWatchMetricExportInfo,
) sdk_resource.RetryFunc {
	return func() *sdk_resource.RetryError {
		apiObj, httpResp, err := cl.GetCloudWatchMetricExportInfo(ctx, clusterID)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return sdk_resource.NonRetryableError(fmt.Errorf(
					"error getting CloudWatch metric export info: %s", formatAPIErrorMessage(err)))
			} else {
				return sdk_resource.RetryableError(fmt.Errorf(
					"encountered a server error while reading CloudWatch metric export status - trying again"))
			}
		}

		*cloudWatchMetricExportInfo = *apiObj
		switch cloudWatchMetricExportInfo.GetStatus() {
		case client.METRICEXPORTSTATUSTYPE_ERROR:
			errMsg := "an error occurred during CloudWatch metric export config update"
			if cloudWatchMetricExportInfo.GetUserMessage() != "" {
				errMsg = fmt.Sprintf("%s: %s", errMsg, cloudWatchMetricExportInfo.GetUserMessage())
			}
			return sdk_resource.NonRetryableError(fmt.Errorf(errMsg))
		case client.METRICEXPORTSTATUSTYPE_ENABLING, client.METRICEXPORTSTATUSTYPE_DISABLING:
			return sdk_resource.RetryableError(fmt.Errorf("the CloudWatch metric export is not ready yet"))
		default:
			return nil
		}
	}
}

// Read
func (r *metricExportCloudWatchConfigResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state ClusterCloudWatchMetricExportConfig
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || state.ID.IsNull() {
		return
	}
	clusterID := state.ID.ValueString()
	apiObj, httpResp, err := r.provider.service.GetCloudWatchMetricExportInfo(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"CloudWatch metric export config not found",
				fmt.Sprintf(
					"The CloudWatch metric export config with cluster ID %s is not found. Removing from state.",
					clusterID,
				))
			resp.State.RemoveResource(ctx)
			return
		} else {
			// Check cluster existence.
			cluster, clusterHttpResp, clusterErr := r.provider.service.GetCluster(ctx, clusterID)
			if clusterErr != nil {
				if clusterHttpResp != nil && clusterHttpResp.StatusCode == http.StatusNotFound {
					resp.Diagnostics.AddWarning(
						"Cluster not found",
						fmt.Sprintf("The CloudWatch metric export config's cluster with ID %s is not found. Removing from state.",
							clusterID,
						))
					resp.State.RemoveResource(ctx)
					return
				} else {
					resp.Diagnostics.AddError(
						"Error getting cluster",
						fmt.Sprintf("Unexpected error retrieving the CloudWatch metric export config's cluster: %s",
							formatAPIErrorMessage(clusterErr),
						))
				}
			}

			if cluster.State == client.CLUSTERSTATETYPE_DELETED {
				resp.Diagnostics.AddWarning(
					"Cluster deleted",
					fmt.Sprintf("The CloudWatch metric export config's cluster with ID %s was deleted. Removing from state.",
						clusterID,
					))
				resp.State.RemoveResource(ctx)
				return
			}

			resp.Diagnostics.AddError(
				"Error getting CloudWatch metric export info",
				fmt.Sprintf("Unexpected error retrieving CloudWatch metric export info: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	loadCloudWatchMetricExportIntoTerraformState(apiObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update
func (r *metricExportCloudWatchConfigResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	// Get plan values
	var plan ClusterCloudWatchMetricExportConfig
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state ClusterCloudWatchMetricExportConfig
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiRequest := client.NewEnableCloudWatchMetricExportRequest(plan.RoleArn.ValueString())
	if !plan.TargetRegion.IsNull() && !plan.TargetRegion.IsUnknown() {
		apiRequest.SetTargetRegion(plan.TargetRegion.ValueString())
	}
	if !plan.LogGroupName.IsNull() && !plan.LogGroupName.IsUnknown() {
		apiRequest.SetLogGroupName(plan.LogGroupName.ValueString())
	}

	clusterID := plan.ID.ValueString()
	cluster := &client.Cluster{}
	apiObj := &client.CloudWatchMetricExportInfo{}
	err := sdk_resource.RetryContext(ctx, clusterUpdateTimeout, retryEnableCloudWatchMetricExport(
		ctx, clusterUpdateTimeout, r.provider.service,
		clusterID, cluster, apiRequest, apiObj,
	))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling CloudWatch metric export", err.Error(),
		)
		return
	}

	err = sdk_resource.RetryContext(ctx, clusterUpdateTimeout,
		waitForCloudWatchMetricExportReadyFunc(ctx, clusterID, r.provider.service, apiObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling CloudWatch metric export",
			fmt.Sprintf("Could not enable CloudWatch metric export: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadCloudWatchMetricExportIntoTerraformState(apiObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Delete
func (r *metricExportCloudWatchConfigResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state ClusterCloudWatchMetricExportConfig
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ID.ValueString()
	cluster := &client.Cluster{}
	retry := func() sdk_resource.RetryFunc {
		return func() *sdk_resource.RetryError {
			_, httpResp, err := r.provider.service.DeleteCloudWatchMetricExport(ctx, clusterID)
			if err != nil {
				apiErrMsg := formatAPIErrorMessage(err)
				if httpResp != nil {
					if httpResp.StatusCode == http.StatusNotFound {
						// CloudWatch metric export config or cluster is already gone. Swallow the error.
						return nil
					}
					if httpResp.StatusCode == http.StatusServiceUnavailable ||
						strings.Contains(apiErrMsg, "lock") {
						// Wait for cluster to be ready.
						clusterErr := sdk_resource.RetryContext(ctx, clusterUpdateTimeout,
							waitForClusterReadyFunc(ctx, clusterID, r.provider.service, cluster))
						if clusterErr != nil {
							return sdk_resource.NonRetryableError(
								fmt.Errorf("error checking cluster availability: %s", clusterErr.Error()))
						}
						return sdk_resource.RetryableError(fmt.Errorf("cluster was not ready - trying again"))
					}
				}
				return sdk_resource.NonRetryableError(
					fmt.Errorf("could not delete CloudWatch metric export config: %v", apiErrMsg))
			}
			return nil
		}
	}

	err := sdk_resource.RetryContext(ctx, clusterUpdateTimeout, retry())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting CloudWatch metric export config", err.Error(),
		)
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

// Import state
func (r *metricExportCloudWatchConfigResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func NewMetricExportCloudWatchConfigResource() resource.Resource {
	return &metricExportCloudWatchConfigResource{}
}
