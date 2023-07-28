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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type organizationDataSource struct {
	provider *provider
}

func (d *organizationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Information about the organization associated with the user's API key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Organization ID.",
			},
			"label": schema.StringAttribute{
				Computed:    true,
				Description: "A short ID used by CockroachDB Support.",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the organization.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Indicates when the organization was created.",
			},
		},
	}
}

func (d *organizationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (d *organizationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if d.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError("Internal provider error",
			fmt.Sprintf("Error in Configure: expected %T but got %T", provider{}, req.ProviderData))
	}
}

func (d *organizationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.provider == nil || !d.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	apiOrg, _, err := d.provider.service.GetOrganizationInfo(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error retrieving organization info", formatAPIErrorMessage(err))
		return
	}

	org := &Organization{
		ID:        types.StringValue(apiOrg.Id),
		Label:     types.StringValue(apiOrg.Label),
		Name:      types.StringValue(apiOrg.Name),
		CreatedAt: types.StringValue(apiOrg.CreatedAt.String()),
	}

	diags := resp.State.Set(ctx, org)
	resp.Diagnostics.Append(diags...)
}

func NewOrganizationDataSource() datasource.DataSource {
	return &organizationDataSource{}
}
