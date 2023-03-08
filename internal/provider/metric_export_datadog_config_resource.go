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

var metricExportDatadogConfigAttributes = map[string]schema.Attribute{
	"id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Cluster ID",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	},
	"site": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The Datadog region to export to",
	},
	"api_key": schema.StringAttribute{
		Required:            true,
		Sensitive:           true,
		MarkdownDescription: "A Datadog API key",
	},
	"status": schema.StringAttribute{
		Computed: true,
	},
	"user_message": schema.StringAttribute{
		Computed: true,
	},
}

type metricExportDatadogConfigResource struct {
	provider *provider
}

func (r *metricExportDatadogConfigResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Metric Export Datadog Config Resource",
		Attributes:          metricExportDatadogConfigAttributes,
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
			"Datadog metric export services are only available for dedicated clusters",
		)
		return
	}

	site, err := client.NewApiDatadogSiteFromValue(plan.Site.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error preparing Datadog metric export config",
			fmt.Sprintf("Invalid Datadog site: %s", err),
		)
		return
	}

	apiObj, _, err := r.provider.service.EnableDatadogMetricExport(
		ctx,
		plan.ID.ValueString(),
		client.NewEnableDatadogMetricExportRequest(plan.ApiKey.ValueString(), *site),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling Datadog metric export",
			fmt.Sprintf("Could not enable Datadog metric export: %v", formatAPIErrorMessage(err)),
		)
		return
	}

	err = sdk_resource.RetryContext(ctx, clusterUpdateTimeout,
		waitForDatdogMetricExportReadyFunc(ctx, plan.ID.ValueString(), r.provider.service, apiObj))
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
) sdk_resource.RetryFunc {
	return func() *sdk_resource.RetryError {
		apiObj, httpResp, err := cl.GetDatadogMetricExportInfo(ctx, clusterID)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return sdk_resource.NonRetryableError(fmt.Errorf(
					"error getting Datadog metric export info: %s", formatAPIErrorMessage(err)))
			} else {
				return sdk_resource.RetryableError(fmt.Errorf(
					"encountered a server error while reading Datadog metric export status - trying again"))
			}
		}

		*datadogMetricExportInfo = *apiObj
		switch datadogMetricExportInfo.GetStatus() {
		case client.METRICEXPORTSTATUS_ERROR:
			errMsg := "an error occurred during Datadog metric export config update"
			if datadogMetricExportInfo.GetUserMessage() != "" {
				errMsg = fmt.Sprintf("%s: %s", errMsg, datadogMetricExportInfo.GetUserMessage())
			}
			return sdk_resource.NonRetryableError(fmt.Errorf(errMsg))
		case client.METRICEXPORTSTATUS_ENABLING, client.METRICEXPORTSTATUS_DISABLING:
			return sdk_resource.RetryableError(fmt.Errorf("the Datadog metric export is not ready yet"))
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
	apiObj, httpResp, err := r.provider.service.GetDatadogMetricExportInfo(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Datadog metric export config not found",
				fmt.Sprintf(
					"the Datadog metric export config with cluster ID %s is not found. Removing from state.",
					clusterID,
				))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting Datadog metric export info",
				fmt.Sprintf("Unexpected error retrieving Datadog metric export info: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	// stateApiKey := state.ApiKey.ValueString()
	// // The API response includes only the last 4 digits of the Datadog API key.
	// truncApiKey := apiObj.GetApiKey()
	// if truncApiKey != stateApiKey[len(stateApiKey)-4:] {
	// 	resp.Diagnostics.AddError(
	// 		"Unexpected Datadog API key for Datadog metric export config",
	// 		fmt.Sprintf(
	// 			"the truncated Datadog API key returned for cluster ID %s should be a substring of the full Datadog API key "+
	// 				"in terraform state. Please update terraform state with the correct full Datadog API key.",
	// 			clusterID,
	// 		))
	// 	return
	// }
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

	site, err := client.NewApiDatadogSiteFromValue(plan.Site.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error preparing Datadog metric export config",
			fmt.Sprintf("Invalid Datadog site: %s", err),
		)
		return
	}

	apiObj, _, err := r.provider.service.EnableDatadogMetricExport(
		ctx,
		plan.ID.ValueString(),
		client.NewEnableDatadogMetricExportRequest(plan.ApiKey.ValueString(), *site),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling Datadog metric export",
			fmt.Sprintf("Could not enable Datadog metric export: %v", formatAPIErrorMessage(err)),
		)
		return
	}

	err = sdk_resource.RetryContext(ctx, clusterUpdateTimeout,
		waitForDatdogMetricExportReadyFunc(ctx, plan.ID.ValueString(), r.provider.service, apiObj))
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
	_, httpResp, err := r.provider.service.DeleteDatadogMetricExport(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			// Datadog metric export config or cluster is already gone. Swallow the error.
		} else {
			resp.Diagnostics.AddError(
				"Error deleting Datadog metric export config",
				fmt.Sprintf("Could not delete Datadog metric export config: %s", formatAPIErrorMessage(err)),
			)
			return
		}
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
