package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
)

type clusterResourceType struct{}

func (r clusterResourceType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
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
			"spec": {
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
								Type:     types.Int64Type,
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
		},
	}, nil
}

func (t clusterResourceType) NewResource(ctx context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
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

	var newCluster CockroachCluster
	diags := req.Config.Get(ctx, &newCluster)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	clusterSpec := client.NewCreateClusterSpecification()
	var regions []string
	for _, region := range newCluster.Spec.Serverless.Regions {
		regions = append(regions, region.Value)
	}
	serverless := client.NewServerlessClusterSpecification(regions, int32(newCluster.Spec.Serverless.SpendLimit.Value))
	clusterSpec.SetServerless(*serverless)
	clusterReq := client.NewCreateClusterRequest(newCluster.Name.Value, client.ApiCloudProvider(newCluster.Provider), *clusterSpec)
	clusterObj, clResp, err := r.provider.service.CreateCluster(ctx, clusterReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating order",
			fmt.Sprintf("Could not create order, unexpected error: %v %v "+err.Error(), clResp),
		)
		return
	}

	var rg []types.String
	for _, x := range clusterObj.Regions {
		rg = append(rg, types.String{Value: x.Name})
	}

	var state = CockroachCluster{
		ID:               types.String{Value: clusterObj.Id},
		Name:             types.String{Value: clusterObj.Name},
		Provider:         CloudProvider(clusterObj.CloudProvider),
		Plan:             types.String{Value: string(clusterObj.Plan)},
		CockroachVersion: types.String{Value: clusterObj.CockroachVersion},
		Spec: ClusterSpec{
			Serverless: &ServerlessClusterSpec{
				SpendLimit: types.Int64{Value: int64(clusterObj.Config.Serverless.SpendLimit)},
				Regions:    rg,
			},
		},
		State:           types.String{Value: string(clusterObj.State)},
		OperationStatus: types.String{Value: string(clusterObj.OperationStatus)},
	}

	diags = resp.State.Set(ctx, state)
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
	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode == http.StatusNotFound {
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

	var state CockroachCluster
	var rg []types.String
	for _, x := range clusterObj.Regions {
		rg = append(rg, types.String{Value: x.Name})
	}
	state = CockroachCluster{
		ID:       types.String{Value: clusterObj.Id},
		Name:     types.String{Value: clusterObj.Name},
		Provider: CloudProvider(clusterObj.CloudProvider),
		Spec: ClusterSpec{
			Serverless: &ServerlessClusterSpec{
				SpendLimit: types.Int64{Value: int64(clusterObj.Config.Serverless.SpendLimit)},
				Regions:    rg,
			},
		},
	}

	diags = resp.State.Set(ctx, state)
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

	var regions []string
	for _, region := range state.Spec.Serverless.Regions {
		regions = append(regions, region.Value)
	}

	clusterReq := client.NewUpdateClusterSpecification()
	serverless := client.ServerlessClusterSpecification{SpendLimit: int32(plan.Spec.Serverless.SpendLimit.Value)}
	clusterReq.SetServerless(serverless)

	apiResp, err := r.provider.service.UpdateCluster(ctx, state.ID.Value, clusterReq, &client.UpdateClusterOptions{})
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
	_, err := r.provider.service.DeleteCluster(ctx, clusterID)
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
