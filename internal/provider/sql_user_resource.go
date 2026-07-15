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
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v8/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type sqlUserResource struct {
	provider *provider
}

// clusterID:name
const (
	sqlUserIDFmt     = "%s:%s"
	sqlUserNameRegex = "[A-Za-z0-9_][A-Za-z0-9\\._\\-]{0,62}"
	passwordLength   = 32
	// SQL users are lightweight, so we can use a large page size.
	sqlUserPaginationLimit = 500
)

const passwordDeprecationMessage = "The `password` attribute persists the clear text password in Terraform state. " +
	"Migrate to `password_wo` (with `password_wo_version` for rotations) to keep the credentials out of state. " +
	"`password` will be removed in a future major release of the provider."

const (
	unknownPasswordWODiagSummary = "Unknown password_wo value"
	unknownPasswordWODiagDetail  = "The password_wo attribute must be known at apply time. " +
		"Ensure the value is not derived from an unknown expression."

	unknownPasswordDiagSummary = "Unknown password value"
	unknownPasswordDiagDetail  = "The password attribute must be known at apply time. " +
		"Ensure the value is not derived from an unknown expression."
)

var sqlUserIDRegex = regexp.MustCompile(fmt.Sprintf("^(%s):(%s)$", uuidRegexString, sqlUserNameRegex))

func (r *sqlUserResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "CockroachDB SQL user.",
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
				Description: "SQL user name.",
			},
			"password": schema.StringAttribute{
				Optional:           true,
				Sensitive:          true,
				DeprecationMessage: passwordDeprecationMessage,
				Validators:         []validator.String{stringvalidator.LengthBetween(12, 500)},
				Description: "Deprecated. If provided, this field sets the password of the SQL user when created. " +
					"The value is persisted in Terraform state, which is the reason for deprecation; " +
					"prefer `password_wo`. If omitted, a random password is generated and not saved to state. " +
					"The password must be changed via the CockroachDB cloud console.",
			},
			"password_wo": schema.StringAttribute{
				Optional:   true,
				Sensitive:  true,
				WriteOnly:  true,
				Validators: []validator.String{stringvalidator.LengthBetween(12, 500)},
				Description: "Write-only password for the SQL user (Terraform CLI 1.11+ required). " +
					"The value is sent on create and on rotation but never stored in Terraform state. " +
					"To rotate, change the value and bump `password_wo_version`; without a version bump, " +
					"Terraform cannot detect changes to a write-only attribute. Mutually exclusive with `password`.",
			},
			"password_wo_version": schema.Int64Attribute{
				Optional: true,
				Description: "Trigger attribute for rotating `password_wo`. Increment this integer to force " +
					"Terraform to re-apply the current `password_wo` value. Only meaningful when `password_wo` is set.",
			},
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "A unique identifier with format `<cluster ID>:<SQL user name>`.",
			},
		},
	}
}

// ConfigValidators makes `password` mutually exclusive with the `password_wo`/`password_wo_version` pair, and requires that pair to be set together.
func (r *sqlUserResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.Conflicting(
			path.MatchRoot("password"),
			path.MatchRoot("password_wo"),
		),
		resourcevalidator.Conflicting(
			path.MatchRoot("password"),
			path.MatchRoot("password_wo_version"),
		),
		resourcevalidator.RequiredTogether(
			path.MatchRoot("password_wo"),
			path.MatchRoot("password_wo_version"),
		),
	}
}

func (r *sqlUserResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_sql_user"
}

func (r *sqlUserResource) Configure(
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

// generateRandomPassword generates a password of length passwordLength.
// The password has 6 bits of entropy per character, so a password
// of length 32 will have an entropy of 192.
var generateRandomPassword = func() (string, error) {
	// Base64 is 4/3 the size of the raw data.
	// It's easier to read an extra 1/3 of the bytes than to do math.
	bytes := make([]byte, passwordLength)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}

	// Use RawURLEncoding which is case-sensitive alphanumeric with '-_', and no padding.
	longPassword := base64.RawURLEncoding.EncodeToString(bytes)
	result := longPassword[:passwordLength]

	return result, nil
}

