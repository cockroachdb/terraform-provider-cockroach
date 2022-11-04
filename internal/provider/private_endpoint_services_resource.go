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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

type privateEndpointServicesResourceType struct{}

func (n privateEndpointServicesResourceType) GetSchema(_ context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		MarkdownDescription: "PrivateEndpointServices contains services that allow for VPC communication, either via PrivateLink (AWS) or Peering (GCP)",
		Attributes: map[string]tfsdk.Attribute{
			"cluster_id": {
				Required: true,
				Type:     types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.RequiresReplace(),
				},
			},
			"id": {
				Computed: true,
				Type:     types.StringType,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
				MarkdownDescription: "Always matches the cluster ID. Required by Terraform.",
			},
			"services": {
				Computed: true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
				Attributes: tfsdk.ListNestedAttributes(map[string]tfsdk.Attribute{
					"region_name": {
						Computed: true,
						Type:     types.StringType,
					},
					"cloud_provider": {
						Computed: true,
						Type:     types.StringType,
					},
					"status": {
						Computed: true,
						Type:     types.StringType,
					},
					"aws": {
						Computed: true,
						PlanModifiers: tfsdk.AttributePlanModifiers{
							tfsdk.UseStateForUnknown(),
						},
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
								Computed:            true,
								Type:                types.ListType{ElemType: types.StringType},
								MarkdownDescription: "AZ IDs users should create their VPCs in to minimize their cost.",
							},
						}),
					},
				}, tfsdk.ListNestedAttributesOptions{}),
			},
		},
	}, nil
}

func (n privateEndpointServicesResourceType) NewResource(_ context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return privateEndpointServicesResource{
		provider: provider,
	}, diags
}

type privateEndpointServicesResource struct {
	provider provider
}

func (n privateEndpointServicesResource) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	if !n.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var config PrivateEndpointServices
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cluster, _, err := n.provider.service.GetCluster(ctx, config.ClusterID.Value)
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

	body := make(map[string]interface{}, 0)
	// If private endpoint services already exist for this cluster,
	// this is a no-op. The API will gracefully return the existing services.
	_, _, err = n.provider.service.CreatePrivateEndpointServices(ctx, config.ClusterID.Value, &body)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling private endpoint services",
			fmt.Sprintf("Could not enable private endpoint services: %s", formatAPIErrorMessage(err)),
		)
		return
	}
	var services client.PrivateEndpointServices
	err = resource.RetryContext(ctx, CREATE_TIMEOUT,
		waitForEndpointServicesCreatedFunc(ctx, cluster.Id, n.provider.service, &services))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling private endpoint services",
			fmt.Sprintf("Could not enable private endpoint services: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state PrivateEndpointServices
	state.ClusterID = config.ClusterID
	state.ID = config.ClusterID
	loadEndpointServicesIntoTerraformState(&services, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (n privateEndpointServicesResource) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	if !n.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state PrivateEndpointServices
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiResp, httpResp, err := n.provider.service.ListPrivateEndpointServices(ctx, state.ClusterID.Value)
	if err != nil {
		if httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning("Couldn't find endpoint services",
				"Couldn't find endpoint services, which usually means the cluster has been deleted. Removing from state.")
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError("Couldn't retrieve endpoint services",
				fmt.Sprintf("Error retrieving endpoint services: %s", formatAPIErrorMessage(err)))
		}
		return
	}
	loadEndpointServicesIntoTerraformState(apiResp, &state)
	state.ID = state.ClusterID
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func loadEndpointServicesIntoTerraformState(apiServices *client.PrivateEndpointServices, state *PrivateEndpointServices) {
	serviceList := apiServices.GetServices()
	state.Services = make([]PrivateEndpointService, len(serviceList))
	for i, service := range serviceList {
		state.Services[i] = PrivateEndpointService{
			RegionName:    types.String{Value: service.GetRegionName()},
			CloudProvider: types.String{Value: string(service.GetCloudProvider())},
			Status:        types.String{Value: string(service.GetStatus())},
			Aws: PrivateLinkServiceAWSDetail{
				ServiceName: types.String{Value: service.Aws.GetServiceName()},
				ServiceId:   types.String{Value: service.Aws.GetServiceId()},
			},
		}
		apiAZs := service.Aws.GetAvailabilityZoneIds()
		azs := make([]types.String, len(apiAZs))
		for j, az := range apiAZs {
			azs[j] = types.String{Value: az}
		}
		state.Services[i].Aws.AvailabilityZoneIds = azs
	}
}

func (n privateEndpointServicesResource) Update(_ context.Context, _ tfsdk.UpdateResourceRequest, _ *tfsdk.UpdateResourceResponse) {
	// no-op - Endpoint services cannot be updated

	// The only non-computed field is the cluster ID, which requires replace,
	// so no extra warning is necessary here.
}

func (n privateEndpointServicesResource) Delete(_ context.Context, _ tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	// no-op - Endpoint services cannot be deleted
	resp.Diagnostics.AddWarning("Cannot remove endpoint services",
		"Endpoint Services resources can't be deleted once established."+
			"This operation will remove the Terraform resource, but not the underlying services.",
	)
}

func (n privateEndpointServicesResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("cluster_id"), req, resp)
}

func waitForEndpointServicesCreatedFunc(ctx context.Context, clusterID string, cl client.Service, services *client.PrivateEndpointServices) resource.RetryFunc {
	return func() *resource.RetryError {
		apiServices, httpResp, err := cl.ListPrivateEndpointServices(ctx, clusterID)
		if err != nil {
			if httpResp.StatusCode < http.StatusInternalServerError {
				return resource.NonRetryableError(fmt.Errorf("error getting endpoint services: %s", formatAPIErrorMessage(err)))
			} else {
				return resource.RetryableError(fmt.Errorf("encountered a server error while reading endpoint status - trying again"))
			}
		}
		*services = *apiServices
		var creating bool
		// If there's at least one still creating, keep checking.
		// If any have failed or have any other non-available status, something went wrong.
		// If all the services have been created, we're done.
		for _, service := range services.GetServices() {
			if service.GetStatus() == client.PRIVATEENDPOINTSERVICESTATUS_CREATING {
				creating = true
			} else if service.GetStatus() != client.PRIVATEENDPOINTSERVICESTATUS_AVAILABLE {
				return resource.NonRetryableError(fmt.Errorf("endpoint service creation failed"))
			}
		}
		if creating {
			return resource.RetryableError(fmt.Errorf("endpoint services are not ready yet"))
		}
		return nil
	}
}
