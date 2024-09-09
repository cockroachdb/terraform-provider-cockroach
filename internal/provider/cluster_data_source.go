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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v2/pkg/client"
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
		Description: "CockroachDB Cloud cluster. Can be Dedicated or Serverless.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the cluster.",
				Computed:            true,
			},
			"cockroach_version": schema.StringAttribute{
				Computed:    true,
				Description: "Full version of CockroachDB running on the cluster.",
			},
			"plan": schema.StringAttribute{
				Computed:    true,
				Description: "Denotes cluster plan type: 'BASIC' or 'STANDARD' or 'ADVANCED'.",
			},
			"cloud_provider": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Cloud provider used to host the cluster. Allowed values are:" +
					formatEnumMarkdownList(client.AllowedCloudProviderTypeEnumValues),
			},
			"account_id": schema.StringAttribute{
				Computed:    true,
				Description: "The cloud provider account ID that hosts the cluster. Needed for CMEK and other advanced features.",
			},
			"serverless": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"spend_limit": schema.Int64Attribute{
						DeprecationMessage:  "The `spend_limit` attribute is deprecated and will be removed in a future release of the provider. Configure 'usage_limits' instead.",
						Computed:            true,
						MarkdownDescription: "Spend limit in US cents.",
					},
					"usage_limits": schema.SingleNestedAttribute{
						Computed: true,
						Attributes: map[string]schema.Attribute{
							"request_unit_limit": schema.Int64Attribute{
								Computed:            true,
								MarkdownDescription: "Maximum number of Request Units that the cluster can consume during the month.",
							},
							"storage_mib_limit": schema.Int64Attribute{
								Computed:            true,
								MarkdownDescription: "Maximum amount of storage (in MiB) that the cluster can have at any time during the month.",
							},
							"provisioned_vcpus": schema.Int64Attribute{
								Computed:            true,
								MarkdownDescription: "Maximum number of vCPUs that the cluster can use.",
							},
						},
					},
					"routing_id": schema.StringAttribute{
						Computed:    true,
						Description: "Cluster identifier in a connection string.",
					},
					"upgrade_type": schema.StringAttribute{
						Computed:    true,
						Description: "Dictates the behavior of cockroach major version upgrades.",
					},
				},
			},
			"dedicated": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"machine_type": schema.StringAttribute{
						Computed:    true,
						Description: "Machine type identifier within the given cloud provider, ex. m6.xlarge, n2-standard-4.",
					},
					"num_virtual_cpus": schema.Int64Attribute{
						Computed:    true,
						Description: "Number of virtual CPUs per node in the cluster.",
					},
					"storage_gib": schema.Int64Attribute{
						Computed:    true,
						Description: "Storage amount per node in GiB.",
					},
					"memory_gib": schema.Float64Attribute{
						Computed:    true,
						Description: "Memory per node in GiB.",
					},
					"disk_iops": schema.Int64Attribute{
						Computed:    true,
						Description: "Number of disk I/O operations per second that are permitted on each node in the cluster. Zero indicates the cloud provider-specific default.",
					},
					"private_network_visibility": schema.BoolAttribute{
						Computed:    true,
						Description: "Indicates whether private IP addresses are assigned to nodes. Required for CMEK and other advanced networking features.",
					},
				},
			},
			"regions": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "Region code used by the cluster's cloud provider.",
						},
						"sql_dns": schema.StringAttribute{
							Computed:    true,
							Description: "DNS name of the cluster's SQL interface. Used to connect to the cluster with IP allowlisting.",
						},
						"ui_dns": schema.StringAttribute{
							Computed:    true,
							Description: "DNS name used when connecting to the DB Console for the cluster.",
						},
						"internal_dns": schema.StringAttribute{
							Computed:    true,
							Description: "Internal DNS name of the cluster within the cloud provider's network. Used to connect to the cluster with PrivateLink or VPC peering.",
						},
						"node_count": schema.Int64Attribute{
							Computed:    true,
							Description: "Number of nodes in the region. Will always be 0 for serverless clusters.",
						},
						"primary": schema.BoolAttribute{
							Computed:    true,
							Description: "Denotes whether this is the primary region in a serverless cluster. Dedicated clusters don't have a primary region.",
						},
					},
				},
			},
			"state": schema.StringAttribute{
				Computed:    true,
				Description: "Describes whether the cluster is being created, updated, deleted, etc.",
			},
			"creator_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the user who created the cluster.",
			},
			"operation_status": schema.StringAttribute{
				Computed:    true,
				Description: "Describes the current long-running operation, if any.",
			},
			"upgrade_status": schema.StringAttribute{
				Computed:    true,
				Description: "Describes the status of any in-progress CockroachDB upgrade or rollback.",
			},
			"parent_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the cluster's parent folder. 'root' is used for a cluster at the root level.",
			},
			"delete_protection": schema.BoolAttribute{
				Computed:    true,
				Description: "Set to true to enable delete protection on the cluster.",
			},
		},
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

	var state CockroachCluster
	diags := req.Config.Get(ctx, &state)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddWarning("Error loading the cluster", "")
		return
	}

	if state.ID.IsNull() {
		resp.Diagnostics.AddError(
			"ID can't be null",
			"The ID field is null, but it never should be. Please double check the value!",
		)
		return
	}
	clusterID := state.ID.ValueString()
	if !uuidRegex.MatchString(clusterID) {
		resp.Diagnostics.AddError(
			"Unexpected cluster ID format",
			fmt.Sprintf("'%s' is not a valid cluster ID format. Expected UUID.", clusterID),
		)
		return
	}

	traceAPICall("GetCluster")
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
	var newState CockroachCluster
	loadClusterToTerraformState(cockroachCluster, &newState, nil)
	diags = resp.State.Set(ctx, newState)
	resp.Diagnostics.Append(diags...)
}

func NewClusterDataSource() datasource.DataSource {
	return &clusterDataSource{}
}
