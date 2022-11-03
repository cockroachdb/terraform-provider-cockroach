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
	"sort"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

type clusterResourceType struct{}

func (r clusterResourceType) GetSchema(_ context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		MarkdownDescription: "Cluster Resource",
		Attributes: map[string]tfsdk.Attribute{
			"id": {
				Type:     types.StringType,
				Computed: true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
			},
			"name": {
				MarkdownDescription: "Name of cluster",
				Type:                types.StringType,
				Required:            true,
			},
			"cockroach_version": {
				Type:     types.StringType,
				Computed: true,
				Optional: true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
			},
			"account_id": {
				Type:     types.StringType,
				Computed: true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
			},
			"plan": {
				Type:     types.StringType,
				Computed: true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
			},
			"cloud_provider": {
				Type:     types.StringType,
				Required: true,
			},
			"serverless": {
				Optional: true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
				Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
					"spend_limit": {
						Optional: true,
						Type:     types.Int64Type,
						PlanModifiers: tfsdk.AttributePlanModifiers{
							tfsdk.UseStateForUnknown(),
						},
					},
					"routing_id": {
						Computed: true,
						Type:     types.StringType,
						PlanModifiers: tfsdk.AttributePlanModifiers{
							tfsdk.UseStateForUnknown(),
						},
					},
				}),
			},
			"dedicated": {
				Optional: true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
				Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
					"storage_gib": {
						Type:     types.Int64Type,
						Optional: true,
						Computed: true,
						PlanModifiers: tfsdk.AttributePlanModifiers{
							tfsdk.UseStateForUnknown(),
						},
					},
					"disk_iops": {
						Type:     types.Int64Type,
						Optional: true,
						Computed: true,
					},
					"memory_gib": {
						Computed: true,
						Type:     types.Float64Type,
					},
					"machine_type": {
						Type:     types.StringType,
						Optional: true,
						Computed: true,
					},
					"num_virtual_cpus": {
						Optional: true,
						Computed: true,
						Type:     types.Int64Type,
					},
				}),
			},
			"regions": {
				Required: true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
				Attributes: tfsdk.ListNestedAttributes(map[string]tfsdk.Attribute{
					"name": {
						Required: true,
						Type:     types.StringType,
					},
					"sql_dns": {
						Computed: true,
						Type:     types.StringType,
					},
					"ui_dns": {
						Computed: true,
						Type:     types.StringType,
					},
					"node_count": {
						Optional: true,
						Computed: true,
						Type:     types.Int64Type,
						PlanModifiers: tfsdk.AttributePlanModifiers{
							tfsdk.UseStateForUnknown(),
						},
					},
				}, tfsdk.ListNestedAttributesOptions{}),
			},
			"state": {
				Type:     types.StringType,
				Computed: true,
			},
			"creator_id": {
				Type:     types.StringType,
				Computed: true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
			},
			"operation_status": {
				Type:     types.StringType,
				Computed: true,
			},
		},
	}, nil
}

func (r clusterResourceType) NewResource(_ context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return clusterResource{
		provider: provider,
	}, diags
}

type clusterResource struct {
	provider provider
}

