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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type sqlUserResourceType struct{}

func (s sqlUserResourceType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		MarkdownDescription: "SQL user and password",
		Attributes: map[string]tfsdk.Attribute{
			"id": {
				Required: true,
				Type:     types.StringType,
			},
			"name": {
				Required: true,
				Type:     types.StringType,
			},
			"password": {
				Required:  true,
				Type:      types.StringType,
				Sensitive: true,
			},
		},
	}, nil
}

func (s sqlUserResourceType) NewResource(ctx context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
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

	var sqlUserSpec SQLUserSpecification
	diags := req.Config.Get(ctx, &sqlUserSpec)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, err := s.provider.service.GetCluster(ctx, sqlUserSpec.Id.Value)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting the cluster",
			fmt.Sprintf("Could not get the cluster, unexpected error: %v %v "+err.Error(), httpResp),
		)
		return
	}

	var sqlUserRequest client.CreateSQLUserRequest
	sqlUserRequest.Name = sqlUserSpec.Name.Value
	sqlUserRequest.Password = sqlUserSpec.Password.Value

	_, httpResp, err = s.provider.service.CreateSQLUser(ctx, sqlUserSpec.Id.Value, &sqlUserRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating sql user",
			fmt.Sprintf("Could not create sql user, unexpected error: %v %v "+err.Error(), httpResp),
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

	var sqlUser SQLUserSpecification
	diags := req.State.Get(ctx, &sqlUser)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (s sqlUserResource) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	// Get plan values
	var plan SQLUserSpecification
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state SQLUserSpecification
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Name != state.Name || plan.Id != state.Id {
		resp.Diagnostics.AddError(
			"can not change an anything apart from the password",
			"You can only change password of the sql user. Thanks!",
		)
		return
	}

	updateReq := client.UpdateSQLUserPasswordRequest{Password: plan.Password.Value}
	_, httpResp, err := s.provider.service.UpdateSQLUserPassword(ctx, plan.Id.Value, plan.Name.Value, &updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating sql user password",
			fmt.Sprintf("Could not update sql user password, unexpected error: %v %v "+err.Error(), httpResp),
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
	var state SQLUserSpecification
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, err := s.provider.service.DeleteSQLUser(ctx, state.Id.Value, state.Name.Value)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting sql user",
			fmt.Sprintf("Could not delete sql user, unexpected error: %v %v "+err.Error(), httpResp),
		)
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (s sqlUserResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("id"), req, resp)
}
