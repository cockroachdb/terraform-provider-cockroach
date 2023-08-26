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
	"regexp"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type privateEndpointTrustedOwnerResource struct {
	provider *provider
}

// clusterID:ownerID
var (
	trustedOwnerIDFmt   = "%s:%s"
	trustedOWnerIDRegex = regexp.MustCompile(fmt.Sprintf("^(%s):(%s)$", uuidRegexString, uuidRegexString))
)

func (r *privateEndpointTrustedOwnerResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Private Endpoint Trusted Owner.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Used with `terraform import`. Format is \"<cluster ID>:<owner ID>\".",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"owner_id": schema.StringAttribute{
				Computed:    true,
				Description: "UUID of the private endpoint trusted owner entry.",
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "UUID of the cluster the private endpoint trusted owner entry belongs to.",
			},
			"type": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				MarkdownDescription: "Representation of the external_owner_id field. Allowed values are:" +
					formatEnumMarkdownList(client.AllowedPrivateEndpointTrustedOwnerTypeTypeEnumValues),
			},
			"external_owner_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "Owner ID of the private endpoint connection in the cloud provider.",
			},
		},
	}
}

func (r *privateEndpointTrustedOwnerResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_private_endpoint_trusted_owner"
}

func (r *privateEndpointTrustedOwnerResource) Configure(
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

func (r *privateEndpointTrustedOwnerResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan PrivateEndpointTrustedOwner
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	addReq := client.NewCockroachCloudAddPrivateEndpointTrustedOwnerRequest(
		plan.ExternalOwnerID.ValueString(),
		client.PrivateEndpointTrustedOwnerTypeType(plan.Type.ValueString()),
	)
	addResp, _, err := r.provider.service.AddPrivateEndpointTrustedOwner(ctx, plan.ClusterID.ValueString(), addReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating private endpoint trusted owner entry",
			fmt.Sprintf("Unexpected error occurred while creating private endpoint trusted owner entry: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state PrivateEndpointTrustedOwner
	loadPrivateEndpointTrustedOwnerIntoTerraformState(&addResp.TrustedOwner, &state)
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *privateEndpointTrustedOwnerResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state PrivateEndpointTrustedOwner
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate input values.
	clusterID := state.ClusterID.ValueString()
	if !uuidRegex.MatchString(clusterID) {
		resp.Diagnostics.AddError(
			"Unexpected cluster ID format",
			fmt.Sprintf("'%s' is not a valid cluster ID format. Expected UUID.", clusterID),
		)
		return
	}
	ownerUUID := state.OwnerID.ValueString()
	if !uuidRegex.MatchString(ownerUUID) {
		resp.Diagnostics.AddError(
			"Unexpected owner ID format",
			fmt.Sprintf("'%s' is not a valid owner ID format. Expected UUID.", clusterID),
		)
		return
	}

	getResp, httpResp, err := r.provider.service.GetPrivateEndpointTrustedOwner(ctx, clusterID, ownerUUID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Private endpoint trusted owner entry not found",
				fmt.Sprintf("Private endpoint trusted owner entry with ID %s is not found. Removing from state.", ownerUUID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting private endpoint trusted owner entry",
				fmt.Sprintf("Unexpected error retrieving private endpoint trusted owner entry: %s", formatAPIErrorMessage(err)))
		}
		return
	}
	loadPrivateEndpointTrustedOwnerIntoTerraformState(&getResp.TrustedOwner, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func loadPrivateEndpointTrustedOwnerIntoTerraformState(
	trustedOwner *client.PrivateEndpointTrustedOwner, state *PrivateEndpointTrustedOwner,
) {
	state.OwnerID = types.StringValue(trustedOwner.GetId())
	state.ClusterID = types.StringValue(trustedOwner.GetClusterId())
	state.Type = types.StringValue(string(trustedOwner.GetType()))
	state.ExternalOwnerID = types.StringValue(trustedOwner.GetExternalOwnerId())
	state.ID = types.StringValue(
		fmt.Sprintf(trustedOwnerIDFmt, state.ClusterID.ValueString(), state.OwnerID.ValueString()),
	)
}

func (r *privateEndpointTrustedOwnerResource) Update(
	_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse,
) {
	// No-op. Contains only requires-replace or computed fields.
}

func (r *privateEndpointTrustedOwnerResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state PrivateEndpointTrustedOwner
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, _, err := r.provider.service.RemovePrivateEndpointTrustedOwner(
		ctx,
		state.ClusterID.ValueString(),
		state.OwnerID.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting private endpoint trusted owner entry",
			fmt.Sprintf("Unexpected error occurred while deleting private endpoint trusted owner entry: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r *privateEndpointTrustedOwnerResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	// A trusted owner entry is identified by two fields: the cluster ID, and
	// the owner ID. In theory, the entry can be uniquely identified by the
	// owner ID, but we'd like to scope that to the cluster.
	//
	// For import to work here, we will deserialize the terraform ID back into
	// the cluster and owner IDs.
	matches := trustedOWnerIDRegex.FindStringSubmatch(req.ID)
	if len(matches) != 3 {
		resp.Diagnostics.AddError(
			"Invalid trusted owner terraform ID format",
			`When importing a trusted owner entry, the ID field should follow the format "<cluster ID>:<owner ID>")`)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics = resp.State.Set(ctx, &PrivateEndpointTrustedOwner{
		ID:        types.StringValue(req.ID),
		ClusterID: types.StringValue(matches[1]),
		OwnerID:   types.StringValue(matches[2]),
	})
}

func NewPrivateEndpointTrustedOwnerResource() resource.Resource {
	return &privateEndpointTrustedOwnerResource{}
}
