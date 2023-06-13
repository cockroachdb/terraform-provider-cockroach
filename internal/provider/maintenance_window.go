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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var maintenanceWindowAttributes = map[string]schema.Attribute{
	"id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Cluster ID",
	},
	"offset_duration": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The offset duration is the duration in seconds from the beginning of each Monday (UTC) after which the maintenance window starts.",
	},
	"window_duration": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The window duration is the duration in seconds that the maintenance window will remain active for after it starts.",
	},
}

type maintenanceWindowResource struct {
	provider *provider
}

func (r *maintenanceWindowResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Maintenance window resource for a cluster.",
		Attributes:          maintenanceWindowAttributes,
	}
}

func (r *maintenanceWindowResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_maintenance_window"
}

func (r *maintenanceWindowResource) Configure(
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

func (r *maintenanceWindowResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan ClusterMaintenanceWindow
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.setMaintenanceWindow(ctx, &resp.State, &resp.Diagnostics, plan)
}

func (r *maintenanceWindowResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state ClusterMaintenanceWindow
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ID.ValueString()
	mwinObj, httpResp, err := r.provider.service.GetMaintenanceWindow(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Maintenance window not found",
				fmt.Sprintf("Maintenance window for cluster ID %s not found.", clusterID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting maintenance window",
				fmt.Sprintf("Unexpected error retrieving maintenance window: %s", formatAPIErrorMessage(err)))
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}
	state.OffsetDuration = basetypes.NewStringValue(mwinObj.OffsetDuration)
	state.WindowDuration = basetypes.NewStringValue(mwinObj.WindowDuration)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *maintenanceWindowResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	var plan ClusterMaintenanceWindow
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.setMaintenanceWindow(ctx, &resp.State, &resp.Diagnostics, plan)
}

func (r *maintenanceWindowResource) setMaintenanceWindow(
	ctx context.Context, state *tfsdk.State, diags *diag.Diagnostics, maintenanceWindow ClusterMaintenanceWindow,
) {
	clientMaintenanceWindow := client.NewMaintenanceWindowWithDefaults()
	clientMaintenanceWindow.OffsetDuration = maintenanceWindow.OffsetDuration.ValueString()
	clientMaintenanceWindow.WindowDuration = maintenanceWindow.WindowDuration.ValueString()
	_, _, err := r.provider.service.SetMaintenanceWindow(ctx, maintenanceWindow.ID.ValueString(), clientMaintenanceWindow)
	if err != nil {
		diags.AddError(
			"Error setting maintenance window",
			fmt.Sprintf("Could not set maintenance window: %v", formatAPIErrorMessage(err)),
		)
		return
	}
	diags.Append(state.Set(ctx, maintenanceWindow)...)
}

func (r *maintenanceWindowResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state ClusterMaintenanceWindow
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, _, err := r.provider.service.DeleteMaintenanceWindow(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting maintenance window",
			fmt.Sprintf("Could not delete maintenance window: %v", formatAPIErrorMessage(err)),
		)
		return
	}
}

func (r *maintenanceWindowResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func NewMaintenanceWindowResource() resource.Resource {
	return &maintenanceWindowResource{}
}
