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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type sqlUserResource struct {
	provider *provider
}

// clusterID:name
const sqlUserIDFmt = "%s:%s"
const sqlUserNameRegex = "[A-Za-z0-9_][A-Za-z0-9\\._\\-]{0,62}"

var sqlUserIDRegex = regexp.MustCompile(fmt.Sprintf("^(%s):(%s)$", uuidRegex, sqlUserNameRegex))

func (r *sqlUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "SQL user and password",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
			},
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *sqlUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sql_user"
}

func (r *sqlUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if r.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError("Internal provider error",
			fmt.Sprintf("Error in Configure: expected %T but got %T", provider{}, req.ProviderData))
	}
}

func (r *sqlUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var sqlUserSpec SQLUser
	diags := req.Config.Get(ctx, &sqlUserSpec)
	resp.Diagnostics.Append(diags...)
	// Create a unique ID (required by terraform framework) by combining
	// the cluster ID and username.
	sqlUserSpec.ID = types.StringValue(fmt.Sprintf(sqlUserIDFmt, sqlUserSpec.ClusterId.ValueString(), sqlUserSpec.Name.ValueString()))

	if resp.Diagnostics.HasError() {
		return
	}

	_, _, err := r.provider.service.GetCluster(ctx, sqlUserSpec.ClusterId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting the cluster",
			fmt.Sprintf("Could not get the cluster: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var sqlUserRequest client.CreateSQLUserRequest
	sqlUserRequest.Name = sqlUserSpec.Name.ValueString()
	sqlUserRequest.Password = sqlUserSpec.Password.ValueString()

	_, _, err = r.provider.service.CreateSQLUser(ctx, sqlUserSpec.ClusterId.ValueString(), &sqlUserRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating sql user",
			fmt.Sprintf("Could not create sql user: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	diags = resp.State.Set(ctx, sqlUserSpec)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *sqlUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state SQLUser
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Since the state may have come from an import, we need to retrieve
	// the actual user list and make sure this one is in there.
	apiResp, _, err := r.provider.service.ListSQLUsers(ctx, state.ClusterId.ValueString(), &client.ListSQLUsersOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Couldn't retrieve SQL users",
			fmt.Sprintf("Unexpected error retrieving SQL users: %s", formatAPIErrorMessage(err)),
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	for _, user := range apiResp.GetUsers() {
		if user.GetName() == state.Name.ValueString() {
			return
		}
	}
	resp.Diagnostics.AddWarning(
		"Couldn't find user.",
		fmt.Sprintf("This cluster doesn't have a SQL user named '%v'. Removing from state.", state.Name.ValueString()),
	)
	resp.State.RemoveResource(ctx)
}

func (r *sqlUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Get plan values
	var plan SQLUser
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state SQLUser
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := client.UpdateSQLUserPasswordRequest{Password: plan.Password.ValueString()}
	_, _, err := r.provider.service.UpdateSQLUserPassword(ctx, plan.ClusterId.ValueString(), plan.Name.ValueString(), &updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating sql user password",
			fmt.Sprintf("Could not update sql user password: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *sqlUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SQLUser
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, _, err := r.provider.service.DeleteSQLUser(ctx, state.ClusterId.ValueString(), state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting sql user",
			fmt.Sprintf("Could not delete sql user: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r *sqlUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Since a SQL user is uniquely identified by two fields, the cluster ID
	// and the name, we serialize them both into the ID field. To make import
	// work, we need to deserialize an ID back into name and cluster ID.
	matches := sqlUserIDRegex.FindStringSubmatch(req.ID)
	if len(matches) != 3 {
		resp.Diagnostics.AddError(
			"Invalid SQL user ID format",
			`When importing a SQL user, the ID field should follow the format "<cluster ID>:<SQL user name>")`)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	sqlUser := SQLUser{
		ClusterId: types.StringValue(matches[1]),
		Name:      types.StringValue(matches[2]),
		ID:        types.StringValue(req.ID),
	}
	resp.Diagnostics = resp.State.Set(ctx, &sqlUser)
}

func NewSQLUserResource() resource.Resource {
	return &sqlUserResource{}
}
