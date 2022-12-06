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
	"sort"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	sdk_resource "github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

const (
	clusterCreateTimeout = time.Hour
	clusterUpdateTimeout = time.Hour * 2
)

type clusterResource struct {
	provider *provider
}

func (r *clusterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Cluster Resource",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of cluster",
				Required:            true,
			},
			"cockroach_version": schema.StringAttribute{
				Computed: true,
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"account_id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"plan": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cloud_provider": schema.StringAttribute{
				Required: true,
			},
			"serverless": schema.SingleNestedAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"spend_limit": schema.Int64Attribute{
						Required: true,
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
						},
						MarkdownDescription: "Spend limit in US cents",
					},
					"routing_id": schema.StringAttribute{
						Computed: true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
				},
			},
			"dedicated": schema.SingleNestedAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"storage_gib": schema.Int64Attribute{
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
						},
					},
					"disk_iops": schema.Int64Attribute{
						Optional: true,
						Computed: true,
					},
					"memory_gib": schema.Float64Attribute{
						Computed: true,
					},
					"machine_type": schema.StringAttribute{
						Optional: true,
						Computed: true,
					},
					"num_virtual_cpus": schema.Int64Attribute{
						Optional: true,
						Computed: true,
					},
				},
			},
			"regions": schema.ListNestedAttribute{
				Required: true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required: true,
						},
						"sql_dns": schema.StringAttribute{
							Computed: true,
						},
						"ui_dns": schema.StringAttribute{
							Computed: true,
						},
						"node_count": schema.Int64Attribute{
							Optional: true,
							Computed: true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
						},
					},
				},
			},
			"state": schema.StringAttribute{
				Computed: true,
			},
			"creator_id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"operation_status": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (r *clusterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *clusterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if r.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError("Internal provider error",
			fmt.Sprintf("Error in Configure: expected %T but got %T", provider{}, req.ProviderData))
	}
}

