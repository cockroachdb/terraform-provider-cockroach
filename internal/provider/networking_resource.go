package provider

import (
	"context"
	"fmt"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type networkResourceType struct{}

func (n networkResourceType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		MarkdownDescription: "Allow list of IP range",
		Attributes: map[string]tfsdk.Attribute{
			"id": {
				Required: true,
				Type:     types.StringType,
			},
			"cidr_ip": {
				Required: true,
				Type:     types.StringType,
			},
			"cidr_mask": {
				Required: true,
				Type:     types.Int64Type,
			},
			"ui": {
				Required: true,
				Type:     types.BoolType,
			},
			"sql": {
				Required: true,
				Type:     types.BoolType,
			},
			"name": {
				Optional: true,
				Type:     types.StringType,
			},
		},
	}, nil
}

func (n networkResourceType) NewResource(ctx context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return networkResource{
		provider: provider,
	}, diags
}

type networkResource struct {
	provider provider
}

func (n networkResource) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	if !n.provider.configured {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider hasn't been configured before apply, likely because it depends on an unknown value from another resource. This leads to weird stuff happening, so we'd prefer if you didn't do that. Thanks!",
		)
		return
	}

	var plan AllowlistEntry
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	cluster, httpResp, err := n.provider.service.GetCluster(ctx, plan.Id.Value)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting the cluster",
			fmt.Sprintf("Could not get the cluster, unexpected error: %v %v "+err.Error(), httpResp),
		)
		return
	}

	if cluster.Config.Serverless != nil {
		resp.Diagnostics.AddError(
			"Could not add network allow list in serverless cluster",
			fmt.Sprintf("Network allow list is a feature of dedicated cluster, unexpected error: %v %v "+err.Error(), httpResp),
		)
		return
	}

	var allowList = client.AllowlistEntry{
		CidrIp:   plan.CidrIp.Value,
		CidrMask: int32(plan.CidrMask.Value),
		Ui:       plan.Ui.Value,
		Sql:      plan.Sql.Value,
	}

	if !plan.Name.Null {
		allowList.Name = &plan.Name.Value
	}

	_, httpResp, err = n.provider.service.AddAllowlistEntry(ctx, plan.Id.Value, &allowList)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error adding allowed IP range",
			fmt.Sprintf("Could not add allowed IP range, unexpected error: %v %v "+err.Error(), httpResp),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (n networkResource) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	if !n.provider.configured {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider hasn't been configured before apply, likely because it depends on an unknown value from another resource. This leads to weird stuff happening, so we'd prefer if you didn't do that. Thanks!",
		)
		return
	}

	var plan AllowlistEntry
	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (n networkResource) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	// Get plan values
	var plan AllowlistEntry
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state AllowlistEntry
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Id != plan.Id {
		resp.Diagnostics.AddError(
			"can not change cluster id in the network allow list",
			"You can only change network allow list. Thanks!",
		)
		return
	}

	clusterId := plan.Id.Value
	entryCIDRIp := plan.CidrIp.Value
	entryCIDRMask := int32(plan.CidrMask.Value)

	existingAllowList := client.AllowlistEntry{
		CidrIp:   state.CidrIp.Value,
		CidrMask: int32(state.CidrMask.Value),
		Ui:       state.Ui.Value,
		Sql:      state.Sql.Value,
		Name:     &state.Name.Value,
	}

	_, httpResp, err := n.provider.service.UpdateAllowlistEntry(ctx, clusterId, entryCIDRIp, entryCIDRMask,
		&existingAllowList, &client.UpdateAllowlistEntryOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating network allow list",
			fmt.Sprintf("Could not update network allow list, unexpected error: %v %v "+err.Error(), httpResp),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (n networkResource) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var state AllowlistEntry
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, err := n.provider.service.DeleteAllowlistEntry(ctx, state.Id.Value, state.CidrIp.Value, int32(state.CidrMask.Value))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting network allow list",
			fmt.Sprintf("Could not delete network allow list, unexpected error: %v %v "+err.Error(), httpResp),
		)
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (n networkResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("id"), req, resp)
}
