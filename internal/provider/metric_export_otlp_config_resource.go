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
	"strings"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v10/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

var (
	_ resource.Resource                = &metricExportOTLPConfigResource{}
	_ resource.ResourceWithConfigure   = &metricExportOTLPConfigResource{}
	_ resource.ResourceWithImportState = &metricExportOTLPConfigResource{}
)

type metricExportOTLPConfigResource struct {
	provider *provider
}

func NewMetricExportOTLPConfigResource() resource.Resource {
	return &metricExportOTLPConfigResource{}
}

func (r *metricExportOTLPConfigResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_metric_export_otlp_config"
}

func (r *metricExportOTLPConfigResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "OTLP metric export configuration for a dedicated cluster. Currently supports exporting to Amazon CloudWatch using SigV4 authentication. " +
			"The Cloud API marks this feature as limited access; the organization and cluster must have access to OTLP metric export.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:      true,
				Description:   "Cluster ID. Changing this value replaces the export configuration.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"cloudwatch": schema.SingleNestedAttribute{
				Required:    true,
				Description: "Amazon CloudWatch destination for OTLP metrics. This uses the CloudWatch OTLP ingestion API, separately from the existing CloudWatch metric export resource.",
				Attributes: map[string]schema.Attribute{
					"role_arn": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "The IAM role assumed to send metrics via SigV4. The role must trust Cockroach Cloud and grant `cloudwatch:PutMetricData`.",
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"target_region": schema.StringAttribute{
						Optional:    true,
						Description: "AWS region to export metrics to. It must support CloudWatch OTLP ingestion and PromQL. When omitted, metrics are exported to each cluster deployment region.",
						Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"external_id": schema.StringAttribute{
						Optional:    true,
						Description: "External ID included when assuming the IAM role. Set this if required by the role's trust policy.",
						Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
					},
				},
			},
			"status": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: fmt.Sprintf("Export deployment status: %#q. Create and update wait for `%s`.",
					client.AllowedMetricExportStatusTypeEnumValues, client.METRICEXPORTSTATUSTYPE_ENABLED),
			},
			"user_message": schema.StringAttribute{
				Computed:    true,
				Description: "Human-readable description of the export status, including hints for resolving errors.",
			},
		},
	}
}

