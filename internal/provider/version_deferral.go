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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v2/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var versionDeferralAttributes = map[string]schema.Attribute{
	"id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Cluster ID.",
	},
	"deferral_policy": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The policy for managing automated minor version upgrades. Set to FIXED_DEFERRAL to defer upgrades by 60 days or NOT_DEFERRED to apply upgrades immediately.",
	},
}

type versionDeferralResource struct {
	provider *provider
}

func (r *versionDeferralResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Configure minor version upgrade deferral for a cluster.",
		Attributes:          versionDeferralAttributes,
	}
}

func (r *versionDeferralResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_version_deferral"
}

func (r *versionDeferralResource) Configure(
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

func (r *versionDeferralResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var versionDeferral ClusterVersionDeferral
	diags := req.Plan.Get(ctx, &versionDeferral)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.setVersionDeferral(ctx, &resp.State, &resp.Diagnostics, versionDeferral)
}

func (r *versionDeferralResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state ClusterVersionDeferral
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ID.ValueString()
	traceAPICall("GetClusterVersionDeferral")
	obj, httpResp, err := r.provider.service.GetClusterVersionDeferral(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Version deferral policy not found",
				fmt.Sprintf("Version deferral policy for cluster ID %s not found. Deferral policy will be removed from state.", clusterID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting version deferral policy",
				fmt.Sprintf("Unexpected error retrieving version deferral policy: %s", formatAPIErrorMessage(err)))
		}
		return
	}
	state.DeferralPolicy = basetypes.NewStringValue(string(obj.DeferralPolicy))
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *versionDeferralResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	var versionDeferral ClusterVersionDeferral
	diags := req.Plan.Get(ctx, &versionDeferral)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.setVersionDeferral(ctx, &resp.State, &resp.Diagnostics, versionDeferral)
}

func (r *versionDeferralResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var versionDeferral ClusterVersionDeferral
	diags := req.State.Get(ctx, &versionDeferral)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	versionDeferral.DeferralPolicy = basetypes.NewStringValue(string(client.CLUSTERVERSIONDEFERRALPOLICYTYPE_NOT_DEFERRED))
	r.setVersionDeferral(ctx, &resp.State, &resp.Diagnostics, versionDeferral)
}

func (r *versionDeferralResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func NewVersionDeferralResource() resource.Resource {
	return &versionDeferralResource{}
}

func (r *versionDeferralResource) setVersionDeferral(
	ctx context.Context, state *tfsdk.State, diags *diag.Diagnostics, versionDeferral ClusterVersionDeferral,
) {
	clientVersionDeferral := client.NewClusterVersionDeferralWithDefaults()
	clientVersionDeferral.DeferralPolicy = client.ClusterVersionDeferralPolicyType(versionDeferral.DeferralPolicy.ValueString())
	traceAPICall("SetClusterVersionDeferral")
	_, _, err := r.provider.service.SetClusterVersionDeferral(ctx, versionDeferral.ID.ValueString(), clientVersionDeferral)
	if err != nil {
		diags.AddError(
			"Error setting version deferral",
			fmt.Sprintf("Could not set version deferral: %v", formatAPIErrorMessage(err)),
		)
		return
	}
	diags.Append(state.Set(ctx, versionDeferral)...)
}
