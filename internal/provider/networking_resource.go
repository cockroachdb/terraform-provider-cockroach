/*
Copyright 2022 The Cockroach Authors

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
	"regexp"
	"strconv"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type allowListResourceType struct{}

// clusterID:ip/mask
const allowListIDFmt = "%s:%s/%d"
const allowlistEntryRegex = `(([0-9]{1,3}\.){3}[0-9]{1,3})\/([0-9]|[1-2][0-9]|3[0-2])`

var allowlistIDRegex = regexp.MustCompile(fmt.Sprintf("^(%s):%s$", uuidRegex, allowlistEntryRegex))

func (n allowListResourceType) GetSchema(_ context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		MarkdownDescription: "Allow list of IP range",
		Attributes: map[string]tfsdk.Attribute{
			"cluster_id": {
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
			"id": {
				Computed: true,
				Type:     types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
			},
		},
	}, nil
}

func (n allowListResourceType) NewResource(_ context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return allowListResource{
		provider: provider,
	}, diags
}

type allowListResource struct {
	provider provider
}

func (n allowListResource) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	if !n.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var entry AllowlistEntry
	diags := req.Config.Get(ctx, &entry)
	resp.Diagnostics.Append(diags...)
	// Create a unique ID (required by terraform framework) by combining
	// the cluster ID and full CIDR address.
	entry.ID = types.String{
		Value: fmt.Sprintf(allowListIDFmt, entry.ClusterId.Value, entry.CidrIp.Value, entry.CidrMask.Value),
	}

	if resp.Diagnostics.HasError() {
		return
	}

	cluster, _, err := n.provider.service.GetCluster(ctx, entry.ClusterId.Value)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting the cluster",
			fmt.Sprintf("Could not get the cluster, unexpected error: %v", err.Error()),
		)
		return
	}

	if cluster.Config.Serverless != nil {
		resp.Diagnostics.AddError(
			"Could not add network allow list in serverless cluster",
			fmt.Sprintf("Network allow list is a feature of dedicated cluster, unexpected error: %v", err.Error()),
		)
		return
	}

	var allowList = client.AllowlistEntry{
		CidrIp:   entry.CidrIp.Value,
		CidrMask: int32(entry.CidrMask.Value),
		Ui:       entry.Ui.Value,
		Sql:      entry.Sql.Value,
	}

	if !entry.Name.Null {
		allowList.Name = &entry.Name.Value
	}

	_, _, err = n.provider.service.AddAllowlistEntry(ctx, entry.ClusterId.Value, &allowList)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error adding allowed IP range",
			fmt.Sprintf("Could not add allowed IP range, unexpected error: %v", err.Error()),
		)
		return
	}

	diags = resp.State.Set(ctx, entry)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (n allowListResource) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	if !n.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state AllowlistEntry
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Since the state may have come from an import, we need to retrieve
	// the actual entry list and make sure this one is in there.
	apiResp, _, err := n.provider.service.ListAllowlistEntries(ctx, state.ClusterId.Value, &client.ListAllowlistEntriesOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Couldn't retrieve allowlist entries",
			fmt.Sprintf("Unexpected error retrieving allowlist entries: %v", err.Error()),
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	for _, entry := range apiResp.GetAllowlist() {
		if entry.GetCidrIp() == state.CidrIp.Value ||
			int64(entry.GetCidrMask()) == state.CidrMask.Value {
			return
		}
	}
	resp.Diagnostics.AddError(
		"Couldn't find entry.",
		fmt.Sprintf("This cluster's allowlist doesn't contain %s/%d", state.CidrIp.Value, state.CidrMask.Value),
	)
}

func (n allowListResource) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
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

	if state.ClusterId != plan.ClusterId {
		resp.Diagnostics.AddError(
			"can not change cluster id in the network allow list",
			"You can only change network allow list. Thanks!",
		)
		return
	}

	clusterId := plan.ClusterId.Value
	entryCIDRIp := plan.CidrIp.Value
	entryCIDRMask := int32(plan.CidrMask.Value)

	existingAllowList := client.AllowlistEntry1{
		Ui:   state.Ui.Value,
		Sql:  state.Sql.Value,
		Name: &state.Name.Value,
	}

	_, _, err := n.provider.service.UpdateAllowlistEntry(ctx, clusterId, entryCIDRIp, entryCIDRMask,
		&existingAllowList, &client.UpdateAllowlistEntryOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating network allow list",
			fmt.Sprintf("Could not update network allow list, unexpected error: %v", err.Error()),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (n allowListResource) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var state AllowlistEntry
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, _, err := n.provider.service.DeleteAllowlistEntry(ctx, state.ClusterId.Value, state.CidrIp.Value, int32(state.CidrMask.Value))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting network allow list",
			fmt.Sprintf("Could not delete network allow list, unexpected error: %v", err.Error()),
		)
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (n allowListResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	// Since an allowlist entry is uniquely identified by three fields: the cluster ID,
	// CIDR IP, and CIDR mask, and we serialize them all into the ID field. To make import
	// work, we need to deserialize an ID back into its components.
	var mask int
	matches := allowlistIDRegex.FindStringSubmatch(req.ID)
	if len(matches) != 5 {
		resp.Diagnostics.AddError(
			"Invalid allowlist entry ID format",
			`When importing an allowlist entry, the ID field should follow the format "<cluster ID>:<CIDR IP>/<CIDR mask>")`)
	}
	// We can swallow this error because it's already been regex-validated.
	mask, _ = strconv.Atoi(matches[4])
	entry := AllowlistEntry{
		ClusterId: types.String{Value: matches[1]},
		CidrIp:    types.String{Value: matches[2]},
		CidrMask:  types.Int64{Value: int64(mask)},
		ID:        types.String{Value: req.ID},
	}
	resp.Diagnostics = resp.State.Set(ctx, &entry)
}
