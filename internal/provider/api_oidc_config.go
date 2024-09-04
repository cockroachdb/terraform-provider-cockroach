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
	"net/http"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type apiOidcConfigResource struct {
	provider *provider
}

func (r *apiOidcConfigResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Configuration to allow external OIDC providers to issue tokens for use with CC API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "ID of the API OIDC Configuration.",
			},
			"issuer": schema.StringAttribute{
				Required:    true,
				Description: "The issuer of tokens for the API OIDC Configuration. Usually this is a url.",
			},
			"audience": schema.StringAttribute{
				Required:    true,
				Description: "The audience that CC API should accept for this API OIDC Configuration.",
			},
			"jwks": schema.StringAttribute{
				Required:    true,
				Description: "The JSON Web Key Set used to check the signature of the JWTs.",
			},
			"claim": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The JWT claim that should be used as the user identifier. Defaults to the subject.",
			},
			"identity_map": schema.ListNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"token_identity": schema.StringAttribute{
							Required:    true,
							Description: "The token value that needs to be mapped.",
						},
						"cc_identity": schema.StringAttribute{
							Required:    true,
							Description: "The username (email or service account id) of the CC user that the token should map to.",
						},
						"is_regex": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Indicates that the token_principal field is a regex value.",
						},
					},
				},
				Optional:    true,
				Computed:    true,
				Description: "The mapping rules to convert token user identifiers into a new form.",
			},
		},
	}
}

func (r *apiOidcConfigResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_api_oidc_config"
}

func (r *apiOidcConfigResource) Configure(
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

func (r *apiOidcConfigResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var apiOIdcConfigSpec ApiOidcConfig
	diags := req.Plan.Get(ctx, &apiOIdcConfigSpec)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	createRequest := &client.CreateApiOidcConfigRequest{
		Audience:    apiOIdcConfigSpec.Audience.ValueString(),
		Issuer:      apiOIdcConfigSpec.Issuer.ValueString(),
		Jwks:        apiOIdcConfigSpec.Jwks.ValueString(),
		Claim:       apiOIdcConfigSpec.Claim.ValueStringPointer(),
		IdentityMap: identityMapFromTerraformState(apiOIdcConfigSpec.IdentityMap),
	}

	traceAPICall("CreateApiOidcConfig")
	//nolint:staticcheck
	apiResp, _, err := r.provider.service.CreateApiOidcConfig(ctx, createRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating API OIDC Config",
			fmt.Sprintf("Could not create API OIDC Config: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadApiOidcConfigToTerraformState(apiResp, &apiOIdcConfigSpec)
	diags = resp.State.Set(ctx, apiOIdcConfigSpec)
	resp.Diagnostics.Append(diags...)
}

func (r *apiOidcConfigResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state ApiOidcConfig
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("GetApiOidcConfig")
	//nolint:staticcheck
	apiResp, httpResp, err := r.provider.service.GetApiOidcConfig(ctx, state.ID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"API OIDC Config not found",
				"API OIDC Config not found. API OIDC Config will be removed from state.")
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting API OIDC Config",
				fmt.Sprintf("Unexpected error retrieving API OIDC Config: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	loadApiOidcConfigToTerraformState(apiResp, &state)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *apiOidcConfigResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	var plan ApiOidcConfig
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state ApiOidcConfig
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("UpdateApiOidcConfig")
	//nolint:staticcheck
	apiResp, _, err := r.provider.service.UpdateApiOidcConfig(ctx, plan.ID.ValueString(), &client.ApiOidcConfig1{
		Audience:    plan.Audience.ValueString(),
		Claim:       plan.Claim.ValueStringPointer(),
		IdentityMap: identityMapFromTerraformState(plan.IdentityMap),
		Issuer:      plan.Issuer.ValueString(),
		Jwks:        plan.Jwks.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error update API OIDC Config",
			fmt.Sprintf("Could not update API OIDC Config: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadApiOidcConfigToTerraformState(apiResp, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *apiOidcConfigResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state ApiOidcConfig
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("DeleteApiOidcConfig")
	//nolint:staticcheck
	_, _, err := r.provider.service.DeleteApiOidcConfig(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting API OIDC Config",
			fmt.Sprintf("Could not delete API OIDC Config: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r *apiOidcConfigResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func NewApiOidcConfigResource() resource.Resource {
	return &apiOidcConfigResource{}
}

func loadApiOidcConfigToTerraformState(
	apiOidcConfig *client.ApiOidcConfig, state *ApiOidcConfig,
) {
	state.ID = types.StringValue(apiOidcConfig.Id)
	state.Audience = types.StringValue(apiOidcConfig.Audience)
	state.Issuer = types.StringValue(apiOidcConfig.Issuer)
	state.Jwks = types.StringValue(apiOidcConfig.Jwks)
	state.Claim = types.StringPointerValue(apiOidcConfig.Claim)
	state.IdentityMap = identityMapToTerraformState(apiOidcConfig.IdentityMap)
}

func identityMapFromTerraformState(identityMap *[]IdentityMapEntry) *[]client.ApiOidcIdentityMapEntry {
	if identityMap == nil {
		return nil
	}
	var out []client.ApiOidcIdentityMapEntry
	for _, mapEntry := range *identityMap {
		out = append(out, client.ApiOidcIdentityMapEntry{
			CcIdentity:    mapEntry.CcIdentity.ValueStringPointer(),
			IsRegex:       mapEntry.IsRegex.ValueBoolPointer(),
			TokenIdentity: mapEntry.TokenIdentity.ValueStringPointer(),
		})
	}
	return &out
}

func identityMapToTerraformState(identityMap *[]client.ApiOidcIdentityMapEntry) *[]IdentityMapEntry {
	if identityMap == nil {
		return nil
	}
	var out []IdentityMapEntry
	for _, mapEntry := range *identityMap {
		out = append(out, IdentityMapEntry{
			CcIdentity:    types.StringPointerValue(mapEntry.CcIdentity),
			IsRegex:       types.BoolPointerValue(mapEntry.IsRegex),
			TokenIdentity: types.StringPointerValue(mapEntry.TokenIdentity),
		})
	}
	return &out
}
