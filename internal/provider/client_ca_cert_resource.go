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
	"fmt"
	"net/http"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	sdk_resource "github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

const (
	clientCACertEndpointTimeout = time.Minute * 10 // TODO: what's a reasonable value here?
)

type clientCACertResource struct {
	provider *provider
}

func NewClientCACertResource() resource.Resource {
	return &clientCACertResource{}
}

func (r *clientCACertResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages client CA certs.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster ID",
			},
			"x509_pem_cert": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "X509 certificate in PEM format",
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Status of client CA certs on a cluster",
				Computed:            true,
			},
		},
	}
}

func (r *clientCACertResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_client_ca_cert"
}

func (r *clientCACertResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if r.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError("Internal provider error",
			fmt.Sprintf("Error in Configure: expected %T but got %T", provider{}, req.ProviderData))
	}
}

func (r *clientCACertResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	// Retrieve values from plan
	var plan ClientCACertResourceModel
	diags := req.Config.Get(ctx, &plan) // TODO: why is tfsdk.Config used here instead of tfsdk.Plan
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Ensure cluster is DEDICATED
	cluster, _, err := r.provider.service.GetCluster(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting cluster",
			fmt.Sprintf("Could not retrieve cluster info: %s", formatAPIErrorMessage(err)),
		)
		return
	}
	if cluster.Config.Serverless != nil {
		resp.Diagnostics.AddError(
			"Incompatible cluster type",
			"Client CA Cert management is only available for dedicated clusters",
		)
		return
	}

	// Generate API request from plan
	setClientCACertReq := client.NewSetClientCACertRequest(plan.X509PemCert.ValueString())

	certInfo, _, err := r.provider.service.SetClientCACert(ctx, plan.ID.ValueString(), setClientCACertReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating Client CA Cert",
			fmt.Sprintf("Error updating Client CA Cert: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// poll until update completes
	err = sdk_resource.RetryContext(ctx, clientCACertEndpointTimeout,
		waitForClientCACertReady(ctx, plan.ID.ValueString(), r.provider.service, certInfo))
	if err != nil {
		resp.Diagnostics.AddError(
			"Client CA Cert update may have failed",
			fmt.Sprintf("Client CA Cert update has exceeded timeout, and may have failed: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state ClientCACertResourceModel
	clientCACertInfoToTerraformState(certInfo, plan.ID.ValueString(), &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *clientCACertResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state ClientCACertResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterId := state.ID.ValueString()
	certInfo, httpResp, err := r.provider.service.GetClientCACert(ctx, clusterId)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Cluster not found",
				fmt.Sprintf("Cluster '%s' was not found. Removing from state.", clusterId))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting Client CA Cert info",
				fmt.Sprintf("Error getting Client CA Cert info: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	clientCACertInfoToTerraformState(certInfo, clusterId, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *clientCACertResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Get plan values
	var plan ClientCACertResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state ClientCACertResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only update cert if non-null.
	// Setting it to null will essentially cause TF to forget the field.
	if plan.X509PemCert.IsNull() {
		if !state.X509PemCert.IsNull() {
			resp.Diagnostics.AddWarning(
				"Client CA Cert will not be changed",
				"Setting the cert field to null will not clear the cert, it will only remove it from Terraform state. Delete the resource to unset a cert.",
			)
		}
	} else {
		newCert := plan.X509PemCert.ValueString()
		updateReq := client.UpdateClientCACertRequest{X509PemCert: &newCert}
		certInfo, _, err := r.provider.service.UpdateClientCACert(ctx, plan.ID.ValueString(), &updateReq)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating Client CA Cert",
				fmt.Sprintf("Error updating Client CA Cert: %s", formatAPIErrorMessage(err)),
			)
			return
		}

		// poll until update completes
		err = sdk_resource.RetryContext(ctx, clientCACertEndpointTimeout,
			waitForClientCACertReady(ctx, plan.ID.ValueString(), r.provider.service, certInfo))
		if err != nil {
			resp.Diagnostics.AddError(
				"Client CA Cert update may have failed",
				fmt.Sprintf("Client CA Cert update has exceeded timeout, and may have failed: %s", formatAPIErrorMessage(err)),
			)
			return
		}

		clientCACertInfoToTerraformState(certInfo, plan.ID.ValueString(), &state)
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *clientCACertResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ClientCACertResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, err := r.provider.service.DeleteClientCACert(ctx, state.ID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			// Already deleted, ignore error
		} else {
			resp.Diagnostics.AddError(
				"Error deleting Client CA Cert",
				fmt.Sprintf("Error deleting Client CA Cert: %s", formatAPIErrorMessage(err)),
			)
			return
		}
	}

	// TF will automatically remove the resource from state if there are no error diags.
	// https://developer.hashicorp.com/terraform/plugin/framework/resources/delete#recommendations
}

func waitForClientCACertReady(ctx context.Context, clusterId string, cl client.Service, certInfo *client.ClientCACertInfo) sdk_resource.RetryFunc {
	return func() *sdk_resource.RetryError {
		res, httpResp, err := cl.GetClientCACert(ctx, clusterId)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return sdk_resource.NonRetryableError(fmt.Errorf("error getting client ca cert: %s", formatAPIErrorMessage(err)))
			} else {
				return sdk_resource.RetryableError(fmt.Errorf("encountered a server error while reading client ca cert status - trying again"))
			}
		}

		*certInfo = *res
		switch status := certInfo.GetStatus(); status {
		case client.CLIENTCACERTSTATUS_PENDING:
			return sdk_resource.RetryableError(fmt.Errorf("client ca cert update is still pending"))
		case client.CLIENTCACERTSTATUS_FAILED:
			return sdk_resource.NonRetryableError(fmt.Errorf("client ca cert update failed"))
		default:
			return nil
		}
	}
}

func clientCACertInfoToTerraformState(certInfo *client.ClientCACertInfo, clusterId string, state *ClientCACertResourceModel) {
	state.ID = types.StringValue(clusterId) // not pulled from cert info since not available
	state.X509PemCert = types.StringValue(certInfo.GetX509PemCert())
	state.Status = types.StringValue(string(certInfo.GetStatus()))
}
