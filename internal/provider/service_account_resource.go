package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v3/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type serviceAccountResource struct {
	provider *provider
}

func NewServiceAccountResource() resource.Resource {
	return &serviceAccountResource{}
}

func (r *serviceAccountResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "CockroachDB Cloud service account. A service account represents a non-person user. By default a service account has no access but it can be accompanied by either a [cockroach_user_role_grants](user_role_grants) resource or any number of [cockroach_user_role_grant](user_role_grant) resources to grant it roles.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the service account.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the service account.",
				Optional:            true,
				Computed:            true,
			},
			"creator_name": schema.StringAttribute{
				MarkdownDescription: "Name of the creator of the service account.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Computed: true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation time of the service account.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Computed: true,
			},
		},
	}
}

func (r *serviceAccountResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_service_account"
}

func (r *serviceAccountResource) Configure(
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

func (r *serviceAccountResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan ServiceAccount
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("CreateServiceAccount")
	serviceAccountObj, _, err := r.provider.service.CreateServiceAccount(ctx, &client.CreateServiceAccountRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Roles:       []client.BuiltInRole{}, // Force use of role resources to manage roles for simplicity
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating service account",
			fmt.Sprintf("Could not create service account: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state ServiceAccount
	loadServiceAccountToTerraformState(serviceAccountObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *serviceAccountResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state ServiceAccount
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
	if state.ID.IsNull() {
		return
	}

	serviceAccountID := state.ID.ValueString()

	// In case this was an import, validate the ID format.
	if !uuidRegex.MatchString(serviceAccountID) {
		resp.Diagnostics.AddError(
			"Unexpected service account ID format",
			fmt.Sprintf("'%s' is not a valid service account ID format. Expected UUID.", serviceAccountID),
		)
		return
	}

	traceAPICall("GetServiceAccount")
	serviceAccountObj, httpResp, err := r.provider.service.GetServiceAccount(ctx, serviceAccountID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Service Account not found",
				fmt.Sprintf("Service Account with ID %s is not found. Removing from state.", serviceAccountID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting service account info",
				fmt.Sprintf("Unexpected error retrieving service account info: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	loadServiceAccountToTerraformState(serviceAccountObj, &state)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *serviceAccountResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	// Get service account specification.
	var plan ServiceAccount
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state.
	var state ServiceAccount
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateSpec := &client.UpdateServiceAccountSpecification{
		Name: plan.Name.ValueStringPointer(),
	}

	// Only send the description if the value is present in resource.
	if IsKnown(plan.Description) {
		updateSpec.SetDescription(plan.Description.ValueString())
	}

	traceAPICall("UpdateServiceAccount")
	serviceAccountObj, _, err := r.provider.service.UpdateServiceAccount(
		ctx,
		plan.ID.ValueString(),
		updateSpec,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating service account",
			fmt.Sprintf("Could not update service account: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadServiceAccountToTerraformState(serviceAccountObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *serviceAccountResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state ServiceAccount
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get service account ID from state.
	serviceAccountID := state.ID
	if serviceAccountID.IsNull() {
		return
	}

	traceAPICall("DeleteServiceAccount")
	_, httpResp, err := r.provider.service.DeleteServiceAccount(ctx, serviceAccountID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			// Service account is already gone. Swallow the error.
		} else {
			resp.Diagnostics.AddError(
				"Error deleting service account",
				fmt.Sprintf("Could not delete service account: %s", formatAPIErrorMessage(err)),
			)
		}
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r *serviceAccountResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func loadServiceAccountToTerraformState(serviceAccountObj *client.ServiceAccount, state *ServiceAccount) {
	state.ID = types.StringValue(serviceAccountObj.Id)
	state.Name = types.StringValue(serviceAccountObj.Name)
	state.Description = types.StringValue(serviceAccountObj.Description)
	state.CreatedAt = types.StringValue(serviceAccountObj.CreatedAt.String())
}
