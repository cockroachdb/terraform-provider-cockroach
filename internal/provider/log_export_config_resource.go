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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	sdk_resource "github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

var logExportAttributes = map[string]schema.Attribute{
	"id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Cluster ID",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	},
	"auth_principal": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Either the AWS Role ARN that identifies a role that the cluster account can assume to write to CloudWatch or the GCP Project ID that the cluster service account has permissions to write to for cloud logging",
	},
	"log_name": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "An identifier for the logs in the customer's log sink",
	},
	"type": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The cloud selection that we're exporting to along with the cloud logging platform. Possible values are `GCP_CLOUD_LOGGING` or `AWS_CLOUDWATCH`",
	},
	"redact": schema.BoolAttribute{
		Optional:    true,
		Description: "Controls whether logs are redacted before forwarding to customer sinks",
	},
	"region": schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Controls whether all logs are sent to a specific region in the customer sink",
		Computed:            true,
	},
	"groups": schema.ListNestedAttribute{
		Optional: true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"channels": schema.ListAttribute{
					Required:            true,
					ElementType:         types.StringType,
					MarkdownDescription: "A list of CRDB log channels to include in this group",
				},
				"log_name": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: "The name of the group, reflected in the log sink",
				},
				"min_level": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "The minimum log level to filter to this log group",
				},
				"redact": schema.BoolAttribute{
					Optional:            true,
					MarkdownDescription: "Governs whether this log group should aggregate redacted logs if unset",
					Computed:            true,
				},
			},
		},
	},
	"status": schema.StringAttribute{
		Computed: true,
	},
	"user_message": schema.StringAttribute{
		Computed: true,
	},
	"created_at": schema.StringAttribute{
		Computed: true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	},
	"updated_at": schema.StringAttribute{
		Computed: true,
	},
}

type logExportConfigResource struct {
	provider *provider
}

func (r *logExportConfigResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Log Export Config Resource",
		Attributes:          logExportAttributes,
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

	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check cluster
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
			"Log export services are only available for dedicated clusters",
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

	if cluster.GetCloudProvider() == client.APICLOUDPROVIDER_AWS &&
		*configType != client.LOGEXPORTTYPE_AWS_CLOUDWATCH {
		resp.Diagnostics.AddError(
			"Incompatible log export type",
			fmt.Sprintf("For an AWS cluster, expected %s but got: %s",
				client.LOGEXPORTTYPE_AWS_CLOUDWATCH, plan.Type.ValueString()),
		)
		return
	}
	if cluster.GetCloudProvider() == client.APICLOUDPROVIDER_GCP &&
		*configType != client.LOGEXPORTTYPE_GCP_CLOUD_LOGGING {
		resp.Diagnostics.AddError(
			"Incompatible log export type",
			fmt.Sprintf("For a GCP cluster, expected %s but got: %s",
				client.LOGEXPORTTYPE_GCP_CLOUD_LOGGING, plan.Type.ValueString()),
		)
		return
	}

	logExportRequest := client.NewEnableLogExportRequestWithDefaults()
	if err = loadPlanIntoEnableLogExportRequest(plan, logExportRequest); err != nil {
		resp.Diagnostics.AddError(
			"Error preparing log export config",
			fmt.Sprintf("Invalid log export config: %s", err),
		)
		return
	}

	apiLogExportObj, _, err := r.provider.service.EnableLogExport(ctx, plan.ID.ValueString(), logExportRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling log export",
			fmt.Sprintf("Could not enable log export: %v", formatAPIErrorMessage(err)),
		)
		return
	}

	err = sdk_resource.RetryContext(ctx, clusterUpdateTimeout,
		waitForLogExportReadyFunc(ctx, plan.ID.ValueString(), r.provider.service, apiLogExportObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error enabling log export",
			fmt.Sprintf("Could not enable log export: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state ClusterLogExport
	loadLogExportIntoTerraformState(apiLogExportObj, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
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
			if apiGroup.GetMinLevel() == client.LOGLEVEL_LOG_LEVEL_UNSPECIFIED {
				groupMinLevel = types.StringNull()
			} else {
				groupMinLevel = types.StringValue(string(apiGroup.GetMinLevel()))
			}
			groups[group_idx] = LogExportGroup{
				LogName:  types.StringValue(apiGroup.GetLogName()),
				Channels: channels,
				MinLevel: groupMinLevel,
				Redact:   groupRedact,
			}
		}
	}

	var apiRedact types.Bool
	if spec.Redact == nil {
		apiRedact = types.BoolNull()
	} else {
		apiRedact = types.BoolValue(spec.GetRedact())
	}

	var apiRegion types.String
	if spec.Region == nil {
		apiRegion = types.StringNull()
	} else {
		apiRegion = types.StringValue(spec.GetRegion())
	}

	state.ID = types.StringValue(apiLogExportObj.GetClusterId())
	state.AuthPrincipal = types.StringValue(spec.GetAuthPrincipal())
	state.LogName = types.StringValue(spec.GetLogName())
	state.Type = types.StringValue(string(spec.GetType()))
	state.Redact = apiRedact
	state.Region = apiRegion
	state.Groups = &groups
	state.Status = types.StringValue(string(apiLogExportObj.GetStatus()))
	state.UserMessage = types.StringValue(apiLogExportObj.GetUserMessage())
	state.CreatedAt = types.StringValue(apiLogExportObj.GetCreatedAt().String())
	state.UpdatedAt = types.StringValue(apiLogExportObj.GetUpdatedAt().String())
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
	if resp.Diagnostics.HasError() || state.ID.IsNull() {
		return
	}
	clusterID := state.ID.ValueString()
	apiLogExportObj, httpResp, err := r.provider.service.GetLogExportInfo(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"log export config not found",
				fmt.Sprintf("log export config with cluster ID %s is not found. Removing from state.", clusterID))
			resp.State.RemoveResource(ctx)
		} else {
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

	logExportRequest := client.NewEnableLogExportRequestWithDefaults()
	if err := loadPlanIntoEnableLogExportRequest(plan, logExportRequest); err != nil {
		resp.Diagnostics.AddError(
			"Error preparing log export config update",
			fmt.Sprintf("Invalid log export config: %s", err),
		)
		return
	}

	apiLogExportObj, _, err := r.provider.service.EnableLogExport(ctx, plan.ID.ValueString(), logExportRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating log export config",
			fmt.Sprintf("Could not update log export config: %v", formatAPIErrorMessage(err)),
		)
		return
	}

	err = sdk_resource.RetryContext(ctx, clusterUpdateTimeout,
		waitForLogExportReadyFunc(ctx, plan.ID.ValueString(), r.provider.service, apiLogExportObj))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating log export config",
			fmt.Sprintf("Could not update log export config: %s", formatAPIErrorMessage(err)),
		)
		return
	}

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

	if !group.Redact.IsNull() && !group.Redact.IsUnknown() {
		clientGroup.SetRedact(group.Redact.ValueBool())
	}

	if group.MinLevel.IsNull() {
		clientGroup.SetMinLevel(client.LOGLEVEL_LOG_LEVEL_UNSPECIFIED)
	} else {
		minLevel, err := client.NewLogLevelFromValue(group.MinLevel.ValueString())
		if err != nil {
			return nil, err
		}
		clientGroup.SetMinLevel(*minLevel)
	}

	return &clientGroup, nil
}

