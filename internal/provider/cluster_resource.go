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
			"create_spec": {
				Optional: true,
				Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
					"serverless": {
						Optional: true,
						Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
							"spend_limit": {
								Optional: true,
								Type:     types.Int64Type,
							},
							"regions": {
								Type:     types.ListType{ElemType: types.StringType},
								Optional: true,
							},
						}),
					},
					"dedicated": {
						Optional: true,
						Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
							"region_nodes": {
								Type: types.MapType{
									ElemType: types.Int64Type,
								},
								Optional: true,
							},
							"cockroach_version": {
								Type:     types.StringType,
								Optional: true,
							},
							"hardware": {
								Optional: true,
								Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
									"storage_gib": {
										Type:     types.Int64Type,
										Optional: true,
									},
									"disk_iops": {
										Type:     types.Int64Type,
										Optional: true,
									},
									"machine_spec": {
										Optional: true,
										Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
											"machine_type": {
												Type:     types.StringType,
												Optional: true,
											},
											"num_virtual_cpus": {
												Type:     types.Int64Type,
												Optional: true,
											},
										}),
									},
								}),
							},
						}),
					},
				}),
			},
			"update_spec": {
				Optional: true,
				Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
					"serverless": {
						Optional: true,
						Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
							"spend_limit": {
								Optional: true,
								Type:     types.Int64Type,
							},
						}),
					},
					"dedicated": {
						Optional: true,
						Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
							"region_nodes": {
								Type:     types.MapType{ElemType: types.Int64Type},
								Optional: true,
							},
							"hardware": {
								Optional: true,
								Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
									"storage_gib": {
										Type:     types.Int64Type,
										Optional: true,
									},
									"disk_iops": {
										Type:     types.Int64Type,
										Optional: true,
									},
									"machine_spec": {
										Optional: true,
										Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
											"machine_type": {
												Type:     types.StringType,
												Optional: true,
											},
											"num_virtual_cpus": {
												Type:     types.Int64Type,
												Optional: true,
											},
										}),
									},
								}),
							},
						}),
					},
				}),
			},
			"config": {
				Computed: true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
				Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
					"serverless": {
						Computed: true,
						PlanModifiers: tfsdk.AttributePlanModifiers{
							tfsdk.UseStateForUnknown(),
						},
						Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
							"spend_limit": {
								Computed: true,
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
						Computed: true,
						PlanModifiers: tfsdk.AttributePlanModifiers{
							tfsdk.UseStateForUnknown(),
						},
						Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
							"machine_type": {
								Computed: true,
								Type:     types.StringType,
							},
							"num_virtual_cpus": {
								Computed: true,
								Type:     types.Int64Type,
							},
							"storage_gib": {
								Computed: true,
								Type:     types.Int64Type,
							},
							"memory_gib": {
								Computed: true,
								Type:     types.Float64Type,
							},
							"disk_iops": {
								Computed: true,
								Type:     types.Int64Type,
							},
						}),
					},
				}),
			},
			"regions": {
				Computed: true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
				Attributes: tfsdk.ListNestedAttributes(map[string]tfsdk.Attribute{
					"name": {
						Computed: true,
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
						Computed: true,
						Type:     types.Int64Type,
					},
				}, tfsdk.ListNestedAttributesOptions{}),
			},
			"state": {
				Type:     types.StringType,
				Computed: true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
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
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
				},
			},
			"wait_for_cluster_ready": {
				Type:     types.BoolType,
				Optional: true,
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
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider hasn't been configured before apply, likely because it depends on an unknown value from another resource. This leads to weird stuff happening, so we'd prefer if you didn't do that. Thanks!",
		)
		return
	}

	var plan CockroachCluster

	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	clusterSpec := client.NewCreateClusterSpecification()
	if plan.CreateSpec == nil {
		resp.Diagnostics.AddError(
			"Error creating cluster",
			"Could not create cluster as create_spec can not be nil",
		)
		return
	}

	if plan.CreateSpec.Serverless != nil {
		var regions []string
		for _, region := range plan.CreateSpec.Serverless.Regions {
			regions = append(regions, region.Value)
		}
		serverless := client.NewServerlessClusterCreateSpecification(regions, int32(plan.CreateSpec.Serverless.SpendLimit.Value))
		clusterSpec.SetServerless(*serverless)
	} else if plan.CreateSpec.Dedicated != nil {
		dedicated := client.DedicatedClusterCreateSpecification{}
		if plan.CreateSpec.Dedicated.RegionNodes != nil {
			regionNodes := plan.CreateSpec.Dedicated.RegionNodes
			dedicated.RegionNodes = *regionNodes
		}
		if plan.CreateSpec.Dedicated.Hardware != nil {
			hardware := client.DedicatedHardwareCreateSpecification{}
			if plan.CreateSpec.Dedicated.Hardware.MachineSpec != nil {
				machineSpec := client.DedicatedMachineTypeSpecification{}
				if !plan.CreateSpec.Dedicated.Hardware.MachineSpec.NumVirtualCpus.Null {
					cpus := int32(plan.CreateSpec.Dedicated.Hardware.MachineSpec.NumVirtualCpus.Value)
					machineSpec.NumVirtualCpus = &cpus
				}
				if !plan.CreateSpec.Dedicated.Hardware.MachineSpec.MachineType.Null {
					machineType := plan.CreateSpec.Dedicated.Hardware.MachineSpec.MachineType.Value
					machineSpec.MachineType = &machineType
				}
				hardware.MachineSpec = machineSpec
				if !plan.CreateSpec.Dedicated.Hardware.StorageGib.Null {
					hardware.StorageGib = int32(plan.CreateSpec.Dedicated.Hardware.StorageGib.Value)
				}
				if !plan.CreateSpec.Dedicated.Hardware.DiskIops.Null {
					diskiops := int32(plan.CreateSpec.Dedicated.Hardware.DiskIops.Value)
					hardware.DiskIops = &diskiops
				}
			}
			dedicated.Hardware = hardware
		}
		clusterSpec.SetDedicated(dedicated)
	}
	clusterReq := client.NewCreateClusterRequest(plan.Name.Value, client.ApiCloudProvider(plan.CloudProvider), *clusterSpec)
	clusterObj, clResp, err := r.provider.service.CreateCluster(ctx, clusterReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating cluster",
			fmt.Sprintf("Could not create cluster, unexpected error: %v %v "+err.Error(), clResp),
		)
		return
	}

	if !plan.WaitForClusterReady.Null && plan.WaitForClusterReady.Value {
		err := resource.RetryContext(ctx, CREATE_TIMEOUT,
			waitForClusterCreatedFunc(ctx, clusterObj.Id, r.provider.service))
		if err != nil {
			resp.Diagnostics.AddError(
				"cluster ready timeout",
				fmt.Sprintf("cluster is not ready: %v %v "+err.Error(), clResp),
			)
			return
		}
	}

	clusterObj, clResp, err = r.provider.service.GetCluster(ctx, clusterObj.Id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting cluster",
			fmt.Sprintf("Could not getting cluster, unexpected error: %v %v "+err.Error(), clResp),
		)
		return
	}

	loadClusterToTerraformState(clusterObj, &plan)
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r clusterResource) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	if !r.provider.configured {
		resp.Diagnostics.AddError(
			"provider not configured",
			"The provider hasn't been configured before apply, likely because it depends on an unknown value from another resource. This leads to weird stuff happening, so we'd prefer if you didn't do that. Thanks!",
		)
		return
	}

	var cluster CockroachCluster
	diags := req.State.Get(ctx, &cluster)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := cluster.ID.Value

	clusterObj, httpResp, err := r.provider.service.GetCluster(ctx, clusterID)
	if httpResp.StatusCode == http.StatusNotFound {
		resp.Diagnostics.AddError(
			"cluster not found",
			fmt.Sprintf("cluster with clusterID %s is not found", clusterID))
		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"error in getting cluster",
			"")
	}

	loadClusterToTerraformState(clusterObj, &cluster)
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

	if plan.UpdateSpec != nil {
		clusterReq := client.NewUpdateClusterSpecification()
		if plan.UpdateSpec.Serverless != nil {
			serverless := client.NewServerlessClusterUpdateSpecification(int32(plan.UpdateSpec.Serverless.SpendLimit.Value))
			clusterReq.SetServerless(*serverless)
		} else if plan.UpdateSpec.Dedicated != nil {
			dedicated := client.NewDedicatedClusterUpdateSpecification()
			if plan.UpdateSpec.Dedicated.RegionNodes != nil {
				dedicated.RegionNodes = plan.UpdateSpec.Dedicated.RegionNodes
			}
			if plan.UpdateSpec.Dedicated.Hardware != nil {
				dedicated.Hardware = client.NewDedicatedHardwareUpdateSpecification()
				if !plan.UpdateSpec.Dedicated.Hardware.StorageGib.Null {
					storage := int32(plan.UpdateSpec.Dedicated.Hardware.StorageGib.Value)
					dedicated.Hardware.StorageGib = &storage
				}
				if !plan.UpdateSpec.Dedicated.Hardware.DiskIops.Null {
					diskiops := int32(plan.UpdateSpec.Dedicated.Hardware.DiskIops.Value)
					dedicated.Hardware.DiskIops = &diskiops
				}
				if plan.UpdateSpec.Dedicated.Hardware.MachineSpec != nil {
					machineSpec := client.DedicatedMachineTypeSpecification{}
					if !plan.UpdateSpec.Dedicated.Hardware.MachineSpec.MachineType.Null {
						machineSpec.MachineType = &plan.UpdateSpec.Dedicated.Hardware.MachineSpec.MachineType.Value
					}
					if !plan.UpdateSpec.Dedicated.Hardware.MachineSpec.NumVirtualCpus.Null {
						cpus := int32(plan.UpdateSpec.Dedicated.Hardware.MachineSpec.NumVirtualCpus.Value)
						machineSpec.NumVirtualCpus = &cpus
					}
					dedicated.Hardware.MachineSpec = &machineSpec
				}
			}

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

		if !plan.WaitForClusterReady.Null && plan.WaitForClusterReady.Value {
			err := resource.RetryContext(ctx, CREATE_TIMEOUT,
				waitForClusterCreatedFunc(ctx, clusterObj.Id, r.provider.service))
			if err != nil {
				resp.Diagnostics.AddError(
					"cluster ready timeout",
					fmt.Sprintf("cluster is not ready: %v %v "+err.Error()),
				)
				return
			}
		}

		_, apiResp, err = r.provider.service.GetCluster(ctx, clusterObj.Id)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating cluster %v"+plan.ID.Value+": "+err.Error(),
				fmt.Sprintf("Could not update clusterID %v", apiResp),
			)
			return
		}

		// Set state
		diags = resp.State.Set(ctx, plan)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
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
	clusterID := state.ID.Value

	// Delete order by calling API
	_, _, err := r.provider.service.DeleteCluster(ctx, clusterID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting order",
			"Could not delete clusterID "+clusterID+": "+err.Error(),
		)
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r clusterResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("id"), req, resp)
}

