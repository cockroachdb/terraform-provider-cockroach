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
	"reflect"
	"sort"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

var cmekAttributes = map[string]schema.Attribute{
	"id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Cluster ID.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	},
	"status": schema.StringAttribute{
		MarkdownDescription: "Aggregated status of the cluster's encryption key(s).",
		Computed:            true,
		Optional:            true,
	},
	"regions": schema.ListNestedAttribute{
		Required: true,
		PlanModifiers: []planmodifier.List{
			listplanmodifier.UseStateForUnknown(),
		},
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"region": schema.StringAttribute{
					Required:    true,
					Description: "Cloud provider region code.",
				},
				"status": schema.StringAttribute{
					Computed:    true,
					Description: "Describes the status of the current encryption key within the region.",
				},
				"key": schema.SingleNestedAttribute{
					Required: true,
					Attributes: map[string]schema.Attribute{
						"auth_principal": schema.StringAttribute{
							Required:    true,
							Description: "Principal to authenticate as in order to access the key.",
						},
						"type": schema.StringAttribute{
							MarkdownDescription: "Type of encryption key. Current allowed values are:" +
								formatEnumMarkdownList(client.AllowedCMEKKeyTypeEnumValues),
							Required: true,
						},
						"uri": schema.StringAttribute{
							Required:    true,
							Description: "Provider-specific URI pointing to the encryption key.",
						},
						"status": schema.StringAttribute{
							Computed:    true,
							Description: "Current status of this key.",
						},
						"user_message": schema.StringAttribute{
							Computed:    true,
							Description: "Elaborates on the key's status and hints at how to fix issues that may have occurred during asynchronous key operations.",
						},
						"created_at": schema.StringAttribute{
							Computed:    true,
							Description: "Indicates when the key was created.",
						},
						"updated_at": schema.StringAttribute{
							Computed:    true,
							Description: "Indicates when the key was last updated.",
						},
					},
				},
			},
		},
	},
	"additional_regions": schema.ListNestedAttribute{
		NestedObject:        regionSchema,
		Optional:            true,
		MarkdownDescription: "Once CMEK is enabled for a cluster, no new regions can be added to the cluster resource, since they need encryption key info stored in the CMEK resource. New regions can be added and maintained here instead.",
	},
}

type cmekResource struct {
	provider *provider
}

func (r *cmekResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Customer-managed encryption keys (CMEK) resource for a single cluster.",
		Attributes:          cmekAttributes,
	}
}

func (r *cmekResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_cmek"
}

