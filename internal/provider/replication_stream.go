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
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v5/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var replicationStreamAttributes = map[string]schema.Attribute{
	"id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Replication stream ID",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	},
	"source_cluster_id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "ID of the replication stream source cluster",
	},
	"target_cluster_id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "ID of the replication stream target cluster",
	},
	"status": schema.StringAttribute{
		MarkdownDescription: "Status of the replication stream: STARTING, REPLICATING, FAILING_OVER, or COMPLETED",
		Computed:            true,
		Optional:            true,
	},
	"failover_at": schema.StringAttribute{
		MarkdownDescription: "Time at which failover should occur when status is set to FAILING_OVER.",
		Optional:            true,
		Computed:            true,
	},
	"replicated_time": schema.StringAttribute{
		MarkdownDescription: "Timestamp indicating the point up to which data has been replicated.",
		Computed:            true,
	},
	"replication_lag_seconds": schema.Int64Attribute{
		MarkdownDescription: "Replication lag in seconds.",
		Computed:            true,
	},
	"retained_time": schema.StringAttribute{
		MarkdownDescription: "Timestamp indicating the lower bound that the replication stream can failover to.",
		Computed:            true,
	},
	"created_at": schema.StringAttribute{
		MarkdownDescription: "Timestamp when the replication stream was created.",
		Computed:            true,
	},
	"activation_at": schema.StringAttribute{
		MarkdownDescription: "Time at which failover was finalized. Present when status is COMPLETED.",
		Computed:            true,
	},
}

type replicationStreamResource struct {
	provider *provider
}

func (*replicationStreamResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_replication_stream"
}

func (*replicationStreamResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Replication stream between a source and a target cluster",
		Attributes:          replicationStreamAttributes,
	}
}

