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
	"errors"
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
- [Terraform resource docs source](https://github.com/cockroachdb/terraform-provider-cockroach/blob/main/docs/resources/egress_private_endpoint.md)
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
				MarkdownDescription: "target_service_identifier represents the identifier of the target service that the CockroachDB endpoint connects to. For example, this would be the Service Name for AWS, Service Attachment for GCP, or ARN for AWS MSK",
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
				MarkdownDescription: "endpoint_connection_id is the cloud service-specific ID for egress private endpoints. This connection ID is visible in CockroachDB and cloud service provider consoles as \"VPC endpoint ID\" and is used to uniquely identify the endpoint in external configurations",
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
	if err := retry.RetryContext(ctx, clusterUpdateTimeout,
		createEgressPrivateEndpointFunc(
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

	if err := retry.RetryContext(ctx, clusterUpdateTimeout,
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
	endpoint, err := getEgressPrivateEndpoint(ctx, r.provider.service, state.ClusterID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to get egress private endpoint",
			fmt.Sprintf("Unexpected error retrieving egress private endpoint: %s", err.Unwrap()),
		)
		return
	} else if endpoint == nil {
		resp.Diagnostics.AddWarning(
			"Egress private endpoint not found",
			"Egress private endpoint could not be found. Removing from state.",
		)
		resp.State.RemoveResource(ctx)
		return
	}

	loadEgressPrivateEndpointIntoTerraformState(endpoint, state.ClusterID.ValueString(), &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
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

func createEgressPrivateEndpointFunc(
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
			if httpResp != nil {
				apiErrMsg := formatAPIErrorMessage(createErr)

				if httpResp.StatusCode == http.StatusServiceUnavailable || strings.Contains(apiErrMsg, "lock") {
					return retry.RetryableError(fmt.Errorf(
						"cluster is locked, retrying: %s", formatAPIErrorMessage(createErr)),
					)
				} else if httpResp.StatusCode < http.StatusInternalServerError {
					return retry.NonRetryableError(fmt.Errorf(
						"encounted error creating egress private endpoint: %s",
						formatAPIErrorMessage(createErr),
					))
				}
			}
			return retry.RetryableError(fmt.Errorf(
				"encounted server error creating egress private endpoint, will retry: %s",
				formatAPIErrorMessage(createErr),
			))
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
		ep, err := getEgressPrivateEndpoint(ctx, cl, clusterID, endpointID)
		if err != nil {
			return err
		} else if ep == nil {
			return retry.NonRetryableError(fmt.Errorf("egress private endpoint not found"))
		}

		if !isEgressPrivateEndpointCreatedInCloud(ep) {
			return retry.RetryableError(fmt.Errorf("egress private endpoint has not been created yet"))
		}

		*endpoint = *ep
		return nil
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
			if httpResp != nil {
				apiErrMsg := formatAPIErrorMessage(err)

				if httpResp.StatusCode == http.StatusNotFound {
					// Already deleted
					return nil
				} else if httpResp.StatusCode == http.StatusServiceUnavailable || strings.Contains(apiErrMsg, "lock") {
					return retry.RetryableError(fmt.Errorf(
						"cluster is locked, retrying: %s", formatAPIErrorMessage(err)),
					)
				} else if httpResp.StatusCode < http.StatusInternalServerError {
					return retry.NonRetryableError(fmt.Errorf(
						"encounted error deleting egress private endpoint: %s",
						formatAPIErrorMessage(err),
					))
				}
			}
			return retry.RetryableError(fmt.Errorf(
				"encounted server error deleting egress private endpoint, will retry: %s",
				formatAPIErrorMessage(err),
			))
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
		endpoint, err := getEgressPrivateEndpoint(ctx, cl, clusterID, endpointID)
		if err != nil {
			return err
		} else if endpoint != nil {
			return retry.RetryableError(fmt.Errorf("egress private endpoint still exists"))
		}
		return nil
	}
}

func getEgressPrivateEndpoint(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	endpointID string,
) (*client.EgressPrivateEndpoint, *retry.RetryError) {
	traceAPICall("GetEgressPrivateEndpoint")
	resp, httpResp, err := cl.GetEgressPrivateEndpoint(ctx, clusterID, endpointID)
	if err != nil {
		if httpResp != nil {
			if httpResp.StatusCode == http.StatusNotFound {
				return nil, nil
			} else if httpResp.StatusCode < http.StatusInternalServerError {
				return nil, retry.NonRetryableError(fmt.Errorf(
					"encounted error getting egress private endpoint: %s",
					formatAPIErrorMessage(err),
				))
			}
		}
		return nil, retry.RetryableError(fmt.Errorf(
			"encounted server error getting egress private endpoint, will retry: %s",
			formatAPIErrorMessage(err),
		))
	} else if resp == nil {
		return nil, retry.RetryableError(
			errors.New("got no response when getting egress private endpoint"),
		)
	}
	return resp.EgressPrivateEndpoint, nil
}
