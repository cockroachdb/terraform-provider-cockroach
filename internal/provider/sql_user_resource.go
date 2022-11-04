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
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type sqlUserResourceType struct{}

// clusterID:name
const sqlUserIDFmt = "%s:%s"
const sqlUserNameRegex = "[A-Za-z0-9_][A-Za-z0-9\\._\\-]{0,62}"

var sqlUserIDRegex = regexp.MustCompile(fmt.Sprintf("^(%s):(%s)$", uuidRegex, sqlUserNameRegex))

func (s sqlUserResourceType) GetSchema(_ context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		MarkdownDescription: "SQL user and password",
		Attributes: map[string]tfsdk.Attribute{
			"cluster_id": {
				Required: true,
				Type:     types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.RequiresReplace(),
				},
			},
			"name": {
				Required: true,
				Type:     types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.RequiresReplace(),
				},
			},
			"password": {
				Required:  true,
				Type:      types.StringType,
				Sensitive: true,
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

func (s sqlUserResourceType) NewResource(_ context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return sqlUserResource{
		provider: provider,
	}, diags
}

type sqlUserResource struct {
	provider provider
}

func (s sqlUserResource) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	if !s.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var sqlUserSpec SQLUser
	diags := req.Config.Get(ctx, &sqlUserSpec)
	resp.Diagnostics.Append(diags...)
	// Create a unique ID (required by terraform framework) by combining
	// the cluster ID and username.
	sqlUserSpec.ID = types.String{
		Value: fmt.Sprintf(sqlUserIDFmt, sqlUserSpec.ClusterId.Value, sqlUserSpec.Name.Value),
	}

	if resp.Diagnostics.HasError() {
		return
	}

	_, _, err := s.provider.service.GetCluster(ctx, sqlUserSpec.ClusterId.Value)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting the cluster",
			fmt.Sprintf("Could not get the cluster: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var sqlUserRequest client.CreateSQLUserRequest
	sqlUserRequest.Name = sqlUserSpec.Name.Value
	sqlUserRequest.Password = sqlUserSpec.Password.Value

	_, _, err = s.provider.service.CreateSQLUser(ctx, sqlUserSpec.ClusterId.Value, &sqlUserRequest)
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

func (s sqlUserResource) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	if !s.provider.configured {
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
	apiResp, _, err := s.provider.service.ListSQLUsers(ctx, state.ClusterId.Value, &client.ListSQLUsersOptions{})
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
		if user.GetName() == state.Name.Value {
			return
		}
	}
	resp.Diagnostics.AddWarning(
		"Couldn't find user.",
		fmt.Sprintf("This cluster doesn't have a SQL user named '%v'. Removing from state.", state.Name.Value),
	)
	resp.State.RemoveResource(ctx)
}

func (s sqlUserResource) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
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

	updateReq := client.UpdateSQLUserPasswordRequest{Password: plan.Password.Value}
	_, _, err := s.provider.service.UpdateSQLUserPassword(ctx, plan.ClusterId.Value, plan.Name.Value, &updateReq)
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

func (s sqlUserResource) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var state SQLUser
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, _, err := s.provider.service.DeleteSQLUser(ctx, state.ClusterId.Value, state.Name.Value)
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

func (s sqlUserResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
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
		ClusterId: types.String{Value: matches[1]},
		Name:      types.String{Value: matches[2]},
		ID:        types.String{Value: req.ID},
	}
	resp.Diagnostics = resp.State.Set(ctx, &sqlUser)
}
