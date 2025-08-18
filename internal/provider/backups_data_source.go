package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type backupsDataSource struct {
	provider *provider
}

func (d *backupsDataSource) Schema(
	_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "Information about backups stored on a specific cluster.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Description: "The ID of the cluster containing the managed backups to be retrieved.",
				Required:    true,
				Validators:  uuidValidator,
			},
			"start_time": schema.StringAttribute{
				MarkdownDescription: "Beginning timestamp of the range (inclusive) used to search for backups based on their restore point. If this field is provided, end_time must also be included in the request. Uses a date format with optional timestamp, for example: `2023-01-15` or `2023-01-15T10:30:00Z`.",
				Optional:            true,
			},
			"end_time": schema.StringAttribute{
				MarkdownDescription: "Ending timestamp of the range (exclusive) used to search for backups based on their restore point. If this field is provided, start_time must also be included in the request. Uses a date format with optional timestamp, for example: `2023-01-15` or `2023-01-15T10:30:00Z`.",
				Optional:            true,
			},
			"sort_order": schema.StringAttribute{
				MarkdownDescription: "Specifies the sort direction for the returned results, which are ordered based on the `as_of_time` field. Use `ASC` for ascending or `DESC` for descending order.",
				Optional:            true,
			},
			"limit": schema.Int32Attribute{
				Description: "The maximum number of backups to return. If not set, only the first 500 backups will be returned.",
				Optional:    true,
			},
			"backups": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The unique identifier associated with the backup.",
							Computed:            true,
						},
						"as_of_time": schema.StringAttribute{
							MarkdownDescription: "The point in time (in UTC) the backup restores to.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *backupsDataSource) Metadata(
	_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_backups"
}

func (d *backupsDataSource) Configure(
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

func (d *backupsDataSource) Read(
	ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse,
) {
	if d.provider == nil || !d.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var backups Backups
	diags := req.Config.Get(ctx, &backups)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddWarning("Error loading backups", "")
		return
	}

	clusterID := backups.ClusterID.ValueString()
	listBackupOptions := &client.ListBackupsOptions{
		PaginationSortOrder: backups.SortOrder.ValueStringPointer(),
		PaginationLimit:     backups.Limit.ValueInt32Pointer(),
	}

	if !backups.StartTime.IsNull() {
		startTime, err := parseFlexibleTime(backups.StartTime.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid start_time",
				fmt.Sprintf("could not parse start_time: %v", err))
			return
		}
		listBackupOptions.StartTime = &startTime
	}
	if !backups.EndTime.IsNull() {
		endTime, err := parseFlexibleTime(backups.EndTime.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid end_time",
				fmt.Sprintf("could not parse end_time: %v", err))
			return
		}
		listBackupOptions.EndTime = &endTime
	}

	traceAPICall("ListBackups")
	backupsResp, _, err := d.provider.service.ListBackups(ctx, clusterID, listBackupOptions)
	if err != nil || backupsResp == nil {
		resp.Diagnostics.AddError(
			"Error fetching backups",
			fmt.Sprintf("Unexpected error while retrieving backups: %v", formatAPIErrorMessage(err)))
		return
	}

	for _, backup := range backupsResp.Backups {
		backups.Backups = append(backups.Backups, Backup{
			ID:       types.StringValue(backup.Id),
			AsOfTime: types.StringValue(backup.AsOfTime.Format(time.RFC3339)),
		})
	}

	diags = resp.State.Set(ctx, backups)
	resp.Diagnostics.Append(diags...)
}

func NewBackupsDataSource() datasource.DataSource {
	return &backupsDataSource{}
}
