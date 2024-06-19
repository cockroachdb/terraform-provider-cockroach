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
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type privateEndpointServicesResource struct {
	provider *provider
}

const endpointServicesCreateTimeout = time.Hour

var endpointServicesSchema = schema.Schema{
	MarkdownDescription: "PrivateEndpointServices contains services that allow for private connectivity to the CockroachDB Cloud cluster.",
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
		"services_map": schema.MapNestedAttribute{
			Computed:    true,
			Description: "a map of services keyed by the region name",
			PlanModifiers: []planmodifier.Map{
				mapplanmodifier.UseStateForUnknown(),
			},
			NestedObject: serviceObject,
		},
		"services": schema.ListNestedAttribute{
			Computed: true,
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
			NestedObject: serviceObject,
		},
	},
}

var serviceObject = schema.NestedAttributeObject{
	Attributes: map[string]schema.Attribute{
		"region_name": schema.StringAttribute{
			Computed:    true,
			Description: "Cloud provider region code associated with this service.",
		},
		"cloud_provider": schema.StringAttribute{
			Computed:    true,
			Description: "Cloud provider associated with this service.",
		},
		"status": schema.StringAttribute{
			Computed:    true,
			Description: "Operation status of the service.",
		},
		"name": schema.StringAttribute{
			Computed:    true,
			Description: "Name of the endpoint service.",
		},
		"endpoint_service_id": schema.StringAttribute{
			Computed:    true,
			Description: "Server side ID of the private endpoint connection.",
		},
		"availability_zone_ids": schema.ListAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Availability Zone IDs of the private endpoint service. It is recommended, for cost optimization purposes, to create the private endpoint spanning these same availability zones. For more information, see data transfer cost information for your cloud provider.",
		},
		"aws": schema.SingleNestedAttribute{
			DeprecationMessage: "nested aws fields have been moved one level up. These fields will be removed in a future version",
			Computed:           true,
			PlanModifiers: []planmodifier.Object{
				objectplanmodifier.UseStateForUnknown(),
			},
			Attributes: map[string]schema.Attribute{
				"service_name": schema.StringAttribute{
					Computed:    true,
					Description: "AWS service name used to create endpoints.",
				},
				"service_id": schema.StringAttribute{
					Computed:    true,
					Description: "Server side ID of the PrivateLink connection.",
				},
				"availability_zone_ids": schema.ListAttribute{
					Computed:            true,
					ElementType:         types.StringType,
					MarkdownDescription: "AZ IDs users should create their VPCs in to minimize their cost.",
				},
			},
		},
	},
}

func (r *privateEndpointServicesResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = endpointServicesSchema
}

func (r *privateEndpointServicesResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_private_endpoint_services"
}

