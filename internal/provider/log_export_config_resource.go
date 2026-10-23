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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v10/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

var logExportAttributes = map[string]schema.Attribute{
	"id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Cluster ID.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	},
	"auth_principal": schema.StringAttribute{
		Optional: true,
		MarkdownDescription: "Either the AWS Role ARN that identifies a role that the cluster account can assume to write to CloudWatch, " +
			"the GCP Project ID that the cluster service account has permissions to write to for cloud logging, or the Azure workspace ID. " +
			"Required for AWS_CLOUDWATCH, GCP_CLOUD_LOGGING, and AZURE_LOG_ANALYTICS. Not used for OTLP_HTTP, which authenticates through otlp_headers_wo.",
	},
	"log_name": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "An identifier for the logs in the customer's log sink.",
	},
	"type": schema.StringAttribute{
		Required: true,
		MarkdownDescription: "The cloud selection being exported to along with the cloud logging platform. Possible values are:" +
			formatEnumMarkdownList(client.AllowedLogExportTypeEnumValues),
	},
	"redact": schema.BoolAttribute{
		Optional:    true,
		Description: "Controls whether logs are redacted before forwarding to customer sinks.",
	},
	"region": schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Controls whether all logs are sent to a specific region in the customer sink.",
	},
	"groups": schema.ListNestedAttribute{
		Optional: true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"channels": schema.ListAttribute{
					Required:            true,
					ElementType:         types.StringType,
					MarkdownDescription: "A list of CockroachDB log channels to include in this group.",
				},
				"log_name": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: "The name of the group, reflected in the log sink.",
				},
				"min_level": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "The minimum log level to filter to this log group.",
				},
				"redact": schema.BoolAttribute{
					Optional:            true,
					MarkdownDescription: "Governs whether this log group should aggregate redacted logs if unset.",
				},
				"enable_sending_queue": schema.BoolAttribute{
					Optional:            true,
					MarkdownDescription: "Enables the sending queue for logs in this group. Only one group can have enable_sending_queue enabled.",
				},
			},
		},
	},
	"status": schema.StringAttribute{
		Computed:    true,
		Description: "Encodes the possible states that a log export configuration can be in as it is created, deployed, and disabled.",
	},
	"user_message": schema.StringAttribute{
		Computed:    true,
		Description: "Elaborates on the log export status and hints at how to fix issues that may have occurred during asynchronous operations.",
	},
	"created_at": schema.StringAttribute{
		Computed: true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
		Description: "Indicates when log export was initially configured.",
	},
	"updated_at": schema.StringAttribute{
		Computed:    true,
		Description: "Indicates when the log export configuration was last updated.",
	},
	"omitted_channels": schema.ListAttribute{
		Optional:    true,
		ElementType: types.StringType,
		Description: "Controls what CRDB channels do not get exported.",
	},
	"aws_external_id": schema.StringAttribute{
		Optional:    true,
		Description: "The external ID to use when assuming the AWS role.",
	},
	"otlp_endpoint": schema.StringAttribute{
		Optional: true,
		MarkdownDescription: "The HTTPS OTLP/HTTP endpoint. Accepts either a base URL, such as `https://logs.example.com`, " +
			"or a full URL ending in `/v1/logs`, such as `https://logs.example.com/v1/logs`. " +
			"For a base URL, `/v1/logs` is appended when sending logs; for a full URL, the path is not appended again. " +
			"The API and Terraform state preserve the configured URL. Required when `type` is `OTLP_HTTP`; not supported for other export types.",
	},
	"otlp_headers_wo": schema.MapAttribute{
		Optional:    true,
		Sensitive:   true,
		WriteOnly:   true,
		ElementType: types.StringType,
		MarkdownDescription: "Write-only authentication headers for `OTLP_HTTP`, such as `authorization` (Terraform CLI 1.11+ required). " +
			"Values are never stored in Terraform plan or state. Headers are sent when the export is created, when `type` changes to `OTLP_HTTP`, " +
			"when `otlp_endpoint` changes, and when `otlp_headers_wo_version` changes. Other updates, and an empty or omitted map, leave stored headers unchanged. " +
			"To rotate, change the values and increment `otlp_headers_wo_version`; changing values alone does not trigger an update. " +
			"To remove stored headers, disable log export or switch to another export type.",
	},
	"otlp_headers_wo_version": schema.Int64Attribute{
		Optional: true,
		MarkdownDescription: "Trigger attribute for rotating `otlp_headers_wo`. Increment this integer to send the current `otlp_headers_wo` values. " +
			"Configure it together with `otlp_headers_wo`. Import does not recover this value, so configuring it after import sends the supplied headers.",
	},
}

