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

type awsEndpointConnectionResourceType struct{}

func (n awsEndpointConnectionResourceType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		MarkdownDescription: "AWS PrivateLink Endpoint Connection",
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
			"endpoint_id": {
				Required: true,
				Type:     types.StringType,
			},
			"status": {
				Required: true,
				Type:     types.StringType,
			},
			"service_id": {
				Computed: true,
				Type:     types.StringType,
			},
		},
	}, nil
}

func (n awsEndpointConnectionResourceType) NewResource(ctx context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return awsEndpointConnectionResource{
		provider: provider,
	}, diags
}

type awsEndpointConnectionResource struct {
	provider provider
}

func (n awsEndpointConnectionResource) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	if !n.provider.configured {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider hasn't been configured before apply, likely because it depends on an unknown value from another resource. This leads to weird stuff happening, so we'd prefer if you didn't do that. Thanks!",
		)
		return
	}

	var plan AwsPrivateEndpointConnection
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

	connectionStateRequest := client.CockroachCloudSetAwsEndpointConnectionStateRequest{
		Status: plan.Status, // needs to be type AWSEndpointConnectionStatus
	}

	_, httpResp, err = n.provider.service.SetAwsEndpointConnectionState(ctx, plan.Id.Value, plan.EndpointId.Value, &connectionStateRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error establishing AWS Endpoint Connection",
			fmt.Sprintf("Could not establish AWS Endpoint Connection, unexpected error: %v %v "+err.Error(), httpResp),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (n awsEndpointConnectionResource) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	if !n.provider.configured {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider hasn't been configured before apply, likely because it depends on an unknown value from another resource. This leads to weird stuff happening, so we'd prefer if you didn't do that. Thanks!",
		)
		return
	}

	var plan AwsPrivateEndpointConnection
	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (n awsEndpointConnectionResource) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	// Get plan values
	var plan AwsPrivateEndpointConnection
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state AwsPrivateEndpointConnection
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Id != plan.Id {
		resp.Diagnostics.AddError(
			"can not change the cluster id for an endpoint connection",
			"You can only change the status of the connection.",
		)
		return
	}

	if state.ServiceId != plan.ServiceId {
		resp.Diagnostics.AddError(
			"can not change the service id for an endpoint connection",
			"You can only change the status of the connection.",
		)
		return
	}

	if state.EndpointId != plan.EndpointId {
		resp.Diagnostics.AddError(
			"can not change the endpoint id for an endpoint connection",
			"You can only change the status of the connection.",
		)
		return
	}

	connectionStateRequest := client.CockroachCloudSetAwsEndpointConnectionStateRequest{
		Status: plan.Status, // needs to be type AWSEndpointConnectionStatus
	}

	_, httpResp, err := n.provider.service.SetAwsEndpointConnectionState(ctx, plan.Id.Value, plan.EndpointId.Value, &connectionStateRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating endpoint connection",
			fmt.Sprintf("Could not update endpoint connection, unexpected error: %v %v "+err.Error(), httpResp),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (n awsEndpointConnectionResource) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	// no-op - Endpoint connections cannot be deleted
}

func (n awsEndpointConnectionResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("id"), req, resp)
}