func (r *metricExportOTLPConfigResource) Configure(
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

func (r *metricExportOTLPConfigResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}
	var plan ClusterOTLPMetricExportConfig
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiObj, err := r.enable(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error enabling OTLP metric export", err.Error())
		return
	}

	// Track the export as soon as the API accepts it, even if deployment later fails.
	var state ClusterOTLPMetricExportConfig
	loadOTLPMetricExportIntoTerraformState(apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err = retry.RetryContext(ctx, clusterUpdateTimeout,
		waitForOTLPMetricExportFunc(ctx, plan.ID.ValueString(), r.provider.service, apiObj, false))
	loadOTLPMetricExportIntoTerraformState(apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if err != nil {
		resp.Diagnostics.AddError("Error enabling OTLP metric export", err.Error())
	}
}

func (r *metricExportOTLPConfigResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}
	var plan ClusterOTLPMetricExportConfig
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiObj, err := r.enable(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating OTLP metric export", err.Error())
		return
	}
	var state ClusterOTLPMetricExportConfig
	loadOTLPMetricExportIntoTerraformState(apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err = retry.RetryContext(ctx, clusterUpdateTimeout,
		waitForOTLPMetricExportFunc(ctx, plan.ID.ValueString(), r.provider.service, apiObj, false))
	loadOTLPMetricExportIntoTerraformState(apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if err != nil {
		resp.Diagnostics.AddError("Error updating OTLP metric export", err.Error())
	}
}

func (r *metricExportOTLPConfigResource) enable(
	ctx context.Context, plan ClusterOTLPMetricExportConfig,
) (*client.OtlpMetricExportInfo, error) {
	cloudwatch := client.NewOtlpMetricExportCloudWatch(plan.CloudWatch.RoleArn.ValueString())
	if IsKnown(plan.CloudWatch.TargetRegion) {
		cloudwatch.SetTargetRegion(plan.CloudWatch.TargetRegion.ValueString())
	}
	if IsKnown(plan.CloudWatch.ExternalID) {
		cloudwatch.SetExternalId(plan.CloudWatch.ExternalID.ValueString())
	}
	body := client.NewEnableOtlpMetricExportBody()
	body.SetCloudwatch(*cloudwatch)
	var apiObj *client.OtlpMetricExportInfo
	err := retry.RetryContext(ctx, clusterUpdateTimeout,
		retryOTLPMetricExportOperation(ctx, plan.ID.ValueString(), r.provider.service, func() (*http.Response, error) {
			traceAPICall("EnableOtlpMetricExport")
			var httpResp *http.Response
			var err error
			apiObj, httpResp, err = r.provider.service.EnableOtlpMetricExport(ctx, plan.ID.ValueString(), body)
			return httpResp, err
		}))
	if err == nil && apiObj == nil {
		return nil, fmt.Errorf("the API returned no OTLP metric export configuration")
	}
	return apiObj, err
}

func (r *metricExportOTLPConfigResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}
	var state ClusterOTLPMetricExportConfig
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !IsKnown(state.ID) {
		return
	}
	traceAPICall("GetOtlpMetricExportInfo")
	apiObj, httpResp, err := r.provider.service.GetOtlpMetricExportInfo(ctx, state.ID.ValueString())
	if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
		resp.Diagnostics.AddWarning("OTLP metric export config not found",
			fmt.Sprintf("The OTLP metric export config for cluster ID %s was not found. Removing from Terraform state.", state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading OTLP metric export", formatAPIErrorMessage(err))
		return
	}
	if apiObj.GetStatus() == client.METRICEXPORTSTATUSTYPE_NOT_DEPLOYED {
		resp.Diagnostics.AddWarning("OTLP metric export is not deployed",
			fmt.Sprintf("The OTLP metric export for cluster ID %s is not deployed. Removing from Terraform state.", state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	loadOTLPMetricExportIntoTerraformState(apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *metricExportOTLPConfigResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}
	var state ClusterOTLPMetricExportConfig
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	alreadyGone := false
	err := retry.RetryContext(ctx, clusterUpdateTimeout,
		retryOTLPMetricExportOperation(ctx, state.ID.ValueString(), r.provider.service, func() (*http.Response, error) {
			traceAPICall("DeleteOtlpMetricExport")
			_, httpResp, err := r.provider.service.DeleteOtlpMetricExport(ctx, state.ID.ValueString())
			if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
				alreadyGone = true
				return httpResp, nil
			}
			return httpResp, err
		}))
	if err == nil && !alreadyGone {
		err = retry.RetryContext(ctx, clusterUpdateTimeout,
			waitForOTLPMetricExportFunc(ctx, state.ID.ValueString(), r.provider.service, &client.OtlpMetricExportInfo{}, true))
	}
	if err != nil {
		resp.Diagnostics.AddError("Error deleting OTLP metric export", err.Error())
		return
	}
	resp.State.RemoveResource(ctx)
}

func (r *metricExportOTLPConfigResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func loadOTLPMetricExportIntoTerraformState(apiObj *client.OtlpMetricExportInfo, state *ClusterOTLPMetricExportConfig) {
	state.ID = types.StringValue(apiObj.GetClusterId())
	state.Status = types.StringValue(string(apiObj.GetStatus()))
	state.UserMessage = types.StringValue(apiObj.GetUserMessage())
	state.CloudWatch = nil
	if apiObj.Cloudwatch != nil {
		state.CloudWatch = &ClusterOTLPMetricExportCloudWatchConfig{
			RoleArn:      types.StringValue(apiObj.Cloudwatch.GetRoleArn()),
			TargetRegion: types.StringNull(),
			ExternalID:   types.StringNull(),
		}
		if apiObj.Cloudwatch.GetTargetRegion() != "" {
			state.CloudWatch.TargetRegion = types.StringValue(apiObj.Cloudwatch.GetTargetRegion())
		}
		if apiObj.Cloudwatch.GetExternalId() != "" {
			state.CloudWatch.ExternalID = types.StringValue(apiObj.Cloudwatch.GetExternalId())
		}
	}
}

func retryOTLPMetricExportOperation(
	ctx context.Context, clusterID string, service client.Service, operation func() (*http.Response, error),
) retry.RetryFunc {
	return func() *retry.RetryError {
		httpResp, err := operation()
		if err == nil {
			return nil
		}
		message := formatAPIErrorMessage(err)
		if (httpResp != nil && httpResp.StatusCode == http.StatusServiceUnavailable) || strings.Contains(message, "lock") {
			if err := retry.RetryContext(ctx, clusterUpdateTimeout,
				waitForClusterReadyFunc(ctx, clusterID, service, &client.Cluster{})); err != nil {
				return retry.NonRetryableError(fmt.Errorf("error checking cluster availability: %w", err))
			}
			return retry.RetryableError(fmt.Errorf("cluster was not ready; retrying OTLP metric export operation"))
		}
		return retry.NonRetryableError(fmt.Errorf("OTLP metric export operation failed: %s", message))
	}
}

func waitForOTLPMetricExportFunc(
	ctx context.Context, clusterID string, service client.Service, result *client.OtlpMetricExportInfo, deleting bool,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("GetOtlpMetricExportInfo")
		apiObj, httpResp, err := service.GetOtlpMetricExportInfo(ctx, clusterID)
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			if deleting {
				return nil
			}
			return retry.RetryableError(fmt.Errorf("OTLP metric export configuration is not visible yet"))
		}
		if err != nil {
			if httpResp == nil || httpResp.StatusCode >= http.StatusInternalServerError {
				return retry.RetryableError(fmt.Errorf("error reading OTLP metric export status: %s", formatAPIErrorMessage(err)))
			}
			return retry.NonRetryableError(fmt.Errorf("error reading OTLP metric export status: %s", formatAPIErrorMessage(err)))
		}
		*result = *apiObj
		switch apiObj.GetStatus() {
		case client.METRICEXPORTSTATUSTYPE_ERROR:
			return retry.NonRetryableError(fmt.Errorf("OTLP metric export failed: %s", apiObj.GetUserMessage()))
		case client.METRICEXPORTSTATUSTYPE_ENABLED:
			if !deleting {
				return nil
			}
		case client.METRICEXPORTSTATUSTYPE_NOT_DEPLOYED:
			if deleting {
				return nil
			}
		case client.METRICEXPORTSTATUSTYPE_ENABLING, client.METRICEXPORTSTATUSTYPE_DISABLING:
		default:
			return retry.NonRetryableError(fmt.Errorf("unexpected OTLP metric export status %q", apiObj.GetStatus()))
		}
		return retry.RetryableError(fmt.Errorf("waiting for OTLP metric export deployment to finish (status: %s)", apiObj.GetStatus()))
	}
}
