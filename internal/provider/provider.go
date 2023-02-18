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
	"os"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	tf_provider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// NewService overrides the client method for testing.
var NewService = client.NewService

// provider satisfies the tfsdk.Provider interface and usually is included
// with all Resource and DataSource implementations.
type provider struct {
	// client can contain the upstream provider SDK or HTTP client used to
	// communicate with the upstream service. Resource and DataSource
	// implementations can then make calls using this client.
	service client.Service

	// configured is set to true at the end of the Configure method.
	// This can be used in Resource and DataSource implementations to verify
	// that the provider was previously configured.
	configured bool

	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// providerData can be used to store data from the Terraform configuration.
type providerData struct {
	ApiKey types.String `tfsdk:"apikey"`
}

func (p *provider) Configure(
	ctx context.Context, req tf_provider.ConfigureRequest, resp *tf_provider.ConfigureResponse,
) {
	var config providerData
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var apiKey string
	if config.ApiKey.IsUnknown() {
		// cannot connect to client with an unknown value
		resp.Diagnostics.AddWarning(
			"Unable to create client",
			"Cannot use unknown value as apikey",
		)
		return
	}

	if config.ApiKey.IsNull() {
		apiKey = os.Getenv(CockroachAPIKey)
	} else {
		apiKey = config.ApiKey.ValueString()
	}

	if apiKey == "" {
		// Error vs warning - empty value must stop execution
		resp.Diagnostics.AddError(
			"Unable to find apikey",
			"apikey cannot be an empty string",
		)
		return
	}

	cfg := client.NewConfiguration(apiKey)
	if server := os.Getenv(APIServerURLKey); server != "" {
		cfg.ServerURL = server
	}
	cfg.UserAgent = UserAgent
	cl := client.NewClient(cfg)
	p.service = NewService(cl)
	resp.ResourceData = p
	resp.DataSourceData = p

	p.configured = true
}

func (p *provider) Metadata(
	_ context.Context, _ tf_provider.MetadataRequest, resp *tf_provider.MetadataResponse,
) {
	resp.TypeName = "cockroach"
	resp.Version = p.version
}

func (p *provider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewClusterResource,
		NewSQLUserResource,
		NewAllowlistResource,
		NewPrivateEndpointServicesResource,
		NewPrivateEndpointConnectionResource,
		NewCMEKResource,
		NewDatabaseResource,
		NewLogExportConfigResource,
	}
}

func (p *provider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewClusterDataSource,
		NewClusterCertDataSource,
		NewConnectionStringDataSource,
	}
}

func (p *provider) Schema(
	_ context.Context, _ tf_provider.SchemaRequest, resp *tf_provider.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"apikey": schema.StringAttribute{
				MarkdownDescription: "apikey to access cockroach cloud",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

func New(version string) func() tf_provider.Provider {
	return func() tf_provider.Provider {
		return &provider{
			version: version,
		}
	}
}
