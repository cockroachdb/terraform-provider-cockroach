/*
Copyright 2024 The Cockroach Authors

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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v4/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var backupConfigAttributes = map[string]schema.Attribute{
	"id": schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Cluster ID.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	},
	"enabled": schema.BoolAttribute{
		Required:    true,
		Description: "Indicates whether backups are enabled. If set to false, no backups will be created.",
	},
	"retention_days": schema.Int64Attribute{
		Optional:   true,
		Computed:   true,
		MarkdownDescription: "The number of days to retain backups for.  Valid values are [2, 7, 30, 90, 365]. Can only be set once, further changes require opening a support ticket. [See more](#retention_days-edit-restrictions).",
	},
	"frequency_minutes": schema.Int64Attribute{
		Optional:   true,
		Computed:   true,
		Description: "The frequency of backups in minutes.  Valid values are [5, 10, 15, 30, 60, 240, 1440]",
	},
}

type backupConfigResource struct {
	provider *provider
}

func NewBackupConfigResource() resource.Resource {
	return &backupConfigResource{}
}

func (r *backupConfigResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `The backup settings for a cluster.

Each cluster has backup settings that determine if backups are enabled,
how frequently they are taken, and how long they are retained for.
The ` + "`cockroach_backup_config`" + ` resource allows you to manage backup
settings.  It's important to note that the existence, or lack of this resource
does not indicate whether a backup configuration exists or not. If
the ` + "`cockroach_backup_config`" + ` resource exists, it indicates that Terraform is
managing the backup configuration.

As a result, removing the resource from the Terraform configuration will not
affect the settings in the backup configuration.

` + "### `retention_days`" + ` Edit Restrictions

` + "`retention_days`" + ` can only be set once. It is necessary to open a
support ticket to modify the setting again. For this reason, consider the
following when managing the value in Terraform:
* (Optional) Refrain from including ` + "`retention_days`" + ` in the Terraform resource to
rely on server-side management of the value instead.
* If the initial value for ` + "`retention_days`" + ` in
the ` + "`cockroach_backup_config`" + ` resource is the default value (i.e. 30), it will be
possible to modify the retention setting one more time.
* If the initial value set for ` + "`retention_days`" + ` in
the ` + "`cockroach_backup_config`" + ` resource is not the default, it will not be
possible to modify the retention setting again. Further modifications will
require a support ticket.

Changing the value of ` + "`retention_days`" + ` after using your one change will be a
multi-step operation. Here are two workflows that will work.  Both of these
options assume you already have a ` + "`cockroach_backup_config`" + ` resource
that can no longer be updated:

* Change it and then open a ticket before applying:
	1. Update ` + "`retention_days`" + ` to the new value in Terraform.
	1. Before applying the run, contact support to change the ` + "`retention_days`" + ` to the new value.
	1. Apply the changes in Terraform. A Terraform READ operation will complete,
	   recognize the existing value, and update the tf state.
* Temporarily remove management of the resource from Terraform, update it via
  ticket, and then add it back.
	1. Remove management of ` + "`retention_days`" + ` from the Terraform resource
	1. Run the apply. Nothing will change but Terraform is no longer managing that value.
	1. Open a support ticket to update ` + "`retention_days`" + ` to the new value.
	1. (Optional) Add ` + "`retention_days`" + ` back to the Terraform config with the
	   new value and apply the no-op update.
`,
		Attributes:          backupConfigAttributes,
	}
}

func (r *backupConfigResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_backup_config"
}

func (r *backupConfigResource) Configure(
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

func (r *backupConfigResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan ClusterBackupConfiguration

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ID.ValueString()

	traceAPICall("GetBackupConfiguration")
	remoteBackupConfig, _, err := r.provider.service.GetBackupConfiguration(ctx, clusterID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting backup configuration",
			fmt.Sprintf("Could not retrieve backup configuration: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var state ClusterBackupConfiguration
	loadBackupConfigIntoTerraformState(clusterID, remoteBackupConfig, &state)

	if state != plan {

		updateRequest := &client.UpdateBackupConfigurationSpec{
			Enabled: ptr(plan.Enabled.ValueBool()),
		}

		if !plan.RetentionDays.IsNull() && !plan.RetentionDays.IsUnknown() {
			updateRequest.RetentionDays = ptr(int32(plan.RetentionDays.ValueInt64()))
		}

		if !plan.FrequencyMinutes.IsNull() && !plan.FrequencyMinutes.IsUnknown() {
			updateRequest.FrequencyMinutes = ptr(int32(plan.FrequencyMinutes.ValueInt64()))
		}

		traceAPICall("UpdateBackupConfiguration")
		remoteBackupConfig, _, err := r.provider.service.UpdateBackupConfiguration(ctx, clusterID, updateRequest)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error getting backup configuration",
				fmt.Sprintf("Could not retrieve backup configuration: %s", formatAPIErrorMessage(err)),
			)
			return
		}

		loadBackupConfigIntoTerraformState(clusterID, remoteBackupConfig, &state)
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func loadBackupConfigIntoTerraformState(
	clusterID string, apiBackupConfig *client.BackupConfiguration, state *ClusterBackupConfiguration,
) {
	state.ID = types.StringValue(clusterID)
	state.Enabled = types.BoolValue(apiBackupConfig.GetEnabled())
	state.FrequencyMinutes = types.Int64Value(int64(apiBackupConfig.GetFrequencyMinutes()))
	state.RetentionDays = types.Int64Value(int64(apiBackupConfig.GetRetentionDays()))
}

func (r *backupConfigResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state ClusterBackupConfiguration
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || !IsKnown(state.ID) {
		return
	}
	clusterID := state.ID.ValueString()

	traceAPICall("GetBackupConfiguration")
	remoteBackupConfig, _, err := r.provider.service.GetBackupConfiguration(ctx, clusterID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting backup configuration",
			fmt.Sprintf("Could not retrieve backup configuration: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadBackupConfigIntoTerraformState(clusterID, remoteBackupConfig, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *backupConfigResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	// Get plan values
	var plan ClusterBackupConfiguration
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state ClusterBackupConfiguration
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ID.ValueString()

	updateRequest := &client.UpdateBackupConfigurationSpec{
		Enabled: ptr(plan.Enabled.ValueBool()),
	}

	if !plan.RetentionDays.IsNull() && !plan.RetentionDays.IsUnknown() {
		updateRequest.RetentionDays = ptr(int32(plan.RetentionDays.ValueInt64()))
	}

	if !plan.FrequencyMinutes.IsNull() && !plan.FrequencyMinutes.IsUnknown() {
		updateRequest.FrequencyMinutes = ptr(int32(plan.FrequencyMinutes.ValueInt64()))
	}

	traceAPICall("UpdateBackupConfiguration")
	remoteBackupConfig, _, err := r.provider.service.UpdateBackupConfiguration(ctx, clusterID, updateRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating backup configuration",
			fmt.Sprintf("Could not update backup configuration: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	loadBackupConfigIntoTerraformState(clusterID, remoteBackupConfig, &state)
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Delete
func (r *backupConfigResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	// Since the backup configuration is ever existing, we don't delete it.
	// Removing it from the terraform config means terraform will no longer
	// manage it.  In other words it will be left in the state it was before
	// removal.
	resp.State.RemoveResource(ctx)
}

func (r *backupConfigResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
