/*
Copyright 2025 The Cockroach Authors

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
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

var egressTrafficPolicyStableTimeout = 10 * time.Minute

type egressTrafficPolicyResource struct {
	provider *provider
}

func NewEgressTrafficPolicyResource() resource.Resource {
	return &egressTrafficPolicyResource{}
}

func (r *egressTrafficPolicyResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the egress traffic policy for a CockroachDB Cloud cluster. When set to deny-by-default (allow_all = false), the cluster blocks all outbound traffic except to destinations explicitly allowed by egress rules (cockroach_egress_rule). When set to allow-all (allow_all = true), all outbound traffic is permitted and egress rules have no effect.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster ID.",
			},
			"allow_all": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "If true, all outbound connections from CockroachDB are allowed. If false, CockroachDB can only initiate network connections to destinations explicitly allowed by egress rules.",
			},
		},
	}
}

func (r *egressTrafficPolicyResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_egress_traffic_policy"
}

func (r *egressTrafficPolicyResource) Configure(
	_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	if r.provider, ok = req.ProviderData.(*provider); !ok {
		resp.Diagnostics.AddError("Internal provider error",
			fmt.Sprintf("Error in Configure: expected %T but got %T", provider{}, req.ProviderData))
	}
}

func (r *egressTrafficPolicyResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan EgressTrafficPolicy
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.applyEgressTrafficPolicy(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *egressTrafficPolicyResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state EgressTrafficPolicy
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ID.ValueString()

	traceAPICall("GetCluster")
	cluster, httpResp, err := r.provider.service.GetCluster(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Cluster not found",
				fmt.Sprintf("Cluster %s not found. Removing egress traffic policy from state.", clusterID),
			)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading cluster",
			fmt.Sprintf("Could not read cluster: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	state.AllowAll = types.BoolValue(cluster.GetEgressTrafficPolicy() == client.EGRESSTRAFFICPOLICYTYPE_ALLOW_ALL)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *egressTrafficPolicyResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan EgressTrafficPolicy
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.applyEgressTrafficPolicy(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *egressTrafficPolicyResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state EgressTrafficPolicy
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Reset to ALLOW_ALL (default state) - reuse apply but set allow_all = true
	state.AllowAll = types.BoolValue(true)
	r.applyEgressTrafficPolicy(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r *egressTrafficPolicyResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyEgressTrafficPolicy sets the egress traffic policy and waits for it to stabilize.
// This is shared logic used by Create, Update, and Delete operations.
func (r *egressTrafficPolicyResource) applyEgressTrafficPolicy(
	ctx context.Context,
	policy *EgressTrafficPolicy,
	diagnostics *diag.Diagnostics,
) {
	clusterID := policy.ID.ValueString()
	allowAll := policy.AllowAll.ValueBool()

	if err := retry.RetryContext(ctx, clusterUpdateTimeout,
		setEgressTrafficPolicyRetryFunc(ctx, r.provider.service, clusterID, allowAll),
	); err != nil {
		diagnostics.AddError(
			"Error setting egress traffic policy",
			fmt.Sprintf("Could not set egress traffic policy: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	if err := retry.RetryContext(ctx, egressTrafficPolicyStableTimeout,
		waitForEgressTrafficPolicyStableFunc(ctx, r.provider.service, clusterID),
	); err != nil {
		diagnostics.AddError(
			"Error waiting for egress traffic policy",
			fmt.Sprintf("Egress traffic policy did not stabilize: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// Read back the current state to confirm
	traceAPICall("GetCluster")
	cluster, _, err := r.provider.service.GetCluster(ctx, clusterID)
	if err != nil {
		diagnostics.AddError(
			"Error reading cluster",
			fmt.Sprintf("Could not read cluster: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	policy.AllowAll = types.BoolValue(cluster.GetEgressTrafficPolicy() == client.EGRESSTRAFFICPOLICYTYPE_ALLOW_ALL)
}

// setEgressTrafficPolicyRetryFunc returns a retry function that sets the egress traffic policy.
func setEgressTrafficPolicyRetryFunc(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	allowAll bool,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("SetEgressTrafficPolicy")
		httpResp, err := cl.SetEgressTrafficPolicy(ctx, clusterID, &client.SetEgressTrafficPolicyRequest{AllowAll: allowAll})
		if err != nil {
			apiErrMsg := formatAPIErrorMessage(err)
			if httpResp != nil {
				if httpResp.StatusCode == http.StatusServiceUnavailable ||
					strings.Contains(apiErrMsg, "lock") ||
					strings.Contains(apiErrMsg, "POLICY_UPDATING") {
					return retry.RetryableError(fmt.Errorf("cluster is busy, retrying: %s", apiErrMsg))
				}
				if httpResp.StatusCode < http.StatusInternalServerError {
					return retry.NonRetryableError(fmt.Errorf("error setting egress traffic policy: %s", apiErrMsg))
				}
			}
			return retry.RetryableError(fmt.Errorf("server error setting egress traffic policy, will retry: %s", apiErrMsg))
		}
		return nil
	}
}

// waitForEgressTrafficPolicyStableFunc returns a retry function that waits for the egress traffic policy to stabilize.
func waitForEgressTrafficPolicyStableFunc(
	ctx context.Context,
	cl client.Service,
	clusterID string,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("GetCluster")
		cluster, httpResp, err := cl.GetCluster(ctx, clusterID)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return retry.NonRetryableError(fmt.Errorf("error getting cluster: %s", formatAPIErrorMessage(err)))
			}
			return retry.RetryableError(fmt.Errorf("server error getting cluster, will retry: %s", formatAPIErrorMessage(err)))
		}

		policy := cluster.GetEgressTrafficPolicy()
		if policy == client.EGRESSTRAFFICPOLICYTYPE_UPDATING {
			return retry.RetryableError(fmt.Errorf("egress traffic policy is still updating"))
		}
		if policy == client.EGRESSTRAFFICPOLICYTYPE_ERROR {
			return retry.NonRetryableError(fmt.Errorf("egress traffic policy entered error state"))
		}
		return nil
	}
}
