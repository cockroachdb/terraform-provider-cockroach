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
	"net/http"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v5/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type jwtIssuerResource struct {
	provider *provider
}

func NewJWTIssuerResource() resource.Resource {
	return &jwtIssuerResource{}
}

func (r *jwtIssuerResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Configuration to manage external JSON Web Token (JWT) Issuers for " +
			"authentication to the CockroachDB Cloud API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "The unique identifier of the JWT Issuer resource.",
			},
			"issuer_url": schema.StringAttribute{
				Required:    true,
				Description: "The URL of the server issuing JWTs.",
			},
			"audience": schema.StringAttribute{
				Required:    true,
				Description: "The intended audience for consuming the JWT.",
			},
			"jwks": schema.StringAttribute{
				Optional:    true,
				Description: "A set of public keys (JWKS) used to verify the JWT.",
			},
			"claim": schema.StringAttribute{
				Optional: true,
				Description: "Used to identify the user from the external Identity Provider. " +
					"Defaults to \"sub\".",
			},
			"identity_map": schema.ListNestedAttribute{
				Optional: true,
				Description: "A list of mappings to map the external token identity into " +
					"CockroachDB Cloud.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"token_identity": schema.StringAttribute{
							Required: true,
							Description: "Specifies how to fetch external identity from the " +
								"token claim. A regular expression must start with a forward " +
								"slash. The regular expression must be in RE2 compatible syntax. " +
								"For further details, please see " +
								"https://github.com/google/re2/wiki/Syntax.",
						},
						"cc_identity": schema.StringAttribute{
							Required: true,
							Description: "Specifies how to map the fetched token identity to an " +
								"identity in CockroachDB Cloud. In case of a regular expression " +
								"for token_identity, this must contain a \\1 placeholder for the " +
								"matched content. Note that you will need to escape the " +
								"backslash in the string as in the example usage (\\\\\\1).",
						},
					},
				},
			},
		},
	}
}

func (r *jwtIssuerResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_jwt_issuer"
}

func (r *jwtIssuerResource) Configure(
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

func (r *jwtIssuerResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *jwtIssuerResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var jwtIssuerSpec JWTIssuer
	diags := req.Plan.Get(ctx, &jwtIssuerSpec)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	createRequest := &client.AddJWTIssuerRequest{
		Audience:    jwtIssuerSpec.Audience.ValueString(),
		IssuerUrl:   jwtIssuerSpec.IssuerURL.ValueString(),
		Jwks:        jwtIssuerSpec.Jwks.ValueStringPointer(),
		Claim:       jwtIssuerSpec.Claim.ValueStringPointer(),
		IdentityMap: identityMapFromTerraformState(jwtIssuerSpec.IdentityMap),
	}

	traceAPICall("AddJWTIssuer")
	apiResp, _, err := r.provider.service.AddJWTIssuer(ctx, createRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating JWT Issuer",
			fmt.Sprintf("Could not create JWT Issuer: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadJWTIssuerToTerraformState(apiResp, &jwtIssuerSpec)
	diags = resp.State.Set(ctx, jwtIssuerSpec)
	resp.Diagnostics.Append(diags...)
}

func (r *jwtIssuerResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state JWTIssuer
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("GetJWTIssuer")
	apiResp, httpResp, err := r.provider.service.GetJWTIssuer(ctx, state.ID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"JWT Issuer not found",
				"JWT Issuer not found. JWT Issuer will be removed from state.")
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting JWT Issuer",
				fmt.Sprintf("Unexpected error retrieving JWT Issuer: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	loadJWTIssuerToTerraformState(apiResp, &state)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *jwtIssuerResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	var plan JWTIssuer
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state JWTIssuer
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("UpdateJWTIssuer")
	apiResp, _, err := r.provider.service.UpdateJWTIssuer(
		ctx,
		plan.ID.ValueString(),
		&client.UpdateJWTIssuerRequest{
			Audience:    plan.Audience.ValueStringPointer(),
			Claim:       plan.Claim.ValueStringPointer(),
			IdentityMap: identityMapFromTerraformState(plan.IdentityMap),
			IssuerUrl:   plan.IssuerURL.ValueStringPointer(),
			Jwks:        plan.Jwks.ValueStringPointer(),
		},
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating JWT Issuer",
			fmt.Sprintf("Could not update JWT Issuer: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadJWTIssuerToTerraformState(apiResp, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *jwtIssuerResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state JWTIssuer
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("DeleteJWTIssuer")
	_, _, err := r.provider.service.DeleteJWTIssuer(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting JWT Issuer",
			fmt.Sprintf("Could not delete JWT Issuer: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	resp.State.RemoveResource(ctx)
}

func loadJWTIssuerToTerraformState(
	jwtIssuer *client.JWTIssuer, state *JWTIssuer,
) {
	state.ID = types.StringValue(jwtIssuer.Id)
	state.Audience = types.StringValue(jwtIssuer.Audience)
	state.IssuerURL = types.StringValue(jwtIssuer.IssuerUrl)
	state.Jwks = types.StringPointerValue(jwtIssuer.Jwks)
	state.Claim = types.StringPointerValue(jwtIssuer.Claim)
	state.IdentityMap = identityMapToTerraformState(jwtIssuer.IdentityMap)
}

func identityMapFromTerraformState(identityMap *[]IdentityMapEntry) *[]client.JWTIssuerIdentityMapEntry {
	if identityMap == nil {
		return nil
	}
	var out []client.JWTIssuerIdentityMapEntry
	for _, mapEntry := range *identityMap {
		out = append(out, client.JWTIssuerIdentityMapEntry{
			TokenIdentity: mapEntry.TokenIdentity.ValueString(),
			CcIdentity:    mapEntry.CcIdentity.ValueString(),
		})
	}
	return &out
}

func identityMapToTerraformState(identityMap *[]client.JWTIssuerIdentityMapEntry) *[]IdentityMapEntry {
	if identityMap == nil {
		return nil
	}
	var out []IdentityMapEntry
	for _, mapEntry := range *identityMap {
		out = append(out, IdentityMapEntry{
			TokenIdentity: types.StringValue(mapEntry.TokenIdentity),
			CcIdentity:    types.StringValue(mapEntry.CcIdentity),
		})
	}
	return &out
}
