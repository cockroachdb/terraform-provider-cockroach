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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusterDataSource struct {
	provider *provider
}

func (d *clusterDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of cluster",
				Computed:            true,
			},
			"cockroach_version": schema.StringAttribute{
				Computed: true,
			},
			"plan": schema.StringAttribute{
				Computed: true,
			},
			"cloud_provider": schema.StringAttribute{
				Computed: true,
			},
			"account_id": schema.StringAttribute{
				Computed: true,
			},
			"serverless": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"spend_limit": schema.Int64Attribute{
						Computed: true,
					},
					"routing_id": schema.StringAttribute{
						Computed: true,
					},
				},
			},
			"dedicated": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"machine_type": schema.StringAttribute{
						Computed: true,
					},
					"num_virtual_cpus": schema.Int64Attribute{
						Computed: true,
					},
					"storage_gib": schema.Int64Attribute{
						Computed: true,
					},
					"memory_gib": schema.Float64Attribute{
						Computed: true,
					},
					"disk_iops": schema.Int64Attribute{
						Computed: true,
					},
				},
			},
			"regions": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed: true,
						},
						"sql_dns": schema.StringAttribute{
							Computed: true,
						},
						"ui_dns": schema.StringAttribute{
							Computed: true,
						},
						"node_count": schema.Int64Attribute{
							Computed: true,
						},
					},
				},
			},
			"state": schema.StringAttribute{
				Computed: true,
			},
			"creator_id": schema.StringAttribute{
				Computed: true,
			},
			"operation_status": schema.StringAttribute{
				Computed: true,
			},
		},
		MarkdownDescription: "Cluster Data Source",
	}
}

func (d *clusterDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (d *clusterDataSource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if d.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError("Internal provider error",
			fmt.Sprintf("Error in Configure: expected %T but got %T", provider{}, req.ProviderData))
	}
}

func (d *clusterDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cluster CockroachCluster
	diags := req.Config.Get(ctx, &cluster)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddWarning("Error loading the cluster", "")
		return
	}

	if cluster.ID.IsNull() {
		resp.Diagnostics.AddError(
			"ID can't be null",
			"The ID field is null, but it never should be. Please double check the value!",
		)
		return
	}

	cockroachCluster, httpResp, err := d.provider.service.GetCluster(ctx, cluster.ID.ValueString())
	if httpResp.StatusCode == http.StatusNotFound {
		resp.Diagnostics.AddError(
			"Cluster not found",
			fmt.Sprintf("Couldn't find a cluster with ID %s", cluster.ID.ValueString()))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting cluster info",
			fmt.Sprintf("Unexpected error while retrieving cluster info: %v", formatAPIErrorMessage(err)))
	}

	cluster.Name = types.StringValue(cockroachCluster.Name)
	cluster.CloudProvider = types.StringValue(string(cockroachCluster.CloudProvider))
	cluster.State = types.StringValue(string(cockroachCluster.State))
	cluster.CockroachVersion = types.StringValue(cockroachCluster.CockroachVersion)
	cluster.Plan = types.StringValue(string(cockroachCluster.Plan))
	cluster.OperationStatus = types.StringValue(string(cockroachCluster.OperationStatus))
	if cockroachCluster.Config.Serverless != nil {
		cluster.ServerlessConfig = &ServerlessClusterConfig{
			SpendLimit: types.Int64Value(int64(cockroachCluster.Config.Serverless.SpendLimit)),
			RoutingId:  types.StringValue(cockroachCluster.Config.Serverless.RoutingId),
		}
	}
	if cockroachCluster.Config.Dedicated != nil {
		cluster.DedicatedConfig = &DedicatedClusterConfig{
			MachineType:    types.StringValue(cockroachCluster.Config.Dedicated.MachineType),
			NumVirtualCpus: types.Int64Value(int64(cockroachCluster.Config.Dedicated.NumVirtualCpus)),
			StorageGib:     types.Int64Value(int64(cockroachCluster.Config.Dedicated.StorageGib)),
			MemoryGib:      types.Float64Value(float64(cockroachCluster.Config.Dedicated.MemoryGib)),
			DiskIops:       types.Int64Value(int64(cockroachCluster.Config.Dedicated.DiskIops)),
		}
	}

	for _, r := range cockroachCluster.Regions {
		reg := Region{
			Name:      types.StringValue(r.Name),
			SqlDns:    types.StringValue(r.SqlDns),
			UiDns:     types.StringValue(r.UiDns),
			NodeCount: types.Int64Value(int64(r.NodeCount)),
		}
		cluster.Regions = append(cluster.Regions, reg)
	}

	diags = resp.State.Set(ctx, cluster)
	resp.Diagnostics.Append(diags...)
}

func NewClusterDataSource() datasource.DataSource {
	return &clusterDataSource{}
}