func loadClusterToTerraformState(clusterObj *client.Cluster, state *CockroachCluster) {
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
	state.CockroachVersion = types.String{Value: clusterObj.CockroachVersion}
	state.Plan = types.String{Value: string(clusterObj.Plan)}
	state.AccountId = types.String{Value: *clusterObj.AccountId}
	state.State = types.String{Value: string(clusterObj.State)}
	state.CreatorId = types.String{Value: clusterObj.CreatorId}
	state.OperationStatus = types.String{Value: string(clusterObj.OperationStatus)}
	state.Regions = rgs
	state.Config = &ClusterConfig{}

	if clusterObj.Config.Serverless != nil {
		state.Config.Serverless = &ServerlessClusterConfig{
			SpendLimit: types.Int64{Value: int64(clusterObj.Config.Serverless.SpendLimit)},
			RoutingId:  types.String{Value: clusterObj.Config.Serverless.RoutingId},
		}
	} else if clusterObj.Config.Dedicated != nil {
		state.Config.Dedicated = &DedicatedHardwareConfig{
			MachineType:    types.String{Value: clusterObj.Config.Dedicated.MachineType},
			NumVirtualCpus: types.Int64{Value: int64(clusterObj.Config.Dedicated.NumVirtualCpus)},
			StorageGib:     types.Int64{Value: int64(clusterObj.Config.Dedicated.StorageGib)},
			MemoryGib:      types.Float64{Value: float64(clusterObj.Config.Dedicated.MemoryGib)},
			DiskIops:       types.Int64{Value: int64(clusterObj.Config.Dedicated.DiskIops)},
		}
	}
}

func waitForClusterCreatedFunc(ctx context.Context, id string, cl client.Service) resource.RetryFunc {
	return func() *resource.RetryError {
		clusterObj, httpResp, err := cl.GetCluster(ctx, id)
		if err != nil {
			resource.NonRetryableError(fmt.Errorf("error getting cluster %v %v", err, httpResp))
		}
		if string(clusterObj.State) == CLUSTERSTATETYPE_CREATED {
			return nil
		}
		if string(clusterObj.State) == CLUSTERSTATETYPE_CREATION_FAILED {
			resource.NonRetryableError(fmt.Errorf("cluster creation failed"))
		}
		return resource.RetryableError(fmt.Errorf("cluster is not ready yet"))
	}
}
