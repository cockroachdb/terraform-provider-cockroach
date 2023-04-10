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
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	sdk_resource "github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

const (
	// clusterID:endpointID
	privateEndpointConnectionIDFmt  = "%s:%s"
	endpointConnectionCreateTimeout = time.Minute * 10
)

var privateEndpointConnectionIDRegex = regexp.MustCompile(fmt.Sprintf("^(%s):(.*)$", uuidRegexString))

type privateEndpointConnectionResource struct {
	provider *provider
}

func (r *privateEndpointConnectionResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "AWS PrivateLink Endpoint Connection",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Used with `terrform import`. Format is \"<cluster ID>:<endpoint ID>\"",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"region_name": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cloud_provider": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"endpoint_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"service_id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *privateEndpointConnectionResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_private_endpoint_connection"
}

func (r *privateEndpointConnectionResource) Configure(
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

func (r *privateEndpointConnectionResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan PrivateEndpointConnection
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cluster, _, err := r.provider.service.GetCluster(ctx, plan.ClusterID.ValueString())
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
			"Private endpoint services are only available for dedicated clusters",
		)
		return
	} else if cluster.CloudProvider != client.CLOUDPROVIDERTYPE_AWS {
		resp.Diagnostics.AddError(
			"Incompatible cluster cloud provider",
			"Private endpoint services are only available for AWS clusters",
		)
		return
	}

	connectionStateRequest := client.SetAwsEndpointConnectionStateRequest{
		Status: client.SETAWSENDPOINTCONNECTIONSTATUSTYPE_AVAILABLE,
	}

	_, _, err = r.provider.service.SetAwsEndpointConnectionState(ctx, plan.ClusterID.ValueString(), plan.EndpointID.ValueString(), &connectionStateRequest)
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
	err = sdk_resource.RetryContext(ctx, endpointConnectionCreateTimeout,
		waitForEndpointConnectionCreatedFunc(ctx, cluster.Id, plan.EndpointID.ValueString(), r.provider.service, &connection))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error accepting private endpoint connection",
			fmt.Sprintf("Could not accept private endpoint connection: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state PrivateEndpointConnection
	state.ClusterID = plan.ClusterID
	loadEndpointConnectionIntoTerraformState(&connection, &state)
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *privateEndpointConnectionResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state PrivateEndpointConnection
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	connections, _, err := r.provider.service.ListAwsEndpointConnections(ctx, state.ClusterID.ValueString())
	if err != nil {
		diags.AddError("Unable to get endpoint connection status",
			fmt.Sprintf("Unexpected error retrieving endpoint status: %s", formatAPIErrorMessage(err)))
		return
	}
	for _, connection := range connections.GetConnections() {
		if connection.GetEndpointId() == state.EndpointID.ValueString() {
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

func loadEndpointConnectionIntoTerraformState(
	apiConnection *client.AwsEndpointConnection, state *PrivateEndpointConnection,
) {
	state.EndpointID = types.StringValue(apiConnection.GetEndpointId())
	state.ID = types.StringValue(fmt.Sprintf(
		privateEndpointConnectionIDFmt,
		state.ClusterID.ValueString(),
		apiConnection.GetEndpointId()))
	state.ServiceID = types.StringValue(apiConnection.GetServiceId())
	state.CloudProvider = types.StringValue(string(apiConnection.GetCloudProvider()))
	state.RegionName = types.StringValue(apiConnection.GetRegionName())
}

func (r *privateEndpointConnectionResource) Update(
	_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse,
) {
	// No-op. Contains only requires-replace or computed fields.
}

func (r *privateEndpointConnectionResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state PrivateEndpointConnection
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, err := r.provider.service.SetAwsEndpointConnectionState(
		ctx,
		state.ClusterID.ValueString(),
		state.EndpointID.ValueString(),
		&client.SetAwsEndpointConnectionStateRequest{
			Status: client.SETAWSENDPOINTCONNECTIONSTATUSTYPE_REJECTED,
		})
	if err != nil && httpResp != nil && httpResp.StatusCode != http.StatusNotFound {
		diags.AddError("Couldn't delete connection",
			fmt.Sprintf("Unexpected error occurred while setting connection status: %s", formatAPIErrorMessage(err)))
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r *privateEndpointConnectionResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
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
		ClusterID:  types.StringValue(matches[1]),
		EndpointID: types.StringValue(matches[2]),
		ID:         types.StringValue(req.ID),
	}
	resp.Diagnostics = resp.State.Set(ctx, &connection)
}

func waitForEndpointConnectionCreatedFunc(
	ctx context.Context,
	clusterID, endpointID string,
	cl client.Service,
	connection *client.AwsEndpointConnection,
) sdk_resource.RetryFunc {
	return func() *sdk_resource.RetryError {
		connections, httpResp, err := cl.ListAwsEndpointConnections(ctx, clusterID)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return sdk_resource.NonRetryableError(fmt.Errorf("error getting endpoint connections: %s", formatAPIErrorMessage(err)))
			} else {
				return sdk_resource.RetryableError(fmt.Errorf("encountered a server error while reading connection status - trying again"))
			}
		}

		for _, *connection = range connections.GetConnections() {
			if connection.GetEndpointId() == endpointID {
				switch status := connection.GetStatus(); status {
				case client.AWSENDPOINTCONNECTIONSTATUSTYPE_AVAILABLE:
					return nil
				case client.AWSENDPOINTCONNECTIONSTATUSTYPE_PENDING,
					client.AWSENDPOINTCONNECTIONSTATUSTYPE_PENDING_ACCEPTANCE:
					return sdk_resource.RetryableError(fmt.Errorf("endpoint connection is not ready yet"))
				default:
					return sdk_resource.NonRetryableError(fmt.Errorf("endpoint connection failed with state: %s", status))
				}
			}
		}
		return sdk_resource.NonRetryableError(fmt.Errorf("endpoint connection lost, presumed failed"))
	}
}

func NewPrivateEndpointConnectionResource() resource.Resource {
	return &privateEndpointConnectionResource{}
}