func (r clusterResource) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	if !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan CockroachCluster

	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	clusterSpec := client.NewCreateClusterSpecification()

	if (plan.ServerlessConfig == nil && plan.DedicatedConfig == nil) ||
		(plan.ServerlessConfig != nil && plan.DedicatedConfig != nil) {
		resp.Diagnostics.AddError(
			"Invalid cluster configuration",
			"You must set either 'dedicated' or 'serverless', but not both.",
		)
	}

	if plan.ServerlessConfig != nil {
		var regions []string
		for _, region := range plan.Regions {
			regions = append(regions, region.Name.Value)
		}
		serverless := client.NewServerlessClusterCreateSpecification(regions, int32(plan.ServerlessConfig.SpendLimit.Value))
		clusterSpec.SetServerless(*serverless)
	} else if plan.DedicatedConfig != nil {
		dedicated := client.DedicatedClusterCreateSpecification{}
		if !plan.CockroachVersion.Null {
			dedicated.CockroachVersion = &plan.CockroachVersion.Value
		}
		if plan.Regions != nil {
			regionNodes := make(map[string]int32, len(plan.Regions))
			for _, region := range plan.Regions {
				regionNodes[region.Name.Value] = int32(region.NodeCount.Value)
			}
			dedicated.RegionNodes = regionNodes
		}
		if plan.DedicatedConfig != nil {
			hardware := client.DedicatedHardwareCreateSpecification{}
			machineSpec := client.DedicatedMachineTypeSpecification{}
			if !plan.DedicatedConfig.NumVirtualCpus.Null {
				cpus := int32(plan.DedicatedConfig.NumVirtualCpus.Value)
				machineSpec.NumVirtualCpus = &cpus
			} else if !plan.DedicatedConfig.MachineType.Null {
				machineType := plan.DedicatedConfig.MachineType.Value
				machineSpec.MachineType = &machineType
			} else {
				resp.Diagnostics.AddError(
					"Invalid dedicated cluster configuration",
					"A dedicated cluster needs either num_virtual_cpus or machine_type to be set.",
				)
			}
			hardware.MachineSpec = machineSpec
			if !plan.DedicatedConfig.StorageGib.Null {
				hardware.StorageGib = int32(plan.DedicatedConfig.StorageGib.Value)
			}
			if !plan.DedicatedConfig.DiskIops.Null {
				diskiops := int32(plan.DedicatedConfig.DiskIops.Value)
				hardware.DiskIops = &diskiops
			}
			dedicated.Hardware = hardware
		}
		clusterSpec.SetDedicated(dedicated)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	clusterReq := client.NewCreateClusterRequest(plan.Name.Value, client.ApiCloudProvider(plan.CloudProvider.Value), *clusterSpec)
	clusterObj, _, err := r.provider.service.CreateCluster(ctx, clusterReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating cluster",
			fmt.Sprintf("Could not create cluster, unexpected error: %v", err.Error()),
		)
		return
	}

	err = resource.RetryContext(ctx, CREATE_TIMEOUT,
		waitForClusterCreatedFunc(ctx, clusterObj.Id, r.provider.service, clusterObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"cluster ready timeout",
			fmt.Sprintf("cluster is not ready: %v", err.Error()),
		)
		return
	}

	var state CockroachCluster
	loadClusterToTerraformState(clusterObj, &state, &plan)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r clusterResource) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	if !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var cluster CockroachCluster
	diags := req.State.Get(ctx, &cluster)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if cluster.ID.Null {
		return
	}
	clusterID := cluster.ID.Value

	clusterObj, httpResp, err := r.provider.service.GetCluster(ctx, clusterID)
	if err != nil {
		if httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Cluster not found",
				fmt.Sprintf("Cluster with clusterID %s is not found. Deleting from state.", clusterID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting cluster info",
				fmt.Sprintf("Unexpected error retrieving cluster info: %v", err.Error()))
		}
		return
	}

	// We actually want to use the current state as the plan here,
	// since we're trying to see if it changed.
	loadClusterToTerraformState(clusterObj, &cluster, &cluster)
	diags = resp.State.Set(ctx, cluster)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

}

func (r clusterResource) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	// Get plan values
	var plan CockroachCluster
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state CockroachCluster
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Name != state.Name {
		resp.Diagnostics.AddError("Cannot update cluster name",
			"To prevent accidental deletion of data, renaming clusters isn't allowed. "+
				"Please explicitly destroy this cluster before changing its name.")
		return
	}
	if plan.CloudProvider != state.CloudProvider {
		resp.Diagnostics.AddError("Cannot update cluster cloud provider",
			"To prevent accidental deletion of data, changing a cluster's cloud provider "+
				"isn't allowed. Please explicitly destroy this cluster before changing its cloud provider.")
		return
	}

	clusterReq := client.NewUpdateClusterSpecification()
	if plan.ServerlessConfig != nil {
		serverless := client.NewServerlessClusterUpdateSpecification(int32(plan.ServerlessConfig.SpendLimit.Value))
		clusterReq.SetServerless(*serverless)
	} else if plan.DedicatedConfig != nil {
		dedicated := client.NewDedicatedClusterUpdateSpecification()
		if plan.Regions != nil {
			regionNodes := make(map[string]int32, len(plan.Regions))
			for _, region := range plan.Regions {
				regionNodes[region.Name.Value] = int32(region.NodeCount.Value)
			}
			dedicated.RegionNodes = &regionNodes
		}
		dedicated.Hardware = client.NewDedicatedHardwareUpdateSpecification()
		if !plan.DedicatedConfig.StorageGib.Null {
			storage := int32(plan.DedicatedConfig.StorageGib.Value)
			dedicated.Hardware.StorageGib = &storage
		}
		if !plan.DedicatedConfig.DiskIops.Null {
			diskiops := int32(plan.DedicatedConfig.DiskIops.Value)
			dedicated.Hardware.DiskIops = &diskiops
		}
		machineSpec := client.DedicatedMachineTypeSpecification{}
		if !plan.DedicatedConfig.MachineType.Null {
			machineSpec.MachineType = &plan.DedicatedConfig.MachineType.Value
		} else if !plan.DedicatedConfig.NumVirtualCpus.Null {
			cpus := int32(plan.DedicatedConfig.NumVirtualCpus.Value)
			machineSpec.NumVirtualCpus = &cpus
		}
		dedicated.Hardware.MachineSpec = &machineSpec

		clusterReq.SetDedicated(*dedicated)
	}

	clusterObj, apiResp, err := r.provider.service.UpdateCluster(ctx, state.ID.Value, clusterReq, &client.UpdateClusterOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating cluster %v"+plan.ID.Value+": "+err.Error(),
			fmt.Sprintf("Could not update clusterID %v", apiResp),
		)
		return
	}

	err = resource.RetryContext(ctx, CREATE_TIMEOUT,
		waitForClusterCreatedFunc(ctx, clusterObj.Id, r.provider.service, clusterObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"cluster ready timeout",
			fmt.Sprintf("cluster is not ready: %v %v "+err.Error()),
		)
		return
	}

	// Set state
	loadClusterToTerraformState(clusterObj, &state, &plan)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r clusterResource) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var state CockroachCluster
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddWarning("this is error loading cluster ", "")
		return
	}

	// Get cluster ID from state
	if state.ID.Null {
		return
	}
	clusterID := state.ID.Value

	// Delete order by calling API
	_, httpResp, err := r.provider.service.DeleteCluster(ctx, clusterID)
	if err != nil {
		if httpResp.StatusCode == http.StatusNotFound {
			// Cluster is already gone. Swallow the error.
		} else {
			resp.Diagnostics.AddError(
				"Error deleting order",
				"Could not delete clusterID "+clusterID+": "+err.Error(),
			)
		}
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r clusterResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("id"), req, resp)
}

