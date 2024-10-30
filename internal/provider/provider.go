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
	"os"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v5/pkg/client"
	"github.com/hashicorp/go-retryablehttp"
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
	ApiJWT types.String `tfsdk:"apijwt"`
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
	if !IsKnown(config.ApiKey) {
		apiKey = os.Getenv(CockroachAPIKey)
	} else {
		apiKey = config.ApiKey.ValueString()
	}

	var apiJWT string
	if !IsKnown(config.ApiJWT) {
		apiJWT = os.Getenv(CockroachAPIJWT)
	} else {
		apiJWT = config.ApiJWT.ValueString()
	}

	if apiKey == "" && apiJWT == "" {
		// Error vs warning - empty value must stop execution
		resp.Diagnostics.AddError(
			"Unable to find authentication token",
			"at least one of apikey or apijwt must be provided",
		)
		return
	}

	cfg := getClientConfiguration(apiKey, apiJWT)

	logLevel := os.Getenv("TF_LOG")
	if logLevel == "DEBUG" || logLevel == "TRACE" {
		cfg.Debug = true
	} else {
		logLevel = os.Getenv("TF_LOG_PROVIDER")
		if logLevel == "DEBUG" || logLevel == "TRACE" {
			cfg.Debug = true
		}
	}

	// retryablehttp gives us automatic retries with exponential backoff.
	httpClient := retryablehttp.NewClient()
	// The TF framework will pick up the default global logger.
	// HTTP requests are logged at DEBUG level.
	httpClient.Logger = &leveledTFLogger{baseCtx: ctx}
	httpClient.ErrorHandler = retryablehttp.PassthroughErrorHandler
	httpClient.CheckRetry = retryGetRequestsOnly
	cfg.HTTPClient = httpClient.StandardClient()

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
		NewPrivateEndpointTrustedOwnerResource,
		NewCMEKResource,
		NewDatabaseResource,
		NewFinalizeVersionUpgradeResource,
		NewLogExportConfigResource,
		NewMetricExportDatadogConfigResource,
		NewMetricExportCloudWatchConfigResource,
		NewMetricExportPrometheusConfigResource,
		NewClientCACertResource,
		NewUserRoleGrantsResource,
		NewUserRoleGrantResource,
		NewMaintenanceWindowResource,
		NewVersionDeferralResource,
		NewFolderResource,
		NewJWTIssuerResource,
		NewServiceAccountResource,
		NewAPIKeyResource,
	}
}

func (p *provider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewClusterDataSource,
		NewClusterCertDataSource,
		NewConnectionStringDataSource,
		NewFolderDataSource,
		NewOrganizationDataSource,
		NewPersonUserDataSource,
	}
}

func (p *provider) Schema(
	_ context.Context, _ tf_provider.SchemaRequest, resp *tf_provider.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"apikey": schema.StringAttribute{
				MarkdownDescription: "The API key to access CockroachDB Cloud.\n" +
					"If this field is provided, it is used and `apijwt` is ignored.",
				Optional:  true,
				Sensitive: true,
			},
			"apijwt": schema.StringAttribute{
				MarkdownDescription: "The JWT from a JWT Issuer configured for the " +
					"CockroachDB Cloud Organization.\n" +
					"In this case, the vanity name of the organization is required " +
					"and can be provided using the `COCKROACH_VANITY_NAME` environment variable. " +
					"If the JWT is mapped to multiple identities, the identity to " +
					"impersonate should be provided using the `COCKROACH_USERNAME` environment " +
					"variable, and should contain either a user email address or a " +
					"service account ID.",
				Optional:  true,
				Sensitive: true,
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

func getClientConfiguration(apiKey, apiJWT string) *client.Configuration {
	// If the API key is provided, use it, else use the JWT for auth.
	apiToken := apiKey
	if apiToken == "" {
		apiToken = apiJWT
	}

	var cfgOpts []client.ConfigurationOption
	if vanityName := os.Getenv(CockroachVanityName); vanityName != "" {
		cfgOpts = append(cfgOpts, client.WithVanityName(vanityName))
	}
	if username := os.Getenv(CockroachUsername); username != "" {
		cfgOpts = append(cfgOpts, client.WithUsername(username))
	}

	cfg := client.NewConfiguration(apiToken, cfgOpts...)
	if server := os.Getenv(APIServerURLKey); server != "" {
		cfg.ServerURL = server
	}
	cfg.UserAgent = UserAgent

	return cfg
}