type logExportConfigResource struct {
	provider *provider
}

func (r *logExportConfigResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Log Export configuration for a cluster.\n\n`OTLP_HTTP` exports directly to an OTLP/HTTP destination and requires " +
			"a dedicated cluster with OTLP/HTTP log export enabled for its organization and cluster. Contact support to request access. " +
			"The `enable_sending_queue` option is not supported for `OTLP_HTTP`.",
		Attributes: logExportAttributes,
	}
}

func (r *logExportConfigResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.RequiredTogether(path.MatchRoot("otlp_headers_wo"), path.MatchRoot("otlp_headers_wo_version")),
	}
}

func (r *logExportConfigResource) ValidateConfig(
	ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse,
) {
	var configType, authPrincipal, endpoint types.String
	var headers types.Map
	var headersVersion types.Int64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("type"), &configType)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("auth_principal"), &authPrincipal)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("otlp_endpoint"), &endpoint)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("otlp_headers_wo"), &headers)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("otlp_headers_wo_version"), &headersVersion)...)
	if resp.Diagnostics.HasError() || !IsKnown(configType) {
		return
	}

	if client.LogExportType(configType.ValueString()) == client.LOGEXPORTTYPE_OTLP_HTTP {
		if endpoint.IsNull() || (IsKnown(endpoint) && strings.TrimSpace(endpoint.ValueString()) == "") {
			resp.Diagnostics.AddAttributeError(path.Root("otlp_endpoint"), "Missing Attribute",
				"otlp_endpoint is required when type is OTLP_HTTP.")
		}
		if IsKnown(authPrincipal) {
			resp.Diagnostics.AddAttributeError(path.Root("auth_principal"), "Invalid Attribute Combination",
				"OTLP_HTTP authenticates through otlp_headers_wo, not auth_principal.")
		}
		for name, value := range headers.Elements() {
			if value.IsNull() {
				resp.Diagnostics.AddAttributeError(path.Root("otlp_headers_wo").AtMapKey(name), "Invalid Attribute Value",
					"otlp_headers_wo values must not be null.")
			}
		}
		var groups types.List
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("groups"), &groups)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for i, group := range groups.Elements() {
			if group.IsNull() || group.IsUnknown() {
				continue
			}
			if group.(types.Object).Attributes()["enable_sending_queue"].Equal(types.BoolValue(true)) {
				resp.Diagnostics.AddAttributeError(path.Root("groups").AtListIndex(i).AtName("enable_sending_queue"),
					"Invalid Attribute Combination", "enable_sending_queue is not supported for OTLP_HTTP.")
			}
		}
		return
	}

	if IsKnown(endpoint) {
		resp.Diagnostics.AddAttributeError(path.Root("otlp_endpoint"), "Invalid Attribute Combination",
			"otlp_endpoint can only be configured when type is OTLP_HTTP.")
	}
	if IsKnown(headers) {
		resp.Diagnostics.AddAttributeError(path.Root("otlp_headers_wo"), "Invalid Attribute Combination",
			"otlp_headers_wo can only be configured when type is OTLP_HTTP.")
	}
	if IsKnown(headersVersion) {
		resp.Diagnostics.AddAttributeError(path.Root("otlp_headers_wo_version"), "Invalid Attribute Combination",
			"otlp_headers_wo_version can only be configured when type is OTLP_HTTP.")
	}
	// Azure V2 uses its own authentication fields instead of auth_principal.
	switch client.LogExportType(configType.ValueString()) {
	case client.LOGEXPORTTYPE_AWS_CLOUDWATCH, client.LOGEXPORTTYPE_GCP_CLOUD_LOGGING, client.LOGEXPORTTYPE_AZURE_LOG_ANALYTICS:
		if authPrincipal.IsNull() || (IsKnown(authPrincipal) && strings.TrimSpace(authPrincipal.ValueString()) == "") {
			resp.Diagnostics.AddAttributeError(path.Root("auth_principal"), "Missing Attribute",
				"auth_principal is required for AWS_CLOUDWATCH, GCP_CLOUD_LOGGING, and AZURE_LOG_ANALYTICS.")
		}
	}
}

func (r *logExportConfigResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_log_export_config"
}