func (r *replicationStreamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan ReplicationStream
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// For an existing replication stream import with a specified ID, we skip creation
	if !plan.ID.IsNull() {
		// Get existing replication stream
		replicationStreamObj, httpResp, err := r.provider.service.GetReplicationStream(ctx, plan.ID.ValueString())
		if err != nil {
			if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
				resp.Diagnostics.AddError(
					"Replication stream not found",
					fmt.Sprintf("Replication stream with ID %s is not found.", plan.ID.ValueString()),
				)
			} else {
				resp.Diagnostics.AddError(
					"Error getting replication stream info",
					fmt.Sprintf("Unexpected error retrieving replication stream info: %s", formatAPIErrorMessage(err)),
				)
			}
			return
		}

		// Load the existing replication stream into state
		loadReplicationStreamToTerraformState(replicationStreamObj, &plan)
		diags = resp.State.Set(ctx, plan)
		resp.Diagnostics.Append(diags...)
		return
	}

	// Create new replication stream
	createRequest := client.CreateReplicationStreamRequest{
		SourceClusterId: plan.SourceClusterID.ValueString(),
		TargetClusterId: plan.TargetClusterID.ValueString(),
	}

	traceAPICall("CreateReplicationStream")
	createdStream, _, err := r.provider.service.CreateReplicationStream(ctx, &createRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating replication stream",
			fmt.Sprintf("Could not create replication stream: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// Load the created replication stream into state
	loadReplicationStreamToTerraformState(createdStream, &plan)
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *replicationStreamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state ReplicationStream
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
	if state.ID.IsNull() {
		return
	}

	replicationStreamID := state.ID.ValueString()

	// In case this was an import, validate the ID format.
	if !uuidRegex.MatchString(replicationStreamID) {
		resp.Diagnostics.AddError(
			"Unexpected replication stream ID format",
			fmt.Sprintf("'%s' is not a valid replication stream ID format. Expected UUID.", replicationStreamID),
		)
		return
	}

	replicationStreamObj, httpResp, err := r.provider.service.GetReplicationStream(ctx, replicationStreamID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Replication stream not found",
				fmt.Sprintf("Replication stream with ID %s is not found. Removing from state.", replicationStreamID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting replication stream info",
				fmt.Sprintf("Unexpected error retrieving replication stream info: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	loadReplicationStreamToTerraformState(replicationStreamObj, &state)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *replicationStreamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan, state ReplicationStream
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Check if status is being updated
	if !plan.Status.Equal(state.Status) {
		updateSpec := client.UpdateReplicationStreamSpec{}

		status := client.ReplicationStreamStatusType(plan.Status.ValueString())
		updateSpec.SetStatus(status)

		// If status is being set to FAILING_OVER and failover_at is provided
		if plan.Status.ValueString() == string(client.REPLICATIONSTREAMSTATUSTYPE_FAILING_OVER) && !plan.FailoverAt.IsNull() {
			failoverTime, err := time.Parse(time.RFC3339, plan.FailoverAt.ValueString())
			if err != nil {
				resp.Diagnostics.AddError(
					"Invalid failover time",
					fmt.Sprintf("Invalid failover time format: %s. Expected RFC3339 format (e.g., 2023-01-01T12:00:00Z)", err),
				)
				return
			}
			updateSpec.SetFailoverAt(failoverTime)
		}

		// Update the replication stream
		traceAPICall("UpdateReplicationStream")
		updatedStream, _, err := r.provider.service.UpdateReplicationStream(ctx, state.ID.ValueString(), &updateSpec)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating replication stream",
				fmt.Sprintf("Could not update replication stream: %s", formatAPIErrorMessage(err)),
			)
			return
		}

		// Load the updated stream into state
		loadReplicationStreamToTerraformState(updatedStream, &plan)
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *replicationStreamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Note: The API doesn't provide a DeleteReplicationStream method
	// For replication streams, we have these options:
	// 1. Do nothing and let it be removed from state (current implementation)
	// 2. Set status to COMPLETED to finalize the failover (optional implementation)

	// If you want to implement the second option, here's how that would look:
	/*
		if r.provider == nil || !r.provider.configured {
			addConfigureProviderErr(&resp.Diagnostics)
			return
		}

		var state ReplicationStream
		diags := req.State.Get(ctx, &state)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		// Only complete the stream if it's in a state that can be completed
		currentStatus := state.Status.ValueString()
		if currentStatus != string(client.REPLICATIONSTREAMSTATUSTYPE_COMPLETED) {
			updateSpec := client.UpdateReplicationStreamSpec{}
			status := client.REPLICATIONSTREAMSTATUSTYPE_FAILING_OVER
			updateSpec.SetStatus(status)

			traceAPICall("UpdateReplicationStream")
			_, _, err := r.provider.service.UpdateReplicationStream(ctx, state.ID.ValueString(), &updateSpec)
			if err != nil {
				resp.Diagnostics.AddWarning(
					"Error finalizing replication stream",
					fmt.Sprintf("Could not finalize replication stream before deletion: %s", formatAPIErrorMessage(err)),
				)
			}
		}
	*/

	// For now, we'll just allow removal from state without API action
}

func (r *replicationStreamResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if r.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError("Internal provider error",
			fmt.Sprintf("Error in Configure: expected %T but got %T", provider{}, req.ProviderData))
	}
}

func (rs *replicationStreamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func loadReplicationStreamToTerraformState(replicationStreamObj *client.ReplicationStream, state *ReplicationStream) {
	state.ID = types.StringValue(replicationStreamObj.Id)
	state.SourceClusterID = types.StringValue(replicationStreamObj.SourceClusterId)
	state.TargetClusterID = types.StringValue(replicationStreamObj.TargetClusterId)
	state.Status = types.StringValue(string(replicationStreamObj.Status))
	state.CreatedAt = types.StringValue(replicationStreamObj.CreatedAt.Format(time.RFC3339))

	// Set optional fields
	if replicationStreamObj.ActivationAt != nil {
		state.ActivationAt = types.StringValue(replicationStreamObj.ActivationAt.Format(time.RFC3339))
	} else {
		state.ActivationAt = types.StringNull()
	}

	if replicationStreamObj.FailoverAt != nil {
		state.FailoverAt = types.StringValue(replicationStreamObj.FailoverAt.Format(time.RFC3339))
	} else {
		state.FailoverAt = types.StringNull()
	}

	if replicationStreamObj.ReplicatedTime != nil {
		state.ReplicatedTime = types.StringValue(replicationStreamObj.ReplicatedTime.Format(time.RFC3339))
	} else {
		state.ReplicatedTime = types.StringNull()
	}

	if replicationStreamObj.RetainedTime != nil {
		state.RetainedTime = types.StringValue(replicationStreamObj.RetainedTime.Format(time.RFC3339))
	} else {
		state.RetainedTime = types.StringNull()
	}

	if replicationStreamObj.ReplicationLagSeconds != nil {
		state.ReplicationLagSeconds = types.Int64Value(int64(*replicationStreamObj.ReplicationLagSeconds))
	} else {
		state.ReplicationLagSeconds = types.Int64Null()
	}
}

// NewReplicationStreamResource is a helper function to simplify provider server
func NewReplicationStreamResource() resource.Resource {
	return &replicationStreamResource{}
}
