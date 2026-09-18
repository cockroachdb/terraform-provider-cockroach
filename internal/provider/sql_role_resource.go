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
	"regexp"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v10/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type sqlRoleResource struct {
	provider *provider
}

// clusterID:name, the same shape cockroach_sql_user uses. Role names share the
// SQL user namespace in CockroachDB, so they share the name rule too.
const (
	sqlRoleIDFmt     = "%s:%s"
	sqlRoleNameRegex = "[A-Za-z0-9_][A-Za-z0-9\\._\\-]{0,62}"
)

var sqlRoleIDRegex = regexp.MustCompile(fmt.Sprintf("^(%s):(%s)$", uuidRegexString, sqlRoleNameRegex))

func (r *sqlRoleResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "CockroachDB SQL role.\n\n" +
			"A role groups privileges. Set `login` to also make it a principal that can " +
			"authenticate, which is how a service account is provisioned without granting " +
			"`admin`. Unlike `cockroach_sql_user`, a role starts with no privileges at all.\n\n" +
			"Every operation opens a SQL connection to the cluster, so `terraform apply` " +
			"fails while the cluster is paused.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "The ID of the cluster the role belongs to.",
			},
			"name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(fmt.Sprintf("^%s$", sqlRoleNameRegex)),
						"must start with a letter, digit or underscore and may contain only "+
							"letters, digits, underscores, periods and hyphens (63 characters maximum)",
					),
				},
				MarkdownDescription: "SQL role name. CockroachDB folds unquoted identifiers, so " +
					"the name is lowercased before use. Reserved principals such as `admin`, " +
					"`root` and `public` are rejected.",
			},
			"login": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Whether the role can authenticate. Defaults to `false`, " +
					"which creates a role that groups privileges and cannot connect. This can " +
					"be toggled in place; it never forces a replacement.",
			},
			"password_wo": schema.StringAttribute{
				Optional:   true,
				Sensitive:  true,
				WriteOnly:  true,
				Validators: []validator.String{stringvalidator.LengthBetween(12, 500)},
				MarkdownDescription: "Write-only password for the role (Terraform CLI 1.11+ " +
					"required). The value is sent on create and on rotation but is never stored " +
					"in Terraform state. To rotate, change the value and bump " +
					"`password_wo_version`; without a version bump, Terraform cannot detect " +
					"changes to a write-only attribute. A password on a role with " +
					"`login = false` is accepted and has no effect until login is granted.",
			},
			"password_wo_version": schema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "Trigger attribute for rotating `password_wo`. Increment " +
					"this integer to force Terraform to re-apply the current `password_wo` " +
					"value. Required when `password_wo` is set.",
			},
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "A unique identifier with format `<cluster ID>:<SQL role name>`.",
			},
		},
	}
}

// ConfigValidators requires the write-only password and its version trigger to
// be set together. Without the version, a changed password_wo is invisible to
// Terraform and the rotation would silently never happen.
func (r *sqlRoleResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.RequiredTogether(
			path.MatchRoot("password_wo"),
			path.MatchRoot("password_wo_version"),
		),
	}
}

func (r *sqlRoleResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_sql_role"
}

