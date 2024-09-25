package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v4/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type apiKeyResource struct {
	provider *provider
}

func NewAPIKeyResource() resource.Resource {
	return &apiKeyResource{}
}

func (r *apiKeyResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `API Keys can be used for programmatic access to the cockroach cloud api. Each key is mapped to a [cockroach_service_account](service_account).
		
To access the secret, declare an output value for it and use the terraform output command. i.e. ` + "`terraform output -raw example_secret`" + ` 

During API key creation, a sensitive key is created and stored in the terraform state. Always follow [best practices](https://developer.hashicorp.com/terraform/tutorials/configuration-language/sensitive-variables#sensitive-values-in-state) when managing sensitive data.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_account_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: uuidValidator,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the api key.",
				Required:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation time of the api key.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Computed: true,
			},
			"secret": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *apiKeyResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *apiKeyResource) Configure(
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

func (r *apiKeyResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan APIKey
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("CreateAPIKey")
	createResp, _, err := r.provider.service.CreateApiKey(ctx, &client.CreateApiKeyRequest{
		Name:             plan.Name.ValueString(),
		ServiceAccountId: plan.ServiceAccountID.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating api key",
			fmt.Sprintf("Could not create api key: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state APIKey
	loadAPIKeyToTerraformState(&createResp.ApiKey, &createResp.Secret, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *apiKeyResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state APIKey
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
	if state.ID.IsNull() {
		return
	}

	apiKeyID := state.ID.ValueString()

	traceAPICall("GetApiKey")
	apiKeyObj, httpResp, err := r.provider.service.GetApiKey(ctx, apiKeyID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"API key not found",
				fmt.Sprintf("API key with ID %s is not found. Removing from state.", apiKeyID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting api key info",
				fmt.Sprintf("Unexpected error retrieving api key info: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	loadAPIKeyToTerraformState(apiKeyObj, nil /* secret */, &state)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *apiKeyResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	// Get api key specification.
	var plan APIKey
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state.
	var state APIKey
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// State is the only updateable field currently so if state didn't change,
	// just return.
	newName := plan.Name.ValueString()
	if state.Name.ValueString() == newName {
		return
	}

	traceAPICall("UpdateAPIKey")
	apiKeyObj, _, err := r.provider.service.UpdateApiKey(
		ctx,
		plan.ID.ValueString(),
		&client.UpdateApiKeySpecification{
			Name: &newName,
		})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating api key",
			fmt.Sprintf("Could not update api key: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadAPIKeyToTerraformState(apiKeyObj, nil, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *apiKeyResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state APIKey
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get api key ID from state.
	apiKeyID := state.ID
	if apiKeyID.IsNull() {
		return
	}

	traceAPICall("DeleteAPIKey")
	_, httpResp, err := r.provider.service.DeleteApiKey(ctx, apiKeyID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			// API key is already gone. Swallow the error.
		} else {
			resp.Diagnostics.AddError(
				"Error deleting api key",
				fmt.Sprintf("Could not delete api key: %s", formatAPIErrorMessage(err)),
			)
		}
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r *apiKeyResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	// Since the api key secret is not accessible via the api after create has
	// already occurred, in order to support import it must also be specified.
	// The secret also includes the api key id as a prefix so we can just ask
	// for the secret and get both values.
	parts := strings.Split(req.ID, "_")

	if len(parts) != 3 {
		resp.Diagnostics.AddError(
			"Invalid API Key secret format",
			`When importing an api key, the secret must be provided because it cannot be fetched via the api. The format should match CCDB1_<22 chars>_<40 chars>.`)
		return
	}

	// build the api key and let the remote READ handle the remaining validations.
	apiKeyID := fmt.Sprintf("%s_%s", parts[0], parts[1])
	secret := req.ID

	apiKey := APIKey{
		ID:     types.StringValue(apiKeyID),
		Secret: types.StringValue(secret),
	}
	resp.Diagnostics = resp.State.Set(ctx, &apiKey)
}

func loadAPIKeyToTerraformState(apiKeyObj *client.ApiKey, secret *string, state *APIKey) {
	state.ID = types.StringValue(apiKeyObj.Id)
	state.Name = types.StringValue(apiKeyObj.Name)
	state.ServiceAccountID = types.StringValue(apiKeyObj.ServiceAccountId)
	state.CreatedAt = types.StringValue(apiKeyObj.CreatedAt.String())
	if secret != nil {
		state.Secret = types.StringValue(*secret)
	}
}
