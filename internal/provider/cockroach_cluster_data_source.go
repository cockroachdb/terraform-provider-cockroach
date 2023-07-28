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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

type clusterDataSource struct {
	provider *provider
}

func (d *clusterDataSource) Schema(
	_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse,
) {
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
					"usage_limits": schema.SingleNestedAttribute{
						Computed: true,
						Attributes: map[string]schema.Attribute{
							"request_unit_limit": schema.Int64Attribute{
								Computed: true,
							},
							"storage_mib_limit": schema.Int64Attribute{
								Computed: true,
							},
						},
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
					"private_network_visibility": schema.BoolAttribute{
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
						"internal_dns": schema.StringAttribute{
							Computed: true,
						},
						"node_count": schema.Int64Attribute{
							Computed: true,
						},
						"primary": schema.BoolAttribute{
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
			"upgrade_status": schema.StringAttribute{
				Computed: true,
			},
		},
		MarkdownDescription: "Cluster Data Source",
	}
}

func (d *clusterDataSource) Metadata(
	_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (d *clusterDataSource) Configure(
	_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if d.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError("Internal provider error",
			fmt.Sprintf("Error in Configure: expected %T but got %T", provider{}, req.ProviderData))
	}
}

func (d *clusterDataSource) Read(
	ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse,
) {
	if d.provider == nil || !d.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

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
	clusterID := cluster.ID.ValueString()
	if !uuidRegex.MatchString(clusterID) {
		resp.Diagnostics.AddError(
			"Unexpected cluster ID format",
			fmt.Sprintf("'%s' is not a valid cluster ID format. Expected UUID.", clusterID),
		)
		return
	}

	cockroachCluster, httpResp, err := d.provider.service.GetCluster(ctx, clusterID)
	if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
		resp.Diagnostics.AddError(
			"Cluster not found",
			fmt.Sprintf("Couldn't find a cluster with ID %s", clusterID))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting cluster info",
			fmt.Sprintf("Unexpected error while retrieving cluster info: %v", formatAPIErrorMessage(err)))
		return
	}

	// The concept of a plan doesn't apply to data sources.
	// Using a nil plan means we won't try to re-sort the region list.
	loadClusterToTerraformState(cockroachCluster, &cluster, nil)

	diags = resp.State.Set(ctx, cluster)
	resp.Diagnostics.Append(diags...)
}

func NewClusterDataSource() datasource.DataSource {
	return &clusterDataSource{}
}
