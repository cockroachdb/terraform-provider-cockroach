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
	"strings"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	sdk_resource "github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

const (
	clusterCreateTimeout = time.Hour
	clusterUpdateTimeout = time.Hour * 2

	clusterVersionPreview = "preview"
)

type clusterResource struct {
	provider *provider
}

var regionSchema = schema.NestedAttributeObject{
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
		"primary": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.UseStateForUnknown(),
			},
		},
	},
}

func (r *clusterResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
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
					"private_network_visibility": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Description: "Set to true to assign private IP addresses to nodes. Required for CMEK and other advanced networking features.",
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
						},
					},
				},
			},
			"regions": schema.ListNestedAttribute{
				Required: true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				NestedObject: regionSchema,
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
			"upgrade_status": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (r *clusterResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *clusterResource) Configure(
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

func (r *clusterResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.Conflicting(
			path.MatchRoot("dedicated"),
			path.MatchRoot("serverless"),
		),
		resourcevalidator.Conflicting(
			path.MatchRoot("dedicated").AtName("num_virtual_cpus"),
			path.MatchRoot("dedicated").AtName("machine_type"),
		),
	}
}

func (r *clusterResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
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

	if plan.ServerlessConfig != nil {
		var regions []string
		var primaryRegion string
		for _, region := range plan.Regions {
			if region.Primary.ValueBool() {
				primaryRegion = region.Name.ValueString()
			}
			regions = append(regions, region.Name.ValueString())
		}
		if len(regions) > 1 && primaryRegion == "" {
			resp.Diagnostics.AddError("Primary region missing",
				"One region must be marked primary when creating a multi-region Serverless cluster.")
		}
		serverless := client.NewServerlessClusterCreateSpecification(regions)
		if primaryRegion != "" {
			serverless.PrimaryRegion = &primaryRegion
		}
		spendLimit := int32(plan.ServerlessConfig.SpendLimit.ValueInt64())
		serverless.SpendLimit = &spendLimit

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
		if cfg := plan.DedicatedConfig; cfg != nil {
			hardware := client.DedicatedHardwareCreateSpecification{}
			machineSpec := client.DedicatedMachineTypeSpecification{}
			if !cfg.NumVirtualCpus.IsNull() {
				cpus := int32(cfg.NumVirtualCpus.ValueInt64())
				machineSpec.NumVirtualCpus = &cpus
			} else if !cfg.MachineType.IsNull() {
				machineType := cfg.MachineType.ValueString()
				machineSpec.MachineType = &machineType
			}
			hardware.MachineSpec = machineSpec
			if !cfg.StorageGib.IsNull() {
				hardware.StorageGib = int32(cfg.StorageGib.ValueInt64())
			}
			if !cfg.DiskIops.IsNull() {
				diskiops := int32(cfg.DiskIops.ValueInt64())
				hardware.DiskIops = &diskiops
			}
			dedicated.Hardware = hardware
			if cfg.PrivateNetworkVisibility.ValueBool() {
				visibilityPrivate := client.NETWORKVISIBILITYTYPE_PRIVATE
				dedicated.NetworkVisibility = &visibilityPrivate
			}
		}
		clusterSpec.SetDedicated(dedicated)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	clusterReq := client.NewCreateClusterRequest(plan.Name.ValueString(), client.CloudProviderType(plan.CloudProvider.ValueString()), *clusterSpec)
	clusterObj, _, err := r.provider.service.CreateCluster(ctx, clusterReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating cluster",
			fmt.Sprintf("Could not create cluster: %v", formatAPIErrorMessage(err)),
		)
		return
	}

	err = sdk_resource.RetryContext(ctx, clusterCreateTimeout,
		waitForClusterReadyFunc(ctx, clusterObj.Id, r.provider.service, clusterObj))
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

func (r *clusterResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
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
	// In case this was an import, validate the ID format.
	if !uuidRegex.MatchString(clusterID) {
		resp.Diagnostics.AddError(
			"Unexpected cluster ID format",
			fmt.Sprintf("'%s' is not a valid cluster ID format. Expected UUID.", clusterID),
		)
		return
	}

	clusterObj, httpResp, err := r.provider.service.GetCluster(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Cluster not found",
				fmt.Sprintf("Cluster with ID %s is not found. Removing from state.", clusterID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting cluster info",
				fmt.Sprintf("Unexpected error retrieving cluster info: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	if clusterObj.State == client.CLUSTERSTATETYPE_DELETED {
		resp.Diagnostics.AddWarning(
			"Cluster has been deleted",
			fmt.Sprintf("Cluster with ID %s has been externally deleted. Removing from state.", clusterID))
		resp.State.RemoveResource(ctx)
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

// ModifyPlan is used to make sure the user isn't trying to modify an
// immutable attribute. We can't use `RequiresReplace` because that
// would destroy the cluster and all of its data.
func (r *clusterResource) ModifyPlan(
	ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse,
) {
	var state *CockroachCluster
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || state == nil {
		return
	}

	var plan *CockroachCluster
	diags = req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || plan == nil {
		return
	}

	if plan.Name != state.Name {
		resp.Diagnostics.AddError("Cannot update cluster name",
			"To prevent accidental deletion of data, renaming clusters isn't allowed. "+
				"Please explicitly destroy this cluster before changing its name.")
	}
	if plan.CloudProvider != state.CloudProvider {
		resp.Diagnostics.AddError("Cannot update cluster cloud provider",
			"To prevent accidental deletion of data, changing a cluster's cloud provider "+
				"isn't allowed. Please explicitly destroy this cluster before changing its cloud provider.")
	}
	if ((plan.DedicatedConfig == nil) != (state.DedicatedConfig == nil)) ||
		((plan.ServerlessConfig == nil) != (state.ServerlessConfig == nil)) {
		resp.Diagnostics.AddError("Cannot update cluster plan type",
			"To prevent accidental deletion of data, changing a cluster's plan type "+
				"isn't allowed. Please explicitly destroy this cluster before changing between "+
				"dedicated and serverless plans.")
		return
	}
	if dedicated := plan.DedicatedConfig; dedicated != nil && dedicated.PrivateNetworkVisibility != state.DedicatedConfig.PrivateNetworkVisibility {
		resp.Diagnostics.AddError("Cannot update network visibility",
			"To prevent accidental deletion of data, changing a cluster's network "+
				"visibility isn't allowed. Please explicitly destroy this cluster before changing "+
				"network visibility.")
	}
}

// Comparator for two CRDB versions to make validation simpler. Assumes biannual releases of the form vYY.H.
// Result is the number of releases apart, negative if r is later and positive is l is later. For example,
// compareCrdbVersions("v22.1", "v22.2") = -1, compareCrdbVersions("v22.1", "v19.2") = 3
func compareCrdbVersions(l, r string, d *diag.Diagnostics) int {
	var lMaj, lMin, rMaj, rMin int
	if _, err := fmt.Sscanf(l, "v%d.%d", &lMaj, &lMin); err != nil {
		d.AddError("Couldn't parse version number", fmt.Sprintf("Couldn't parse version '%s'.", l))
		return 0
	}
	if _, err := fmt.Sscanf(r, "v%d.%d", &rMaj, &rMin); err != nil {
		d.AddError("Couldn't parse version number", fmt.Sprintf("Couldn't parse version '%s'.", r))
		return 0
	}

	return (rMaj*2 + rMin - 1) - (lMaj*2 + lMin - 1)
}

func (r *clusterResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
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

	waitForClusterLock(ctx, state, r.provider.service, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// CRDB Versions
	if !plan.CockroachVersion.IsNull() && plan.CockroachVersion != state.CockroachVersion {
		// Validate that the target version is valid.
		planVersion := plan.CockroachVersion.ValueString()
		stateVersion := state.CockroachVersion.ValueString()
		apiResp, _, err := r.provider.service.ListMajorClusterVersions(ctx, &client.ListMajorClusterVersionsOptions{})
		if err != nil {
			resp.Diagnostics.AddError("Couldn't retrieve CockroachDB version list", formatAPIErrorMessage(err))
			return
		}
		var versionValid bool
		for _, v := range apiResp.Versions {
			if v.Version == planVersion {
				versionValid = true
				continue
			}
		}
		if !versionValid {
			validVersions := make([]string, len(apiResp.Versions))
			for i, v := range apiResp.Versions {
				validVersions[i] = v.Version
			}
			resp.Diagnostics.AddError("Invalid CockroachDB version",
				fmt.Sprintf(
					"'%s' is not a valid major CockroachDB version. Valid versions include [%s]",
					planVersion,
					strings.Join(validVersions, "|")))
			return
		}

		upgradeStatus := client.CLUSTERUPGRADESTATUSTYPE_MAJOR_UPGRADE_RUNNING
		cmp := compareCrdbVersions(stateVersion, planVersion, &resp.Diagnostics)
		if cmp < 0 {
			// Make sure we're rolling back to the previous version.
			if cmp < -1 {
				resp.Diagnostics.AddError("Invalid rollback version", "Can only roll back to the previous version.")
				return
			}
			upgradeStatus = client.CLUSTERUPGRADESTATUSTYPE_ROLLBACK_RUNNING
		} else if cmp > 1 {
			resp.Diagnostics.AddError("Invalid upgrade version", "Can't skip versions. Upgrades must be performed one version at a time.")
			return
		}

		clusterObj, _, err := r.provider.service.UpdateCluster(ctx, plan.ID.ValueString(), &client.UpdateClusterSpecification{
			UpgradeStatus: &upgradeStatus,
		})
		if err != nil {
			resp.Diagnostics.AddError("Error updating cluster", formatAPIErrorMessage(err))
			return
		}

		err = sdk_resource.RetryContext(ctx, clusterUpdateTimeout,
			waitForClusterReadyFunc(ctx, clusterObj.Id, r.provider.service, clusterObj))
		if err != nil {
			resp.Diagnostics.AddError(
				"Cluster version update failed",
				formatAPIErrorMessage(err),
			)
			return
		}
	}

	clusterReq := client.NewUpdateClusterSpecification()
	if plan.ServerlessConfig != nil {
		serverless := client.NewServerlessClusterUpdateSpecification()
		spendLimit := int32(plan.ServerlessConfig.SpendLimit.ValueInt64())
		serverless.SpendLimit = &spendLimit
		clusterReq.SetServerless(*serverless)
	} else if cfg := plan.DedicatedConfig; cfg != nil {
		dedicated := client.NewDedicatedClusterUpdateSpecification()
		if plan.Regions != nil {
			dedicated.RegionNodes, diags = reconcileRegionUpdate(ctx, state.Regions, plan.Regions, state.ID.ValueString(), r.provider.service) //&regionNodes
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}
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

	clusterObj, _, err := r.provider.service.UpdateCluster(ctx, state.ID.ValueString(), clusterReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating cluster",
			fmt.Sprintf("Could not update cluster: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	err = sdk_resource.RetryContext(ctx, clusterUpdateTimeout,
		waitForClusterReadyFunc(ctx, clusterObj.Id, r.provider.service, clusterObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"Cluster update failed",
			fmt.Sprintf("Cluster is not ready: %v", formatAPIErrorMessage(err)),
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

func (r *clusterResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state CockroachCluster
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	waitForClusterLock(ctx, state, r.provider.service, &resp.Diagnostics)
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
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
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

func (r *clusterResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// versionRE is the regexp that is used to verify that a version string is
// of the form "vMAJOR.MINOR.PATCH[-PRERELEASE][+METADATA]". This
// conforms to https://semver.org/spec/v2.0.0.html
var versionRE = regexp.MustCompile(
	`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-.]+)?(\+[0-9A-Za-z-.]+|)?$`,
	// ^major           ^minor           ^patch         ^preRelease       ^metadata
)

// simplifyClusterVersion returns the cluster's major version. If planSpecifiesPreviewString
// is true and the full cluster version has a preview component, returns the magic string
// "preview".
func simplifyClusterVersion(version string, planSpecifiesPreviewString bool) string {
	parts := versionRE.FindStringSubmatch(version)
	if parts == nil {
		return version
	}
	if planSpecifiesPreviewString && parts[4] != "" {
		return clusterVersionPreview
	}
	return fmt.Sprintf("v%s.%s", parts[1], parts[2])
}

// Since the API response will always sort regions by name, we need to
// resort the list, so it matches up with the plan. If the response and
// plan regions don't match up, the sort won't work right, but we can
// ignore it. Terraform will handle it.
func sortRegionsByPlan(regions *[]client.Region, plan []Region) {
	if regions == nil || plan == nil {
		return
	}
	regionOrdinals := make(map[string]int, len(*regions))
	for i, region := range plan {
		regionOrdinals[region.Name.ValueString()] = i
	}
	sort.Slice(*regions, func(i, j int) bool {
		return regionOrdinals[(*regions)[i].Name] < regionOrdinals[(*regions)[j].Name]
	})
}

func loadClusterToTerraformState(
	clusterObj *client.Cluster, state *CockroachCluster, plan *CockroachCluster,
) {
	state.ID = types.StringValue(clusterObj.Id)
	state.Name = types.StringValue(clusterObj.Name)
	state.CloudProvider = types.StringValue(string(clusterObj.CloudProvider))
	planSpecifiesPreviewString := plan != nil && plan.CockroachVersion.ValueString() == clusterVersionPreview
	state.CockroachVersion = types.StringValue(simplifyClusterVersion(clusterObj.CockroachVersion, planSpecifiesPreviewString))
	state.Plan = types.StringValue(string(clusterObj.Plan))
	if clusterObj.AccountId == nil {
		state.AccountId = types.StringNull()
	} else {
		state.AccountId = types.StringValue(*clusterObj.AccountId)
	}
	state.State = types.StringValue(string(clusterObj.State))
	state.CreatorId = types.StringValue(clusterObj.CreatorId)
	state.OperationStatus = types.StringValue(string(clusterObj.OperationStatus))
	state.Regions = getManagedRegions(&clusterObj.Regions, plan.Regions)
	state.UpgradeStatus = types.StringValue(string(clusterObj.UpgradeStatus))

	if clusterObj.Config.Serverless != nil {
		state.ServerlessConfig = &ServerlessClusterConfig{
			SpendLimit: types.Int64Value(int64(clusterObj.Config.Serverless.GetSpendLimit())),
			RoutingId:  types.StringValue(clusterObj.Config.Serverless.RoutingId),
		}
	} else if clusterObj.Config.Dedicated != nil {
		state.DedicatedConfig = &DedicatedClusterConfig{
			MachineType:              types.StringValue(clusterObj.Config.Dedicated.MachineType),
			NumVirtualCpus:           types.Int64Value(int64(clusterObj.Config.Dedicated.NumVirtualCpus)),
			StorageGib:               types.Int64Value(int64(clusterObj.Config.Dedicated.StorageGib)),
			MemoryGib:                types.Float64Value(float64(clusterObj.Config.Dedicated.MemoryGib)),
			DiskIops:                 types.Int64Value(int64(clusterObj.Config.Dedicated.DiskIops)),
			PrivateNetworkVisibility: types.BoolValue(clusterObj.GetNetworkVisibility() == client.NETWORKVISIBILITYTYPE_PRIVATE),
		}
	}
}

// Due to the cyclic dependency issues of CMEK, there may be additional
// regions that are managed by another resource (i.e. cockroach_cmek) that
// we can safely omit from the state.
func getManagedRegions(apiRegions *[]client.Region, plan []Region) []Region {
	if apiRegions == nil {
		return nil
	}
	regions := make([]Region, 0, len(*apiRegions))
	sortRegionsByPlan(apiRegions, plan)
	planRegions := make(map[string]bool, len(plan))
	for _, region := range plan {
		planRegions[region.Name.ValueString()] = true
	}
	isImport := len(plan) == 0
	for _, x := range *apiRegions {
		if isImport || planRegions[x.Name] {
			rg := Region{
				Name:      types.StringValue(x.Name),
				SqlDns:    types.StringValue(x.SqlDns),
				UiDns:     types.StringValue(x.UiDns),
				NodeCount: types.Int64Value(int64(x.NodeCount)),
				Primary:   types.BoolValue(x.GetPrimary()),
			}
			regions = append(regions, rg)
		}
	}
	return regions
}

func waitForClusterReadyFunc(
	ctx context.Context, id string, cl client.Service, cluster *client.Cluster,
) sdk_resource.RetryFunc {
	return func() *sdk_resource.RetryError {
		apiCluster, httpResp, err := cl.GetCluster(ctx, id)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
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

// waitForClusterLock checks to see if the cluster is locked by any sort of automatic job,
// and waits if necessary before proceeding.
func waitForClusterLock(ctx context.Context, state CockroachCluster, s client.Service, diags *diag.Diagnostics) {
	if state.State.ValueString() == string(client.CLUSTERSTATETYPE_LOCKED) {
		tflog.Info(ctx, "Cluster is locked. Waiting for the operation to finish.")
		clusterObj, _, err := s.GetCluster(ctx, state.ID.ValueString())
		if err != nil {
			diags.AddError("Couldn't retrieve cluster info", formatAPIErrorMessage(err))
			return
		}
		err = sdk_resource.RetryContext(ctx, clusterUpdateTimeout,
			waitForClusterReadyFunc(ctx, clusterObj.Id, s, clusterObj))
		if err != nil {
			diags.AddError("Cluster is not ready", err.Error())
		}
	}
}

// To build an update request, we need to reconcile three region lists:
// the current regions managed by this resource (state), the planned regions
// managed by this resource (plan), and the full list of regions in the
// cluster. We need to update the current resource's regions without impacting
// the regions managed by another resource.
//
// A nil return value means no region update is required.
func reconcileRegionUpdate(
	ctx context.Context, state, plan []Region, clusterID string, service client.Service,
) (*map[string]int32, diag.Diagnostics) {
	type regionInfo struct {
		inState   bool
		inPlan    bool
		nodeCount int64
	}
	var regionUpdateRequired bool
	regions := make(map[string]*regionInfo, len(state))
	for _, region := range state {
		regions[region.Name.ValueString()] = &regionInfo{true, false, region.NodeCount.ValueInt64()}
	}
	for _, planRegion := range plan {
		region, ok := regions[planRegion.Name.ValueString()]
		if !ok {
			regions[planRegion.Name.ValueString()] = &regionInfo{false, true, planRegion.NodeCount.ValueInt64()}
			regionUpdateRequired = true
		} else {
			region.inPlan = true
			if region.nodeCount != planRegion.NodeCount.ValueInt64() {
				region.nodeCount = planRegion.NodeCount.ValueInt64()
				regionUpdateRequired = true
			}
		}
	}
	for _, region := range regions {
		if !region.inPlan {
			regionUpdateRequired = true
		}
	}
	if regionUpdateRequired {
		cluster, _, err := service.GetCluster(ctx, clusterID)
		if err != nil {
			diags := diag.Diagnostics{}
			diags.AddError("Error retrieving cluster info", formatAPIErrorMessage(err))
			return nil, diags
		}
		for _, region := range cluster.Regions {
			_, ok := regions[region.Name]
			if !ok {
				regions[region.Name] = &regionInfo{
					inState:   false,
					inPlan:    false,
					nodeCount: int64(region.NodeCount),
				}
			}
		}
		regionNodes := make(map[string]int32, len(regions))
		for name, info := range regions {
			// Omit regions that are in the state (meaning we had been managing them)
			// but not the plan. Everything else stays.
			if info.inState && !info.inPlan {
				continue
			}
			regionNodes[name] = int32(info.nodeCount)
		}
		return &regionNodes, nil
	}
	return nil, nil
}

func NewClusterResource() resource.Resource {
	return &clusterResource{}
}