func (r *sqlUserResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var sqlUserSpec SQLUser
	diags := req.Plan.Get(ctx, &sqlUserSpec)
	resp.Diagnostics.Append(diags...)

	// Write-only attributes are absent from plan and state, so read password_wo
	// directly from configuration.
	var passwordWO types.String
	diags = req.Config.GetAttribute(ctx, path.Root("password_wo"), &passwordWO)
	resp.Diagnostics.Append(diags...)

	// Create a unique ID (required by terraform framework) by combining
	// the cluster ID and username.
	sqlUserSpec.ID = types.StringValue(fmt.Sprintf(
		sqlUserIDFmt, sqlUserSpec.ClusterId.ValueString(), sqlUserSpec.Name.ValueString()))

	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("GetCluster")
	_, _, err := r.provider.service.GetCluster(ctx, sqlUserSpec.ClusterId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting the cluster",
			fmt.Sprintf("Could not get the cluster: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// Resolve the password from whichever source the consumer chose. The
	// ConfigValidators above guarantee `password` and `password_wo` are not
	// both set, so the order below is a fallthrough, not a precedence rule.
	var sqlUserRequest client.CreateSQLUserBody
	sqlUserRequest.Name = sqlUserSpec.Name.ValueString()
	switch {
	case passwordWO.IsUnknown():
		// password_wo is sourced from configuration, so it is fully known by
		// the time Create runs under a normal apply. Guard the pathological
		// case rather than silently creating a user with an empty password.
		resp.Diagnostics.AddError(unknownPasswordWODiagSummary, unknownPasswordWODiagDetail)
		return

	case sqlUserSpec.Password.IsUnknown():
		// Symmetric with the password_wo guard above: an unknown legacy
		// `password` is not null, so it would otherwise fall through to the
		// case below and ValueString() would yield "", sending an empty
		// password to the API. Surface a clear error instead.
		resp.Diagnostics.AddError(unknownPasswordDiagSummary, unknownPasswordDiagDetail)
		return

	case !passwordWO.IsNull():
		sqlUserRequest.Password = passwordWO.ValueString()

	case !sqlUserSpec.Password.IsNull():
		sqlUserRequest.Password = sqlUserSpec.Password.ValueString()

	default:
		sqlUserRequest.Password, err = generateRandomPassword()
		if err != nil {
			resp.Diagnostics.AddError("Error generating password", err.Error())
			return
		}
	}

	traceAPICall("CreateSQLUser")
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

func (r *sqlUserResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
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
	var page string
	limit := int32(sqlUserPaginationLimit)

	for {
		options := &client.ListSQLUsersOptions{
			PaginationPage:  &page,
			PaginationLimit: &limit,
		}

		traceAPICall("ListSQLUsers")
		apiResp, httpResp, err := r.provider.service.ListSQLUsers(ctx, state.ClusterId.ValueString(), options)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
				resp.Diagnostics.AddWarning(
					"Cluster not found",
					fmt.Sprintf("SQL User's parent cluster with clusterID %s is not found. Removing from state.",
						state.ClusterId.ValueString()))
				resp.State.RemoveResource(ctx)
			} else {
				resp.Diagnostics.AddError(
					"Couldn't retrieve SQL users",
					fmt.Sprintf("Unexpected error retrieving SQL users: %s", formatAPIErrorMessage(err)),
				)
			}
			return
		}

		for _, user := range apiResp.GetUsers() {
			if user.GetName() == state.Name.ValueString() {
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
		fmt.Sprintf("This cluster doesn't have a SQL user named '%v'. Removing from state.", state.Name.ValueString()),
	)
	resp.State.RemoveResource(ctx)
}

func (r *sqlUserResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	var plan SQLUser
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state SQLUser
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// password_wo is write-only and absent from both plan and state, so a
	// change to the secret is invisible to Terraform on its own. The
	// password_wo_version trigger (a plain Int64 that does live in state) is
	// what makes rotation observable: bumping it signals that the current
	// password_wo value in configuration should be re-applied.
	var passwordWO types.String
	diags = req.Config.GetAttribute(ctx, path.Root("password_wo"), &passwordWO)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	newPassword, shouldUpdate, pwDiags := resolveUpdatedPassword(plan, state, passwordWO)
	resp.Diagnostics.Append(pwDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if shouldUpdate {
		updateReq := client.UpdateSQLUserPasswordBody{Password: newPassword}
		traceAPICall("UpdateSQLUserPassword")
		_, _, err := r.provider.service.UpdateSQLUserPassword(
			ctx, plan.ClusterId.ValueString(), plan.Name.ValueString(), &updateReq)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating sql user password",
				fmt.Sprintf("Could not update sql user password: %s", formatAPIErrorMessage(err)),
			)
			return
		}
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// resolveUpdatedPassword returns the password to push for an Update (with
// a bool gating the push) and any user-facing diagnostics.
func resolveUpdatedPassword(plan, state SQLUser, passwordWO types.String) (string, bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	// Legacy `password` rotation: planned value differs from state.
	if !plan.Password.IsNull() {
		// An unknown value is not null, so guard it before Equal/ValueString
		// would treat it as a real (empty) password. Mirrors Create's guard.
		if plan.Password.IsUnknown() {
			diags.AddError(unknownPasswordDiagSummary, unknownPasswordDiagDetail)
			return "", false, diags
		}
		if !plan.Password.Equal(state.Password) {
			return plan.Password.ValueString(), true, diags
		}
		return "", false, diags
	}

	versionChanged := !plan.PasswordWOVersion.Equal(state.PasswordWOVersion)
	if versionChanged && !passwordWO.IsNull() {
		if passwordWO.IsUnknown() {
			diags.AddError(unknownPasswordWODiagSummary, unknownPasswordWODiagDetail)
			return "", false, diags
		}
		return passwordWO.ValueString(), true, diags
	}

	if IsKnown(state.Password) {
		diags.AddWarning(
			"Password will not be changed",
			"Setting the password field to null will not change the password. It will simply remove it from Terraform state.",
		)
		return "", false, diags
	}
	return "", false, diags
}

func (r *sqlUserResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state SQLUser
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("DeleteSQLUser")
	_, httpResp, err := r.provider.service.DeleteSQLUser(ctx, state.ClusterId.ValueString(), state.Name.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			// User or cluster is already gone. Swallow the error.
		} else {
			resp.Diagnostics.AddError(
				"Error deleting sql user",
				fmt.Sprintf("Could not delete sql user: %s", formatAPIErrorMessage(err)),
			)
			return
		}
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r *sqlUserResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	// Since a SQL user is uniquely identified by two fields, the cluster ID
	// and the name, we serialize them both into the ID field. To make import
	// work, we need to deserialize an ID back into name and cluster ID.
	matches := sqlUserIDRegex.FindStringSubmatch(req.ID)
	if len(matches) != 3 {
		resp.Diagnostics.AddError(
			"Invalid SQL user ID format",
			`When importing a SQL user, the ID field should follow the format "<cluster ID>:<SQL user name>")`)
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