func (r *privateEndpointServicesResource) Configure(
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

func (r *privateEndpointServicesResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan PrivateEndpointServices
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

	// Only create an endpoint service if it is a dedicated cluster. Serverless
	// clusters have endpoint services by default, and will error out if the
	// Create API is called.
	if cluster.Config.Serverless == nil {
		// If private endpoint services already exist for this dedicated cluster,
		// this is a no-op. The API will gracefully return the existing services.
		traceAPICall("CreatePrivateEndpointServices")
		_, _, err = r.provider.service.CreatePrivateEndpointServices(ctx, plan.ClusterID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Error enabling private endpoint services",
				fmt.Sprintf("Could not enable private endpoint services: %s", formatAPIErrorMessage(err)),
			)
			return
		}
	}
	var services client.PrivateEndpointServices
	err = retry.RetryContext(ctx, endpointServicesCreateTimeout,
		waitForEndpointServicesCreatedFunc(ctx, cluster.Id, r.provider.service, &services))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling private endpoint services",
			fmt.Sprintf("Could not enable private endpoint services: %s", formatAPIErrorMessage(err)),
		)
		return
	}
	var state PrivateEndpointServices
	state.ClusterID = plan.ClusterID
	state.ID = plan.ClusterID
	diags = loadEndpointServicesIntoTerraformState(ctx, &services, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *privateEndpointServicesResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
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
	traceAPICall("ListPrivateEndpointServices")
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
	loadEndpointServicesIntoTerraformState(ctx, apiResp, &state)
	state.ID = state.ClusterID
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func loadEndpointServicesIntoTerraformState(
	ctx context.Context, apiServices *client.PrivateEndpointServices, state *PrivateEndpointServices,
) diag.Diagnostics {
	serviceList := apiServices.GetServices()
	services := make([]PrivateEndpointService, len(serviceList))
	for i, service := range serviceList {
		services[i] = PrivateEndpointService{
			RegionName:        types.StringValue(service.GetRegionName()),
			CloudProvider:     types.StringValue(string(service.GetCloudProvider())),
			Status:            types.StringValue(string(service.GetStatus())),
			Name:              types.StringValue(service.GetName()),
			EndpointServiceId: types.StringValue(service.GetEndpointServiceId()),
		}

		traceAPICall("GetAvailabilityZoneIds")
		apiAZs := service.GetAvailabilityZoneIds()
		azs := make([]types.String, len(apiAZs))
		for j, az := range apiAZs {
			azs[j] = types.StringValue(az)
		}
		services[i].AvailabilityZoneIds = azs

		// Set the .AWS field for backward compatibility.
		if services[i].CloudProvider.ValueString() == "AWS" {
			services[i].Aws = PrivateLinkServiceAWSDetail{
				ServiceName:         types.StringValue(service.GetName()),
				ServiceId:           types.StringValue(service.GetEndpointServiceId()),
				AvailabilityZoneIds: azs,
			}
		}
	}
	var diags diag.Diagnostics
	state.Services, diags = types.ListValueFrom(
		ctx,
		// Yes, it really does apparently need to be this complicated.
		// https://github.com/hashicorp/terraform-plugin-framework/issues/713
		endpointServicesSchema.Attributes["services"].(schema.ListNestedAttribute).NestedObject.Type(),
		services,
	)

	servicesMap := map[string]PrivateEndpointService{}
	for _, svc := range services {
		servicesMap[svc.RegionName.ValueString()] = svc
	}

	var diags2 diag.Diagnostics
	state.ServicesMap, diags2 = types.MapValueFrom(
		ctx,
		endpointServicesSchema.Attributes["services_map"].(schema.MapNestedAttribute).NestedObject.Type(),
		servicesMap,
	)

	diags.Append(diags2...)

	return diags
}

func (r *privateEndpointServicesResource) Update(
	_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse,
) {
	// no-op - Endpoint services cannot be updated

	// The only non-computed field is the cluster ID, which requires replace,
	// so no extra warning is necessary here.
}

func (r *privateEndpointServicesResource) Delete(
	_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	// no-op - Endpoint services cannot be deleted
	resp.Diagnostics.AddWarning("Cannot remove endpoint services",
		"Endpoint Services resources can't be deleted once established."+
			"This operation will remove the Terraform resource, but not the underlying services.",
	)
}

func (r *privateEndpointServicesResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("cluster_id"), req, resp)
}

func waitForEndpointServicesCreatedFunc(
	ctx context.Context,
	clusterID string,
	cl client.Service,
	services *client.PrivateEndpointServices,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("ListPrivateEndpointServices")
		apiServices, httpResp, err := cl.ListPrivateEndpointServices(ctx, clusterID)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return retry.NonRetryableError(fmt.Errorf("error getting endpoint services: %s", formatAPIErrorMessage(err)))
			} else {
				return retry.RetryableError(fmt.Errorf("encountered a server error while reading endpoint status - trying again: %v", err))
			}
		}

		if len(apiServices.Services) == 0 {
			return retry.RetryableError(fmt.Errorf("private endpoint services not yet created"))
		}

		*services = *apiServices
		var creating bool
		// If there's at least one still creating, keep checking.
		// If any have failed or have any other non-available status, something went wrong.
		// If all the services have been created, we're done.
		for _, service := range services.GetServices() {
			if service.GetStatus() == client.PRIVATEENDPOINTSERVICESTATUSTYPE_CREATING {
				creating = true
			} else if service.GetStatus() != client.PRIVATEENDPOINTSERVICESTATUSTYPE_AVAILABLE {
				return retry.NonRetryableError(fmt.Errorf("endpoint service creation failed"))
			}
		}
		if creating {
			return retry.RetryableError(fmt.Errorf("endpoint services are not ready yet"))
		}
		return nil
	}
}

func NewPrivateEndpointServicesResource() resource.Resource {
	return &privateEndpointServicesResource{}
}
