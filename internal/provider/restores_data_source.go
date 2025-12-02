package provider

import (
	"context"
	"fmt"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type restoresDataSource struct {
	provider *provider
}

func (d *restoresDataSource) Schema(
	_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "Information about restore jobs on a specific cluster.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Description: "The ID of the cluster where the restore jobs ran or are currently running.",
				Required:    true,
				Validators:  uuidValidator,
			},
			"start_time": schema.StringAttribute{
				Description: "Beginning timestamp of the range (inclusive) used to search for restore jobs based on their creation time. If this field is provided, end_time must also be included in the request. Uses a date format with optional timestamp, for example: `2023-01-15` or `2023-01-15T10:30:00Z`.",
				Optional:    true,
			},
			"end_time": schema.StringAttribute{
				Description: "Ending timestamp of the range (exclusive) used to search for restore jobs based on their creation time. If this field is provided, start_time must also be included in the request. Uses a date format with optional timestamp, for example: `2023-01-15` or `2023-01-15T10:30:00Z`.",
				Optional:    true,
			},
			"sort_order": schema.StringAttribute{
				MarkdownDescription: "Specifies the sort direction for the returned results. Use `ASC` for ascending or `DESC` for descending order.",
				Optional:            true,
			},
			"limit": schema.Int32Attribute{
				Description: "The maximum number of restore jobs to return. If not set, only the first 500 restore jobs will be returned.",
				Optional:    true,
			},
			"restores": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The unique identifier associated with the restore job.",
							Computed:            true,
						},
						"backup_id": schema.StringAttribute{
							MarkdownDescription: "The ID of the managed backup used for this restore job.",
							Computed:            true,
						},
						"status": schema.StringAttribute{
							MarkdownDescription: "The current status of the restore job.",
							Computed:            true,
						},
						"created_at": schema.StringAttribute{
							MarkdownDescription: "The time at which the restore job was initiated.",
							Computed:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: "The type of object (to be) restored by this job.",
							Computed:            true,
						},
						"completion_percent": schema.Float32Attribute{
							MarkdownDescription: "Decimal value showing the percentage of the restore job that has been completed. Value ranges from 0 to 1.",
							Computed:            true,
						},
						"source_cluster_name": schema.StringAttribute{
							MarkdownDescription: "The name of the cluster from which the backup was taken.",
							Computed:            true,
						},
						"destination_cluster_name": schema.StringAttribute{
							MarkdownDescription: "The name of the cluster to which the restore is being applied.",
							Computed:            true,
						},
						"backup_end_time": schema.StringAttribute{
							MarkdownDescription: "The timestamp at which the backup data was captured.",
							Computed:            true,
						},
						"completed_at": schema.StringAttribute{
							MarkdownDescription: "The timestamp at which the restore job completed.",
							Computed:            true,
						},
						"crdb_job_id": schema.StringAttribute{
							MarkdownDescription: "The CockroachDB internal job ID for the restore job.",
							Computed:            true,
						},
						"client_error_code": schema.Int32Attribute{
							MarkdownDescription: "Error code from the restore job, only populated if it has failed.",
							Computed:            true,
						},
						"client_error_message": schema.StringAttribute{
							MarkdownDescription: "Error message from the restore job, only populated if it has failed.",
							Computed:            true,
						},
						"objects": schema.ListNestedAttribute{
							MarkdownDescription: "The list of database objects (databases, tables) that were restored.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"database": schema.StringAttribute{
										MarkdownDescription: "The database name in the fully qualified name of the objects that were restored.",
										Computed:            true,
									},
									"schema": schema.StringAttribute{
										MarkdownDescription: "The schema name in the fully qualified name of the objects that were restored.",
										Computed:            true,
									},
									"tables": schema.ListAttribute{
										MarkdownDescription: "The list of table names in the fully qualified name of the objects that were restored.",
										ElementType:         types.StringType,
										Computed:            true,
									},
								},
							},
						},
						"restore_opts": schema.SingleNestedAttribute{
							MarkdownDescription: "Additional options controlling the behavior of the restore job.",
							Computed:            true,
							Attributes: map[string]schema.Attribute{
								"new_db_name": schema.StringAttribute{
									MarkdownDescription: "Specifies the name of the database to create during a database restore job. If not set, the name defaults to the original database name from the source cluster.",
									Computed:            true,
								},
								"into_db": schema.StringAttribute{
									MarkdownDescription: "Specifies the target database to restore the table into during a table restore job. If not set, the table will be restored into the database it belonged to in the source backup.",
									Computed:            true,
								},
								"skip_localities_check": schema.BoolAttribute{
									MarkdownDescription: "Allows the restore job to continue in the event that there are mismatched localities between the backup and target cluster. Useful when restoring multi-region tables to a cluster missing some localities.",
									Computed:            true,
								},
								"skip_missing_foreign_keys": schema.BoolAttribute{
									MarkdownDescription: "Allows a table to be restored even if it has foreign key constraints referencing rows that no longer exist in the target cluster.",
									Computed:            true,
								},
								"skip_missing_sequences": schema.BoolAttribute{
									MarkdownDescription: "Allows a table to be restored even if it contains a column whose `DEFAULT` value depends on a sequence. (See https://www.cockroachlabs.com/docs/stable/show-sequences)",
									Computed:            true,
								},
								"skip_missing_views": schema.BoolAttribute{
									MarkdownDescription: "Allows the job to skip restoring views that cannot be restored because their dependencies are not included in the current restore job.",
									Computed:            true,
								},
								"schema_only": schema.BoolAttribute{
									MarkdownDescription: "If set, only the schema will be restored and no user data will be included.",
									Computed:            true,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *restoresDataSource) Metadata(
	_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_restores"
}

func (d *restoresDataSource) Configure(
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

func (d *restoresDataSource) Read(
	ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse,
) {
	if d.provider == nil || !d.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var restores Restores
	diags := req.Config.Get(ctx, &restores)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddWarning("Error loading restore jobs", "")
		return
	}

	clusterID := restores.ClusterID.ValueString()
	listRestoreOptions := &client.ListRestoresOptions{
		PaginationSortOrder: restores.SortOrder.ValueStringPointer(),
		PaginationLimit:     restores.Limit.ValueInt32Pointer(),
	}
	if !restores.StartTime.IsNull() {
		startTime, err := parseFlexibleTime(restores.StartTime.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid start_time",
				fmt.Sprintf("could not parse start_time: %v", err))
			return
		}
		listRestoreOptions.StartTime = &startTime
	}
	if !restores.EndTime.IsNull() {
		endTime, err := parseFlexibleTime(restores.EndTime.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid end_time",
				fmt.Sprintf("could not parse end_time: %v", err))
			return
		}
		listRestoreOptions.EndTime = &endTime
	}

	traceAPICall("ListRestores")
	restoresResp, _, err := d.provider.service.ListRestores(ctx, clusterID, listRestoreOptions)
	if err != nil || restoresResp == nil {
		resp.Diagnostics.AddError(
			"Error fetching restore jobs",
			fmt.Sprintf("Unexpected error while retrieving restore jobs: %v", formatAPIErrorMessage(err)))
		return
	}

	for _, restore := range restoresResp.Restores {
		summary := RestoreSummary{
			ID:                     types.StringValue(restore.Id),
			BackupID:               types.StringValue(restore.BackupId),
			Status:                 types.StringValue(string(restore.Status)),
			CreatedAt:              types.StringValue(restore.CreatedAt.String()),
			Type:                   types.StringValue(string(restore.Type)),
			CompletionPercent:      types.Float32Value(restore.CompletionPercent),
			SourceClusterName:      types.StringValue(restore.SourceClusterName),
			DestinationClusterName: types.StringValue(restore.DestinationClusterName),
			BackupEndTime:          types.StringValue(restore.BackupEndTime.String()),
		}

		if restore.CompletedAt != nil && !restore.CompletedAt.IsZero() {
			summary.CompletedAt = types.StringValue(restore.CompletedAt.String())
		}
		if restore.CrdbJobId != nil {
			summary.CrdbJobID = types.StringValue(*restore.CrdbJobId)
		}
		if restore.ClientErrorCode != nil {
			summary.ClientErrorCode = types.Int32Value(*restore.ClientErrorCode)
		}
		if restore.ClientErrorMessage != nil {
			summary.ClientErrorMessage = types.StringValue(*restore.ClientErrorMessage)
		}

		summary.Objects = convertObjectsToTerraform(restore.Objects)
		summary.RestoreOpts = convertRestoreOptsToTerraform(restore.RestoreOpts)

		restores.Restores = append(restores.Restores, summary)
	}

	diags = resp.State.Set(ctx, restores)
	resp.Diagnostics.Append(diags...)
}

