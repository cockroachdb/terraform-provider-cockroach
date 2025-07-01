package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type PhysicalReplicationStream struct {
	PrimaryClusterId      types.String `tfsdk:"primary_cluster_id"`
	StandbyClusterId      types.String `tfsdk:"standby_cluster_id"`
	ID                    types.String `tfsdk:"id"`
	CreatedAt             types.String `tfsdk:"created_at"`
	ReplicatedTime        types.String `tfsdk:"replicated_time"`
	ReplicationLagSeconds types.Int64  `tfsdk:"replication_lag_seconds"`
	RetainedTime          types.String `tfsdk:"retained_time"`
	Status                types.String `tfsdk:"status"`
}

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
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "The timestamp at which the replication stream was created.",
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

	err = retry.RetryContext(ctx, clusterCreateTimeout,
		waitForPhysicalReplicationStreamReadyFunc(ctx, streamObj.Id, r.provider.service, streamObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"Cluster creation failed",
			fmt.Sprintf("Cluster is not ready: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadPCRStreamToTerraformState(streamObj, &plan)
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func waitForPhysicalReplicationStreamReadyFunc(
	ctx context.Context, id string, cl client.Service, stream *client.PhysicalReplicationStream,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("GetPhysicalReplicationStream")
		apiStream, httpResp, err := cl.GetPhysicalReplicationStream(ctx, id)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return retry.NonRetryableError(fmt.Errorf("error getting physical replication stream: %s", formatAPIErrorMessage(err)))
			} else {
				return retry.RetryableError(fmt.Errorf("encountered a server error while reading physical replication stream status - trying again"))
			}
		}
		*stream = *apiStream
		if stream.Status == client.REPLICATIONSTREAMSTATUSTYPE_REPLICATING {
			return nil
		}
		return retry.RetryableError(fmt.Errorf("physical replication stream is not ready yet"))
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
	// TODO FIX THIS
	// All fields are either ForcesNew or Computed, so there is nothing to update.
}

func (r *physicalReplicationStreamResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	// TODO implement this
}

func (r *physicalReplicationStreamResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
