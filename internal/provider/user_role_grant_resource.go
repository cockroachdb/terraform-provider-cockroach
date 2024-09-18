/*
Copyright 2024 The Cockroach Authors

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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v3/pkg/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type userRoleGrantResource struct {
	provider *provider
}

func (r *userRoleGrantResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A role grant for a user. This resource is recommended to be used when a user's roles are managed across multiple terraform projects or in conjunction with console UI granted roles. For authoritative management over a user's roles, use the [cockroach_user_role_grants](user_role_grants) resource.\n\n As with all terraform resources, care must be taken to limit management of the same resource to a single project.",
		Attributes: map[string]schema.Attribute{
			"user_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the user to grant these roles to.",
			},
			"role": schema.SingleNestedAttribute{
				Required: true,
				Attributes: map[string]schema.Attribute{
					"role_name": schema.StringAttribute{
						Required: true,
						MarkdownDescription: "Name of the role to grant. Allowed values are:" +
							formatEnumMarkdownList(client.AllowedOrganizationUserRoleTypeEnumValues),
					},
					"resource_type": schema.StringAttribute{
						Required: true,
						MarkdownDescription: "Type of resource. Allowed values are:" +
							formatEnumMarkdownList(client.AllowedResourceTypeTypeEnumValues),
					},
					"resource_id": schema.StringAttribute{
						Optional:    true,
						Description: "ID of the resource. Required if the resource_type is 'FOLDER' or 'CLUSTER'. It should be omitted otherwise.",
					},
				},
			},
		},
	}
}

func (r *userRoleGrantResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_user_role_grant"
}

func (r *userRoleGrantResource) Configure(
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

func (r *userRoleGrantResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan UserRoleGrant
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := fromRoleToBuiltInRole(plan.Role)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid role format",
			fmt.Sprintf("Could not parse role: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// Verify that the user does not already have this role.  AddUserToRole is
	// idempotent so it could return successful even if it already existed.  In
	// that case, the project would think it owns this resource when it should not.
	// There is a race condition here where another process adds the resource
	// after our check but before adding the role.
	traceAPICall("GetAllRolesForUser")
	rolesResp, _, err := r.provider.service.GetAllRolesForUser(ctx, plan.UserID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error adding role for user",
			fmt.Sprintf("Failed during check for pre-existing role: %s", formatAPIErrorMessage(err)),
		)
		return
	}
	for _, existingRole := range *rolesResp.Roles {
		if roleEqualsBuiltInRole(&plan.Role, &existingRole) {
			resp.Diagnostics.AddError(
				"Error adding role for user",
				"Role already exists",
			)
			return
		}
	}

	traceAPICall("AddUserToRole")
	_, _, err = r.provider.service.AddUserToRole(
		ctx,
		plan.UserID.ValueString(),
		plan.Role.ResourceType.ValueString(),
		plan.Role.ResourceId.ValueString(),
		plan.Role.RoleName.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error adding role for user",
			fmt.Sprintf("Could not add role for user: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *userRoleGrantResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state UserRoleGrant
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	stateUserID := state.UserID.ValueString()

	traceAPICall("GetAllRolesRoleUser")
	apiResp, _, err := r.provider.service.GetAllRolesForUser(ctx, stateUserID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error listing user roles",
			fmt.Sprintf("Unexpected error retrieving user roles: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	respUserRoles := *apiResp.Roles

	for _, respRole := range respUserRoles {
		if roleEqualsBuiltInRole(&state.Role, &respRole) {
			// If we did find the matching role grant there isn't anything
			// else to do because these roles are immutable.
			return
		}
	}

	// If we never found the matching role grant we need to remove
	// it from the state.
	if len(respUserRoles) == 0 {
		resp.Diagnostics.AddWarning(
			"Couldn't find user role grant.",
			fmt.Sprintf(
				"Could not find any roles for a user matching the id=%s. Removing role grant from state.",
				stateUserID,
			),
		)
	} else {
		resp.Diagnostics.AddWarning(
			"Couldn't find user role grant.",
			fmt.Sprintf(
				"Could not find user role grant for user (id=%s) and role (%s/%s/%s). Removing role grant from state.",
				stateUserID,
				state.Role.RoleName.ValueString(),
				state.Role.ResourceType.ValueString(),
				state.Role.ResourceId.ValueString(),
			),
		)
	}
	resp.State.RemoveResource(ctx)
}

// This resource is immutable so no action is necessary
func (r *userRoleGrantResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
}

func (r *userRoleGrantResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state UserRoleGrant
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("RemoveUserFromRole")
	_, _, err := r.provider.service.RemoveUserFromRole(ctx, state.UserID.ValueString(), state.Role.ResourceType.ValueString(), state.Role.ResourceId.ValueString(), state.Role.RoleName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error remove user role grant for user",
			fmt.Sprintf("Could not remove user role grant for user: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r *userRoleGrantResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	userID, roleName, resourceType, resourceID, err := parseRoleGrantResourceID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			err.Error(),
		)
		return
	}

	role := Role{
		RoleName:     types.StringValue(roleName),
		ResourceType: types.StringValue(resourceType),
		ResourceId:   types.StringValue(resourceID),
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), userID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role"), role)...)
}

func NewUserRoleGrantResource() resource.Resource {
	return &userRoleGrantResource{}
}

const resourceTypeOrg = string(client.RESOURCETYPETYPE_ORGANIZATION)

func parseRoleGrantResourceID(
	id string,
) (userID, roleName, resourceType, resourceID string, err error) {
	idParts := strings.Split(id, ",")
	lengthIDParts := len(idParts)
	if lengthIDParts == 3 || lengthIDParts == 4 {

		userID = idParts[0]
		roleName = idParts[1]
		resourceType = idParts[2]

		if _, err := uuid.Parse(userID); err != nil {
			return "", "", "", "", fmt.Errorf("unable to parse valid uuid from userID, got: %q", userID)
		}

		if len(idParts) == 4 {
			resourceID := idParts[3]

			if resourceType == resourceTypeOrg && resourceID != "" {
				return "", "", "", "", fmt.Errorf("when resourceType is %s, resourceID must be empty, got: %q", resourceTypeOrg, resourceID)
			}
			if resourceID != "" {
				if _, err := uuid.Parse(resourceID); err != nil {
					return "", "", "", "", fmt.Errorf("unable to parse valid uuid from resourceID, got: %q", resourceID)
				}
			}
			return userID, roleName, resourceType, resourceID, nil
		}
		if len(idParts) == 3 && resourceType == resourceTypeOrg {
			return userID, roleName, resourceType, "", nil
		}
	}

	return "", "", "", "", fmt.Errorf("expected import identifier with format: <user_id>,<role_name>,<resource_type>,<resource_id>. Got: %q", id)
}

func roleEqualsBuiltInRole(role *Role, builtIn *client.BuiltInRole) bool {
	builtInResourceIDString := ""
	if builtIn.Resource.Id != nil {
		builtInResourceIDString = *builtIn.Resource.Id
	}

	return role.ResourceType.ValueString() == string(builtIn.Resource.Type) &&
		role.RoleName.ValueString() == string(builtIn.Name) &&
		role.ResourceId.ValueString() == builtInResourceIDString
}