func NewRestoresDataSource() datasource.DataSource {
	return &restoresDataSource{}
}

// makeRestoreItem creates a RestoreItem with proper null handling for tables.
func makeRestoreItem(database string, schema string, tables []string) RestoreItem {
	item := RestoreItem{
		Database: types.StringValue(database),
	}
	if schema != "" {
		item.Schema = types.StringValue(schema)
	}

	// Set Tables: use null for database-level restores, actual list for table-level restores
	if len(tables) > 0 {
		tableValues := make([]attr.Value, len(tables))
		for i, table := range tables {
			tableValues[i] = types.StringValue(table)
		}
		item.Tables, _ = types.ListValue(types.StringType, tableValues)
	} else {
		item.Tables = types.ListNull(types.StringType)
	}

	return item
}

// convertObjectsToTerraform converts API objects to Terraform RestoreItems.
// API returns objects in lexicographically sorted order, so we group consecutive items
// with the same database+schema to consolidate tables.
//
// Example:
// Input (from API):
//   [{Database: "db1", Schema: "public", Table: "orders"},
//    {Database: "db1", Schema: "public", Table: "users"},
//    {Database: "db2", Schema: "private", Table: "settings"}]
// Output (to Terraform):
//   [{Database: "db1", Schema: "public", Tables: ["orders", "users"]},
//    {Database: "db2", Schema: "private", Tables: ["settings"]}]
func convertObjectsToTerraform(objects *[]client.RestoreItem) []RestoreItem {
	if objects == nil || len(*objects) == 0 {
		return nil
	}

	var tfObjects []RestoreItem
	var groupDB, groupSchema string
	var groupTables []string

	for i, obj := range *objects {
		objSchema := ""
		if obj.Schema != nil {
			objSchema = *obj.Schema
		}

		// Check if this starts a new database+schema group
		if i > 0 && (obj.Database != groupDB || objSchema != groupSchema) {
			// Finalize the previous group
			tfObjects = append(tfObjects, makeRestoreItem(groupDB, groupSchema, groupTables))
			groupTables = nil
		}

		groupDB = obj.Database
		groupSchema = objSchema

		if obj.Table != nil {
			groupTables = append(groupTables, *obj.Table)
		}
	}

	// Add the last group to the list
	tfObjects = append(tfObjects, makeRestoreItem(groupDB, groupSchema, groupTables))
	return tfObjects
}

// convertRestoreOptsToTerraform converts API restore options to Terraform RestoreOpts.
func convertRestoreOptsToTerraform(opts *client.RestoreOpts) *RestoreOpts {
	if opts == nil {
		return nil
	}

	result := &RestoreOpts{}
	hasValue := false

	setString := func(src *string, dest *types.String) {
		if src != nil && *src != "" {
			*dest = types.StringValue(*src)
			hasValue = true
		}
	}

	setBool := func(src *bool, dest *types.Bool) {
		if src != nil && *src {
			*dest = types.BoolValue(*src)
			hasValue = true
		}
	}

	setString(opts.NewDbName, &result.NewDBName)
	setString(opts.IntoDb, &result.IntoDB)
	setBool(opts.SkipLocalitiesCheck, &result.SkipLocalitiesCheck)
	setBool(opts.SkipMissingForeignKeys, &result.SkipMissingForeignKeys)
	setBool(opts.SkipMissingSequences, &result.SkipMissingSequences)
	setBool(opts.SkipMissingViews, &result.SkipMissingViews)
	setBool(opts.SchemaOnly, &result.SchemaOnly)

	if !hasValue {
		return nil
	}
	return result
}
