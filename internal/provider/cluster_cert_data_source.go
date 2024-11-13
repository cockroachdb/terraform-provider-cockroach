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
	"io"
	"net/http"
	"os"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v5/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusterCertDataSource struct {
	provider *provider
}

func (d *clusterCertDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "Cluster ID.",
			},
			"cert": schema.StringAttribute{
				Computed:    true,
				Description: "Certificate contents.",
			},
		},
		MarkdownDescription: "TLS certificate for the specified CockroachDB cluster. Certificates " +
			"for dedicated clusters should be written to `$HOME/Library/CockroachCloud/certs/<cluster name>-ca.crt` " +
			"on MacOS or Linux, or `$env:appdata\\CockroachCloud\\certs\\<cluster name>-ca.crt` on Windows. \n\n" +
			"Serverless clusters use the root PostgreSQL CA cert. If it isn't already installed, the certificate " +
			"can be appended to `$HOME/.postgresql/root.crt` on MacOS or Linux, or `$env:appdata\\postgresql\\root.crt` " +
			"on Windows.",
	}
}

func (d *clusterCertDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_cert"
}

func (d *clusterCertDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if d.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError("Internal provider error",
			fmt.Sprintf("Error in Configure: expected %T but got %T", provider{}, req.ProviderData))
	}
}

var DownloadClusterCert = func(cluster *client.Cluster, diags *diag.Diagnostics) []byte {
	serverURL := client.DefaultServerURL
	if value, found := os.LookupEnv(APIServerURLKey); found {
		serverURL = value
	}
	downloadURL := fmt.Sprintf("%s/clusters/%s/cert", serverURL, cluster.Id)
	httpResp, err := http.Get(downloadURL)
	if err != nil {
		diags.AddError("Couldn't download certificate", err.Error())
		return nil
	}
	certContents, err := io.ReadAll(httpResp.Body)
	if err != nil {
		diags.AddError("Error reading cert", err.Error())
		return nil
	}
	return certContents
}

func (d *clusterCertDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.provider == nil || !d.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var cert ClusterCert
	diags := req.Config.Get(ctx, &cert)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	traceAPICall("GetCluster")
	cluster, _, err := d.provider.service.GetCluster(ctx, cert.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to retrieve cluster info", formatAPIErrorMessage(err))
		return
	}

	certContents := DownloadClusterCert(cluster, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	cert.Cert = types.StringValue(string(certContents))

	diags = resp.State.Set(ctx, cert)
	resp.Diagnostics.Append(diags...)
}

func NewClusterCertDataSource() datasource.DataSource {
	return &clusterCertDataSource{}
}
