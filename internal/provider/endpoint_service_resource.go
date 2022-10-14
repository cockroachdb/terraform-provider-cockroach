/*
Copyright 2022 The Cockroach Authors

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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type endpointServiceResourceType struct{}

func (n endpointServiceResourceType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		MarkdownDescription: "Private endpoint service",
		Attributes: map[string]tfsdk.Attribute{
			"id": {
				Required: true,
				Type:     types.StringType,
			},
			"region_name": {
				Required: true,
				Type:     types.StringType,
			},
			"cloud_provider": {
				Required: true,
				Type:     types.Int64Type,
			},
			"status": {
				Type:     types.StringType,
				Computed: true,
				// TODO: what is this?
				// PlanModifiers: tfsdk.AttributePlanModifiers{
				// 	tfsdk.UseStateForUnknown(),
				// }
			},
			"aws": {
				Computed: true,
				Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
					"service_name": {
						Computed: true,
						Type:     types.StringType,
					},
					"service_id": {
						Computed: true,
						Type:     types.StringType,
					},
					"availability_zone_ids": {
						Computed: true,
						Type:     types.ListType{ElemType: types.StringType},
					},
				}),
			},
		},
	}, nil
}

func (n endpointServiceResourceType) NewResource(ctx context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return endpointServiceResource{
		provider: provider,
	}, diags
}

type endpointServiceResource struct {
	provider provider
}

func (n endpointServiceResource) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	if !n.provider.configured {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider hasn't been configured before apply, likely because it depends on an unknown value from another resource. This leads to weird stuff happening, so we'd prefer if you didn't do that. Thanks!",
		)
		return
	}

	var plan EndpointService
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	cluster, httpResp, err := n.provider.service.GetCluster(ctx, plan.Id.Value)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting the cluster",
			fmt.Sprintf("Could not get the cluster, unexpected error: %v %v "+err.Error(), httpResp),
		)
		return
	}

	if cluster.Config.Serverless != nil {
		resp.Diagnostics.AddError(
			"Could not establish private endpoint services in serverless cluster",
			fmt.Sprintf("Private endpoint services is a feature of dedicated cluster, unexpected error: %v %v "+err.Error(), httpResp),
		)
		return
	}

	var endpointServices = client.PrivateEndpointService{
		Id: plan.Id.Value,
	}

	_, httpResp, err = n.provider.service.AddEndpointService(ctx, plan.Id.Value, &endpointService)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error adding allowed IP range",
			fmt.Sprintf("Could not add allowed IP range, unexpected error: %v %v "+err.Error(), httpResp),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (n endpointServiceResource) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	if !n.provider.configured {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider hasn't been configured before apply, likely because it depends on an unknown value from another resource. This leads to weird stuff happening, so we'd prefer if you didn't do that. Thanks!",
		)
		return
	}

	var plan EndpointService
	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (n endpointServiceResource) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	// no-op - Endpoint services cannot be updated
}

func (n endpointServiceResource) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	// no-op - Endpoint services cannot be deleted
}

func (n endpointServiceResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("id"), req, resp)
}