func (r *sqlRoleResource) Configure(
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

func (r *sqlRoleResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan SQLRole
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	// Write-only attributes are absent from plan and state, so password_wo has
	// to be read straight out of configuration.
	var passwordWO types.String
	resp.Diagnostics.Append(
		req.Config.GetAttribute(ctx, path.Root("password_wo"), &passwordWO)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if passwordWO.IsUnknown() {
		// password_wo comes from configuration, so it is known by the time
		// Create runs under a normal apply. Guard the pathological case rather
		// than send an empty password.
		resp.Diagnostics.AddError(unknownPasswordWODiagSummary, unknownPasswordWODiagDetail)
		return
	}

	traceAPICall("GetCluster")
	if _, _, err := r.provider.service.GetCluster(ctx, plan.ClusterId.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error getting the cluster",
			fmt.Sprintf("Could not get the cluster: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	createReq := &client.CreateSQLRoleBody{
		Name:  plan.Name.ValueString(),
		Login: plan.Login.ValueBoolPointer(),
	}
	if !passwordWO.IsNull() {
		createReq.Password = passwordWO.ValueStringPointer()
	}

	traceAPICall("CreateSQLRole")
	role, _, err := r.provider.service.CreateSQLRole(ctx, plan.ClusterId.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating SQL role",
			fmt.Sprintf("Could not create SQL role: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// Take the name from the response: the API lowercases it, and storing the
	// configured casing would make every subsequent Read look like drift.
	loadSQLRoleToTerraformState(plan.ClusterId.ValueString(), role, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *sqlRoleResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state SQLRole
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("GetSQLRole")
	role, httpResp, err := r.provider.service.GetSQLRole(
		ctx, state.ClusterId.ValueString(), state.Name.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			// The role, or its cluster, is gone. The API also reports a
			// reserved principal as absent, and a principal owned by
			// cockroach_sql_user, so this covers a bad import too.
			resp.Diagnostics.AddWarning(
				"SQL role not found",
				fmt.Sprintf("SQL role %s is not present on cluster %s. Removing from state.",
					state.Name.ValueString(), state.ClusterId.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error getting SQL role",
			fmt.Sprintf("Could not get SQL role: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadSQLRoleToTerraformState(state.ClusterId.ValueString(), role, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *sqlRoleResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan, state SQLRole
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	var passwordWO types.String
	resp.Diagnostics.Append(
		req.Config.GetAttribute(ctx, path.Root("password_wo"), &passwordWO)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := &client.UpdateSQLRoleBody{}
	if !plan.Login.Equal(state.Login) {
		updateReq.Login = plan.Login.ValueBoolPointer()
	}

	// password_wo lives in neither plan nor state, so a change to the secret is
	// invisible on its own. password_wo_version is a plain Int64 that does live
	// in state, and bumping it is the signal to re-apply the current value.
	if !plan.PasswordWOVersion.Equal(state.PasswordWOVersion) && !passwordWO.IsNull() {
		if passwordWO.IsUnknown() {
			resp.Diagnostics.AddError(unknownPasswordWODiagSummary, unknownPasswordWODiagDetail)
			return
		}
		updateReq.Password = passwordWO.ValueStringPointer()
	}

	// Skip the call when nothing changed. The API treats an empty PATCH as a
	// no-op, but there is no reason to open a SQL connection to the cluster for
	// it, and doing so would fail on a paused cluster.
	if updateReq.Login == nil && updateReq.Password == nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		return
	}

	traceAPICall("UpdateSQLRole")
	role, _, err := r.provider.service.UpdateSQLRole(
		ctx, plan.ClusterId.ValueString(), plan.Name.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating SQL role",
			fmt.Sprintf("Could not update SQL role: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadSQLRoleToTerraformState(plan.ClusterId.ValueString(), role, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *sqlRoleResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state SQLRole
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("DeleteSQLRole")
	_, httpResp, err := r.provider.service.DeleteSQLRole(
		ctx, state.ClusterId.ValueString(), state.Name.ValueString())
	if err != nil && (httpResp == nil || httpResp.StatusCode != http.StatusNotFound) {
		// A 404 means the role or its cluster is already gone, which is the
		// outcome delete wanted. Anything else, including the refusal to drop a
		// role that still owns objects, is surfaced.
		resp.Diagnostics.AddError(
			"Error deleting SQL role",
			fmt.Sprintf("Could not delete SQL role: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r *sqlRoleResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	// A SQL role is identified by two fields, so both are serialized into the
	// ID and split back out here.
	matches := sqlRoleIDRegex.FindStringSubmatch(req.ID)
	if len(matches) != 3 {
		resp.Diagnostics.AddError(
			"Invalid SQL role ID format",
			`When importing a SQL role, the ID field should follow the format "<cluster ID>:<SQL role name>"`)
		return
	}
	role := SQLRole{
		ClusterId: types.StringValue(matches[1]),
		Name:      types.StringValue(matches[2]),
		ID:        types.StringValue(req.ID),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &role)...)
}

// loadSQLRoleToTerraformState copies the server's view of the role onto state.
// The password is never part of that view, which is what keeps it out of the
// state file.
func loadSQLRoleToTerraformState(clusterID string, role *client.SQLRole, state *SQLRole) {
	state.ClusterId = types.StringValue(clusterID)
	state.Name = types.StringValue(role.GetName())
	state.Login = types.BoolValue(role.GetLogin())
	state.ID = types.StringValue(fmt.Sprintf(sqlRoleIDFmt, clusterID, role.GetName()))
}

func NewSQLRoleResource() resource.Resource {
	return &sqlRoleResource{}
}
