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
		MarkdownDescription: "Private Endpoint Connection.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Used with `terraform import`. Format is \"<cluster ID>:<endpoint ID>\".",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"region_name": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "Cloud provider region code associated with this connection.",
			},
			"cloud_provider": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "Cloud provider associated with this connection.",
			},
			"endpoint_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "Client side ID of the Private Endpoint Connection.",
			},
			"service_id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "Server side ID of the Private Endpoint Connection.",
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
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	svc := r.provider.service
	cluster, _, err := svc.GetCluster(ctx, plan.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting cluster",
			fmt.Sprintf("Could not retrieve cluster info: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	addRequest := client.AddPrivateEndpointConnectionRequest{
		EndpointId: plan.EndpointID.ValueString(),
	}

	_, _, err = svc.AddPrivateEndpointConnection(ctx, cluster.Id, &addRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error establishing Private Endpoint Connection",
			fmt.Sprintf("Could not establish Private Endpoint Connection: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var connection client.PrivateEndpointConnection
	err = sdk_resource.RetryContext(ctx, endpointConnectionCreateTimeout,
		waitForEndpointConnectionCreatedFunc(ctx, cluster.Id, plan.EndpointID.ValueString(), svc, &connection))
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

	connections, _, err := r.provider.service.ListPrivateEndpointConnections(ctx, state.ClusterID.ValueString())
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
	apiConnection *client.PrivateEndpointConnection, state *PrivateEndpointConnection,
) {
	state.EndpointID = types.StringValue(apiConnection.GetEndpointId())
	state.ID = types.StringValue(fmt.Sprintf(
		privateEndpointConnectionIDFmt,
		state.ClusterID.ValueString(),
		apiConnection.GetEndpointId()))
	state.ServiceID = types.StringValue(apiConnection.GetEndpointServiceId())
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

	httpResp, err := r.provider.service.DeletePrivateEndpointConnection(
		ctx,
		state.ClusterID.ValueString(),
		state.EndpointID.ValueString(),
	)
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
	connection *client.PrivateEndpointConnection,
) sdk_resource.RetryFunc {
	return func() *sdk_resource.RetryError {
		connections, httpResp, err := cl.ListPrivateEndpointConnections(ctx, clusterID)
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
				case client.PRIVATEENDPOINTCONNECTIONSTATUS_AVAILABLE:
					return nil
				case client.PRIVATEENDPOINTCONNECTIONSTATUS_PENDING,
					client.PRIVATEENDPOINTCONNECTIONSTATUS_PENDING_ACCEPTANCE,
					client.PRIVATEENDPOINTCONNECTIONSTATUS_REJECTED:
					// Note: A REJECTED state means the user previously called
					// DeletePrivateEndpointConnection() on an existing
					// connection. A user can re-attach a rejected connection
					// by calling AddPrivateEndpointConnection() with the same
					// endpointId.
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
