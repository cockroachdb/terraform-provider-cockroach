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
	"net/url"
	"runtime"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const defaultDB = "defaultdb"

type connectionStringDataSource struct {
	provider *provider
}

func (d *connectionStringDataSource) Schema(
	_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "Cluster ID.",
			},
			"os": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "Used to determine the SSL certificate path for dedicated clusters. " +
					"Defaults to the Terraform user's OS. " +
					"Options are:" + formatEnumMarkdownList(client.AllowedOperatingSystemTypeEnumValues),
			},
			"database": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: fmt.Sprintf("Database to connect to. Defaults to '%s'.", defaultDB),
			},
			"sql_user": schema.StringAttribute{
				Optional:    true,
				Description: "Database username.",
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Database user password. Must also include `sql_user`.",
			},
			"connection_string": schema.StringAttribute{
				Computed:    true,
				Description: "Fully formatted connection string. Assumes the cluster certificate is stored in the default location.",
			},
			"connection_params": schema.MapAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "List of individual connection string parameters. Can be used to build nonstandard connection strings.",
			},
		},
		Description: "Generic connection string for a cluster.",
	}
}

func (d *connectionStringDataSource) Metadata(
	_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_connection_string"
}

func (d *connectionStringDataSource) Configure(
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

func (d *connectionStringDataSource) Read(
	ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse,
) {
	if d.provider == nil || !d.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var config ConnectionString
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddWarning("Error loading the connection string config", "")
		return
	}

	database := defaultDB
	if config.Database.IsNull() {
		config.Database = types.StringValue(database)
	} else {
		database = config.Database.ValueString()
	}
	var sqlUser *string
	if !config.SqlUser.IsNull() {
		sqlUser = new(string)
		*sqlUser = config.SqlUser.ValueString()
	}
	var os string
	if !config.OS.IsNull() {
		os = config.OS.ValueString()
	} else {
		switch runtime.GOOS {
		case "windows":
			os = "WINDOWS"
		case "darwin":
			os = "MAC"
		default:
			// Likely some other POSIX variant that we can treat like Linux.
			os = "LINUX"
		}
	}

	options := &client.GetConnectionStringOptions{
		Database: &database,
		SqlUser:  sqlUser,
		Os:       &os,
	}

	apiResp, _, err := d.provider.service.GetConnectionString(ctx, config.ID.ValueString(), options)
	if err != nil {
		resp.Diagnostics.AddError("Unable to retrieve cluster info", formatAPIErrorMessage(err))
		return
	}

	connectionString := apiResp.GetConnectionString()
	connectionParams := apiResp.GetParams()

	if !config.Password.IsNull() {
		connectionParams["Password"] = config.Password.ValueString()
		if connectionURL, err := url.Parse(connectionString); err != nil {
			resp.Diagnostics.AddWarning("Couldn't parse connection URL to inject password", err.Error())
		} else {
			connectionURL.User = url.UserPassword(config.SqlUser.ValueString(), config.Password.ValueString())
			connectionString = connectionURL.String()
		}
	}

	config.ConnectionString = types.StringValue(connectionString)
	config.ConnectionParams, diags = types.MapValueFrom(ctx, types.StringType, connectionParams)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, config)
	resp.Diagnostics.Append(diags...)
}

func NewConnectionStringDataSource() datasource.DataSource {
	return &connectionStringDataSource{}
}
