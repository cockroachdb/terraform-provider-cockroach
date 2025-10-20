package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type blackoutWindowDataSource struct {
	provider *provider
}

func NewBlackoutWindowDataSource() datasource.DataSource {
	return &blackoutWindowDataSource{}
}

func (d *blackoutWindowDataSource) Schema(
	_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "Retrieve blackout windows defined for a CockroachDB Cloud cluster.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "ID of the cluster to query for blackout windows.",
				Required:            true,
				Validators:          uuidValidator,
			},
			"page": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A pagination token used to request the next page of results. This value should come from the `next_page` attribute of a previous data source call.",
			},
			"sort_order": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Specifies the sort direction for the returned results, which are ordered based on the `start_time` field. Use `ASC` for ascending or `DESC` for descending order. Defaults to `ASC`.",
				Validators:          []validator.String{stringvalidator.OneOf("ASC", "DESC")},
			},
			"limit": schema.Int32Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum number of blackout windows to return in a single response. Defaults to 100 when not set.",
			},
			"next_page": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Pagination token for the next page of results returned by the API. Pass this value into the `page` argument in a subsequent data source call to retrieve the next set of blackout windows. Empty if there are no more pages.",
			},
			"blackout_windows": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of blackout windows returned by the API.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"cluster_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "ID of the cluster the blackout window belongs to.",
						},
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Unique blackout window identifier.",
						},
						"start_time": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "UTC start time in RFC3339 format (e.g. `2025-03-15T09:00:00Z`).",
						},
						"end_time": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "UTC end time in RFC3339 format (e.g. `2025-03-18T09:00:00Z`).",
						},
					},
				},
			},
		},
	}
}

func (d *blackoutWindowDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_blackout_windows"
}

func (d *blackoutWindowDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if d.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError(
			"Internal provider error",
			fmt.Sprintf("Error in Configure: expected %T but got %T", provider{}, req.ProviderData),
		)
	}
}

func (d *blackoutWindowDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.provider == nil || !d.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var listConfig BlackoutWindowList
	diags := req.Config.Get(ctx, &listConfig)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddWarning("Error loading the blackout window list", "")
		return
	}

	clusterID := listConfig.ClusterID.ValueString()

	listBlackoutWindowsOptions := &client.ListBlackoutWindowsOptions{}

	if IsKnown(listConfig.Limit) {
		listBlackoutWindowsOptions.PaginationLimit = listConfig.Limit.ValueInt32Pointer()
	}

	if IsKnown(listConfig.Page) {
		page := strings.TrimSpace(listConfig.Page.ValueString())
		if page != "" {
			listBlackoutWindowsOptions.PaginationPage = &page
		}
	}

	if IsKnown(listConfig.SortOrder) {
		sortOrder := strings.ToUpper(strings.TrimSpace(listConfig.SortOrder.ValueString()))
		if sortOrder != "" {
			listBlackoutWindowsOptions.PaginationSortOrder = &sortOrder
			listConfig.SortOrder = types.StringValue(sortOrder)
		}
	}

	traceAPICall("ListBlackoutWindows")
	listResp, _, err := d.provider.service.ListBlackoutWindows(ctx, clusterID, listBlackoutWindowsOptions)
	if err != nil || listResp == nil {
		resp.Diagnostics.AddError(
			"Error listing blackout windows",
			fmt.Sprintf("Unexpected error retrieving blackout windows: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	listConfig.BlackoutWindows = make([]BlackoutWindow, 0)
	if listResp.BlackoutWindows != nil {
		for _, window := range *listResp.BlackoutWindows {
			listConfig.BlackoutWindows = append(listConfig.BlackoutWindows, BlackoutWindow{
				ClusterID: types.StringValue(window.GetClusterId()),
				ID:        types.StringValue(window.GetId()),
				StartTime: types.StringValue(window.GetStartTime().UTC().Format(time.RFC3339)),
				EndTime:   types.StringValue(window.GetEndTime().UTC().Format(time.RFC3339)),
			})
		}
	}

	if listResp.Pagination != nil && listResp.Pagination.NextPage != nil && *listResp.Pagination.NextPage != "" {
		listConfig.NextPage = types.StringValue(*listResp.Pagination.NextPage)
	} else {
		listConfig.NextPage = types.StringNull()
	}

	diags = resp.State.Set(ctx, listConfig)
	resp.Diagnostics.Append(diags...)
}
