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
	"strconv"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// clusterID:ip/mask
const (
	allowListIDFmt      = "%s:%s/%d"
	allowlistEntryRegex = `(([0-9]{1,3}\.){3}[0-9]{1,3})\/([0-9]|[1-2][0-9]|3[0-2])`
	// Allowlist entries are lightweight, so we can use a large limit.
	allowlistEntryPaginationLimit = 500
)

var allowlistIDRegex = regexp.MustCompile(fmt.Sprintf("^(%s):%s$", uuidRegexString, allowlistEntryRegex))

type allowListResource struct {
	provider *provider
}

func (r *allowListResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "List of IP ranges allowed to access the cluster.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cidr_ip": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				MarkdownDescription: "IP address component of the [CIDR](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing#CIDR_notation) range for this entry.",
			},
			"cidr_mask": schema.Int64Attribute{
				Required: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				MarkdownDescription: "Map component of the [CIDR](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing#CIDR_notation) range for this entry.",
			},
			"ui": schema.BoolAttribute{
				Required:    true,
				Description: "Set to 'true' to allow access to the management console from this CIDR range.",
			},
			"sql": schema.BoolAttribute{
				Required:    true,
				Description: "Set to 'true' to allow SQL connections from this CIDR range.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Description: "Name of this allowlist entry.",
			},
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "A unique identifier with format `<cluster ID>:<CIDR IP>/<CIDR mask>`.",
			},
		},
	}
}

func (r *allowListResource) Configure(
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

func (r *allowListResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_allow_list"
}

func (r *allowListResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var entry AllowlistEntry
	diags := req.Plan.Get(ctx, &entry)
	resp.Diagnostics.Append(diags...)
	// Create a unique ID (required by terraform framework) by combining
	// the cluster ID and full CIDR address.
	entry.ID = types.StringValue(fmt.Sprintf(
		allowListIDFmt, entry.ClusterId.ValueString(),
		entry.CidrIp.ValueString(), entry.CidrMask.ValueInt64()))

	if resp.Diagnostics.HasError() {
		return
	}

	var allowList = client.AllowlistEntry{
		CidrIp:   entry.CidrIp.ValueString(),
		CidrMask: int32(entry.CidrMask.ValueInt64()),
		Ui:       entry.Ui.ValueBool(),
		Sql:      entry.Sql.ValueBool(),
	}

	if !entry.Name.IsNull() {
		name := entry.Name.ValueString()
		allowList.Name = &name
	}

	_, _, err := r.provider.service.AddAllowlistEntry(ctx, entry.ClusterId.ValueString(), &allowList)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error adding allowed IP range",
			fmt.Sprintf("Could not add allowed IP range: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	diags = resp.State.Set(ctx, entry)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *allowListResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
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
	var page string
	limit := int32(allowlistEntryPaginationLimit)
	for {
		apiResp, httpResp, err := r.provider.service.ListAllowlistEntries(
			ctx, state.ClusterId.ValueString(), &client.ListAllowlistEntriesOptions{
				PaginationPage:  &page,
				PaginationLimit: &limit,
			},
		)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
				resp.Diagnostics.AddWarning(
					"Cluster not found",
					fmt.Sprintf(
						"Allowlist's parent cluster with clusterID %s is not found. Removing from state.",
						state.ClusterId.ValueString()),
				)
				resp.State.RemoveResource(ctx)
			} else {
				resp.Diagnostics.AddError(
					"Couldn't retrieve allowlist entries",
					fmt.Sprintf("Unexpected error retrieving allowlist entries: %s", formatAPIErrorMessage(err)),
				)
			}
			return
		}
		if resp.Diagnostics.HasError() {
			return
		}

		for _, entry := range apiResp.GetAllowlist() {
			if entry.GetCidrIp() == state.CidrIp.ValueString() &&
				int64(entry.GetCidrMask()) == state.CidrMask.ValueInt64() {
				// Update flags in case they've changed externally.
				state.Sql = types.BoolValue(entry.GetSql())
				state.Ui = types.BoolValue(entry.GetUi())
				if entry.Name == nil {
					state.Name = types.StringNull()
				} else {
					state.Name = types.StringValue(entry.GetName())
				}
				diags = resp.State.Set(ctx, &state)
				resp.Diagnostics.Append(diags...)
				return
			}
		}

		pagination := apiResp.GetPagination()
		if pagination.NextPage != nil && *pagination.NextPage != "" {
			page = *pagination.NextPage
		} else {
			break
		}
	}
	resp.Diagnostics.AddWarning(
		"Couldn't find entry.",
		fmt.Sprintf("This cluster's allowlist doesn't contain %s/%d. Removing from state.",
			state.CidrIp.ValueString(), state.CidrMask.ValueInt64()),
	)
	resp.State.RemoveResource(ctx)
}

func (r *allowListResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
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

	clusterId := plan.ClusterId.ValueString()
	entryCIDRIp := plan.CidrIp.ValueString()
	entryCIDRMask := int32(plan.CidrMask.ValueInt64())

	name := plan.Name.ValueString()
	updatedAllowList := client.AllowlistEntry1{
		Ui:   plan.Ui.ValueBool(),
		Sql:  plan.Sql.ValueBool(),
		Name: &name,
	}

	_, _, err := r.provider.service.UpdateAllowlistEntry(
		ctx, clusterId, entryCIDRIp, entryCIDRMask, &updatedAllowList)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating network allowlist",
			fmt.Sprintf("Could not update network allowlist: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *allowListResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state AllowlistEntry
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, err := r.provider.service.DeleteAllowlistEntry(
		ctx, state.ClusterId.ValueString(), state.CidrIp.ValueString(), int32(state.CidrMask.ValueInt64()))
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			// Entry or cluster is already gone. Swallow the error.
		} else {
			resp.Diagnostics.AddError(
				"Error deleting network allowlist",
				fmt.Sprintf("Could not delete network allowlist: %s", formatAPIErrorMessage(err)),
			)
			return
		}
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r *allowListResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
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
		ClusterId: types.StringValue(matches[1]),
		CidrIp:    types.StringValue(matches[2]),
		CidrMask:  types.Int64Value(int64(mask)),
		ID:        types.StringValue(req.ID),
	}
	resp.Diagnostics = resp.State.Set(ctx, &entry)
}

func NewAllowlistResource() resource.Resource {
	return &allowListResource{}
}
