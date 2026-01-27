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
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

const (
	egressRuleTypeFQDN = "FQDN"
	egressRuleTypeCIDR = "CIDR"
)

var (
	egressRuleActiveTimeout  = 10 * time.Minute // Wait for rule to reach ACTIVE state
	egressRuleDeletedTimeout = 10 * time.Minute // Wait for rule deletion to complete
)

type egressPerimeterResource struct {
	provider *provider
}

func NewEgressPerimeterResource() resource.Resource {
	return &egressPerimeterResource{}
}

func (r *egressPerimeterResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Egress rule for a CockroachDB Cloud cluster. Egress perimeter controls allow CockroachDB Cloud Advanced clusters to restrict outgoing traffic to a defined set of external destinations.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique identifier for the egress rule.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the cluster to which this egress rule applies.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A name for the rule.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The intended purpose of egress to the destination.",
			},
			"type": schema.StringAttribute{
				Required: true,
				MarkdownDescription: fmt.Sprintf(
					"Whether the destination will be specified as a fully-qualified domain name (FQDN), or as a range of IP addresses specified in CIDR notation. Value is `%s` or `%s`.",
					egressRuleTypeFQDN, egressRuleTypeCIDR,
				),
				Validators: []validator.String{
					stringvalidator.OneOf(egressRuleTypeFQDN, egressRuleTypeCIDR),
				},
			},
			"destination": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Either a fully qualified domain name, for example www.cockroachlabs.com, or a CIDR range, for example, 123.45.67.89/32.",
			},
			"ports": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.Int32Type,
				MarkdownDescription: "An array of allowed ports, for example [44,8080].",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplaceIf(portsRequiresReplaceModifier,
						"Ports cannot be cleared via API update",
						"The CockroachDB Cloud API does not support clearing ports from an egress rule. The rule will be recreated.",
					),
				},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current state of the egress rule",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"crl_managed": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Indicates whether this egress rule is managed by CockroachDB Cloud services.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Timestamp when the egress rule was created",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *egressPerimeterResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_egress_rule"
}

