package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusterDataSourceType struct{}

func (t clusterDataSourceType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		Attributes: map[string]tfsdk.Attribute{
			"id": {
				Type:     types.StringType,
				Required: true,
			},
			"name": {
				MarkdownDescription: "Name of cluster",
				Type:                types.StringType,
				Computed:            true,
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
			"config": {
				Optional: true,
				Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
					"serverless": {
						Computed: true,
						Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
							"spend_limit": {
								Computed: true,
								Type:     types.Int64Type,
							},
							"routing_id": {
								Type:     types.ListType{ElemType: types.StringType},
								Computed: true,
							},
						}),
					},
					"dedicated": {
						Computed: true,
						Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
							"machine_type": {
								Type:     types.StringType,
								Computed: true,
							},
							"num_virtual_cpus": {
								Type:     types.Int64Type,
								Computed: true,
							},
							"storage_gib": {
								Type:     types.Int64Type,
								Computed: true,
							},
							"memory_gib": {
								Type:     types.Float64Type,
								Computed: true,
							},
							"disk_iops": {
								Type:     types.Int64Type,
								Computed: true,
							},
						}),
					},
				}),
			},
			"regions": {
				Computed: true,
				Attributes: tfsdk.ListNestedAttributes(map[string]tfsdk.Attribute{
					"name": {
						Type:     types.StringType,
						Computed: true,
					},
					"sql_dns": {
						Type:     types.StringType,
						Computed: true,
					},
					"ui_dns": {
						Type:     types.StringType,
						Computed: true,
					},
					"node_count": {
						Type:     types.Int64Type,
						Computed: true,
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
		},
		Blocks:              nil,
		Version:             0,
		DeprecationMessage:  "",
		Description:         "",
		MarkdownDescription: "clusterSourceType Data Source",
	}, nil
}

func (t clusterDataSourceType) NewDataSource(ctx context.Context, in tfsdk.Provider) (tfsdk.DataSource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return clusterDataSource{
		provider: provider,
	}, diags
}

type clusterDataSource struct {
	provider provider
}

func (d clusterDataSource) Read(ctx context.Context, req tfsdk.ReadDataSourceRequest, resp *tfsdk.ReadDataSourceResponse) {
	var cluster CockroachClusterData
	diags := req.Config.Get(ctx, &cluster)

	cockroachCluster, httpResp, err := d.provider.service.GetCluster(ctx, cluster.ID.Value)
	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode == http.StatusNotFound {
		resp.Diagnostics.AddError(
			"cluster not found",
			fmt.Sprintf("cluster with clusterID %s is not found", cluster.ID.Value))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"error in getting cluster",
			"")
	}

	cluster.Name = types.String{Value: cockroachCluster.Name}
	cluster.Provider = CloudProvider(cockroachCluster.CloudProvider)
	cluster.State = types.String{Value: string(cockroachCluster.State)}
	cluster.CockroachVersion = types.String{Value: cockroachCluster.CockroachVersion}
	cluster.Plan = types.String{Value: string(cockroachCluster.Plan)}
	cluster.OperationStatus = types.String{Value: string(cockroachCluster.OperationStatus)}
	if cockroachCluster.Config.Serverless != nil {
		cluster.Config.Serverless = ServerlessClusterConfig{
			SpendLimit: types.Int64{Value: int64(cockroachCluster.Config.Serverless.SpendLimit)},
			RoutingId:  types.String{Value: cockroachCluster.Config.Serverless.RoutingId},
		}
	}
	if cockroachCluster.Config.Dedicated != nil {
		cluster.Config.Dedicated = DedicatedHardwareConfig{
			MachineType:    types.String{Value: cockroachCluster.Config.Dedicated.MachineType},
			NumVirtualCpus: types.Int64{Value: int64(cockroachCluster.Config.Dedicated.NumVirtualCpus)},
			StorageGib:     types.Int64{Value: int64(cockroachCluster.Config.Dedicated.StorageGib)},
			MemoryGib:      types.Float64{Value: float64(cockroachCluster.Config.Dedicated.MemoryGib)},
			DiskIops:       types.Int64{Value: int64(cockroachCluster.Config.Dedicated.DiskIops)},
		}
	}

	for _, r := range cockroachCluster.Regions {
		reg := Region{
			Name:      types.String{Value: r.Name},
			SqlDns:    types.String{Value: r.SqlDns},
			UiDns:     types.String{Value: r.UiDns},
			NodeCount: types.Int64{Value: int64(r.NodeCount)},
		}
		cluster.Regions = append(cluster.Regions, reg)
	}

	diags = resp.State.Set(ctx, cluster)
	resp.Diagnostics.Append(diags...)
}
