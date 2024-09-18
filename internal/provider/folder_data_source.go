/*
Copyright 2024 The Cockroach Authors

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
	"strings"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v3/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &folderDataSource{}
	_ datasource.DataSourceWithConfigure = &folderDataSource{}
)

type folderDataSource struct {
	provider *provider
}

func (d *folderDataSource) Schema(
	_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "A CockroachDB Cloud folder. Folders can contain clusters or other folders.  They can be used to group resources together for the purposes of access control, organization or fine grained invoicing.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The id the folder.",
				Optional:            true,
				Validators:          uuidValidator,
			},
			"path": schema.StringAttribute{
				MarkdownDescription: "An absolute path to the folder. Trailing slashes are optional. (i.e. /folder1/folder2)",
				Optional:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the folder.",
				Computed:            true,
			},
			"parent_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the folders's parent folder. 'root' is used for a folder at the root level.",
			},
		},
	}
}

func (r *folderDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("path"),
			path.MatchRoot("id"),
		),
	}
}

func (d *folderDataSource) Metadata(
	_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_folder"
}

func (d *folderDataSource) Configure(
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

func (d *folderDataSource) Read(
	ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse,
) {
	if d.provider == nil || !d.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var folderDataSource FolderDataSourceModel
	diags := req.Config.Get(ctx, &folderDataSource)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddWarning("Error loading the folder", "")
		return
	}

	var folder *client.FolderResource
	if !folderDataSource.ID.IsNull() {
		folderID := folderDataSource.ID.ValueString()

		var httpResp *http.Response
		var err error
		traceAPICall("GetFolders")
		folder, httpResp, err = d.provider.service.GetFolder(ctx, folderID)
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Folder not found",
				fmt.Sprintf("Couldn't find a folder with ID %s", folderID))
			return
		}
		if err != nil {
			resp.Diagnostics.AddError(
				"Error fetching folder",
				fmt.Sprintf("Unexpected error while retrieving folder: %v", formatAPIErrorMessage(err)))
			return
		}

	} else if !folderDataSource.Path.IsNull() {
		path := folderDataSource.Path.ValueString()
		traceAPICall("ListFolders")
		apiResp, _, err := d.provider.service.ListFolders(ctx, &client.ListFoldersOptions{Path: &path})
		if err != nil {
			resp.Diagnostics.AddError(
				"Error fetching folders",
				fmt.Sprintf("Unexpected error while retrieving folder: %v", formatAPIErrorMessage(err)))
			return
		}
		if len(apiResp.Folders) == 0 {
			resp.Diagnostics.AddError(
				"Folder not found",
				fmt.Sprintf("Couldn't find a folder with path %s", path))
			return
		}
		folder = &apiResp.Folders[0]
	}

	folderDataSource.ID = types.StringValue(folder.ResourceId)
	folderDataSource.Name = types.StringValue(folder.Name)
	folderDataSource.ParentId = types.StringValue(folder.ParentId)
	if folderDataSource.Path.IsNull() {
		folderDataSource.Path = types.StringValue(buildFolderPathString(folder))
	}

	diags = resp.State.Set(ctx, folderDataSource)
	resp.Diagnostics.Append(diags...)
}

func buildFolderPathString(folder *client.FolderResource) string {
	var sb strings.Builder
	sb.WriteString("/")
	for _, segment := range folder.Path {
		sb.WriteString(*segment.Name)
		sb.WriteString("/")
	}
	sb.WriteString(folder.Name)
	return sb.String()
}

func NewFolderDataSource() datasource.DataSource {
	return &folderDataSource{}
}
