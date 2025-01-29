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

func (*replicationStreamResource) Create(context.Context, resource.CreateRequest, *resource.CreateResponse) {
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
			"Unexpected folder ID format",
			fmt.Sprintf("'%s' is not a valid folder ID format. Expected UUID.", replicationStreamID),
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
				"Error getting folder info",
				fmt.Sprintf("Unexpected error retrieving folder info: %s", formatAPIErrorMessage(err)))
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

func (*replicationStreamResource) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {
}

func (*replicationStreamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {

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
}
