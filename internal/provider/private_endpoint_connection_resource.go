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
	"net/http"
	"regexp"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

type privateEndpointConnectionResourceType struct{}

// clusterID:endpointID
const privateEndpointConnectionIDFmt = "%s:%s"

var privateEndpointConnectionIDRegex = regexp.MustCompile(fmt.Sprintf("^(%s):(.*)$", uuidRegex))

func (r privateEndpointConnectionResourceType) GetSchema(_ context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		MarkdownDescription: "AWS PrivateLink Endpoint Connection",
		Attributes: map[string]tfsdk.Attribute{
			"id": {
				Computed:            true,
				Type:                types.StringType,
				MarkdownDescription: "Used with `terrform import`. Format is \"<cluster ID>:<endpoint ID>\"",
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
			},
			"region_name": {
				Computed: true,
				Type:     types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
			},
			"cloud_provider": {
				Computed: true,
				Type:     types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
			},
			"endpoint_id": {
				Required: true,
				Type:     types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.RequiresReplace(),
				},
			},
			"service_id": {
				Computed: true,
				Type:     types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
			},
			"cluster_id": {
				Required: true,
				Type:     types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.RequiresReplace(),
				},
			},
		},
	}, nil
}

func (r privateEndpointConnectionResourceType) NewResource(_ context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return privateEndpointConnectionResource{
		provider: provider,
	}, diags
}

type privateEndpointConnectionResource struct {
	provider provider
}

