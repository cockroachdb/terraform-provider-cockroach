package provider

import (
	"context"
	"fmt"
	"net/http"
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

const (
	physicalReplicationStreamCreateTimeout   = time.Hour
	physicalReplicationStreamCompleteTimeout = 10 * time.Minute
)

type physicalReplicationStreamResource struct {
	provider *provider
}

func NewPhysicalReplicationStreamResource() resource.Resource {
	return &physicalReplicationStreamResource{}
}

func (r *physicalReplicationStreamResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "Physical replication stream.",
		Attributes: map[string]schema.Attribute{
			"primary_cluster_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "ID of the primary cluster, which is the replication source.",
			},
			"standby_cluster_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "ID of the standby cluster, which is the replication target.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A unique identifier for the replication stream.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "The timestamp at which the replication stream was created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"replicated_time": schema.StringAttribute{
				Computed:    true,
				Description: "The timestamp indicating the point up to which data has been replicated.",
			},
			"replication_lag_seconds": schema.Int64Attribute{
				Computed:    true,
				Description: "The replication lag in seconds.",
			},
			"retained_time": schema.StringAttribute{
				Computed:    true,
				Description: "The timestamp indicating the earliest time that the replication stream can failover to.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "The status of the replication stream.",
			},
			"failover_at": schema.StringAttribute{
				Optional:    true,
				Description: "The timestamp at which to perform failover. If not specified, failover will occur at the latest consistent replicated time. Cannot be used with failover_immediately.",
			},
			"failover_immediately": schema.BoolAttribute{
				Optional:    true,
				Description: "If true, failover will occur immediately at the latest consistent replicated time. Cannot be used with failover_at.",
			},
			"activated_at": schema.StringAttribute{
				Computed:    true,
				Description: "The timestamp indicating the actual timestamp at which failover was finalized.",
			},
		},
	}
}

func (r *physicalReplicationStreamResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_physical_replication_stream"
}

