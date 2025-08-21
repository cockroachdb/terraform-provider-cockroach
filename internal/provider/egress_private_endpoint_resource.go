/*
 Copyright 2025 The Cockroach Authors

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
	"strings"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type egressPrivateEndpointResource struct {
	provider *provider
}

var AllowedEgressPrivateEndpointTargetServiceTypeTypeEnumValues = func() []string {
	var strings []string
	for i := range client.AllowedEgressPrivateEndpointTargetServiceTypeTypeEnumValues {
		strings = append(strings, string(client.AllowedEgressPrivateEndpointTargetServiceTypeTypeEnumValues[i]))
	}
	return strings
}()

var (
	egressPrivateEndpointCreateTimeout = 10 * time.Minute
	egressPrivateEndpointDeleteTimeout = 10 * time.Minute
	egressPrivateEndpointListTimeout   = 1 * time.Minute
)

func NewEgressPrivateEndpointResource() resource.Resource {
	return &egressPrivateEndpointResource{}
}

func (r *egressPrivateEndpointResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_egress_private_endpoint"
}

func (r *egressPrivateEndpointResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	p, ok := req.ProviderData.(*provider)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *provider, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.provider = p
}

func (r *egressPrivateEndpointResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Egress private endpoint for a CockroachDB Cloud cluster. Egress private endpoints allow CockroachDB Cloud clusters to connect to external services through private network connections without traversing the public internet.

### _Warning_: Use of *egress private endpoints* requires >=v1.14.0
If you intend to use this provider to provision egress private endpoints you must install/upgrade to [version 1.14.0](https://github.com/cockroachdb/terraform-provider-cockroach/releases/tag/v1.14.0) or [later](https://registry.terraform.io/providers/cockroachdb/cockroach/latest)

- [Terraform resource docs](https://github.com/cockroachdb/terraform-provider-cockroach/blob/main/docs/resources/egress_private_endpoint.md)
- [Cockroach Cloud docs](https://www.cockroachlabs.com/docs/cockroachcloud/egress-private-endpoints.html)
`,
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "cluster_id identifies the cluster to which this egress private endpoint applies",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "region represents the region that the endpoint will be created in",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"target_service_identifier": schema.StringAttribute{
				MarkdownDescription: "target_service_identifier represents the identifier of the target service. User-visible in CRL and CSP consoles as the \"thing\" the user is telling us to connect to, i.e. Service Name for AWS, Service Attachment for GCP, or ARN for AWS MSK.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"target_service_type": schema.StringAttribute{
				MarkdownDescription: "target_service_type represents the type of target service. Allowed values are:" +
					formatEnumMarkdownList(client.AllowedEgressPrivateEndpointTargetServiceTypeTypeEnumValues),
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(AllowedEgressPrivateEndpointTargetServiceTypeTypeEnumValues...),
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "id is a generated ID that uniquely identifies the egress_private_endpoint for use with the CockroachDB Cloud API. This ID is generic and not specific to the cloud provider",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"endpoint_connection_id": schema.StringAttribute{
				MarkdownDescription: "endpoint_connection_id is the cloud-specific id for egress private endpoints. This connection ID is visible in CRL and CSP consoles as \"VPC endpoint ID\" and is used to uniquely identify the endpoint in external configurations",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"endpoint_address": schema.StringAttribute{
				MarkdownDescription: "endpoint_address is the cloud-provider generated address (either IP address or DNS name) through which the endpoint is accessible",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"state": schema.StringAttribute{
				MarkdownDescription: "state represents the current state of the endpoint. Potential values are:" + formatEnumMarkdownList(client.AllowedEgressPrivateEndpointStateTypeEnumValues),
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Create implements resource.Resource.
func (r *egressPrivateEndpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan EgressPrivateEndpoint
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("GetCluster")
	cluster, _, err := r.provider.service.GetCluster(ctx, plan.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting cluster",
			fmt.Sprintf("Could not retrieve cluster info: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	if cluster.CloudProvider == client.CLOUDPROVIDERTYPE_AZURE {
		resp.Diagnostics.AddError(
			"Error creating egress private endpoint",
			"Egress private endpoints cannot be created on Azure clusters",
		)
		return
	} else if cluster.Config.Dedicated == nil {
		resp.Diagnostics.AddError(
			"Error creating egress private endpoint",
			"Egress private endpoints cannot be created on serverless clusters",
		)
		return
	}

	targetServiceType, err := client.NewEgressPrivateEndpointTargetServiceTypeTypeFromValue(plan.TargetServiceType.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating egress private endpoint",
			fmt.Sprintf("Invalid target service type: %s", err),
		)
		return
	}

	// Retry create operation if cluster is locked
	var createdEndpoint client.EgressPrivateEndpoint
	if err := retry.RetryContext(ctx, egressPrivateEndpointCreateTimeout,
		createEgressPrivateEndpoint(
			ctx,
			r.provider.service,
			cluster.Id,
			plan.Region.ValueString(),
			plan.TargetServiceIdentifier.ValueString(),
			*targetServiceType,
			&createdEndpoint,
		),
	); err != nil {
		resp.Diagnostics.AddError(
			"Error creating egress private endpoint",
			fmt.Sprintf("Could not create egress private endpoint: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var endpoint client.EgressPrivateEndpoint
	if err := retry.RetryContext(ctx, egressPrivateEndpointCreateTimeout,
		waitForEgressEndpointCreatedFunc(
			ctx,
			r.provider.service,
			cluster.Id,
			createdEndpoint.Id,
			&endpoint,
		),
	); err != nil {
		resp.Diagnostics.AddError(
			"Error creating egress private endpoint",
			fmt.Sprintf("Could not create egress private endpoint: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state EgressPrivateEndpoint
	loadEgressPrivateEndpointIntoTerraformState(&endpoint, cluster.Id, &state)
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// Delete implements resource.Resource.
func (r *egressPrivateEndpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state EgressPrivateEndpoint
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := retry.RetryContext(ctx, egressPrivateEndpointDeleteTimeout,
		deleteEgressEndpointFunc(
			ctx,
			r.provider.service,
			state.ClusterID.ValueString(),
			state.ID.ValueString(),
		),
	); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting egress private endpoint",
			fmt.Sprintf("Could not delete egress private endpoint: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	if err := retry.RetryContext(ctx, egressPrivateEndpointDeleteTimeout,
		waitForEgressEndpointDeletedFunc(
			ctx,
			r.provider.service,
			state.ClusterID.ValueString(),
			state.ID.ValueString(),
		),
	); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting egress private endpoint",
			fmt.Sprintf("Could not delete egress private endpoint: %s", formatAPIErrorMessage(err)),
		)
		return
	}
}

// Read implements resource.Resource.
func (r *egressPrivateEndpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state EgressPrivateEndpoint
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get all endpoints with pagination
	allEndpoints, err := getAllEgressEndpointsForCluster(ctx, r.provider.service, state.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to get egress private endpoint",
			fmt.Sprintf("Unexpected error retrieving egress private endpoint: %s", err.Error()),
		)
		return
	}

	for _, endpoint := range allEndpoints {
		if endpoint.GetId() == state.ID.ValueString() {
			loadEgressPrivateEndpointIntoTerraformState(&endpoint, state.ClusterID.ValueString(), &state)
			diags = resp.State.Set(ctx, state)
			resp.Diagnostics.Append(diags...)
			return
		}
	}
	resp.Diagnostics.AddWarning(
		"Egress private endpoint not found",
		"Egress private endpoint could not be found. Removing from state.",
	)
	resp.State.RemoveResource(ctx)
}

func (r *egressPrivateEndpointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// No-op. Contains only requires-replace or computed fields.
}

func (r *egressPrivateEndpointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid egress private endpoint ID format",
			`When importing an egress private endpoint, the ID should follow the format "<cluster_id>:<endpoint_id>"`,
		)
		return
	}

	clusterID := parts[0]
	endpointID := parts[1]

	if clusterID == "" || endpointID == "" {
		resp.Diagnostics.AddError(
			"Invalid egress private endpoint ID format",
			"Both cluster_id and endpoint_id must be non-empty",
		)
		return
	}

	// Actual endpoint details will be populated by the Read method
	endpoint := EgressPrivateEndpoint{
		ClusterID: types.StringValue(clusterID),
		ID:        types.StringValue(endpointID),
	}

	diags := resp.State.Set(ctx, &endpoint)
	resp.Diagnostics.Append(diags...)
}

func createEgressPrivateEndpoint(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	region string,
	targetServiceIdentifier string,
	targetServiceType client.EgressPrivateEndpointTargetServiceTypeType,
	createdEndpoint *client.EgressPrivateEndpoint,
) retry.RetryFunc {
	return func() *retry.RetryError {
		createReq := client.CreateEgressPrivateEndpointRequest{
			Region:                  region,
			TargetServiceIdentifier: targetServiceIdentifier,
			TargetServiceType:       targetServiceType,
		}

		traceAPICall("CreateEgressPrivateEndpoint")
		endpoint, httpResp, createErr := cl.CreateEgressPrivateEndpoint(
			ctx,
			clusterID,
			&createReq,
		)
		if createErr != nil {
			if httpResp != nil && httpResp.StatusCode == http.StatusConflict {
				// Cluster is locked - retry after brief delay
				return retry.RetryableError(
					fmt.Errorf("cluster is locked, retrying: %s", formatAPIErrorMessage(createErr)),
				)
			}
			// Other errors are not retryable
			return retry.NonRetryableError(createErr)
		}
		*createdEndpoint = *endpoint
		return nil
	}
}

func waitForEgressEndpointCreatedFunc(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	endpointID string,
	endpoint *client.EgressPrivateEndpoint,
) retry.RetryFunc {
	return func() *retry.RetryError {
		// Get all endpoints with pagination
		allEndpoints, err := getAllEgressEndpointsForCluster(ctx, cl, clusterID)
		if err != nil {
			// Check if it's a server error (should retry) or client error (should not retry)
			return retry.RetryableError(
				fmt.Errorf("encountered an error while getting egress private endpoints, retrying: %s", err.Error()),
			)
		}

		for _, ep := range allEndpoints {
			*endpoint = ep
			if endpoint.GetId() != endpointID {
				continue
			} else if isEgressPrivateEndpointCreatedInCloud(endpoint) {
				return nil
			}
			return retry.RetryableError(fmt.Errorf("egress private endpoint has not been created yet"))
		}
		return retry.NonRetryableError(fmt.Errorf("egress private endpoint not found"))
	}
}

func isEgressPrivateEndpointCreatedInCloud(endpoint *client.EgressPrivateEndpoint) bool {
	return endpoint.EndpointConnectionId != "" && endpoint.EndpointAddress != ""
}

func loadEgressPrivateEndpointIntoTerraformState(endpoint *client.EgressPrivateEndpoint, clusterID string, state *EgressPrivateEndpoint) {
	state.ID = types.StringValue(endpoint.GetId())
	state.ClusterID = types.StringValue(clusterID)
	state.EndpointConnectionID = types.StringValue(endpoint.GetEndpointConnectionId())
	state.Region = types.StringValue(endpoint.GetRegion())
	state.TargetServiceIdentifier = types.StringValue(endpoint.GetTargetServiceIdentifier())
	state.TargetServiceType = types.StringValue(string(endpoint.GetTargetServiceType()))
	state.EndpointAddress = types.StringValue(endpoint.GetEndpointAddress())
	state.State = types.StringValue(string(endpoint.GetState()))
}

func deleteEgressEndpointFunc(ctx context.Context, cl client.Service, clusterID string, endpointID string) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("DeleteEgressPrivateEndpoint")
		httpResp, err := cl.DeleteEgressPrivateEndpoint(
			ctx,
			clusterID,
			endpointID,
		)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
				// Already deleted
				return nil
			} else if httpResp != nil && httpResp.StatusCode == http.StatusConflict {
				// Cluster is locked, retry
				return retry.RetryableError(
					fmt.Errorf("cluster is locked, retrying: %s", formatAPIErrorMessage(err)),
				)
			}
			return retry.NonRetryableError(err)
		}
		return nil
	}
}

func waitForEgressEndpointDeletedFunc(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	endpointID string,
) retry.RetryFunc {
	return func() *retry.RetryError {
		// Get all endpoints with pagination
		allEndpoints, err := getAllEgressEndpointsForCluster(ctx, cl, clusterID)
		if err != nil {
			return retry.RetryableError(
				fmt.Errorf("encountered an error while getting egress private endpoints, retrying: %s", err.Error()),
			)
		}

		for _, endpoint := range allEndpoints {
			if endpoint.GetId() == endpointID {
				return retry.RetryableError(fmt.Errorf("egress private endpoint still exists"))
			}
		}
		return nil
	}
}

func getAllEgressEndpointsForCluster(
	ctx context.Context,
	cl client.Service,
	clusterID string,
) ([]client.EgressPrivateEndpoint, error) {
	var allEndpoints []client.EgressPrivateEndpoint
	var paginationPage *string
	for {
		endpoints, next, err := getEgressEndpointsForClusterOnPage(ctx, cl, clusterID, paginationPage)
		if err != nil {
			return nil, err
		}
		allEndpoints = append(allEndpoints, endpoints...)
		if next == nil {
			break
		}
		paginationPage = next
	}
	return allEndpoints, nil
}

func getEgressEndpointsForClusterOnPage(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	page *string,
) (endpoints []client.EgressPrivateEndpoint, nextPage *string, err error) {
	getEndpoints := func() *retry.RetryError {
		options := client.ListEgressPrivateEndpointsOptions{PaginationPage: page}

		traceAPICall("ListEgressPrivateEndpoints")
		resp, httpResp, err := cl.ListEgressPrivateEndpoints(ctx, clusterID, &options)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return retry.NonRetryableError(
					fmt.Errorf(
						"error getting egress private endpoints: %s",
						formatAPIErrorMessage(err),
					),
				)
			}
			return retry.RetryableError(
				fmt.Errorf(
					"encountered a server error while getting egress private endpoints, retrying: %s",
					formatAPIErrorMessage(err),
				),
			)
		}

		endpoints = append(endpoints, resp.EgressPrivateEndpoints...)
		if resp.Pagination != nil && resp.Pagination.NextPage != nil && *resp.Pagination.NextPage != "" {
			nextPage = resp.Pagination.NextPage
		}
		return nil
	}

	if err := retry.RetryContext(ctx, egressPrivateEndpointListTimeout, getEndpoints); err != nil {
		return nil, nil, fmt.Errorf("failed to list egress private endpoints on page: %s", formatAPIErrorMessage(err))
	}
	return endpoints, nextPage, nil
}