func (r *clusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.provider == nil || !r.provider.configured {
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
			"You must set either 'dedicated' or 'serverless', but not both",
		)
	}

	if plan.ServerlessConfig != nil {
		var regions []string
		for _, region := range plan.Regions {
			regions = append(regions, region.Name.ValueString())
		}
		serverless := client.NewServerlessClusterCreateSpecification(regions, int32(plan.ServerlessConfig.SpendLimit.ValueInt64()))
		clusterSpec.SetServerless(*serverless)
	} else if plan.DedicatedConfig != nil {
		dedicated := client.DedicatedClusterCreateSpecification{}
		if !plan.CockroachVersion.IsNull() {
			version := plan.CockroachVersion.ValueString()
			dedicated.CockroachVersion = &version
		}
		if plan.Regions != nil {
			regionNodes := make(map[string]int32, len(plan.Regions))
			for _, region := range plan.Regions {
				regionNodes[region.Name.ValueString()] = int32(region.NodeCount.ValueInt64())
			}
			dedicated.RegionNodes = regionNodes
		}
		if plan.DedicatedConfig != nil {
			hardware := client.DedicatedHardwareCreateSpecification{}
			machineSpec := client.DedicatedMachineTypeSpecification{}
			if !plan.DedicatedConfig.NumVirtualCpus.IsNull() {
				cpus := int32(plan.DedicatedConfig.NumVirtualCpus.ValueInt64())
				machineSpec.NumVirtualCpus = &cpus
			} else if !plan.DedicatedConfig.MachineType.IsNull() {
				machineType := plan.DedicatedConfig.MachineType.ValueString()
				machineSpec.MachineType = &machineType
			} else {
				resp.Diagnostics.AddError(
					"Invalid dedicated cluster configuration",
					"A dedicated cluster needs either num_virtual_cpus or machine_type to be set",
				)
			}
			hardware.MachineSpec = machineSpec
			if !plan.DedicatedConfig.StorageGib.IsNull() {
				hardware.StorageGib = int32(plan.DedicatedConfig.StorageGib.ValueInt64())
			}
			if !plan.DedicatedConfig.DiskIops.IsNull() {
				diskiops := int32(plan.DedicatedConfig.DiskIops.ValueInt64())
				hardware.DiskIops = &diskiops
			}
			dedicated.Hardware = hardware
		}
		clusterSpec.SetDedicated(dedicated)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	clusterReq := client.NewCreateClusterRequest(plan.Name.ValueString(), client.ApiCloudProvider(plan.CloudProvider.ValueString()), *clusterSpec)
	clusterObj, _, err := r.provider.service.CreateCluster(ctx, clusterReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating cluster",
			fmt.Sprintf("Could not create cluster: %v", formatAPIErrorMessage(err)),
		)
		return
	}

	err = sdk_resource.RetryContext(ctx, clusterCreateTimeout,
		waitForClusterCreatedFunc(ctx, clusterObj.Id, r.provider.service, clusterObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"Cluster creation failed",
			fmt.Sprintf("Cluster is not ready: %s", formatAPIErrorMessage(err)),
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

func (r *clusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var cluster CockroachCluster
	diags := req.State.Get(ctx, &cluster)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if cluster.ID.IsNull() {
		return
	}
	clusterID := cluster.ID.ValueString()

	clusterObj, httpResp, err := r.provider.service.GetCluster(ctx, clusterID)
	if err != nil {
		if httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Cluster not found",
				fmt.Sprintf("Cluster with clusterID %s is not found. Removing from state.", clusterID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting cluster info",
				fmt.Sprintf("Unexpected error retrieving cluster info: %s", formatAPIErrorMessage(err)))
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

func (r *clusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
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
		serverless := client.NewServerlessClusterUpdateSpecification(int32(plan.ServerlessConfig.SpendLimit.ValueInt64()))
		clusterReq.SetServerless(*serverless)
	} else if plan.DedicatedConfig != nil {
		dedicated := client.NewDedicatedClusterUpdateSpecification()
		if plan.Regions != nil {
			regionNodes := make(map[string]int32, len(plan.Regions))
			for _, region := range plan.Regions {
				regionNodes[region.Name.ValueString()] = int32(region.NodeCount.ValueInt64())
			}
			dedicated.RegionNodes = &regionNodes
		}
		dedicated.Hardware = client.NewDedicatedHardwareUpdateSpecification()
		if !plan.DedicatedConfig.StorageGib.IsNull() {
			storage := int32(plan.DedicatedConfig.StorageGib.ValueInt64())
			dedicated.Hardware.StorageGib = &storage
		}
		if !plan.DedicatedConfig.DiskIops.IsNull() {
			diskiops := int32(plan.DedicatedConfig.DiskIops.ValueInt64())
			dedicated.Hardware.DiskIops = &diskiops
		}
		machineSpec := client.DedicatedMachineTypeSpecification{}
		if !plan.DedicatedConfig.MachineType.IsNull() {
			machineType := plan.DedicatedConfig.MachineType.ValueString()
			machineSpec.MachineType = &machineType
		} else if !plan.DedicatedConfig.NumVirtualCpus.IsNull() {
			cpus := int32(plan.DedicatedConfig.NumVirtualCpus.ValueInt64())
			machineSpec.NumVirtualCpus = &cpus
		}
		dedicated.Hardware.MachineSpec = &machineSpec

		clusterReq.SetDedicated(*dedicated)
	}

	clusterObj, _, err := r.provider.service.UpdateCluster(ctx, state.ID.ValueString(), clusterReq, &client.UpdateClusterOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating cluster",
			fmt.Sprintf("Could not update cluster: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	err = sdk_resource.RetryContext(ctx, clusterUpdateTimeout,
		waitForClusterCreatedFunc(ctx, clusterObj.Id, r.provider.service, clusterObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"Cluster update failed",
			fmt.Sprintf("Cluster is not ready: %v %v "+err.Error()),
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

func (r *clusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state CockroachCluster
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get cluster ID from state
	if state.ID.IsNull() {
		return
	}
	clusterID := state.ID.ValueString()

	_, httpResp, err := r.provider.service.DeleteCluster(ctx, clusterID)
	if err != nil {
		if httpResp.StatusCode == http.StatusNotFound {
			// Cluster is already gone. Swallow the error.
		} else {
			resp.Diagnostics.AddError(
				"Error deleting cluster",
				fmt.Sprintf("Could not delete cluster: %s", formatAPIErrorMessage(err)),
			)
		}
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r *clusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// versionRE is the regexp that is used to verify that a version string is
// of the form "vMAJOR.MINOR.PATCH[-PRERELEASE][+METADATA]". This
// conforms to https://semver.org/spec/v2.0.0.html
var versionRE = regexp.MustCompile(
	`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-.]+)?(\+[0-9A-Za-z-.]+|)?$`,
	// ^major           ^minor           ^patch         ^preRelease       ^metadata
)

func simplifyClusterVersion(version string) string {
	parts := versionRE.FindStringSubmatch(version)
	if parts == nil {
		return version
	}
	if parts[4] != "" {
		return "preview"
	}
	return fmt.Sprintf("v%s.%s", parts[1], parts[2])
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
		regionOrdinals[region.Name.ValueString()] = i
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
			Name:      types.StringValue(x.Name),
			SqlDns:    types.StringValue(x.SqlDns),
			UiDns:     types.StringValue(x.UiDns),
			NodeCount: types.Int64Value(int64(x.NodeCount)),
		}
		rgs = append(rgs, rg)
	}

	state.ID = types.StringValue(clusterObj.Id)
	state.Name = types.StringValue(clusterObj.Name)
	state.CloudProvider = types.StringValue(string(clusterObj.CloudProvider))
	state.CockroachVersion = types.StringValue(simplifyClusterVersion(clusterObj.CockroachVersion))
	state.Plan = types.StringValue(string(clusterObj.Plan))
	if clusterObj.AccountId == nil {
		state.AccountId = types.StringNull()
	} else {
		state.AccountId = types.StringValue(*clusterObj.AccountId)
	}
	state.State = types.StringValue(string(clusterObj.State))
	state.CreatorId = types.StringValue(clusterObj.CreatorId)
	state.OperationStatus = types.StringValue(string(clusterObj.OperationStatus))
	state.Regions = rgs

	if clusterObj.Config.Serverless != nil {
		state.ServerlessConfig = &ServerlessClusterConfig{
			SpendLimit: types.Int64Value(int64(clusterObj.Config.Serverless.SpendLimit)),
			RoutingId:  types.StringValue(clusterObj.Config.Serverless.RoutingId),
		}
	} else if clusterObj.Config.Dedicated != nil {
		state.DedicatedConfig = &DedicatedClusterConfig{
			MachineType:    types.StringValue(clusterObj.Config.Dedicated.MachineType),
			NumVirtualCpus: types.Int64Value(int64(clusterObj.Config.Dedicated.NumVirtualCpus)),
			StorageGib:     types.Int64Value(int64(clusterObj.Config.Dedicated.StorageGib)),
			MemoryGib:      types.Float64Value(float64(clusterObj.Config.Dedicated.MemoryGib)),
			DiskIops:       types.Int64Value(int64(clusterObj.Config.Dedicated.DiskIops)),
		}
	}
}

func waitForClusterCreatedFunc(ctx context.Context, id string, cl client.Service, cluster *client.Cluster) sdk_resource.RetryFunc {
	return func() *sdk_resource.RetryError {
		apiCluster, httpResp, err := cl.GetCluster(ctx, id)
		if err != nil {
			if httpResp.StatusCode < http.StatusInternalServerError {
				return sdk_resource.NonRetryableError(fmt.Errorf("error getting cluster: %s", formatAPIErrorMessage(err)))
			} else {
				return sdk_resource.RetryableError(fmt.Errorf("encountered a server error while reading cluster status - trying again"))
			}
		}
		*cluster = *apiCluster
		if cluster.State == client.CLUSTERSTATETYPE_CREATED {
			return nil
		}
		if cluster.State == client.CLUSTERSTATETYPE_CREATION_FAILED {
			return sdk_resource.NonRetryableError(fmt.Errorf("cluster creation failed"))
		}
		return sdk_resource.RetryableError(fmt.Errorf("cluster is not ready yet"))
	}
}

func NewClusterResource() resource.Resource {
	return &clusterResource{}
}