func (r *physicalReplicationStreamResource) Configure(
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

func (r *physicalReplicationStreamResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan PhysicalReplicationStream
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.FailoverAt.IsNull() || !plan.FailoverImmediately.IsNull() {
		resp.Diagnostics.AddError(
			"Error creating PCR stream",
			"Cannot set failover_at or failover_immediately fields when creating PCR stream",
		)
		return
	}

	apiRequest := client.CreatePhysicalReplicationStreamRequest{
		PrimaryClusterId: plan.PrimaryClusterId.ValueString(),
		StandbyClusterId: plan.StandbyClusterId.ValueString(),
	}

	traceAPICall("CreatePhysicalReplicationStream")
	streamObj, _, err := r.provider.service.CreatePhysicalReplicationStream(ctx, &apiRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating PCR stream",
			fmt.Sprintf("Could not create PCR stream: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	err = retry.RetryContext(
		ctx,
		physicalReplicationStreamCreateTimeout,
		waitForPhysicalReplicationStreamStatusFunc(
			ctx, streamObj.Id, r.provider.service, streamObj, client.REPLICATIONSTREAMSTATUSTYPE_REPLICATING,
		),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"PCR stream creation failed",
			fmt.Sprintf("PCR stream failed to create: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadPCRStreamToTerraformState(streamObj, &plan)
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func waitForPhysicalReplicationStreamStatusFunc(
	ctx context.Context, id string, cl client.Service, stream *client.PhysicalReplicationStream,
	desiredStatus client.ReplicationStreamStatusType,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("GetPhysicalReplicationStream")
		apiStream, httpResp, err := cl.GetPhysicalReplicationStream(ctx, id)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return retry.NonRetryableError(fmt.Errorf("error getting physical replication stream: %s", formatAPIErrorMessage(err)))
			} else {
				return retry.RetryableError(fmt.Errorf("encountered a server error while reading physical replication stream status"))
			}
		}
		*stream = *apiStream
		if stream.Status != desiredStatus {
			return retry.RetryableError(fmt.Errorf("physical replication stream has status %s; desired status is %s", stream.Status, desiredStatus))
		}
		return nil
	}
}

func loadPCRStreamToTerraformState(
	apiObj *client.PhysicalReplicationStream, state *PhysicalReplicationStream,
) {
	state.ID = types.StringValue(apiObj.GetId())
	state.PrimaryClusterId = types.StringValue(apiObj.GetPrimaryClusterId())
	state.StandbyClusterId = types.StringValue(apiObj.GetStandbyClusterId())
	state.CreatedAt = types.StringValue(apiObj.GetCreatedAt().String())
	if apiObj.ReplicatedTime != nil {
		state.ReplicatedTime = types.StringValue(apiObj.GetReplicatedTime().String())
	}
	if apiObj.ReplicationLagSeconds != nil {
		state.ReplicationLagSeconds = types.Int64Value(int64(apiObj.GetReplicationLagSeconds()))
	}
	if apiObj.RetainedTime != nil {
		state.RetainedTime = types.StringValue(apiObj.GetRetainedTime().String())
	}
	if apiObj.ActivatedAt != nil {
		state.ActivatedAt = types.StringValue(apiObj.GetActivatedAt().String())
	} else {
		state.ActivatedAt = types.StringNull()
	}
	state.Status = types.StringValue(string(apiObj.GetStatus()))
}

func (r *physicalReplicationStreamResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state PhysicalReplicationStream
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()

	traceAPICall("GetPhysicalReplicationStream")
	apiResp, httpResp, err := r.provider.service.GetPhysicalReplicationStream(ctx, id)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"PCR stream not found",
				fmt.Sprintf(
					"PCR stream with id %s is not found. Removing from state.",
					id,
				),
			)
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting PCR stream",
				fmt.Sprintf("Could not get PCR stream: %s", formatAPIErrorMessage(err)),
			)
		}
		return
	}

	loadPCRStreamToTerraformState(apiResp, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *physicalReplicationStreamResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan, state PhysicalReplicationStream
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	isFailoverAt := !plan.FailoverAt.IsNull()
	isFailoverImmediately := !plan.FailoverImmediately.IsNull()

	// Validate that exactly one of failover_at or failover_immediately is set
	if isFailoverAt == isFailoverImmediately {
		resp.Diagnostics.AddError(
			"Invalid failover configuration",
			"Exactly one of failover_at or failover_immediately must be set",
		)
		return
	}
	if isFailoverImmediately && !plan.FailoverImmediately.ValueBool() {
		resp.Diagnostics.AddError(
			"Invalid failover configuration",
			"If failover_immediately is set, it must be set to true",
		)
		return
	}

	// Create update spec with FAILING_OVER status
	failingOverStatus := client.REPLICATIONSTREAMSTATUSTYPE_FAILING_OVER
	updateSpec := &client.UpdatePhysicalReplicationStreamSpec{
		Status: &failingOverStatus,
	}

	// If failover_at is specified in the plan, set it in the update spec
	// Only set failover_at if failover_immediately is not true
	if isFailoverAt {
		failoverTime, err := time.Parse(time.RFC3339, plan.FailoverAt.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Error parsing failover time",
				fmt.Sprintf("Could not parse failover time: %s", err),
			)
			return
		}
		updateSpec.FailoverAt = &failoverTime
	}

	// Call the update API
	traceAPICall("UpdatePhysicalReplicationStream")
	streamObj, _, err := r.provider.service.UpdatePhysicalReplicationStream(ctx, state.ID.ValueString(), updateSpec)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating PCR stream",
			fmt.Sprintf("Could not update PCR stream: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	err = retry.RetryContext(
		ctx,
		physicalReplicationStreamCompleteTimeout,
		waitForPhysicalReplicationStreamStatusFunc(
			ctx, streamObj.Id, r.provider.service, streamObj, client.REPLICATIONSTREAMSTATUSTYPE_COMPLETED,
		),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"PCR update failed",
			fmt.Sprintf("PCR stream failed to update: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	state.FailoverAt = plan.FailoverAt
	state.FailoverImmediately = plan.FailoverImmediately

	// Update state with response
	loadPCRStreamToTerraformState(streamObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *physicalReplicationStreamResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state PhysicalReplicationStream
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Status != types.StringValue(string(client.REPLICATIONSTREAMSTATUSTYPE_COMPLETED)) {
		resp.Diagnostics.AddError("Cannot remove PCR stream", "A PCR stream must be completed to be removable")
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r *physicalReplicationStreamResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