func (r *egressPerimeterResource) Configure(
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

func (r *egressPerimeterResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan EgressRule
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ClusterID.ValueString()

	// Check if egress traffic policy is already set to deny-by-default
	traceAPICall("GetCluster")
	cluster, _, err := r.provider.service.GetCluster(ctx, clusterID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error retrieving cluster info",
			fmt.Sprintf("Could not retrieve cluster info: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// TODO: Add singleflight-like pattern to deduplicate concurrent SetEgressTrafficPolicy calls.
	// When multiple egress rules are created in parallel for the same cluster, each currently
	// calls GetCluster and potentially SetEgressTrafficPolicy independently. A proper implementation
	// would use sync.Map + sync.Cond (or golang.org/x/sync/singleflight) ensure only one goroutine does the I/O

	// Only set the policy if it's not already DEFAULT_DENY
	if cluster.GetEgressTrafficPolicy() != client.EGRESSTRAFFICPOLICYTYPE_DEFAULT_DENY {
		if err := retry.RetryContext(ctx, clusterUpdateTimeout,
			setEgressTrafficPolicyFunc(ctx, r.provider.service, clusterID),
		); err != nil {
			resp.Diagnostics.AddError(
				"Error setting egress traffic policy",
				fmt.Sprintf("Could not set egress traffic policy: %s", formatAPIErrorMessage(err)),
			)
			return
		}
	}

	// Build the request
	addReq := client.AddEgressRuleRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Type:        plan.Type.ValueString(),
		Destination: plan.Destination.ValueString(),
	}
	ports, portDiags := int32ListToSlice(ctx, plan.Ports)
	resp.Diagnostics.Append(portDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if ports != nil {
		addReq.Ports = &ports
	}

	// Retry create operation if cluster is locked
	var createdRule client.EgressRule
	if err := retry.RetryContext(ctx, clusterUpdateTimeout,
		createEgressRuleFunc(ctx, r.provider.service, clusterID, &addReq, &createdRule),
	); err != nil {
		resp.Diagnostics.AddError(
			"Error creating egress rule",
			fmt.Sprintf("Could not create egress rule: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// Wait for rule to become active
	var rule client.EgressRule
	if err := retry.RetryContext(ctx, egressRuleActiveTimeout,
		waitForEgressRuleActiveFunc(ctx, r.provider.service, clusterID, createdRule.GetId(), &rule),
	); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for egress rule",
			fmt.Sprintf("Egress rule did not become active: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state EgressRule
	loadEgressRuleIntoTerraformState(ctx, &rule, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *egressPerimeterResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state EgressRule
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ClusterID.ValueString()
	ruleID := state.ID.ValueString()

	traceAPICall("GetEgressRule")
	apiResp, httpResp, err := r.provider.service.GetEgressRule(ctx, clusterID, ruleID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Egress rule not found",
				"Egress rule could not be found. Removing from state.",
			)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading egress rule",
			fmt.Sprintf("Could not read egress rule: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	rule := apiResp.GetRule()
	loadEgressRuleIntoTerraformState(ctx, &rule, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *egressPerimeterResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan EgressRule
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state EgressRule
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ClusterID.ValueString()
	ruleID := state.ID.ValueString()

	// EditEgressRuleRequest uses pointer fields
	description := plan.Description.ValueString()
	ruleType := plan.Type.ValueString()
	destination := plan.Destination.ValueString()

	editReq := client.EditEgressRuleRequest{
		Description: &description,
		Type:        &ruleType,
		Destination: &destination,
	}

	// Convert ports using utility function
	ports, portDiags := int32ListToSlice(ctx, plan.Ports)
	resp.Diagnostics.Append(portDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if ports != nil {
		editReq.Ports = &ports
	}

	// Retry update operation if cluster is locked
	var updatedRule client.EgressRule
	if err := retry.RetryContext(ctx, clusterUpdateTimeout,
		updateEgressRuleFunc(ctx, r.provider.service, clusterID, ruleID, &editReq, &updatedRule),
	); err != nil {
		resp.Diagnostics.AddError(
			"Error updating egress rule",
			fmt.Sprintf("Could not update egress rule: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// Wait for rule to become active
	var rule client.EgressRule
	if err := retry.RetryContext(ctx, egressRuleActiveTimeout,
		waitForEgressRuleActiveFunc(ctx, r.provider.service, clusterID, updatedRule.GetId(), &rule),
	); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for egress rule",
			fmt.Sprintf("Egress rule did not become active after update: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadEgressRuleIntoTerraformState(ctx, &rule, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *egressPerimeterResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state EgressRule
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ClusterID.ValueString()
	ruleID := state.ID.ValueString()

	// Retry delete operation if cluster is locked
	if err := retry.RetryContext(ctx, clusterUpdateTimeout,
		deleteEgressRuleFunc(ctx, r.provider.service, clusterID, ruleID),
	); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting egress rule",
			fmt.Sprintf("Could not delete egress rule: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// Wait for rule to be deleted
	if err := retry.RetryContext(ctx, egressRuleDeletedTimeout,
		waitForEgressRuleDeletedFunc(ctx, r.provider.service, clusterID, ruleID),
	); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for egress rule deletion",
			fmt.Sprintf("Egress rule was not deleted: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r *egressPerimeterResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid egress rule ID format",
			`When importing an egress rule, the ID should follow the format "<cluster_id>:<rule_id>"`,
		)
		return
	}

	clusterID := parts[0]
	ruleID := parts[1]

	if clusterID == "" || ruleID == "" {
		resp.Diagnostics.AddError(
			"Invalid egress rule ID format",
			"Both cluster_id and rule_id must be non-empty",
		)
		return
	}

	// Set minimal state; Read will populate the rest
	// Note: Ports must be initialized with proper type info to avoid conversion errors
	state := EgressRule{
		ID:        types.StringValue(ruleID),
		ClusterID: types.StringValue(clusterID),
		Ports:     types.ListNull(types.Int32Type),
	}

	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func loadEgressRuleIntoTerraformState(
	ctx context.Context,
	rule *client.EgressRule,
	state *EgressRule,
	diagnostics *diag.Diagnostics,
) {
	state.ID = types.StringValue(rule.GetId())
	state.ClusterID = types.StringValue(rule.GetClusterId())
	state.Name = types.StringValue(rule.GetName())
	state.Description = types.StringValue(rule.GetDescription())
	state.Type = types.StringValue(rule.GetType())
	state.Destination = types.StringValue(rule.GetDestination())
	state.State = types.StringValue(rule.GetState())
	state.CrlManaged = types.BoolValue(rule.GetCrlManaged())

	state.CreatedAt = types.StringNull()
	if createdAt := rule.GetCreatedAt(); !createdAt.IsZero() {
		state.CreatedAt = types.StringValue(createdAt.Format(time.RFC3339))
	}

	// Convert ports from []int32 to types.List
	portsList, d := int32SliceToList(ctx, rule.GetPorts())
	diagnostics.Append(d...)
	state.Ports = portsList
}

// portsRequiresReplaceModifier determines if the ports attribute change requires resource replacement.
// The CockroachDB Cloud API does not support clearing ports from an egress rule via update,
// so we force replacement when ports go from non-empty to empty/null.
func portsRequiresReplaceModifier(
	ctx context.Context,
	req planmodifier.ListRequest,
	resp *listplanmodifier.RequiresReplaceIfFuncResponse,
) {
	// If state is null/unknown, this is a create - no replacement needed
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}

	// If plan has ports, this is not a "clear ports" operation
	if !req.PlanValue.IsNull() && len(req.PlanValue.Elements()) > 0 {
		return
	}

	// State has ports and plan is clearing them - force replacement
	if len(req.StateValue.Elements()) > 0 {
		resp.RequiresReplace = true
	}
}

// isClusterBusy returns true if the error indicates the cluster is busy and the operation should be retried.
func isClusterBusy(httpResp *http.Response, apiErrMsg string) bool {
	if httpResp == nil {
		return false
	}
	return httpResp.StatusCode == http.StatusServiceUnavailable ||
		strings.Contains(apiErrMsg, "lock") ||
		strings.Contains(apiErrMsg, "POLICY_UPDATING")
}

// handleEgressRetryError processes an API error and returns the appropriate retry error.
// It handles common patterns: cluster busy (retryable), client errors (non-retryable), server errors (retryable).
func handleEgressRetryError(httpResp *http.Response, apiErrMsg, operation string) *retry.RetryError {
	if httpResp != nil {
		if isClusterBusy(httpResp, apiErrMsg) {
			return retry.RetryableError(fmt.Errorf("cluster is busy, retrying: %s", apiErrMsg))
		}
		if httpResp.StatusCode < http.StatusInternalServerError {
			return retry.NonRetryableError(fmt.Errorf("error %s: %s", operation, apiErrMsg))
		}
	}
	return retry.RetryableError(fmt.Errorf("server error %s, will retry: %s", operation, apiErrMsg))
}

// setEgressTrafficPolicyFunc returns a retry function that sets the egress traffic policy to deny-by-default.
func setEgressTrafficPolicyFunc(
	ctx context.Context,
	cl client.Service,
	clusterID string,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("SetEgressTrafficPolicy")
		httpResp, err := cl.SetEgressTrafficPolicy(ctx, clusterID, &client.SetEgressTrafficPolicyRequest{AllowAll: false})
		if err != nil {
			return handleEgressRetryError(httpResp, formatAPIErrorMessage(err), "setting egress traffic policy")
		}
		return nil
	}
}

// createEgressRuleFunc returns a retry function that creates an egress rule.
func createEgressRuleFunc(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	addReq *client.AddEgressRuleRequest,
	createdRule *client.EgressRule,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("AddEgressRule")
		apiResp, httpResp, err := cl.AddEgressRule(ctx, clusterID, addReq)
		if err != nil {
			return handleEgressRetryError(httpResp, formatAPIErrorMessage(err), "creating egress rule")
		}
		*createdRule = apiResp.GetRule()
		return nil
	}
}

// updateEgressRuleFunc returns a retry function that updates an egress rule.
func updateEgressRuleFunc(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	ruleID string,
	editReq *client.EditEgressRuleRequest,
	updatedRule *client.EgressRule,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("EditEgressRule")
		apiResp, httpResp, err := cl.EditEgressRule(ctx, clusterID, ruleID, editReq)
		if err != nil {
			apiErrMsg := formatAPIErrorMessage(err)
			// 404 is a special case for update - rule was deleted externally
			if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
				return retry.NonRetryableError(fmt.Errorf("egress rule not found: %s", apiErrMsg))
			}
			return handleEgressRetryError(httpResp, apiErrMsg, "updating egress rule")
		}
		*updatedRule = apiResp.GetRule()
		return nil
	}
}

// handleGetEgressRuleError processes an error from GetEgressRule and returns the appropriate retry error.
func handleGetEgressRuleError(httpResp *http.Response, apiErrMsg, operation string) *retry.RetryError {
	if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
		return retry.NonRetryableError(fmt.Errorf("error %s: %s", operation, apiErrMsg))
	}
	return retry.RetryableError(fmt.Errorf("server error %s, will retry: %s", operation, apiErrMsg))
}

// waitForEgressRuleActiveFunc returns a retry function that waits for an egress rule to become active.
func waitForEgressRuleActiveFunc(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	ruleID string,
	rule *client.EgressRule,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("GetEgressRule")
		apiResp, httpResp, err := cl.GetEgressRule(ctx, clusterID, ruleID)
		if err != nil {
			return handleGetEgressRuleError(httpResp, formatAPIErrorMessage(err), "getting egress rule")
		}
		*rule = apiResp.GetRule()
		state := rule.GetState()
		if state == "ACTIVE" {
			return nil
		}
		if state == "FAILED" || state == "ERROR" {
			return retry.NonRetryableError(fmt.Errorf("egress rule entered terminal state: %s", state))
		}
		return retry.RetryableError(fmt.Errorf("egress rule is in state %s, waiting for ACTIVE", state))
	}
}

// deleteEgressRuleFunc returns a retry function that deletes an egress rule.
func deleteEgressRuleFunc(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	ruleID string,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("DeleteEgressRule")
		_, httpResp, err := cl.DeleteEgressRule(ctx, clusterID, ruleID, &client.DeleteEgressRuleOptions{})
		if err != nil {
			apiErrMsg := formatAPIErrorMessage(err)
			// 404 means already deleted - success
			if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
				return nil
			}
			return handleEgressRetryError(httpResp, apiErrMsg, "deleting egress rule")
		}
		return nil
	}
}

// waitForEgressRuleDeletedFunc returns a retry function that waits for an egress rule to be deleted.
func waitForEgressRuleDeletedFunc(
	ctx context.Context,
	cl client.Service,
	clusterID string,
	ruleID string,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("GetEgressRule")
		_, httpResp, err := cl.GetEgressRule(ctx, clusterID, ruleID)
		if err != nil {
			// 404 means deleted - success
			if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
				return nil
			}
			return handleGetEgressRuleError(httpResp, formatAPIErrorMessage(err), "checking egress rule deletion")
		}
		return retry.RetryableError(fmt.Errorf("egress rule still exists"))
	}
}