func (r *cmekResource) Configure(
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

func (r *cmekResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan ClusterCMEK

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if len(plan.AdditionalRegions) != 0 {
		resp.Diagnostics.AddError(
			"Invalid initial CMEK plan",
			"`additional_regions` must be empty initially. Add new regions to the parent cluster before creating a CMEK resource")
		return
	}

	cmekSpec := client.NewCMEKClusterSpecificationWithDefaults()
	regionSpecs := make([]client.CMEKRegionSpecification, 0, len(plan.Regions))
	for _, region := range plan.Regions {
		regionSpecs = append(regionSpecs, cmekRegionToClientSpec(region))
	}
	cmekSpec.SetRegionSpecs(regionSpecs)

	traceAPICall("EnableCMEKSpec")
	cmekObj, _, err := r.provider.service.EnableCMEKSpec(ctx, plan.ID.ValueString(), cmekSpec)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling CMEK",
			fmt.Sprintf("Could not enable CMEK: %v", formatAPIErrorMessage(err)),
		)
		return
	}
	err = retry.RetryContext(ctx, clusterUpdateTimeout,
		waitForCMEKReadyFunc(ctx, plan.ID.ValueString(), r.provider.service, cmekObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"CMEK enable failed",
			fmt.Sprintf("CMEK is not ready: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state ClusterCMEK
	loadCMEKToTerraformState(cmekObj, &state, &plan)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *cmekResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var cmek ClusterCMEK
	diags := req.State.Get(ctx, &cmek)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || !IsKnown(cmek.ID) {
		return
	}

	clusterID := cmek.ID.ValueString()
	traceAPICall("GetCMEKClusterInfo")
	cmekObj, httpResp, err := r.provider.service.GetCMEKClusterInfo(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"CMEK spec not found",
				fmt.Sprintf("CMEK specification with cluster ID %s is not found. Removing from state.", clusterID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting CMEK info",
				fmt.Sprintf("Unexpected error retrieving CMEK info: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	// We actually want to use the current state as the plan here,
	// since we're trying to see if it changed.
	loadCMEKToTerraformState(cmekObj, &cmek, &cmek)
	diags = resp.State.Set(ctx, cmek)
	resp.Diagnostics.Append(diags...)
}

func (r *cmekResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	// Get plan values
	var plan ClusterCMEK
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state ClusterCMEK
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	existingRegions := make(map[string]client.CMEKRegionSpecification, len(state.Regions))
	for _, region := range state.Regions {
		existingRegions[region.Region.ValueString()] = cmekRegionToClientSpec(region)
	}

	updateRegions := make([]client.CMEKRegionSpecification, 0, len(state.Regions))
	newRegions := make([]client.CMEKRegionSpecification, 0, len(plan.Regions))
	for _, plannedRegion := range plan.Regions {
		if existingRegion, ok := existingRegions[plannedRegion.Region.ValueString()]; !ok {
			newRegions = append(newRegions, cmekRegionToClientSpec(plannedRegion))
		} else if plannedRegionSpec := cmekRegionToClientSpec(plannedRegion); !reflect.DeepEqual(existingRegion, plannedRegionSpec) {
			updateRegions = append(updateRegions, plannedRegionSpec)
		}
	}

	regionNodes, diags := reconcileRegionUpdate(ctx, state.AdditionalRegions, plan.AdditionalRegions, state.ID.ValueString(), r.provider.service)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Status.ValueString() == string(client.CMEKSTATUS_REVOKED) {
		if regionNodes != nil || len(updateRegions) != 0 || len(newRegions) != 0 {
			resp.Diagnostics.AddError("Invalid CMEK update",
				"Can't revoke access and modify regions in the same operation")
			return
		}
		traceAPICall("UpdateCMEKStatus")
		if _, _, err := r.provider.service.UpdateCMEKStatus(ctx, plan.ID.ValueString(), &client.UpdateCMEKStatusRequest{
			Action: client.CMEKCUSTOMERACTION_REVOKE,
		}); err != nil {
			resp.Diagnostics.AddError("Error revoking CMEK",
				fmt.Sprintf("Error while attempting to revoke CMEK: %s", formatAPIErrorMessage(err)))
			return
		}
	}

	var cluster *client.Cluster
	if regionNodes != nil {
		// UpdateCluster expects only new regions in CmekRegionSpecs. Theoretically,
		// this should be the regions that are in the plan, but not the spec.
		if len(*regionNodes) != len(newRegions)+len(existingRegions) {
			resp.Diagnostics.AddError("Mismatch between CMEK and cluster regions", "Each new addition to `additional_regions` must be accompanied by exactly one corresponding entry in `regions`.")
			return
		}
		var err error
		traceAPICall("UpdateCluster")
		cluster, _, err = r.provider.service.UpdateCluster(ctx, state.ID.ValueString(), &client.UpdateClusterSpecification{
			Dedicated: &client.DedicatedClusterUpdateSpecification{
				RegionNodes:     regionNodes,
				CmekRegionSpecs: &newRegions,
			},
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating cluster",
				fmt.Sprintf("Error updating cluster: %s", formatAPIErrorMessage(err)))
			return
		}

		err = retry.RetryContext(ctx, clusterUpdateTimeout,
			waitForClusterReadyFunc(ctx, plan.ID.ValueString(), r.provider.service, cluster))
		if err != nil {
			resp.Diagnostics.AddError(
				"Adding new regions failed",
				fmt.Sprintf("Error adding new regions: %s", formatAPIErrorMessage(err)),
			)
			return
		}
		state.AdditionalRegions = getManagedRegions(&cluster.Regions, plan.AdditionalRegions)
	}

	if len(updateRegions) != 0 {
		traceAPICall("UpdateCMEKSpec")
		_, _, err := r.provider.service.UpdateCMEKSpec(
			ctx,
			plan.ID.ValueString(),
			&client.CMEKClusterSpecification{RegionSpecs: updateRegions},
		)
		if err != nil {
			resp.Diagnostics.AddError("Error updating CMEK specification", formatAPIErrorMessage(err))
			return
		}
	}

	var clusterInfo client.CMEKClusterInfo
	err := retry.RetryContext(ctx, clusterUpdateTimeout,
		waitForCMEKReadyFunc(ctx, plan.ID.ValueString(), r.provider.service, &clusterInfo))
	if err != nil {
		resp.Diagnostics.AddError(
			"Updating CMEK regions failed",
			fmt.Sprintf("Error updating CMEK regions: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadCMEKToTerraformState(&clusterInfo, &state, &plan)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Delete is a no-op, since you can't disable CMEK once it's set up.
func (r *cmekResource) Delete(
	ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	resp.State.RemoveResource(ctx)
}

func (r *cmekResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Since the API response will always sort regions by name, we need to
// resort the list, so it matches up with the plan. If the response and
// plan regions don't match up, the sort won't work right, but we can
// ignore it. Terraform will handle it.
func sortCMEKRegionsByPlan(cmekObj *client.CMEKClusterInfo, plan *ClusterCMEK) {
	if cmekObj == nil || plan == nil {
		return
	}
	regionOrdinals := make(map[string]int, len(cmekObj.GetRegionInfos()))
	for i, region := range plan.Regions {
		regionOrdinals[region.Region.ValueString()] = i
	}
	sort.Slice(*cmekObj.RegionInfos, func(i, j int) bool {
		return regionOrdinals[cmekObj.GetRegionInfos()[i].GetRegion()] < regionOrdinals[cmekObj.GetRegionInfos()[j].GetRegion()]
	})
}

func loadCMEKToTerraformState(
	cmekObj *client.CMEKClusterInfo, state *ClusterCMEK, plan *ClusterCMEK,
) {
	sortCMEKRegionsByPlan(cmekObj, plan)
	var rgs []CMEKRegion
	for i, region := range cmekObj.GetRegionInfos() {
		var keyInfo client.CMEKKeyInfo
		// If we have a plan, find the key that matches the plan URI.
		// If there's no plan (i.e. import), use the first key that's enabled.
		for _, key := range region.GetKeyInfos() {
			if plan != nil && len(plan.Regions) > 0 {
				if *key.GetSpec().Uri == plan.Regions[i].Key.URI.ValueString() {
					keyInfo = key
					break
				}
			} else {
				if key.GetStatus() == client.CMEKSTATUS_ENABLED {
					keyInfo = key
					break
				}
			}
		}
		rg := CMEKRegion{
			Region: types.StringValue(region.GetRegion()),
			Status: types.StringValue(string(region.GetStatus())),
			Key: CMEKKey{
				Status:        types.StringValue(string(keyInfo.GetStatus())),
				UserMessage:   types.StringValue(keyInfo.GetUserMessage()),
				Type:          types.StringValue(string(keyInfo.Spec.GetType())),
				URI:           types.StringValue(keyInfo.Spec.GetUri()),
				AuthPrincipal: types.StringValue(keyInfo.Spec.GetAuthPrincipal()),
				CreatedAt:     types.StringValue(keyInfo.GetCreatedAt().String()),
				UpdatedAt:     types.StringValue(keyInfo.GetUpdatedAt().String()),
			},
		}
		rgs = append(rgs, rg)
	}

	state.Regions = rgs
	state.ID = plan.ID
	state.Status = types.StringValue(string(cmekObj.GetStatus()))
}

func cmekRegionToClientSpec(region CMEKRegion) client.CMEKRegionSpecification {
	name := region.Region.ValueString()
	keyType := client.CMEKKeyType(region.Key.Type.ValueString())
	uri := region.Key.URI.ValueString()
	authPrincipal := region.Key.AuthPrincipal.ValueString()
	return client.CMEKRegionSpecification{
		Region: &name,
		KeySpec: &client.CMEKKeySpecification{
			Type:          &keyType,
			Uri:           &uri,
			AuthPrincipal: &authPrincipal,
		},
	}
}

func waitForCMEKReadyFunc(
	ctx context.Context, clusterID string, cl client.Service, cmek *client.CMEKClusterInfo,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("GetCMEKClusterInfo")
		apiCMEK, httpResp, err := cl.GetCMEKClusterInfo(ctx, clusterID)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return retry.NonRetryableError(fmt.Errorf("error getting cmek: %s", formatAPIErrorMessage(err)))
			} else {
				return retry.RetryableError(fmt.Errorf("encountered a server error while reading cmek status - trying again"))
			}
		}
		*cmek = *apiCMEK
		for _, region := range cmek.GetRegionInfos() {
			switch region.GetStatus() {
			case client.CMEKSTATUS_ENABLED,
				client.CMEKSTATUS_DISABLED,
				client.CMEKSTATUS_REVOKED:
				continue
			case client.CMEKSTATUS_DISABLE_FAILED,
				client.CMEKSTATUS_ENABLE_FAILED,
				client.CMEKSTATUS_REVOKE_FAILED,
				client.CMEKSTATUS_ROTATE_FAILED:
				return retry.NonRetryableError(fmt.Errorf("cmek update failed"))
			default:
				return retry.RetryableError(fmt.Errorf("cmek is not ready yet"))
			}
		}
		return nil
	}
}

func NewCMEKResource() resource.Resource {
	return &cmekResource{}
}
