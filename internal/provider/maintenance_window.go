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
	"strconv"
	"strings"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v3/pkg/client"
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
		MarkdownDescription: "Cluster ID.",
	},
	"offset_duration": schema.Int64Attribute{
		Required:            true,
		MarkdownDescription: "Duration in seconds from the beginning of each Monday (UTC) after which the maintenance window starts.",
	},
	"window_duration": schema.Int64Attribute{
		Required:            true,
		MarkdownDescription: "Duration in seconds that the maintenance window will remain active for after it starts.",
	},
}

type maintenanceWindowResource struct {
	provider *provider
}

func (r *maintenanceWindowResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Maintenance window configuration for a cluster.",
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
	diags := req.Plan.Get(ctx, &plan)
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
	traceAPICall("GetMaintenanceWindow")
	mwinObj, httpResp, err := r.provider.service.GetMaintenanceWindow(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Maintenance window not found",
				fmt.Sprintf("Maintenance window for cluster ID %s not found. Maintenance window will be removed from state.", clusterID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting maintenance window",
				fmt.Sprintf("Unexpected error retrieving maintenance window: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	// API returns the JSON mapping of
	// google.golang.org/protobuf/types/known/durationpb.Duration.
	//
	// https://pkg.go.dev/google.golang.org/protobuf/types/known/durationpb#hdr-JSON_Mapping.
	if mwinObj.OffsetDuration != "" {
		offsetDuration, err := strconv.ParseFloat(strings.TrimSuffix(mwinObj.OffsetDuration, "s"), 64)
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to parse offset duration",
				fmt.Sprintf("The offset duration returned by the API could not be parsed: %v", err),
			)
			return
		}
		state.OffsetDuration = basetypes.NewInt64Value(int64(offsetDuration))
	} else {
		state.OffsetDuration = basetypes.NewInt64Null()
	}
	if mwinObj.WindowDuration != "" {
		windowDuration, err := strconv.ParseFloat(strings.TrimSuffix(mwinObj.WindowDuration, "s"), 64)
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to parse window duration",
				fmt.Sprintf("The window duration returned by the API could not be parsed: %v", err),
			)
			return
		}
		state.WindowDuration = basetypes.NewInt64Value(int64(windowDuration))
	} else {
		state.WindowDuration = basetypes.NewInt64Null()
	}

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
	// API expects the JSON mapping of
	// google.golang.org/protobuf/types/known/durationpb.Duration.
	//
	// https://pkg.go.dev/google.golang.org/protobuf/types/known/durationpb#hdr-JSON_Mapping.
	clientMaintenanceWindow := client.NewMaintenanceWindowWithDefaults()
	clientMaintenanceWindow.OffsetDuration = fmt.Sprintf("%ds", maintenanceWindow.OffsetDuration.ValueInt64())
	clientMaintenanceWindow.WindowDuration = fmt.Sprintf("%ds", maintenanceWindow.WindowDuration.ValueInt64())
	traceAPICall("SetMaintenanceWindow")
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
	clusterID := state.ID.ValueString()
	traceAPICall("DeleteMaintenanceWindow")
	_, httpResp, err := r.provider.service.DeleteMaintenanceWindow(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Maintenance window not found",
				fmt.Sprintf("Maintenance window for cluster ID %s not found. Maintenance window will be removed from state.", clusterID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error deleting maintenance window",
				fmt.Sprintf("Could not delete maintenance window: %v", formatAPIErrorMessage(err)),
			)
		}
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
