package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	"github.com/cockroachdb/terraform-provider-cockroach/internal/utils"
	"github.com/cockroachdb/terraform-provider-cockroach/internal/validators"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type folderResource struct {
	provider *provider
}

func NewFolderResource() resource.Resource {
	return &folderResource{}
}

func (r *folderResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "CockroachDB Cloud folder.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the folder.",
				Required:            true,
			},
			"parent_id": schema.StringAttribute{
				MarkdownDescription: "ID of the parent folder. Use 'root' for the root level (no parent folder).",
				Required:            true,
				Validators: []validator.String{
					validators.FolderParentID(),
				},
			},
			"labels": schema.MapAttribute{
				Computed:    true,
				Optional:    true,
				ElementType: types.StringType,
				Description: "Map of key-value pairs used to organize and categorize resources. If unset, labels will not be managed by Terraform. If set, labels defined in Terraform will overwrite any labels configured outside this platform.",
				Validators:  labelsValidator,
			},
		},
	}
}

func (r *folderResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_folder"
}

func (r *folderResource) Configure(
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

func (r *folderResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan Folder
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	parentID := plan.ParentId.ValueString()

	createFolderReq := &client.CreateFolderRequest{
		Name:     plan.Name.ValueString(),
		ParentId: &parentID,
	}

	if IsKnown(plan.Labels) {
		labels, err := utils.ToStringMap(plan.Labels)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error processing folder labels",
				fmt.Sprintf("Could not convert folder labels: %v", err),
			)
		}
		createFolderReq.SetLabels(labels)
	}

	traceAPICall("CreateFolder")
	folderObj, _, err := r.provider.service.CreateFolder(ctx, createFolderReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating folder",
			fmt.Sprintf("Could not create folder: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state Folder
	diags = loadFolderToTerraformState(ctx, folderObj, &state)
	resp.Diagnostics.Append(diags...)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *folderResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state Folder
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
	if state.ID.IsNull() {
		return
	}

	folderID := state.ID.ValueString()

	// In case this was an import, validate the ID format.
	if !uuidRegex.MatchString(folderID) {
		resp.Diagnostics.AddError(
			"Unexpected folder ID format",
			fmt.Sprintf("'%s' is not a valid folder ID format. Expected UUID.", folderID),
		)
		return
	}

	traceAPICall("GetFolder")
	folderObj, httpResp, err := r.provider.service.GetFolder(ctx, folderID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Folder not found",
				fmt.Sprintf("Folder with ID %s is not found. Removing from state.", folderID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting folder info",
				fmt.Sprintf("Unexpected error retrieving folder info: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	diags = loadFolderToTerraformState(ctx, folderObj, &state)
	resp.Diagnostics.Append(diags...)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *folderResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	// Get folder specification.
	var plan Folder
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state.
	var state Folder
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		newName      = plan.Name.ValueString()
		destParentID = plan.ParentId.ValueString()
	)

	updateFolderSpec := &client.UpdateFolderSpecification{
		Name:     &newName,
		ParentId: &destParentID,
	}

	if IsKnown(plan.Labels) {
		labels, err := utils.ToStringMap(plan.Labels)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error processing folder labels",
				fmt.Sprintf("Could not convert folder labels: %v", err),
			)
		}
		updateFolderSpec.SetLabels(labels)
	}

	traceAPICall("UpdateFolder")
	folderObj, _, err := r.provider.service.UpdateFolder(ctx, plan.ID.ValueString(), updateFolderSpec)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating folder",
			fmt.Sprintf("Could not update folder: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	diags = loadFolderToTerraformState(ctx, folderObj, &state)
	resp.Diagnostics.Append(diags...)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *folderResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state Folder
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get folder ID from state.
	folderID := state.ID
	if folderID.IsNull() {
		return
	}

	traceAPICall("DeleteFolder")
	httpResp, err := r.provider.service.DeleteFolder(ctx, folderID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			// Folder is already gone. Swallow the error.
		} else {
			resp.Diagnostics.AddError(
				"Error deleting folder",
				fmt.Sprintf("Could not delete folder: %s", formatAPIErrorMessage(err)),
			)
		}
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r *folderResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func loadFolderToTerraformState(
	ctx context.Context, folderObj *client.FolderResource, state *Folder,
) diag.Diagnostics {
	state.ID = types.StringValue(folderObj.ResourceId)
	state.ParentId = types.StringValue(folderObj.ParentId)
	state.Name = types.StringValue(folderObj.Name)

	folderLabels := make(map[string]string)
	if folderObj.Labels != nil {
		folderLabels = folderObj.Labels
	}
	labels, diags := types.MapValueFrom(ctx, types.StringType, folderLabels)
	state.Labels = labels
	return diags
}
