/*
Copyright 2026 The Cockroach Authors

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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v9/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

var clusterRuntimeScanningAttributes = map[string]schema.Attribute{
	"id": schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Cluster ID. Matches `cluster_id`; present to uniquely identify the resource.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	},
	"cluster_id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Cluster ID.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	},
	"type": schema.StringAttribute{
		Required: true,
		MarkdownDescription: "The runtime scanning (HBTD, host-based threat detection) provider to enable on the cluster.\n" +
			"  - Set to `WIZ` to enable the Wiz runtime scanning sensor.\n\nAllowed values are:" +
			formatEnumMarkdownList(enableableRuntimeScanningTypes),
		Validators: []validator.String{stringvalidator.OneOf(enableableRuntimeScanningTypes...)},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	},
}

type clusterRuntimeScanningResource struct {
	provider *provider
}

func (r *clusterRuntimeScanningResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Enable per-cluster runtime scanning (HBTD, host-based threat detection) enforcement. " +
			"Creating this resource enables runtime scanning on the cluster; destroying it disables scanning.",
		Attributes: clusterRuntimeScanningAttributes,
	}
}

func (r *clusterRuntimeScanningResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_cluster_runtime_scanning"
}

func (r *clusterRuntimeScanningResource) Configure(
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

func (r *clusterRuntimeScanningResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var runtimeScanning ClusterRuntimeScanning
	diags := req.Plan.Get(ctx, &runtimeScanning)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := runtimeScanning.ClusterID.ValueString()
	traceAPICall("GetCluster")
	cluster, _, err := r.provider.service.GetCluster(ctx, clusterID)
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
			"Runtime scanning (HBTD) is only available for dedicated clusters",
		)
		return
	}

	r.enableRuntimeScanning(ctx, &resp.State, &resp.Diagnostics, runtimeScanning, cluster)
}

func (r *clusterRuntimeScanningResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state ClusterRuntimeScanning
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ClusterID.ValueString()
	traceAPICall("GetClusterRuntimeScanning")
	runtimeScanningInfo, httpResp, err := r.provider.service.GetClusterRuntimeScanning(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Runtime scanning configuration not found",
				fmt.Sprintf("Runtime scanning configuration for cluster ID %s not found. Runtime scanning will be removed from state.", clusterID))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error getting runtime scanning configuration",
			fmt.Sprintf("Unexpected error retrieving runtime scanning configuration: %s", formatAPIErrorMessage(err)))
		return
	}

	// A NONE type means runtime scanning is disabled on the cluster, which is
	// equivalent to the resource no longer existing.
	if runtimeScanningInfo.GetType() == client.RUNTIMESCANNINGTYPE_NONE {
		resp.Diagnostics.AddWarning(
			"Runtime scanning disabled",
			fmt.Sprintf("Runtime scanning for cluster ID %s is disabled. Runtime scanning will be removed from state.", clusterID))
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(clusterID)
	state.Type = types.StringValue(string(runtimeScanningInfo.GetType()))
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *clusterRuntimeScanningResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var runtimeScanning ClusterRuntimeScanning
	diags := req.Plan.Get(ctx, &runtimeScanning)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Both mutable attributes (cluster_id and type) are RequiresReplace, so Terraform
	// never actually calls Update: any change is a destroy+create that runs Create's
	// dedicated-cluster compatibility check. This method exists only to satisfy the
	// resource interface and is a defensive no-op-equivalent that re-enables with the
	// planned type. The empty cluster is populated by the readiness wait if the enable
	// hits a locked cluster.
	r.enableRuntimeScanning(ctx, &resp.State, &resp.Diagnostics, runtimeScanning, &client.Cluster{})
}

func (r *clusterRuntimeScanningResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var runtimeScanning ClusterRuntimeScanning
	diags := req.State.Get(ctx, &runtimeScanning)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := runtimeScanning.ClusterID.ValueString()
	cluster := &client.Cluster{}
	err := retry.RetryContext(ctx, clusterUpdateTimeout,
		retryDisableClusterRuntimeScanning(ctx, r.provider.service, clusterID, cluster))
	if err != nil {
		resp.Diagnostics.AddError("Error disabling runtime scanning", err.Error())
		return
	}

	resp.State.RemoveResource(ctx)
}

// retryDisableClusterRuntimeScanning disables runtime scanning, treating an
// already-gone config/cluster (404) as success and retrying while the cluster is
// busy (see handleClusterRuntimeScanningRetryError) by waiting for it to become ready.
func retryDisableClusterRuntimeScanning(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	cluster *client.Cluster,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("DisableClusterRuntimeScanning")
		_, httpResp, err := cl.DisableClusterRuntimeScanning(ctx, clusterID)
		if err != nil {
			// The cluster or its runtime scanning config is already gone; treat the
			// delete as successful.
			if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
				return nil
			}
			return handleClusterRuntimeScanningRetryError(
				ctx, cl, clusterID, cluster, httpResp, formatAPIErrorMessage(err), "disable")
		}
		return nil
	}
}

func (r *clusterRuntimeScanningResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	// The import ID is the cluster ID, which is also this resource's id.
	resource.ImportStatePassthroughID(ctx, path.Root("cluster_id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func NewClusterRuntimeScanningResource() resource.Resource {
	return &clusterRuntimeScanningResource{}
}

// enableRuntimeScanning enables runtime scanning and writes the result to state.
// The caller is responsible for cluster-type validation; the passed cluster is used
// only by the retry-on-locked readiness wait (an empty cluster is acceptable).
func (r *clusterRuntimeScanningResource) enableRuntimeScanning(
	ctx context.Context, state *tfsdk.State, diags *diag.Diagnostics, runtimeScanning ClusterRuntimeScanning, cluster *client.Cluster,
) {
	clusterID := runtimeScanning.ClusterID.ValueString()
	requestedType := client.RuntimeScanningType(runtimeScanning.Type.ValueString())
	enableBody := client.NewEnableClusterRuntimeScanningBody(requestedType)
	apiObj := &client.RuntimeScanningInfo{}
	err := retry.RetryContext(ctx, clusterUpdateTimeout,
		retryEnableClusterRuntimeScanning(ctx, r.provider.service, clusterID, cluster, enableBody, apiObj))
	if err != nil {
		diags.AddError("Error enabling runtime scanning", err.Error())
		return
	}

	runtimeScanning.ID = types.StringValue(clusterID)
	runtimeScanning.Type = types.StringValue(string(requestedType))
	diags.Append(state.Set(ctx, runtimeScanning)...)
	if diags.HasError() {
		return
	}

	// Enablement is asynchronous: EnableClusterRuntimeScanning returns the requested
	// type immediately, but GetClusterRuntimeScanning reflects NONE until the sensor
	// has actually rolled out. Wait until a read reports the requested type so that a
	// subsequent refresh/plan doesn't see a transient NONE and (incorrectly) flag the
	// resource as disabled/drifted. The wait only succeeds once the read matches
	// requestedType, so the state written above is already correct.
	readyInfo := &client.RuntimeScanningInfo{}
	if err := retry.RetryContext(ctx, clusterUpdateTimeout,
		waitForClusterRuntimeScanningReadyFunc(ctx, clusterID, requestedType, r.provider.service, readyInfo)); err != nil {
		diags.AddError(
			"Error enabling runtime scanning",
			fmt.Sprintf("Runtime scanning did not become active for cluster %s: %s", clusterID, err.Error()),
		)
		return
	}
}

// waitForClusterRuntimeScanningReadyFunc polls GetClusterRuntimeScanning until it
// reports the desired type (enablement/rollout is asynchronous). Server errors are
// retried; client errors are terminal.
func waitForClusterRuntimeScanningReadyFunc(
	ctx context.Context,
	clusterID string,
	desired client.RuntimeScanningType,
	cl client.Service,
	out *client.RuntimeScanningInfo,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("GetClusterRuntimeScanning")
		info, httpResp, err := cl.GetClusterRuntimeScanning(ctx, clusterID)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return retry.NonRetryableError(fmt.Errorf(
					"error getting runtime scanning status: %s", formatAPIErrorMessage(err)))
			}
			return retry.RetryableError(fmt.Errorf(
				"encountered a server error while reading runtime scanning status - trying again"))
		}
		*out = *info
		if info.GetType() != desired {
			return retry.RetryableError(fmt.Errorf(
				"runtime scanning is not %q yet", desired))
		}
		return nil
	}
}

// retryEnableClusterRuntimeScanning enables runtime scanning, retrying while the
// cluster is busy (see handleClusterRuntimeScanningRetryError) by waiting for it to
// become ready.
func retryEnableClusterRuntimeScanning(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	cluster *client.Cluster,
	body *client.EnableClusterRuntimeScanningBody,
	apiObj *client.RuntimeScanningInfo,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("EnableClusterRuntimeScanning")
		apiResp, httpResp, err := cl.EnableClusterRuntimeScanning(ctx, clusterID, body)
		if err != nil {
			return handleClusterRuntimeScanningRetryError(
				ctx, cl, clusterID, cluster, httpResp, formatAPIErrorMessage(err), "enable")
		}
		*apiObj = *apiResp
		return nil
	}
}

// handleClusterRuntimeScanningRetryError classifies an error from an enable/disable
// call:
//   - a busy cluster (per isClusterBusy: HTTP 503, a "lock" error, or
//     POLICY_UPDATING) waits for the cluster to become ready and then retries;
//   - a client error (HTTP 4xx) is terminal;
//   - a server error (HTTP 5xx) or a transport failure (nil response) is retried,
//     matching waitForClusterRuntimeScanningReadyFunc and handleEgressRetryError.
func handleClusterRuntimeScanningRetryError(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	cluster *client.Cluster,
	httpResp *http.Response,
	apiErrMsg string,
	operation string,
) *retry.RetryError {
	if isClusterBusy(httpResp, apiErrMsg) {
		// Wait for the cluster to be ready, then retry.
		clusterErr := retry.RetryContext(ctx, clusterUpdateTimeout,
			waitForClusterReadyFunc(ctx, clusterID, cl, cluster))
		if clusterErr != nil {
			return retry.NonRetryableError(
				fmt.Errorf("error checking cluster availability: %s", clusterErr.Error()))
		}
		return retry.RetryableError(fmt.Errorf("cluster was not ready - trying again"))
	}
	if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
		return retry.NonRetryableError(
			fmt.Errorf("could not %s runtime scanning: %v", operation, apiErrMsg))
	}
	// A 5xx or a transport-level failure (nil response) is transient; retry.
	return retry.RetryableError(
		fmt.Errorf("server error while trying to %s runtime scanning, retrying: %v", operation, apiErrMsg))
}

// enableableRuntimeScanningTypes are the runtime scanning types that can be enabled
// on a cluster, i.e. every allowed value except the NONE sentinel (which represents
// the disabled state and is expressed by removing the resource). Computed once and
// shared by the schema description and validator.
var enableableRuntimeScanningTypes = func() []string {
	values := make([]string, 0, len(client.AllowedRuntimeScanningTypeEnumValues))
	for _, v := range client.AllowedRuntimeScanningTypeEnumValues {
		if v == client.RUNTIMESCANNINGTYPE_NONE {
			continue
		}
		values = append(values, string(v))
	}
	return values
}()