func (r *logExportConfigResource) Configure(
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

func (r *logExportConfigResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan ClusterLogExport

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	// Write-only values are available only in configuration, never in the plan.
	var headers types.Map
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("otlp_headers_wo"), &headers)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ID.ValueString()
	// Check cluster
	traceAPICall("GetCluster")
	cluster, _, err := r.provider.service.GetCluster(ctx, clusterID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting cluster",
			fmt.Sprintf("Could not retrieve cluster info: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	configType, err := client.NewLogExportTypeFromValue(plan.Type.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error preparing log export config",
			fmt.Sprintf("Invalid log export type: %s", err),
		)
		return
	}

	if cluster.GetCloudProvider() == client.CLOUDPROVIDERTYPE_AWS &&
		*configType != client.LOGEXPORTTYPE_AWS_CLOUDWATCH && *configType != client.LOGEXPORTTYPE_OTLP_HTTP {
		resp.Diagnostics.AddError(
			"Incompatible log export type",
			fmt.Sprintf("For an AWS cluster, expected %s or %s but got: %s",
				client.LOGEXPORTTYPE_AWS_CLOUDWATCH, client.LOGEXPORTTYPE_OTLP_HTTP, plan.Type.ValueString()),
		)
		return
	}
	if cluster.GetCloudProvider() == client.CLOUDPROVIDERTYPE_GCP &&
		*configType != client.LOGEXPORTTYPE_GCP_CLOUD_LOGGING && *configType != client.LOGEXPORTTYPE_OTLP_HTTP {
		resp.Diagnostics.AddError(
			"Incompatible log export type",
			fmt.Sprintf("For a GCP cluster, expected %s or %s but got: %s",
				client.LOGEXPORTTYPE_GCP_CLOUD_LOGGING, client.LOGEXPORTTYPE_OTLP_HTTP, plan.Type.ValueString()),
		)
		return
	}

	logExportRequest := client.NewEnableLogExportBodyWithDefaults()
	if err = loadPlanIntoEnableLogExportRequest(ctx, plan, headers, logExportRequest); err != nil {
		resp.Diagnostics.AddError(
			"Error preparing log export config",
			fmt.Sprintf("Invalid log export config: %s", err),
		)
		return
	}

	apiLogExportObj := &client.LogExportClusterInfo{}
	err = retry.RetryContext(ctx, clusterUpdateTimeout, retryEnableLogExport(
		ctx, clusterUpdateTimeout, r.provider.service,
		clusterID, cluster, logExportRequest, apiLogExportObj,
	))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling log export", err.Error(),
		)
		return
	}

	err = retry.RetryContext(ctx, clusterUpdateTimeout,
		waitForLogExportReadyFunc(ctx, clusterID, r.provider.service, apiLogExportObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling log export",
			fmt.Sprintf("Could not enable log export: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state ClusterLogExport
	state.OTLPHeadersWOVersion = plan.OTLPHeadersWOVersion
	loadLogExportIntoTerraformState(apiLogExportObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func retryEnableLogExport(
	ctx context.Context,
	timeout time.Duration,
	cl client.Service,
	clusterID string,
	cluster *client.Cluster,
	logExportRequest *client.EnableLogExportBody,
	apiLogExportObj *client.LogExportClusterInfo,
) retry.RetryFunc {
	iamHelper := newIAMRetryHelper()

	return func() *retry.RetryError {
		traceAPICall("EnableLogExport")
		apiResp, httpResp, err := cl.EnableLogExport(ctx, clusterID, logExportRequest)
		if err != nil {
			apiErrMsg := formatAPIErrorMessage(err)

			// Check for IAM eventual consistency errors
			if isRetryable, iamErr := iamHelper.checkRetryableCloudError(httpResp, apiErrMsg); isRetryable {
				return retry.RetryableError(fmt.Errorf("retrying: %s", apiErrMsg))
			} else if iamErr != nil {
				return retry.NonRetryableError(iamErr)
			}

			if (httpResp != nil && httpResp.StatusCode == http.StatusServiceUnavailable) ||
				strings.Contains(apiErrMsg, "lock") {
				// Wait for cluster to be ready.
				clusterErr := retry.RetryContext(ctx, clusterUpdateTimeout,
					waitForClusterReadyFunc(ctx, clusterID, cl, cluster))
				if clusterErr != nil {
					return retry.NonRetryableError(
						fmt.Errorf("error checking cluster availability: %s", clusterErr.Error()))
				}
				return retry.RetryableError(fmt.Errorf("cluster was not ready - trying again"))
			}

			return retry.NonRetryableError(
				fmt.Errorf("could not enable log export: %v", apiErrMsg),
			)
		}
		*apiLogExportObj = *apiResp
		return nil
	}
}

func loadLogExportIntoTerraformState(
	apiLogExportObj *client.LogExportClusterInfo, state *ClusterLogExport,
) {
	spec := apiLogExportObj.GetSpec()

	var groups []LogExportGroup
	if len(spec.GetGroups()) != 0 {
		groups = make([]LogExportGroup, len(spec.GetGroups()))
		for group_idx, apiGroup := range spec.GetGroups() {
			channels := make([]types.String, len(apiGroup.GetChannels()))
			for ch_idx, channel := range apiGroup.GetChannels() {
				channels[ch_idx] = types.StringValue(channel)
			}
			var groupRedact types.Bool
			if apiGroup.Redact == nil {
				groupRedact = types.BoolNull()
			} else {
				groupRedact = types.BoolValue(apiGroup.GetRedact())
			}
			var groupMinLevel types.String
			if apiGroup.GetMinLevel() == client.LOGLEVELTYPE_UNSPECIFIED {
				groupMinLevel = types.StringNull()
			} else {
				groupMinLevel = types.StringValue(string(apiGroup.GetMinLevel()))
			}
			var groupEnableSendingQueue types.Bool
			if apiGroup.EnableSendingQueue == nil {
				groupEnableSendingQueue = types.BoolNull()
			} else {
				groupEnableSendingQueue = types.BoolValue(apiGroup.GetEnableSendingQueue())
			}
			groups[group_idx] = LogExportGroup{
				LogName:            types.StringValue(apiGroup.GetLogName()),
				Channels:           channels,
				MinLevel:           groupMinLevel,
				Redact:             groupRedact,
				EnableSendingQueue: groupEnableSendingQueue,
			}
		}
	}

	var apiRedact types.Bool
	if spec.Redact == nil {
		apiRedact = types.BoolNull()
	} else {
		apiRedact = types.BoolValue(spec.GetRedact())
	}

	apiRegion := types.StringNull()
	if spec.Region != nil && *spec.Region != "" {
		apiRegion = types.StringValue(*spec.Region)
	}

	var omittedChannels []types.String
	for _, ochannel := range spec.GetOmittedChannels() {
		omittedChannels = append(omittedChannels, types.StringValue(ochannel))
	}

	awsExternalID := types.StringNull()
	if spec.AwsExternalId != nil && *spec.AwsExternalId != "" {
		awsExternalID = types.StringValue(*spec.AwsExternalId)
	}

	state.ID = types.StringValue(apiLogExportObj.GetClusterId())
	state.AuthPrincipal = types.StringNull()
	if spec.GetAuthPrincipal() != "" {
		state.AuthPrincipal = types.StringValue(spec.GetAuthPrincipal())
	}
	state.LogName = types.StringValue(spec.GetLogName())
	state.Type = types.StringValue(string(spec.GetType()))
	state.Redact = apiRedact
	state.Region = apiRegion
	state.Groups = &groups
	state.Status = types.StringValue(string(apiLogExportObj.GetStatus()))
	state.UserMessage = types.StringValue(apiLogExportObj.GetUserMessage())
	state.CreatedAt = types.StringValue(apiLogExportObj.GetCreatedAt().String())
	state.UpdatedAt = types.StringValue(apiLogExportObj.GetUpdatedAt().String())
	state.OmittedChannels = &omittedChannels
	state.AWSExternalID = awsExternalID
	state.OTLPEndpoint = types.StringNull()
	if spec.GetOtlpEndpoint() != "" {
		state.OTLPEndpoint = types.StringValue(spec.GetOtlpEndpoint())
	}
	state.OTLPHeadersWO = types.MapNull(types.StringType)
	if spec.GetType() != client.LOGEXPORTTYPE_OTLP_HTTP {
		state.OTLPHeadersWOVersion = types.Int64Null()
	}
}

func (r *logExportConfigResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state ClusterLogExport
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || !IsKnown(state.ID) {
		return
	}
	clusterID := state.ID.ValueString()
	traceAPICall("GetLogExportInfo")
	apiLogExportObj, httpResp, err := r.provider.service.GetLogExportInfo(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Log export config not found",
				fmt.Sprintf("The log export config with cluster ID %s is not found. Removing from state.", clusterID))
			resp.State.RemoveResource(ctx)
			return
		} else {
			// Check cluster existence.
			traceAPICall("GetCluster")
			cluster, clusterHttpResp, clusterErr := r.provider.service.GetCluster(ctx, clusterID)
			if clusterErr != nil {
				if clusterHttpResp != nil && clusterHttpResp.StatusCode == http.StatusNotFound {
					resp.Diagnostics.AddWarning(
						"Cluster not found",
						fmt.Sprintf("The log export config's cluster with ID %s is not found. Removing from state.",
							clusterID,
						))
					resp.State.RemoveResource(ctx)
					return
				} else {
					resp.Diagnostics.AddError(
						"Error getting cluster",
						fmt.Sprintf("Unexpected error retrieving the log export config's cluster: %s",
							formatAPIErrorMessage(clusterErr),
						))
				}
			}

			if cluster.State == client.CLUSTERSTATETYPE_DELETED {
				resp.Diagnostics.AddWarning(
					"Cluster deleted",
					fmt.Sprintf("The log export config's cluster with ID %s was deleted. Removing from state.",
						clusterID,
					))
				resp.State.RemoveResource(ctx)
				return
			}

			resp.Diagnostics.AddError(
				"Error getting log export info",
				fmt.Sprintf("Unexpected error retrieving log export info: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	loadLogExportIntoTerraformState(apiLogExportObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *logExportConfigResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	// Get plan values
	var plan ClusterLogExport
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state ClusterLogExport
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The API keeps stored headers when a request omits them, but rejects a
	// different scheme, host, or port without headers when any are stored.
	// Resend configured headers on endpoint or type changes as well as rotation.
	headers := types.MapNull(types.StringType)
	if client.LogExportType(plan.Type.ValueString()) == client.LOGEXPORTTYPE_OTLP_HTTP &&
		(!plan.Type.Equal(state.Type) || !plan.OTLPEndpoint.Equal(state.OTLPEndpoint) ||
			!plan.OTLPHeadersWOVersion.Equal(state.OTLPHeadersWOVersion)) {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("otlp_headers_wo"), &headers)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	logExportRequest := client.NewEnableLogExportBodyWithDefaults()
	if err := loadPlanIntoEnableLogExportRequest(ctx, plan, headers, logExportRequest); err != nil {
		resp.Diagnostics.AddError(
			"Error preparing log export config update",
			fmt.Sprintf("Invalid log export config: %s", err),
		)
		return
	}

	clusterID := plan.ID.ValueString()
	cluster := &client.Cluster{}
	apiLogExportObj := &client.LogExportClusterInfo{}
	err := retry.RetryContext(ctx, clusterUpdateTimeout, retryEnableLogExport(
		ctx, clusterUpdateTimeout, r.provider.service,
		clusterID, cluster, logExportRequest, apiLogExportObj,
	))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating log export config", err.Error(),
		)
		return
	}

	err = retry.RetryContext(ctx, clusterUpdateTimeout,
		waitForLogExportReadyFunc(ctx, clusterID, r.provider.service, apiLogExportObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating log export config",
			fmt.Sprintf("Could not update log export config: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	state.OTLPHeadersWOVersion = plan.OTLPHeadersWOVersion
	loadLogExportIntoTerraformState(apiLogExportObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func logExportGroupToClientGroup(group LogExportGroup) (*client.LogExportGroup, error) {
	channels := make([]string, len(group.Channels))
	for i, channel := range group.Channels {
		channels[i] = channel.ValueString()
	}

	clientGroup := client.LogExportGroup{
		LogName:  group.LogName.ValueString(),
		Channels: channels,
	}

	if IsKnown(group.Redact) {
		clientGroup.SetRedact(group.Redact.ValueBool())
	}

	if IsKnown(group.EnableSendingQueue) {
		clientGroup.SetEnableSendingQueue(group.EnableSendingQueue.ValueBool())
	}

	if !IsKnown(group.MinLevel) {
		clientGroup.SetMinLevel(client.LOGLEVELTYPE_UNSPECIFIED)
	} else {
		minLevel, err := client.NewLogLevelTypeFromValue(group.MinLevel.ValueString())
		if err != nil {
			return nil, err
		}
		clientGroup.SetMinLevel(*minLevel)
	}

	return &clientGroup, nil
}

func loadPlanIntoEnableLogExportRequest(
	ctx context.Context, plan ClusterLogExport, headers types.Map, req *client.EnableLogExportBody,
) error {
	if plan.Groups != nil {
		logExportGroups := make([]client.LogExportGroup, len(*plan.Groups))
		for i, group := range *plan.Groups {
			clientGroup, err := logExportGroupToClientGroup(group)
			if err != nil {
				return err
			}
			logExportGroups[i] = *clientGroup
		}
		req.SetGroups(logExportGroups)
	}

	if IsKnown(plan.AuthPrincipal) {
		req.SetAuthPrincipal(plan.AuthPrincipal.ValueString())
	}
	req.SetLogName(plan.LogName.ValueString())
	if IsKnown(plan.Redact) {
		req.SetRedact(plan.Redact.ValueBool())
	}
	if IsKnown(plan.Region) {
		req.SetRegion(plan.Region.ValueString())
	}
	if plan.OmittedChannels != nil {
		var omittedChannels []string
		for _, ochannel := range *plan.OmittedChannels {
			omittedChannels = append(omittedChannels, ochannel.ValueString())
		}
		req.SetOmittedChannels(omittedChannels)
	}
	if IsKnown(plan.AWSExternalID) {
		req.SetAwsExternalId(plan.AWSExternalID.ValueString())
	}
	if IsKnown(plan.OTLPEndpoint) {
		req.SetOtlpEndpoint(plan.OTLPEndpoint.ValueString())
	}
	if client.LogExportType(plan.Type.ValueString()) == client.LOGEXPORTTYPE_OTLP_HTTP && !headers.IsNull() {
		var values map[string]string
		if diags := headers.ElementsAs(ctx, &values, false); diags.HasError() {
			return fmt.Errorf("invalid otlp_headers_wo: %s", diags)
		}
		req.SetOtlpHeaders(values)
	}

	configType, err := client.NewLogExportTypeFromValue(plan.Type.ValueString())
	if err != nil {
		return err
	}
	req.SetType(*configType)
	return nil
}

// Delete
func (r *logExportConfigResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state ClusterLogExport
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ID.ValueString()
	cluster := &client.Cluster{}
	retryFunc := func() retry.RetryFunc {
		return func() *retry.RetryError {
			traceAPICall("DeleteLogExport")
			_, httpResp, err := r.provider.service.DeleteLogExport(ctx, clusterID)
			if err != nil {
				if httpResp != nil {
					if httpResp.StatusCode == http.StatusNotFound {
						// Log export config or cluster is already gone. Swallow the error.
						return nil
					}
					if httpResp.StatusCode == http.StatusServiceUnavailable {
						// Wait for cluster to be ready.
						clusterErr := retry.RetryContext(ctx, clusterUpdateTimeout,
							waitForClusterReadyFunc(ctx, clusterID, r.provider.service, cluster))
						if clusterErr != nil {
							return retry.NonRetryableError(
								fmt.Errorf("error checking cluster availability: %s", clusterErr.Error()))
						}
						return retry.RetryableError(fmt.Errorf("cluster was not ready - trying again"))
					}
				}
				return retry.NonRetryableError(
					fmt.Errorf("could not delete log export config: %v", formatAPIErrorMessage(err)))
			}
			return nil
		}
	}

	err := retry.RetryContext(ctx, clusterUpdateTimeout, retryFunc())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting log export config", err.Error(),
		)
		return
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r *logExportConfigResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func waitForLogExportReadyFunc(
	ctx context.Context,
	clusterID string,
	cl client.Service,
	logExportClusterInfo *client.LogExportClusterInfo,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("GetLogExportInfo")
		apiLogExport, httpResp, err := cl.GetLogExportInfo(ctx, clusterID)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return retry.NonRetryableError(fmt.Errorf("error getting log export info: %s", formatAPIErrorMessage(err)))
			} else {
				return retry.RetryableError(fmt.Errorf("encountered a server error while reading log export status - trying again"))
			}
		}

		*logExportClusterInfo = *apiLogExport
		switch logExportClusterInfo.GetStatus() {
		case client.LOGEXPORTSTATUS_DISABLE_FAILED, client.LOGEXPORTSTATUS_ENABLE_FAILED:
			return retry.NonRetryableError(fmt.Errorf("log export update failed"))
		case client.LOGEXPORTSTATUS_ENABLING, client.LOGEXPORTSTATUS_DISABLING:
			return retry.RetryableError(fmt.Errorf("log export is not ready yet"))
		default:
			return nil
		}
	}
}

func NewLogExportConfigResource() resource.Resource {
	return &logExportConfigResource{}
}
