package provider

import (
	"context"
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
)

type blackoutWindowResource struct {
	provider *provider
}

func NewBlackoutWindowResource() resource.Resource {
	return &blackoutWindowResource{}
}

func (r *blackoutWindowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Blackout windows schedule a period of up to 14 days for an ADVANCED cluster to block high-risk operations that impact SQL availability or require CRDB pod restarts.

Two consecutive maintenance windows cannot be covered by blackout windows.`,
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the cluster the blackout window applies to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: uuidValidator,
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique blackout window identifier returned by the API.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: uuidValidator,
			},
			"start_time": schema.StringAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "The UTC start time for the blackout window in RFC3339 format (e.g. `2025-03-15T09:00:00Z`). Must be scheduled at least seven days in advance.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"end_time": schema.StringAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "The UTC end time for the blackout window in RFC3339 format (e.g. `2025-03-18T09:00:00Z`). Must fall within 14 days of `start_time` and no later than three months from now.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *blackoutWindowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_blackout_window"
}

func (r *blackoutWindowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if r.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError("Internal provider error",
			fmt.Sprintf("Error in Configure: expected %T but got %T", provider{}, req.ProviderData))
	}
}

func (r *blackoutWindowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan BlackoutWindow
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !IsKnown(plan.StartTime) {
		resp.Diagnostics.AddAttributeError(path.Root("start_time"), "Missing start_time", "Provide start_time in RFC3339 format when creating a blackout window.")
		return
	}
	startTime, err := parseRFC3339Timestamp(plan.StartTime, "start_time")
	if err != nil {
		resp.Diagnostics.AddError("Invalid start_time",
			fmt.Sprintf("could not parse start_time: %v", err))
		return
	}

	if !IsKnown(plan.EndTime) {
		resp.Diagnostics.AddAttributeError(path.Root("end_time"), "Missing end_time", "Provide end_time in RFC3339 format when creating a blackout window")
		return
	}
	endTime, err := parseRFC3339Timestamp(plan.EndTime, "end_time")
	if err != nil {
		resp.Diagnostics.AddError("Invalid end_time",
			fmt.Sprintf("could not parse end_time: %v", err))
		return
	}

	traceAPICall("CreateBlackoutWindow")
	createReq := &client.CreateBlackoutWindowRequest{
		StartTime: startTime,
		EndTime:   endTime,
	}
	createResp, _, err := r.provider.service.CreateBlackoutWindow(ctx, plan.ClusterID.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating blackout window",
			fmt.Sprintf("Could not create blackout window: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state BlackoutWindow
	loadBlackoutWindowToTerraformState(createResp, plan.ClusterID.ValueString(), &state)

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *blackoutWindowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state BlackoutWindow
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ID.IsNull() || state.ClusterID.IsNull() {
		return
	}

	traceAPICall("GetBlackoutWindow")
	blackoutWindowObj, httpResp, err := r.provider.service.GetBlackoutWindow(ctx, state.ClusterID.ValueString(), state.ID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Blackout window not found",
				fmt.Sprintf("Blackout window with ID %s was not found. Removing from state.", state.ID.ValueString()),
			)
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting blackout window",
				fmt.Sprintf("Unexpected error retrieving blackout window: %s", formatAPIErrorMessage(err)),
			)
		}
		return
	}

	loadBlackoutWindowToTerraformState(blackoutWindowObj, state.ClusterID.ValueString(), &state)

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *blackoutWindowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Get the planned blackout window update specification.
	var plan BlackoutWindow
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the current state.
	var state BlackoutWindow
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := &client.UpdateBlackoutWindowRequest{}
	// If no fields are changing, we do a no-op.
	needsUpdate := false

	if IsKnown(plan.StartTime) {
		startTime, err := parseRFC3339Timestamp(plan.StartTime, "start_time")
		if err != nil {
			resp.Diagnostics.AddError("Invalid start_time",
				fmt.Sprintf("could not parse start_time: %v", err))
			return
		}
		if plan.StartTime.ValueString() != state.StartTime.ValueString() {
			updateReq.SetStartTime(startTime)
			needsUpdate = true
		}
	}

	if IsKnown(plan.EndTime) {
		endTime, err := parseRFC3339Timestamp(plan.EndTime, "end_time")
		if err != nil {
			resp.Diagnostics.AddError("Invalid end_time",
				fmt.Sprintf("could not parse end_time: %v", err))
			return
		}
		if plan.EndTime.ValueString() != state.EndTime.ValueString() {
			updateReq.SetEndTime(endTime)
			needsUpdate = true
		}
	}

	if !needsUpdate {
		return
	}

	traceAPICall("UpdateBlackoutWindow")
	blackoutWindowObj, _, err := r.provider.service.UpdateBlackoutWindow(
		ctx,
		state.ClusterID.ValueString(),
		state.ID.ValueString(),
		updateReq,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating blackout window",
			fmt.Sprintf("Could not update blackout window: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadBlackoutWindowToTerraformState(blackoutWindowObj, state.ClusterID.ValueString(), &state)

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *blackoutWindowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BlackoutWindow
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ID.IsNull() || state.ClusterID.IsNull() {
		return
	}

	traceAPICall("DeleteBlackoutWindow")
	_, httpResp, err := r.provider.service.DeleteBlackoutWindow(ctx, state.ClusterID.ValueString(), state.ID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Blackout window already removed",
				fmt.Sprintf("Blackout window %s was not found. Removing from state.", state.ID.ValueString()),
			)
		} else {
			resp.Diagnostics.AddError(
				"Error deleting blackout window",
				fmt.Sprintf("Could not delete blackout window: %s", formatAPIErrorMessage(err)),
			)
			return
		}
	}

	resp.State.RemoveResource(ctx)
}

func (r *blackoutWindowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid blackout window import ID format",
			`Use "<cluster_id>:<blackout_window_id>" when importing a blackout window.`,
		)
		return
	}

	clusterID := strings.TrimSpace(parts[0])
	windowID := strings.TrimSpace(parts[1])

	if clusterID == "" || windowID == "" {
		resp.Diagnostics.AddError(
			"Invalid blackout window import ID",
			"Both cluster_id and blackout_window_id must be provided.",
		)
		return
	}

	// Remaining attributes will be populated by the Read method.
	blackoutWindow := BlackoutWindow{
		ClusterID: types.StringValue(clusterID),
		ID:        types.StringValue(windowID),
	}

	diags := resp.State.Set(ctx, &blackoutWindow)
	resp.Diagnostics.Append(diags...)
}

func parseRFC3339Timestamp(value types.String, attrName string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value.ValueString())
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"%s must be a valid RFC3339 timestamp (example: 2025-01-02T15:04:05Z). Parse error: %w",
			attrName,
			err,
		)
	}

	return parsed, nil
}

func loadBlackoutWindowToTerraformState(obj *client.BlackoutWindow, fallbackClusterID string, state *BlackoutWindow) {
	clusterID := obj.GetClusterId()
	if clusterID == "" {
		clusterID = fallbackClusterID
	}

	state.ID = types.StringValue(obj.GetId())
	state.ClusterID = types.StringValue(clusterID)
	state.StartTime = types.StringValue(obj.GetStartTime().UTC().Format(time.RFC3339))
	state.EndTime = types.StringValue(obj.GetEndTime().UTC().Format(time.RFC3339))
}
