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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v5/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const roleGrantPaginationLimit = 100

type userRoleGrantsResource struct {
	provider *provider
}

func (r *userRoleGrantsResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage all the role grants for a user. This resource is authoritative.  If role grants are added elsewhere, for example, via the console UI or another terraform project, using this resource will try to reset them. Use the [cockroach_user_role_grant](user_role_grant) resource for non-authoritative role grants.  ORG_MEMBER is a required role and must be included.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "Always matches the user ID. Required by Terraform.",
			},
			"user_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the user to grant these roles to.",
			},
			"roles": schema.SetNestedAttribute{
				Required:    true,
				Description: "The list of roles to include. ORG_MEMBER must be included.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"role_name": schema.StringAttribute{
							Required: true,
							MarkdownDescription: "Name of the role to grant. Allowed values are:" +
								formatEnumMarkdownList(client.AllowedOrganizationUserRoleTypeEnumValues),
						},
						"resource_type": schema.StringAttribute{
							Required: true,
							MarkdownDescription: "Type of resource. Allowed values are: " +
								formatEnumMarkdownList(client.AllowedResourceTypeTypeEnumValues),
						},
						"resource_id": schema.StringAttribute{
							Optional:    true,
							Description: "ID of the resource. Required if the resource_type is 'FOLDER' or 'CLUSTER'. It should be omitted otherwise.",
						},
					},
				},
			},
		},
	}
}

func (r *userRoleGrantsResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_user_role_grants"
}

func (r *userRoleGrantsResource) Configure(
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

func (r *userRoleGrantsResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var roleGrantSpec UserRoleGrants
	diags := req.Plan.Get(ctx, &roleGrantSpec)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("GetAllRolesForUser")
	_, _, err := r.provider.service.GetAllRolesForUser(ctx, roleGrantSpec.UserId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting the user's preexisting roles",
			fmt.Sprintf("Could not get roles: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var roles []client.BuiltInRole
	for _, role := range roleGrantSpec.Roles {
		apiRole, err := fromRoleToBuiltInRole(role)
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid role format",
				fmt.Sprintf("Could not parse role: %s", formatAPIErrorMessage(err)),
			)
			return
		}
		roles = append(roles, *apiRole)
	}
	var setRoleRequest client.CockroachCloudSetRolesForUserRequest
	setRoleRequest.SetRoles(roles)

	traceAPICall("SetRolesForUser")
	_, _, err = r.provider.service.SetRolesForUser(ctx, roleGrantSpec.UserId.ValueString(), &setRoleRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error setting roles for user",
			fmt.Sprintf("Could not set roles for user: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	roleGrantSpec.ID = roleGrantSpec.UserId
	diags = resp.State.Set(ctx, roleGrantSpec)
	resp.Diagnostics.Append(diags...)
}

func (r *userRoleGrantsResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state UserRoleGrants
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Since the state may have come from an import, we need to retrieve
	// the actual user list and make sure this one is in there.
	var page string
	limit := int32(roleGrantPaginationLimit)

	for {
		options := &client.ListRoleGrantsOptions{
			PaginationPage:  &page,
			PaginationLimit: &limit,
		}
		traceAPICall("ListRoleGrants")
		apiResp, _, err := r.provider.service.ListRoleGrants(ctx, options)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error listing user roles",
				fmt.Sprintf("Unexpected error retrieving user roles: %s", formatAPIErrorMessage(err)),
			)
			return
		}

		for _, grant := range apiResp.GetGrants() {
			if grant.GetUserId() == state.UserId.ValueString() {
				loadListRolesToTerraformState(state.UserId.ValueString(), &grant, &state)
				state.ID = state.UserId
				diags = resp.State.Set(ctx, state)
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
		"Couldn't find user.",
		fmt.Sprintf(
			"Could not find user with ID '%v'. Removing from state.",
			state.UserId.ValueString(),
		),
	)
	resp.State.RemoveResource(ctx)
}

func (r *userRoleGrantsResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	// Get plan values
	var plan UserRoleGrants
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state UserRoleGrants
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var roles []client.BuiltInRole
	for _, role := range plan.Roles {
		apiRole, err := fromRoleToBuiltInRole(role)
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid role format",
				fmt.Sprintf("Could not parse role: %s", formatAPIErrorMessage(err)),
			)
			return
		}
		roles = append(roles, *apiRole)
	}
	var setRoleRequest client.CockroachCloudSetRolesForUserRequest
	setRoleRequest.SetRoles(roles)

	traceAPICall("SetRolesForUser")
	apiResp, _, err := r.provider.service.SetRolesForUser(ctx, plan.UserId.ValueString(), &setRoleRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error setting roles for user",
			fmt.Sprintf("Could not set roles for user: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadRolesToTerraformState(state.UserId.ValueString(), apiResp, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *userRoleGrantsResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state UserRoleGrants
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	emptyRoleRequest := &client.CockroachCloudSetRolesForUserRequest{}
	traceAPICall("SetRolesForUser")
	_, _, err := r.provider.service.SetRolesForUser(ctx, state.UserId.ValueString(), emptyRoleRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error setting roles for user",
			fmt.Sprintf("Could not set roles for user: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r *userRoleGrantsResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("user_id"), req, resp)
}

func NewUserRoleGrantsResource() resource.Resource {
	return &userRoleGrantsResource{}
}

func fromRoleToBuiltInRole(role Role) (*client.BuiltInRole, error) {
	apiId := role.ResourceId.ValueString()
	apiType, err := client.NewResourceTypeTypeFromValue(role.ResourceType.ValueString())
	if err != nil {
		return nil, err
	}
	apiRoleName, err := client.NewOrganizationUserRoleTypeFromValue(role.RoleName.ValueString())
	if err != nil {
		return nil, err
	}
	apiResource := client.Resource{
		Id:   &apiId,
		Type: *apiType,
	}
	apiRole := client.BuiltInRole{
		Name:     *apiRoleName,
		Resource: apiResource,
	}
	return &apiRole, nil
}

func loadRolesToTerraformState(
	userId string, roles *client.GetAllRolesForUserResponse, state *UserRoleGrants,
) {
	roleGrants := &client.UserRoleGrants{
		Roles:  roles.GetRoles(),
		UserId: userId,
	}
	loadListRolesToTerraformState(userId, roleGrants, state)
}

func loadListRolesToTerraformState(
	userId string, roles *client.UserRoleGrants, state *UserRoleGrants,
) {
	state.UserId = types.StringValue(userId)
	var tfRoles []Role
	for _, role := range roles.GetRoles() {
		tfRole := Role{
			RoleName:     types.StringValue(string(role.Name)),
			ResourceType: types.StringValue(string(role.GetResource().Type)),
			ResourceId:   types.StringValue(role.Resource.GetId()),
		}
		tfRoles = append(tfRoles, tfRole)
	}
	state.Roles = tfRoles
}
