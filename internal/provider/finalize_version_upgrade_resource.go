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
	"strings"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

type finalizeVersionUpgradeResource struct {
	provider *provider
}

func (r *finalizeVersionUpgradeResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "Utility resource that represents the one-time action of finalizing a cluster's pending CockroachDB version upgrade.",
		Attributes: map[string]schema.Attribute{
			"cockroach_version": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "Major version of the cluster to be finalized.",
			},
			"id": schema.StringAttribute{
				Description: "Cluster ID.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *finalizeVersionUpgradeResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_finalize_version_upgrade"
}

func (r *finalizeVersionUpgradeResource) Configure(
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

func (r *finalizeVersionUpgradeResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}
	var plan FinalizeVersionUpgrade
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	clusterID := plan.ID.ValueString()

	traceAPICall("GetCluster")
	clusterObj, _, err := r.provider.service.GetCluster(ctx, clusterID)
	if err != nil {
		resp.Diagnostics.AddError("Error retrieving cluster info", formatAPIErrorMessage(err))
		return
	}

	if clusterObj.UpgradeStatus != client.CLUSTERUPGRADESTATUSTYPE_PENDING_FINALIZATION {
		resp.Diagnostics.AddError(
			"Cluster is not pending upgrade finalization",
			fmt.Sprintf("Expected upgrade status '%s', currently '%s'",
				client.CLUSTERUPGRADESTATUSTYPE_PENDING_FINALIZATION, clusterObj.UpgradeStatus),
		)
	}

	pendingMajorVersion := strings.Join(strings.Split(clusterObj.CockroachVersion, ".")[:2], ".")
	if pendingMajorVersion != plan.CockroachVersion.ValueString() {
		resp.Diagnostics.AddAttributeError(
			path.Root("cockroach_version"),
			"Unexpected version",
			fmt.Sprintf("Set to '%s' to finalize the pending upgrade.", pendingMajorVersion))
		return
	}

	finalizedStatus := client.CLUSTERUPGRADESTATUSTYPE_FINALIZED
	traceAPICall("UpdateCluster")
	if _, _, err = r.provider.service.UpdateCluster(ctx, clusterID, &client.UpdateClusterSpecification{UpgradeStatus: &finalizedStatus}); err != nil {
		resp.Diagnostics.AddError("Error finalizing cluster upgrade", formatAPIErrorMessage(err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *finalizeVersionUpgradeResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	var state FinalizeVersionUpgrade
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update should never be called since all plan changes force replace.
func (r *finalizeVersionUpgradeResource) Update(
	_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse,
) {
}

func (r *finalizeVersionUpgradeResource) Delete(
	ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func NewFinalizeVersionUpgradeResource() resource.Resource {
	return &finalizeVersionUpgradeResource{}
}
