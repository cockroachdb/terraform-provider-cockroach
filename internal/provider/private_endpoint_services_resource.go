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
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	sdk_resource "github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

type privateEndpointServicesResource struct {
	provider *provider
}

const endpointServicesCreateTimeout = time.Hour

func (r *privateEndpointServicesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "PrivateEndpointServices contains services that allow for VPC communication, either via PrivateLink (AWS) or Peering (GCP)",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "Always matches the cluster ID. Required by Terraform.",
			},
			"services": schema.ListNestedAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"region_name": schema.StringAttribute{
							Computed: true,
						},
						"cloud_provider": schema.StringAttribute{
							Computed: true,
						},
						"status": schema.StringAttribute{
							Computed: true,
						},
						"aws": schema.SingleNestedAttribute{
							Computed: true,
							PlanModifiers: []planmodifier.Object{
								objectplanmodifier.UseStateForUnknown(),
							},
							Attributes: map[string]schema.Attribute{
								"service_name": schema.StringAttribute{
									Computed: true,
								},
								"service_id": schema.StringAttribute{
									Computed: true,
								},
								"availability_zone_ids": schema.ListAttribute{
									Computed:            true,
									ElementType:         types.StringType,
									MarkdownDescription: "AZ IDs users should create their VPCs in to minimize their cost.",
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *privateEndpointServicesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_private_endpoint_services"
}

func (r *privateEndpointServicesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if r.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError("Internal provider error",
			fmt.Sprintf("Error in Configure: expected %T but got %T", provider{}, req.ProviderData))
	}
}

func (r *privateEndpointServicesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var config PrivateEndpointServices
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cluster, _, err := r.provider.service.GetCluster(ctx, config.ClusterID.ValueString())
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
	} else if cluster.CloudProvider != client.APICLOUDPROVIDER_AWS {
		resp.Diagnostics.AddError(
			"Incompatible cluster cloud provider",
			"Private endpoint services are currently only available for AWS clusters",
		)
		return
	}

	body := make(map[string]interface{}, 0)
	// If private endpoint services already exist for this cluster,
	// this is a no-op. The API will gracefully return the existing services.
	_, _, err = r.provider.service.CreatePrivateEndpointServices(ctx, config.ClusterID.ValueString(), &body)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling private endpoint services",
			fmt.Sprintf("Could not enable private endpoint services: %s", formatAPIErrorMessage(err)),
		)
		return
	}
	var services client.PrivateEndpointServices
	err = sdk_resource.RetryContext(ctx, endpointServicesCreateTimeout,
		waitForEndpointServicesCreatedFunc(ctx, cluster.Id, r.provider.service, &services))
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

func (r *privateEndpointServicesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state PrivateEndpointServices
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiResp, httpResp, err := r.provider.service.ListPrivateEndpointServices(ctx, state.ClusterID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
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
			RegionName:    types.StringValue(service.GetRegionName()),
			CloudProvider: types.StringValue(string(service.GetCloudProvider())),
			Status:        types.StringValue(string(service.GetStatus())),
			Aws: PrivateLinkServiceAWSDetail{
				ServiceName: types.StringValue(service.Aws.GetServiceName()),
				ServiceId:   types.StringValue(service.Aws.GetServiceId()),
			},
		}
		apiAZs := service.Aws.GetAvailabilityZoneIds()
		azs := make([]types.String, len(apiAZs))
		for j, az := range apiAZs {
			azs[j] = types.StringValue(az)
		}
		state.Services[i].Aws.AvailabilityZoneIds = azs
	}
}

func (r *privateEndpointServicesResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// no-op - Endpoint services cannot be updated

	// The only non-computed field is the cluster ID, which requires replace,
	// so no extra warning is necessary here.
}

func (r *privateEndpointServicesResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	// no-op - Endpoint services cannot be deleted
	resp.Diagnostics.AddWarning("Cannot remove endpoint services",
		"Endpoint Services resources can't be deleted once established."+
			"This operation will remove the Terraform resource, but not the underlying services.",
	)
}

func (r *privateEndpointServicesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("cluster_id"), req, resp)
}

func waitForEndpointServicesCreatedFunc(ctx context.Context, clusterID string, cl client.Service, services *client.PrivateEndpointServices) sdk_resource.RetryFunc {
	return func() *sdk_resource.RetryError {
		apiServices, httpResp, err := cl.ListPrivateEndpointServices(ctx, clusterID)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return sdk_resource.NonRetryableError(fmt.Errorf("error getting endpoint services: %s", formatAPIErrorMessage(err)))
			} else {
				return sdk_resource.RetryableError(fmt.Errorf("encountered a server error while reading endpoint status - trying again"))
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
				return sdk_resource.NonRetryableError(fmt.Errorf("endpoint service creation failed"))
			}
		}
		if creating {
			return sdk_resource.RetryableError(fmt.Errorf("endpoint services are not ready yet"))
		}
		return nil
	}
}

func NewPrivateEndpointServicesResource() resource.Resource {
	return &privateEndpointServicesResource{}
}
