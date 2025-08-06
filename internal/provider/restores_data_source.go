package provider

import (
	"context"
	"fmt"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
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
		restores.Restores = append(restores.Restores, RestoreSummary{
			ID:                types.StringValue(restore.Id),
			BackupID:          types.StringValue(restore.BackupId),
			Status:            types.StringValue(string(restore.Status)),
			CreatedAt:         types.StringValue(restore.CreatedAt.String()),
			Type:              types.StringValue(string(restore.Type)),
			CompletionPercent: types.Float32Value(restore.CompletionPercent),
		})
	}

	diags = resp.State.Set(ctx, restores)
	resp.Diagnostics.Append(diags...)
}

func NewRestoresDataSource() datasource.DataSource {
	return &restoresDataSource{}
}
