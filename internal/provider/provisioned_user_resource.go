/*
Copyright 2026 The Cockroach Authors

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
	"sort"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v7/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// scimUserSchemas returns the schema URI list that every CC SCIM v2 user
// request must carry. Returned as a fresh slice so callers can take its
// address without aliasing a shared backing array.
func scimUserSchemas() []string {
	return []string{"urn:ietf:params:scim:schemas:core:2.0:User"}
}

type provisionedUserResource struct {
	provider *provider
}

func NewProvisionedUserResource() resource.Resource {
	return &provisionedUserResource{}
}

func (r *provisionedUserResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A CockroachDB Cloud console user managed via the SCIM 2.0 user endpoints. " +
			"This is the same record an identity provider creates when autoprovisioning a user on SSO login; " +
			"this resource lets terraform pre-provision (or otherwise own) that record directly. " +
			"Roles for the user are granted independently via the [cockroach_user_role_grant](user_role_grant) resource. " +
			"Coexistence with an IdP-driven SCIM sync against the same organization is not supported — terraform is " +
			"assumed to be the sole writer for any user it manages.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Server-assigned SCIM user ID (UUID).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_name": schema.StringAttribute{
				MarkdownDescription: "Login identifier for the user, typically the primary email address. " +
					"The SCIM endpoint does not support renaming, so changing this forces resource replacement.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Human-readable display name shown in the CC console.",
				Optional:            true,
			},
			"external_id": schema.StringAttribute{
				MarkdownDescription: "Identifier for the user assigned by the provisioning client (typically the IdP). " +
					"Useful when bridging to an external identity store.",
				Optional: true,
			},
			"active": schema.BoolAttribute{
				MarkdownDescription: "Whether the user is active. Setting to `false` disables sign-in without deleting the record. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"emails": schema.ListNestedAttribute{
				MarkdownDescription: "Email addresses for the user. At least one is required by the SCIM endpoint. " +
					"Entries are stored and compared in alphabetical order by `value`, so ordering in configuration does not matter.",
				Required: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"value": schema.StringAttribute{
							MarkdownDescription: "The email address.",
							Required:            true,
						},
						"display": schema.StringAttribute{
							MarkdownDescription: "Human-readable display value for the address.",
							Optional:            true,
						},
						"primary": schema.BoolAttribute{
							MarkdownDescription: "Whether this address is the user's primary email. At most one entry should set this to `true`.",
							Optional:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: "A category label for the address, for example `work` or `home`.",
							Optional:            true,
						},
					},
				},
			},
			"name": schema.SingleNestedAttribute{
				MarkdownDescription: "Structured name of the user.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"given_name": schema.StringAttribute{
						MarkdownDescription: "The user's given (first) name.",
						Optional:            true,
					},
					"family_name": schema.StringAttribute{
						MarkdownDescription: "The user's family (last) name.",
						Optional:            true,
					},
				},
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "ISO 8601 timestamp at which the user record was created on the server.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"last_updated_at": schema.StringAttribute{
				MarkdownDescription: "ISO 8601 timestamp at which the user record was last modified on the server.",
				Computed:            true,
			},
		},
	}
}

func (r *provisionedUserResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_provisioned_user"
}

func (r *provisionedUserResource) Configure(
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

func (r *provisionedUserResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan ProvisionedUser
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only forward optional scalar fields that are known in the plan.
	// ValueStringPointer on an unknown types.String yields a non-nil pointer
	// to an empty string, which the SCIM endpoint would otherwise treat as
	// an explicit empty value.
	createReq := &client.CreateUserRequest{
		Schemas: scimUserSchemas(),
		Emails:  toScimEmails(plan.Emails),
		Name:    toScimName(plan.Name),
	}
	if IsKnown(plan.UserName) {
		createReq.UserName = plan.UserName.ValueStringPointer()
	}
	if IsKnown(plan.DisplayName) {
		createReq.DisplayName = plan.DisplayName.ValueStringPointer()
	}
	if IsKnown(plan.ExternalID) {
		createReq.ExternalId = plan.ExternalID.ValueStringPointer()
	}
	if IsKnown(plan.Active) {
		createReq.Active = plan.Active.ValueBoolPointer()
	}

	traceAPICall("CreateUser")
	userResp, _, err := r.provider.service.CreateUser(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating provisioned user",
			fmt.Sprintf("Could not create provisioned user: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state ProvisionedUser
	loadProvisionedUserToTerraformState(scimUserFromCreateResponse(userResp), &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *provisionedUserResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state ProvisionedUser
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.ID.IsNull() {
		return
	}

	userID := state.ID.ValueString()
	if !uuidRegex.MatchString(userID) {
		resp.Diagnostics.AddError(
			"Unexpected provisioned user ID format",
			fmt.Sprintf("'%s' is not a valid provisioned user ID format. Expected UUID.", userID),
		)
		return
	}

	traceAPICall("GetUser")
	userObj, httpResp, err := r.provider.service.GetUser(ctx, userID, nil /* options */)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Provisioned user not found",
				fmt.Sprintf("Provisioned user with ID %s is not found. Removing from state.", userID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting provisioned user",
				fmt.Sprintf("Unexpected error retrieving provisioned user info: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	loadProvisionedUserToTerraformState(userObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *provisionedUserResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan ProvisionedUser
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ProvisionedUser
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// UpdateUser is a SCIM PUT. Every known field from the plan is wired so
	// the request body reflects the user's full desired state. Fields that
	// are unknown in the plan are skipped to avoid sending non-nil pointers
	// to empty strings (see Create for the same gate). Note that the SDK
	// uses `omitempty` on every optional field, so a field removed entirely
	// from configuration produces a JSON absence and the server preserves
	// the existing value — clearing an optional field requires an explicit
	// empty value in configuration.
	emails := toScimEmails(plan.Emails)
	schemas := scimUserSchemas()
	updateReq := &client.UpdateUserRequest{
		Schemas: &schemas,
		Emails:  &emails,
		Name:    toScimName(plan.Name),
	}
	if IsKnown(plan.UserName) {
		updateReq.UserName = plan.UserName.ValueStringPointer()
	}
	if IsKnown(plan.DisplayName) {
		updateReq.DisplayName = plan.DisplayName.ValueStringPointer()
	}
	if IsKnown(plan.ExternalID) {
		updateReq.ExternalId = plan.ExternalID.ValueStringPointer()
	}
	if IsKnown(plan.Active) {
		updateReq.Active = plan.Active.ValueBoolPointer()
	}

	traceAPICall("UpdateUser")
	userObj, _, err := r.provider.service.UpdateUser(ctx, plan.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating provisioned user",
			fmt.Sprintf("Could not update provisioned user: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadProvisionedUserToTerraformState(userObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *provisionedUserResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state ProvisionedUser
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	userID := state.ID
	if userID.IsNull() {
		return
	}

	traceAPICall("DeleteUser")
	httpResp, err := r.provider.service.DeleteUser(ctx, userID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			// User is already gone; swallow.
		} else {
			resp.Diagnostics.AddError(
				"Error deleting provisioned user",
				fmt.Sprintf("Could not delete provisioned user: %s", formatAPIErrorMessage(err)),
			)
			return
		}
	}

	resp.State.RemoveResource(ctx)
}

func (r *provisionedUserResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// toScimEmails converts the terraform email list to the SDK shape, sorted by
// value so the wire order is deterministic and matches what
// loadProvisionedUserToTerraformState reads back from the server response.
// Optional scalar fields are gated on IsKnown: ValueStringPointer/
// ValueBoolPointer on an unknown value returns a non-nil pointer to the zero
// value, which the SCIM endpoint would otherwise treat as an explicit set.
func toScimEmails(in []ProvisionedUserEmail) []client.ScimEmail {
	out := make([]client.ScimEmail, 0, len(in))
	for _, e := range in {
		var display *string
		if IsKnown(e.Display) {
			display = e.Display.ValueStringPointer()
		}
		var primary *bool
		if IsKnown(e.Primary) {
			primary = e.Primary.ValueBoolPointer()
		}
		var emailType *string
		if IsKnown(e.Type) {
			emailType = e.Type.ValueStringPointer()
		}
		out = append(out, client.ScimEmail{
			Value:   e.Value.ValueString(),
			Display: display,
			Primary: primary,
			Type:    emailType,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

// toScimName converts a terraform name block to the SDK shape. Returning nil
// when the input is nil — or when both inner fields are unknown/null —
// keeps a missing or empty `name` block out of the wire request entirely.
// Inner fields are gated on IsKnown for the same reason as toScimEmails:
// ValueStringPointer on an unknown value yields a non-nil pointer to an
// empty string, which the SCIM endpoint would treat as an explicit set.
func toScimName(in *ProvisionedUserName) *client.ScimName {
	if in == nil {
		return nil
	}
	var givenName *string
	if IsKnown(in.GivenName) {
		givenName = in.GivenName.ValueStringPointer()
	}
	var familyName *string
	if IsKnown(in.FamilyName) {
		familyName = in.FamilyName.ValueStringPointer()
	}
	if givenName == nil && familyName == nil {
		return nil
	}
	return &client.ScimName{
		GivenName:  givenName,
		FamilyName: familyName,
	}
}

// scimUserFromCreateResponse adapts a CreateUserResponse to a ScimUser. The
// two SDK types are structurally identical; the adapter exists so a single
// loader can handle Create, Read, and Update responses.
func scimUserFromCreateResponse(r *client.CreateUserResponse) *client.ScimUser {
	if r == nil {
		return nil
	}
	return &client.ScimUser{
		Active:      r.Active,
		DisplayName: r.DisplayName,
		Emails:      r.Emails,
		ExternalId:  r.ExternalId,
		Groups:      r.Groups,
		Id:          r.Id,
		Meta:        r.Meta,
		Name:        r.Name,
		Schemas:     r.Schemas,
		UserName:    r.UserName,
	}
}

func loadProvisionedUserToTerraformState(u *client.ScimUser, state *ProvisionedUser) {
	state.ID = types.StringValue(u.Id)
	state.UserName = types.StringPointerValue(u.UserName)
	state.DisplayName = types.StringPointerValue(u.DisplayName)
	state.ExternalID = types.StringPointerValue(u.ExternalId)

	// `active` defaults to true in the schema. Folding a nil server value back
	// to that default keeps refresh from flipping state to null on a server
	// that omits the field.
	if u.Active != nil {
		state.Active = types.BoolValue(*u.Active)
	} else {
		state.Active = types.BoolValue(true)
	}

	state.Emails = fromScimEmails(u.Emails)
	state.Name = fromScimName(u.Name)

	if u.Meta != nil {
		state.CreatedAt = types.StringPointerValue(u.Meta.Created)
		state.LastUpdatedAt = types.StringPointerValue(u.Meta.LastModified)
	} else {
		state.CreatedAt = types.StringNull()
		state.LastUpdatedAt = types.StringNull()
	}
}

func fromScimEmails(in *[]client.ScimEmail) []ProvisionedUserEmail {
	if in == nil {
		return nil
	}
	src := append([]client.ScimEmail(nil), *in...)
	sort.SliceStable(src, func(i, j int) bool { return src[i].Value < src[j].Value })
	out := make([]ProvisionedUserEmail, 0, len(src))
	for _, e := range src {
		out = append(out, ProvisionedUserEmail{
			Value:   types.StringValue(e.Value),
			Display: types.StringPointerValue(e.Display),
			Primary: types.BoolPointerValue(e.Primary),
			Type:    types.StringPointerValue(e.Type),
		})
	}
	return out
}

// fromScimName collapses a name object whose sub-fields are all nil back to a
// nil terraform value. Some SCIM servers return a non-nil Name with empty
// inner pointers even when the user never set one; without this collapse,
// refresh would force a permadiff against an unset `name` block.
func fromScimName(in *client.ScimName) *ProvisionedUserName {
	if in == nil {
		return nil
	}
	if in.GivenName == nil && in.FamilyName == nil {
		return nil
	}
	return &ProvisionedUserName{
		GivenName:  types.StringPointerValue(in.GivenName),
		FamilyName: types.StringPointerValue(in.FamilyName),
	}
}