func loadPlanIntoEnableLogExportRequest(
	plan ClusterLogExport, req *client.EnableLogExportRequest,
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

	req.SetAuthPrincipal(plan.AuthPrincipal.ValueString())
	req.SetLogName(plan.LogName.ValueString())
	if !plan.Redact.IsNull() && !plan.Redact.IsUnknown() {
		req.SetRedact(plan.Redact.ValueBool())
	}
	if !plan.Region.IsNull() && !plan.Redact.IsUnknown() {
		req.SetRegion(plan.Region.ValueString())
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
	_, httpResp, err := r.provider.service.DeleteLogExport(ctx, clusterID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			// Log export config or cluster is already gone. Swallow the error.
		} else {
			resp.Diagnostics.AddError(
				"Error deleting log export config",
				fmt.Sprintf("Could not delete log export config: %s", formatAPIErrorMessage(err)),
			)
			return
		}
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
) sdk_resource.RetryFunc {
	return func() *sdk_resource.RetryError {
		apiLogExport, httpResp, err := cl.GetLogExportInfo(ctx, clusterID)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return sdk_resource.NonRetryableError(fmt.Errorf("error getting log export info: %s", formatAPIErrorMessage(err)))
			} else {
				return sdk_resource.RetryableError(fmt.Errorf("encountered a server error while reading log export status - trying again"))
			}
		}

		*logExportClusterInfo = *apiLogExport
		switch logExportClusterInfo.GetStatus() {
		case client.LOGEXPORTSTATUS_DISABLE_FAILED, client.LOGEXPORTSTATUS_ENABLE_FAILED:
			return sdk_resource.NonRetryableError(fmt.Errorf("log export update failed"))
		case client.LOGEXPORTSTATUS_ENABLING, client.LOGEXPORTSTATUS_DISABLING:
			return sdk_resource.RetryableError(fmt.Errorf("log export is not ready yet"))
		default:
			return nil
		}
	}
}

func NewLogExportConfigResource() resource.Resource {
	return &logExportConfigResource{}
}