func (r privateEndpointConnectionResource) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	if !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan PrivateEndpointConnection
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cluster, _, err := r.provider.service.GetCluster(ctx, plan.ClusterID.Value)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting cluster",
			fmt.Sprintf("Could not retrieve cluster info: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	if cluster.Config.Serverless != nil {
		resp.Diagnostics.AddError(
			"Incompatible cluster type",
			"Private endpoint services are only available for dedicated clusters.",
		)
		return
	} else if cluster.CloudProvider != client.APICLOUDPROVIDER_AWS {
		resp.Diagnostics.AddError(
			"Incompatible cluster cloud provider",
			"Private endpoint services are only available for AWS clusters.",
		)
		return
	}

	status := client.AWSENDPOINTCONNECTIONSTATUS_AVAILABLE
	connectionStateRequest := client.CockroachCloudSetAwsEndpointConnectionStateRequest{
		Status: &status,
	}

	_, _, err = r.provider.service.SetAwsEndpointConnectionState(ctx, plan.ClusterID.Value, plan.EndpointID.Value, &connectionStateRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error establishing AWS Endpoint Connection",
			fmt.Sprintf("Could not establish AWS Endpoint Connection: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var connection client.AwsEndpointConnection
	err = resource.RetryContext(ctx, CREATE_TIMEOUT,
		waitForEndpointConnectionCreatedFunc(ctx, cluster.Id, plan.EndpointID.Value, r.provider.service, &connection))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling private endpoint services",
			fmt.Sprintf("Could not enable private endpoint services: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state PrivateEndpointConnection
	state.ClusterID = plan.ClusterID
	loadEndpointConnectionIntoTerraformState(&connection, &state)
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r privateEndpointConnectionResource) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	if !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state PrivateEndpointConnection
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	connections, _, err := r.provider.service.ListAwsEndpointConnections(ctx, state.ClusterID.Value)
	if err != nil {
		diags.AddError("Unable to get endpoint connection status",
			fmt.Sprintf("Unexpected error retrieving endpoint status: %s", formatAPIErrorMessage(err)))
		return
	}
	for _, connection := range connections.GetConnections() {
		if connection.GetEndpointId() == state.EndpointID.Value {
			loadEndpointConnectionIntoTerraformState(&connection, &state)
			diags = resp.State.Set(ctx, state)
			resp.Diagnostics.Append(diags...)
			return
		}
	}
	resp.Diagnostics.AddWarning("Couldn't find connection",
		"Endpoint connection couldn't be located. Removing from state.")
	resp.State.RemoveResource(ctx)
}

func loadEndpointConnectionIntoTerraformState(apiConnection *client.AwsEndpointConnection, state *PrivateEndpointConnection) {
	state.EndpointID = types.String{Value: apiConnection.GetEndpointId()}
	state.ID = types.String{
		Value: fmt.Sprintf(
			privateEndpointConnectionIDFmt,
			state.ClusterID.Value,
			apiConnection.GetEndpointId()),
	}
	state.ServiceID = types.String{Value: apiConnection.GetServiceId()}
	state.CloudProvider = types.String{Value: string(apiConnection.GetCloudProvider())}
	state.RegionName = types.String{Value: apiConnection.GetRegionName()}
}

func (r privateEndpointConnectionResource) Update(_ context.Context, _ tfsdk.UpdateResourceRequest, _ *tfsdk.UpdateResourceResponse) {
	// No-op. Contains only requires-replace or computed fields.
}

func (r privateEndpointConnectionResource) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var state PrivateEndpointConnection
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	status := client.AWSENDPOINTCONNECTIONSTATUS_REJECTED
	_, httpResp, err := r.provider.service.SetAwsEndpointConnectionState(
		ctx,
		state.ClusterID.Value,
		state.EndpointID.Value,
		&client.CockroachCloudSetAwsEndpointConnectionStateRequest{
			Status: &status,
		})
	if err != nil && httpResp.StatusCode != http.StatusNotFound {
		diags.AddError("Couldn't delete connection",
			fmt.Sprintf("Unexpected error occurred while setting connection status: %s", formatAPIErrorMessage(err)))
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r privateEndpointConnectionResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	// Since an endpoint connection is uniquely identified by two fields, the cluster ID
	// and the endpoint ID, we serialize them both into the ID field. To make import
	// work, we need to deserialize an ID back into endpoint ID and cluster ID.
	matches := privateEndpointConnectionIDRegex.FindStringSubmatch(req.ID)
	// One match for the full regex, and one for both subgroups
	if len(matches) != 3 {
		resp.Diagnostics.AddError(
			"Invalid private endpoint connection ID format",
			`When importing a private endpoint connection, the ID field should follow the format "<cluster ID>:<endpoint ID>")`)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	connection := PrivateEndpointConnection{
		ClusterID:  types.String{Value: matches[1]},
		EndpointID: types.String{Value: matches[2]},
		ID:         types.String{Value: req.ID},
	}
	resp.Diagnostics = resp.State.Set(ctx, &connection)
}

func waitForEndpointConnectionCreatedFunc(ctx context.Context, clusterID, endpointID string, cl client.Service, connection *client.AwsEndpointConnection) resource.RetryFunc {
	return func() *resource.RetryError {
		connections, httpResp, err := cl.ListAwsEndpointConnections(ctx, clusterID)
		if err != nil {
			if httpResp.StatusCode < http.StatusInternalServerError {
				return resource.NonRetryableError(fmt.Errorf("error getting endpoint connections: %s", formatAPIErrorMessage(err)))
			} else {
				return resource.RetryableError(fmt.Errorf("encountered a server error while reading connection status - trying again"))
			}
		}
		var found bool
		for _, *connection = range connections.GetConnections() {
			if connection.GetEndpointId() == endpointID {
				if connection.GetStatus() == client.AWSENDPOINTCONNECTIONSTATUS_AVAILABLE {
					return nil
				} else if connection.GetStatus() != client.AWSENDPOINTCONNECTIONSTATUS_PENDING {
					return resource.NonRetryableError(fmt.Errorf("endpoint connection failed"))
				}
				found = true
				break
			}
		}
		if !found {
			return resource.NonRetryableError(fmt.Errorf("endpoint connection failed"))
		}
		return resource.RetryableError(fmt.Errorf("endpoint connection is not ready yet"))
	}
}
