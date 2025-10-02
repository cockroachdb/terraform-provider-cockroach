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
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

var (
	egressPrivateEndpointAvailableTimeout            = 10 * time.Minute
	egressPrivateEndpointDomainNamesAvailableTimeout = 10 * time.Minute
)

type egressPrivateEndpointDomainNamesResource struct {
	provider *provider
}

func NewEgressPrivateEndpointDomainNamesResource() resource.Resource {
	return &egressPrivateEndpointDomainNamesResource{}
}

func (r *egressPrivateEndpointDomainNamesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_egress_private_endpoint_domain_names"
}

func (r *egressPrivateEndpointDomainNamesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *egressPrivateEndpointDomainNamesResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manages custom domain names for an egress private endpoint in a CockroachDB Cloud cluster, in environments that require a custom name resolution to the endpoint_address of the endpoint.
- [Terraform resource docs source](https://github.com/cockroachdb/terraform-provider-cockroach/blob/main/docs/resources/egress_private_endpoint.md)
- [Cockroach Cloud docs](https://www.cockroachlabs.com/docs/cockroachcloud/egress-private-endpoints.html)
`,
		Attributes: map[string]schema.Attribute{
			"endpoint_id": schema.StringAttribute{
				MarkdownDescription: "endpoint_id is the `id` value of the egress private endpoint in CockroachDB Cloud.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "cluster_id identifies the cluster to which this egress private endpoint applies",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"domain_names": schema.ListAttribute{
				MarkdownDescription: "domain_names contains a list of domain names to associate with the egress private endpoint.",
				Required:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (r *egressPrivateEndpointDomainNamesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan EgressPrivateEndpointDomainNames
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	newState := r.upsertEgressPrivateEndpointDomainNames(ctx, &resp.Diagnostics, plan)
	if newState == nil {
		return
	}
	diags = resp.State.Set(ctx, newState)
	resp.Diagnostics.Append(diags...)
}

func (r *egressPrivateEndpointDomainNamesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state EgressPrivateEndpointDomainNames
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, err := getEgressPrivateEndpoint(ctx, r.provider.service, state.ClusterID.ValueString(), state.EndpointID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting egress private endpoint",
			fmt.Sprintf("Could not retrieve egress private endpoint info: %s", formatAPIErrorMessage(err.Unwrap())),
		)
		return
	} else if endpoint == nil {
		// Parent endpoint doesn't exist, we can successfully consider this resource deleted as well
		return
	}

	if err := updateEgressPrivateEndpointDomainNames(
		ctx,
		r.provider.service,
		state.ClusterID.ValueString(),
		state.EndpointID.ValueString(),
		// Set empty domain names for this endpoint
		[]string{},
	); err != nil {
		resp.Diagnostics.AddError(
			"Error updating egress private endpoint domain names",
			fmt.Sprintf("Could not update egress private endpoint: %s", formatAPIErrorMessage(err)),
		)
		return
	}
}

func (r *egressPrivateEndpointDomainNamesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state EgressPrivateEndpointDomainNames
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, err := getEgressPrivateEndpoint(ctx, r.provider.service, state.ClusterID.ValueString(), state.EndpointID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting egress private endpoint",
			fmt.Sprintf("Could not retrieve egress private endpoint info: %s", formatAPIErrorMessage(err.Unwrap())),
		)
		return
	} else if endpoint == nil {
		// Parent endpoint doesn't exist, we can remove this resource from the state
		resp.Diagnostics.AddWarning(
			"Egress private endpoint not found",
			"Egress private endpoint could not be found. Removing from state.",
		)
		resp.State.RemoveResource(ctx)
		return
	}

	domainNamesValues := []attr.Value{}
	for _, domainName := range endpoint.DomainNames {
		domainNamesValues = append(domainNamesValues, types.StringValue(domainName))
	}
	domainNamesList, diags := types.ListValue(types.StringType, domainNamesValues)
	resp.Diagnostics.Append(diags...)

	state.DomainNames = domainNamesList
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *egressPrivateEndpointDomainNamesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan EgressPrivateEndpointDomainNames
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	newState := r.upsertEgressPrivateEndpointDomainNames(ctx, &resp.Diagnostics, plan)
	if newState == nil {
		return
	}
	diags = resp.State.Set(ctx, newState)
	resp.Diagnostics.Append(diags...)
}

func (r *egressPrivateEndpointDomainNamesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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

	endpoint := EgressPrivateEndpointDomainNames{
		ClusterID:   types.StringValue(clusterID),
		EndpointID:  types.StringValue(endpointID),
		DomainNames: types.ListNull(types.StringType),
	}

	diags := resp.State.Set(ctx, &endpoint)
	resp.Diagnostics.Append(diags...)
}

func (r *egressPrivateEndpointDomainNamesResource) upsertEgressPrivateEndpointDomainNames(
	ctx context.Context,
	diags *diag.Diagnostics,
	plan EgressPrivateEndpointDomainNames,
) *EgressPrivateEndpointDomainNames {
	endpoint, err := getEgressPrivateEndpoint(ctx, r.provider.service, plan.ClusterID.ValueString(), plan.EndpointID.ValueString())
	if err != nil {
		diags.AddError(
			"Error getting egress private endpoint",
			fmt.Sprintf("Could not retrieve egress private endpoint info: %s", formatAPIErrorMessage(err.Unwrap())),
		)
		return nil
	} else if endpoint == nil {
		diags.AddError(
			"Error getting egress private endpoint",
			fmt.Sprintf("egress private endpoint with ID %s does not exist", plan.EndpointID.ValueString()),
		)
		return nil
	}

	domainNames := []string{}
	for _, domainNameValue := range plan.DomainNames.Elements() {
		domainName, ok := domainNameValue.(types.String)
		if !ok {
			diags.AddError(
				"Unexpected error",
				fmt.Sprintf("domain names value %s is not a string", domainNameValue.String()),
			)
			return nil
		}
		domainNames = append(domainNames, domainName.ValueString())
	}

	if err := updateEgressPrivateEndpointDomainNames(
		ctx,
		r.provider.service,
		plan.ClusterID.ValueString(),
		plan.EndpointID.ValueString(),
		domainNames,
	); err != nil {
		diags.AddError(
			"Error updating egress private endpoint domain names",
			fmt.Sprintf("Could not update egress private endpoint: %s", formatAPIErrorMessage(err)),
		)
		return nil
	}

	newState := EgressPrivateEndpointDomainNames{
		ClusterID:   plan.ClusterID,
		EndpointID:  plan.EndpointID,
		DomainNames: plan.DomainNames,
	}
	return &newState

}

func updateEgressPrivateEndpointDomainNames(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	endpointID string,
	domainNames []string,
) error {
	if err := retry.RetryContext(
		ctx,
		egressPrivateEndpointAvailableTimeout,
		waitForEgressEndpointAvailableFunc(ctx, cl, clusterID, endpointID),
	); err != nil {
		return err
	}

	if err := retry.RetryContext(
		ctx,
		clusterUpdateTimeout,
		updateEgressPrivateEndpointFunc(ctx, cl, clusterID, endpointID, domainNames),
	); err != nil {
		return err
	}

	desiredState := client.EGRESSPRIVATEENDPOINTDOMAINNAMESSTATETYPE_AVAILABLE
	if len(domainNames) == 0 {
		desiredState = client.EGRESSPRIVATEENDPOINTDOMAINNAMESSTATETYPE_EMPTY
	}

	return retry.RetryContext(
		ctx,
		egressPrivateEndpointDomainNamesAvailableTimeout,
		waitForEgressEndpointDomainNamesStateFunc(ctx, cl, clusterID, endpointID, desiredState),
	)
}

func waitForEgressEndpointAvailableFunc(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	endpointID string,
) retry.RetryFunc {
	return func() *retry.RetryError {
		endpoint, err := getEgressPrivateEndpoint(ctx, cl, clusterID, endpointID)
		if err != nil {
			return err
		} else if endpoint == nil {
			return retry.NonRetryableError(fmt.Errorf("egress private endpoint not found"))
		} else if endpoint.GetState() == client.EGRESSPRIVATEENDPOINTSTATETYPE_PENDING_ACCEPTANCE {
			// Endpoint will never become available without user intervention.
			// Do not retry, and return information required for the user to proceed (endpoint_connection_id).
			return retry.NonRetryableError(fmt.Errorf("egress private endpoint requires acceptance. endpoint_connection_id: %s", endpoint.GetEndpointConnectionId()))
		} else if endpoint.GetState() == client.EGRESSPRIVATEENDPOINTSTATETYPE_PENDING {
			return retry.RetryableError(errors.New("egress private endpoint is pending"))
		} else if endpoint.GetState() != client.EGRESSPRIVATEENDPOINTSTATETYPE_AVAILABLE {
			return retry.NonRetryableError(fmt.Errorf("egress private endpoint is in an unexpected state: %v", endpoint.GetState()))
		}
		return nil
	}
}

func updateEgressPrivateEndpointFunc(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	endpointID string,
	domainNames []string,
) retry.RetryFunc {
	return func() *retry.RetryError {
		req := client.UpdateEgressPrivateEndpointRequest{
			DomainNames: domainNames,
		}

		traceAPICall("UpdateEgressPrivateEndpoint")
		_, httpResp, err := cl.UpdateEgressPrivateEndpoint(ctx, clusterID, endpointID, &req)
		if err != nil {
			if httpResp != nil {
				apiErrMsg := formatAPIErrorMessage(err)

				if httpResp.StatusCode == http.StatusServiceUnavailable || strings.Contains(apiErrMsg, "lock") {
					return retry.RetryableError(fmt.Errorf(
						"cluster is locked, retrying: %s", formatAPIErrorMessage(err)),
					)
				} else if httpResp.StatusCode < http.StatusInternalServerError {
					return retry.NonRetryableError(fmt.Errorf(
						"encounted error updating egress private endpoint: %s",
						formatAPIErrorMessage(err),
					))
				}
			}
			return retry.RetryableError(fmt.Errorf(
				"encounted server error updating egress private endpoint, will retry: %s",
				formatAPIErrorMessage(err),
			))
		}
		return nil
	}
}

func waitForEgressEndpointDomainNamesStateFunc(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	endpointID string,
	desiredState client.EgressPrivateEndpointDomainNamesStateType,
) retry.RetryFunc {
	return func() *retry.RetryError {
		endpoint, err := getEgressPrivateEndpoint(ctx, cl, clusterID, endpointID)
		if err != nil {
			return err
		} else if endpoint == nil {
			return retry.NonRetryableError(errors.New("egress private endpoint not found"))
		} else if endpoint.DomainNamesState == nil {
			return retry.NonRetryableError(errors.New("egress private endpoint is missing domain name state"))
		}

		switch *endpoint.DomainNamesState {
		case client.EGRESSPRIVATEENDPOINTDOMAINNAMESSTATETYPE_PENDING:
			return retry.RetryableError(errors.New("egress private endpoint domain name setup is still pending"))
		case client.EGRESSPRIVATEENDPOINTDOMAINNAMESSTATETYPE_FAILED:
			return retry.NonRetryableError(errors.New("egress private endpoint domain name setup has failed"))
		case desiredState:
			return nil
		default:
			return retry.NonRetryableError(fmt.Errorf("egress private endpoint domain names are in an unexpected state: %v", endpoint.GetDomainNamesState()))
		}
	}
}
