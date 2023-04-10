/*
Copyright 2022 The Cockroach Authors

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
		MarkdownDescription: "Cluster ID",
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
		Computed: true,
	},
	"user_message": schema.StringAttribute{
		Computed: true,
	},
}

type metricExportCloudWatchConfigResource struct {
	provider *provider
}

func (r *metricExportCloudWatchConfigResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Metric Export CloudWatch Config Resource",
		Attributes:          metricExportCloudWathConfigAttributes,
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
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check cluster
	cluster, _, err := r.provider.service.GetCluster(ctx, plan.ID.ValueString())
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
	apiObj, _, err := r.provider.service.EnableCloudWatchMetricExport(ctx, plan.ID.ValueString(), apiRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling CloudWatch metric export",
			fmt.Sprintf("Could not enable CloudWatch metric export: %v", formatAPIErrorMessage(err)),
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
					"the CloudWatch metric export config with cluster ID %s is not found. Removing from state.",
					clusterID,
				))
			resp.State.RemoveResource(ctx)
		} else {
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
	apiObj, _, err := r.provider.service.EnableCloudWatchMetricExport(ctx, plan.ID.ValueString(), apiRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling CloudWatch metric export",
			fmt.Sprintf("Could not enable CloudWatch metric export: %v", formatAPIErrorMessage(err)),
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
	_, httpResp, err := r.provider.service.DeleteCloudWatchMetricExport(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			// CloudWatch metric export config or cluster is already gone. Swallow the error.
		} else {
			resp.Diagnostics.AddError(
				"Error deleting CloudWatch metric export config",
				fmt.Sprintf("Could not delete CloudWatch metric export config: %s", formatAPIErrorMessage(err)),
			)
			return
		}
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