// Since the API response will always sort regions by name, we need to
// resort the list, so it matches up with the plan. If the response and
// plan regions don't match up, the sort won't work right, but we can
// ignore it. Terraform will handle it.
func sortRegionsByPlan(clusterObj *client.Cluster, plan *CockroachCluster) {
	if clusterObj == nil || plan == nil {
		return
	}
	regionOrdinals := make(map[string]int, len(clusterObj.Regions))
	for i, region := range plan.Regions {
		regionOrdinals[region.Name.Value] = i
	}
	sort.Slice(clusterObj.Regions, func(i, j int) bool {
		return regionOrdinals[clusterObj.Regions[i].Name] < regionOrdinals[clusterObj.Regions[j].Name]
	})
}

func loadClusterToTerraformState(clusterObj *client.Cluster, state *CockroachCluster, plan *CockroachCluster) {
	sortRegionsByPlan(clusterObj, plan)
	var rgs []Region
	for _, x := range clusterObj.Regions {
		rg := Region{
			Name:      types.String{Value: x.Name},
			SqlDns:    types.String{Value: x.SqlDns},
			UiDns:     types.String{Value: x.UiDns},
			NodeCount: types.Int64{Value: int64(x.NodeCount)},
		}
		rgs = append(rgs, rg)
	}

	state.ID = types.String{Value: clusterObj.Id}
	state.Name = types.String{Value: clusterObj.Name}
	state.CloudProvider = types.String{Value: string(clusterObj.CloudProvider)}
	state.CockroachVersion = types.String{Value: clusterObj.CockroachVersion}
	state.Plan = types.String{Value: string(clusterObj.Plan)}
	if clusterObj.AccountId == nil {
		state.AccountId.Null = true
	} else {
		state.AccountId = types.String{Value: *clusterObj.AccountId}
	}
	state.State = types.String{Value: string(clusterObj.State)}
	state.CreatorId = types.String{Value: clusterObj.CreatorId}
	state.OperationStatus = types.String{Value: string(clusterObj.OperationStatus)}
	state.Regions = rgs

	if clusterObj.Config.Serverless != nil {
		state.ServerlessConfig = &ServerlessClusterConfig{
			SpendLimit: types.Int64{Value: int64(clusterObj.Config.Serverless.SpendLimit)},
			RoutingId:  types.String{Value: clusterObj.Config.Serverless.RoutingId},
		}
	} else if clusterObj.Config.Dedicated != nil {
		state.DedicatedConfig = &DedicatedClusterConfig{
			MachineType:    types.String{Value: clusterObj.Config.Dedicated.MachineType},
			NumVirtualCpus: types.Int64{Value: int64(clusterObj.Config.Dedicated.NumVirtualCpus)},
			StorageGib:     types.Int64{Value: int64(clusterObj.Config.Dedicated.StorageGib)},
			MemoryGib:      types.Float64{Value: float64(clusterObj.Config.Dedicated.MemoryGib)},
			DiskIops:       types.Int64{Value: int64(clusterObj.Config.Dedicated.DiskIops)},
		}
	}
}

func waitForClusterCreatedFunc(ctx context.Context, id string, cl client.Service, cluster *client.Cluster) resource.RetryFunc {
	return func() *resource.RetryError {
		var httpResp *http.Response
		var err error
		cluster, httpResp, err = cl.GetCluster(ctx, id)
		if err != nil {
			if httpResp.StatusCode < http.StatusInternalServerError {
				return resource.NonRetryableError(fmt.Errorf("error getting cluster %v", err))
			} else {
				return resource.RetryableError(fmt.Errorf("encountered a server error while reading cluster status - trying again"))
			}
		}
		if string(cluster.State) == CLUSTERSTATETYPE_CREATED {
			return nil
		}
		if string(cluster.State) == CLUSTERSTATETYPE_CREATION_FAILED {
			return resource.NonRetryableError(fmt.Errorf("cluster creation failed"))
		}
		return resource.RetryableError(fmt.Errorf("cluster is not ready yet"))
	}
}
