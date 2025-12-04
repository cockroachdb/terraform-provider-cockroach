package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

const createRestoreEndpointTimeout = time.Hour * 2

var errCreateRestorePending = errors.New("restore job is still pending")

type restoreResource struct {
	provider *provider
}

func NewRestoreResource() resource.Resource {
	return &restoreResource{}
}

func (r *restoreResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "CockroachDB Cloud restore. Used to start a restore job from a managed backup.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"destination_cluster_id": schema.StringAttribute{
				Description: "ID of the cluster where the backup will be restored.",
				Required:    true,
				Validators:  uuidValidator,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Type describes the scope of the restore job. When using a DATABASE or TABLE restore, additional details must be passed in the objects attribute. Allowed values are: `CLUSTER` or `DATABASE` or `TABLE`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf("CLUSTER", "DATABASE", "TABLE")},
			},
			"backup_id": schema.StringAttribute{
				Description: "ID of the managed backup from which data will be restored. If this value is not set, the restore job uses the most recent available backup on the source cluster.",
				Optional:    true,
				Computed:    true,
				Validators:  uuidValidator,
			},
			"source_cluster_id": schema.StringAttribute{
				Description: "ID of the source cluster containing the managed backup to be restored.",
				Optional:    true,
				Validators:  uuidValidator,
			},
			"restore_opts": schema.SingleNestedAttribute{
				Description: "Additional options controlling the behavior of the restore job.",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"new_db_name": schema.StringAttribute{
						Description: "Optionally specifies the name of a new database to create as the target of a database restore job. If not set, the name defaults to the original database name from the source cluster.",
						Optional:    true,
					},
					"into_db": schema.StringAttribute{
						Description: "Optionally specifies a target database to restore the table into during a table restore job. If not set, the table is restored into the database it belonged to in the source backup.",
						Optional:    true,
					},
					"skip_localities_check": schema.BoolAttribute{
						Description: "Allows the restore job to continue in the event that there are mismatched localities between the backup and target cluster. Useful when restoring multi-region tables to a cluster missing some localities.",
						Optional:    true,
					},
					"skip_missing_foreign_keys": schema.BoolAttribute{
						Description: "Allows a table to be restored even if it has foreign key constraints referencing rows that no longer exist in the target cluster.",
						Optional:    true,
					},
					"skip_missing_sequences": schema.BoolAttribute{
						MarkdownDescription: "Allows a table to be restored even if it contains a column whose `DEFAULT` value depends on a sequence. More information can be found [here](https://www.cockroachlabs.com/docs/stable/show-sequences).",
						Optional:            true,
					},
					"skip_missing_views": schema.BoolAttribute{
						Description: "Allows the job to skip restoring views that cannot be restored because their dependencies are not included in the current restore job.",
						Optional:    true,
					},
					"schema_only": schema.BoolAttribute{
						Description: "If set, only the schema without any user data is restored.",
						Optional:    true,
					},
				},
			},
			"status": schema.StringAttribute{
				Description: "The current status of the restore job.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "The time at which the restore job was initiated.",
				Computed:    true,
			},
			"completion_percent": schema.Float32Attribute{
				Description: "Decimal value showing the percentage of the restore job that has been completed. Value ranges from 0 to 1.",
				Computed:    true,
			},
			"source_cluster_name": schema.StringAttribute{
				Description: "The name of the cluster from which the backup was taken.",
				Computed:    true,
			},
			"destination_cluster_name": schema.StringAttribute{
				Description: "The name of the cluster to which the restore is being applied.",
				Computed:    true,
			},
			"backup_end_time": schema.StringAttribute{
				Description: "The timestamp at which the backup data was captured.",
				Computed:    true,
			},
			"completed_at": schema.StringAttribute{
				Description: "The timestamp at which the restore job completed.",
				Computed:    true,
			},
			"crdb_job_id": schema.StringAttribute{
				Description: "The CockroachDB internal job ID for the restore job.",
				Computed:    true,
			},
			"client_error_code": schema.Int32Attribute{
				Description: "Error code from the restore job, only populated if it has failed.",
				Computed:    true,
			},
			"client_error_message": schema.StringAttribute{
				Description: "Error message from the restore job, only populated if it has failed.",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"objects": schema.ListNestedBlock{
				MarkdownDescription: "The list of objects to restore. Required when the restore job type is `DATABASE` or `TABLE`.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"database": schema.StringAttribute{
							MarkdownDescription: "The database name in the fully qualified name of the objects to be restored. In the case of a job that restores `tpcc.public.warehouse`, this value would be `tpcc`.",
							Required:            true,
						},
						"schema": schema.StringAttribute{
							MarkdownDescription: "The schema name in the fully qualified name of the objects to be restored. In the case of a job that restores `tpcc.public.warehouse`, this value would be `public`.",
							Optional:            true,
						},
						"tables": schema.ListAttribute{
							MarkdownDescription: "The table name in the fully qualified name of the objects to be restored. In the case of a job that restores `tpcc.public.warehouse`, this would value be `warehouse`.",
							Optional:            true,
							ElementType:         types.StringType,
						},
					},
				},
			},
		},
	}
}

func (r *restoreResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_restore"
}

func (r *restoreResource) Configure(
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

func (r *restoreResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var plan Restore
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	createRestoreReq := &client.CockroachCloudCreateRestoreRequest{
		Type: client.RestoreTypeType(plan.Type.ValueString()),
	}

	if IsKnown(plan.BackupID) {
		createRestoreReq.BackupId = plan.BackupID.ValueStringPointer()
	}

	if IsKnown(plan.SourceClusterID) {
		createRestoreReq.SourceClusterId = plan.SourceClusterID.ValueStringPointer()
	}

	var objects []client.RestoreItem
	for _, object := range plan.Objects {
		// Convert the tables list to individual RestoreItem objects.
		var tables []types.String
		diags = object.Tables.ElementsAs(ctx, &tables, false)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		if len(tables) > 0 {
			// If tables are specified, create individual RestoreItem objects for each table.
			for _, table := range tables {
				objects = append(objects, client.RestoreItem{
					Database: object.Database.ValueString(),
					Schema:   object.Schema.ValueStringPointer(),
					Table:    table.ValueStringPointer(),
				})
			}
		} else {
			// If no tables are specified, create a single RestoreItem for the database.
			objects = append(objects, client.RestoreItem{
				Database: object.Database.ValueString(),
			})
		}
	}
	if len(objects) > 0 {
		createRestoreReq.Objects = &objects
	}

	var restoreOpts *client.RestoreOpts
	if plan.RestoreOpts != nil {
		restoreOpts = &client.RestoreOpts{}

		if IsKnown(plan.RestoreOpts.NewDBName) {
			restoreOpts.NewDbName = plan.RestoreOpts.NewDBName.ValueStringPointer()
		}
		if IsKnown(plan.RestoreOpts.IntoDB) {
			restoreOpts.IntoDb = plan.RestoreOpts.IntoDB.ValueStringPointer()
		}
		if IsKnown(plan.RestoreOpts.SkipLocalitiesCheck) {
			restoreOpts.SkipLocalitiesCheck = plan.RestoreOpts.SkipLocalitiesCheck.ValueBoolPointer()
		}
		if IsKnown(plan.RestoreOpts.SkipMissingForeignKeys) {
			restoreOpts.SkipMissingForeignKeys = plan.RestoreOpts.SkipMissingForeignKeys.ValueBoolPointer()
		}
		if IsKnown(plan.RestoreOpts.SkipMissingSequences) {
			restoreOpts.SkipMissingSequences = plan.RestoreOpts.SkipMissingSequences.ValueBoolPointer()
		}
		if IsKnown(plan.RestoreOpts.SkipMissingViews) {
			restoreOpts.SkipMissingViews = plan.RestoreOpts.SkipMissingViews.ValueBoolPointer()
		}
		if IsKnown(plan.RestoreOpts.SchemaOnly) {
			restoreOpts.SchemaOnly = plan.RestoreOpts.SchemaOnly.ValueBoolPointer()
		}

		createRestoreReq.RestoreOpts = restoreOpts
	}

	traceAPICall("CreateRestore")

	restoreObj, _, err := r.provider.service.CreateRestore(ctx, plan.DestinationClusterID.ValueString(), createRestoreReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating restore",
			fmt.Sprintf("Could not create restore: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	// Poll until restore job completes.
	err = retry.RetryContext(ctx, createRestoreEndpointTimeout,
		waitForRestoreReadyFunc(ctx, plan.DestinationClusterID.ValueString(), restoreObj.Id, r.provider.service, restoreObj))
	if err != nil {
		if errors.Is(err, errCreateRestorePending) {
			resp.Diagnostics.AddError("Restore job timed out", "The restore job has timed out but will continue to progress in the background.")
		} else {
			resp.Diagnostics.AddError("Restore job failed", formatAPIErrorMessage(err))
		}
		return
	}

	var state Restore
	loadRestoreToTerraformState(
		restoreObj,
		&state,
		&plan.SourceClusterID,
		&plan.DestinationClusterID,
	)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *restoreResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state Restore
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
	if state.ID.IsNull() {
		return
	}

	restoreID := state.ID.ValueString()
	clusterID := state.DestinationClusterID.ValueString()

	traceAPICall("GetRestore")
	restoreObj, httpResp, err := r.provider.service.GetRestore(ctx, clusterID, restoreID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Restore not found",
				fmt.Sprintf("Restore with ID %s is not found. Removing from state.", restoreID))
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Error getting restore info",
				fmt.Sprintf("Unexpected error retrieving restore info: %s", formatAPIErrorMessage(err)))
		}
		return
	}

	loadRestoreToTerraformState(restoreObj, &state, nil, nil)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *restoreResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	if !req.State.Raw.IsNull() && !req.Plan.Raw.Equal(req.State.Raw) {
		resp.Diagnostics.AddError(
			"Immutable resource update attempted",
			"This resource cannot be updated. Once a restore job is initiated, no changes are permitted.",
		)
	}
}

func (r *restoreResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	// The restore job cannot be cancelled or deleted, so this method is only
	// responsible for removing the resource state.
	resp.State.RemoveResource(ctx)
}

func (r *restoreResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.AddError(
		"Import not supported",
		"Restore resources cannot be imported. Restore jobs are one-time operations.",
	)
}

// loadRestoreToTerraformState populates the Terraform state from a client.Restore object
// and additional parameters. The pointer fields (sourceClusterId, destinationClusterId,
// objects, restoreOpts) are only set during Create operations, and remain unset on Read
// to prevent unintended state overwrites.
func loadRestoreToTerraformState(
	restoreObj *client.Restore,
	state *Restore,
	sourceClusterId *types.String,
	destinationClusterId *types.String,
) {
	state.ID = types.StringValue(restoreObj.Id)
	state.Type = types.StringValue(string(restoreObj.Type))
	state.BackupID = types.StringValue(restoreObj.BackupId)
	state.Status = types.StringValue(string(restoreObj.Status))
	state.CreatedAt = types.StringValue(restoreObj.CreatedAt.String())
	state.CompletionPercent = types.Float32Value(restoreObj.CompletionPercent)
	state.SourceClusterName = types.StringValue(restoreObj.SourceClusterName)
	state.DestinationClusterName = types.StringValue(restoreObj.DestinationClusterName)
	state.BackupEndTime = types.StringValue(restoreObj.BackupEndTime.String())

	if restoreObj.CompletedAt != nil && !restoreObj.CompletedAt.IsZero() {
		state.CompletedAt = types.StringValue(restoreObj.CompletedAt.String())
	}
	if restoreObj.CrdbJobId != nil {
		state.CrdbJobID = types.StringValue(*restoreObj.CrdbJobId)
	}
	if restoreObj.ClientErrorCode != nil {
		state.ClientErrorCode = types.Int32Value(*restoreObj.ClientErrorCode)
	}
	if restoreObj.ClientErrorMessage != nil {
		state.ClientErrorMessage = types.StringValue(*restoreObj.ClientErrorMessage)
	}

	state.Objects = convertObjectsToTerraform(restoreObj.Objects)
	state.RestoreOpts = convertRestoreOptsToTerraform(restoreObj.RestoreOpts)

	if sourceClusterId != nil {
		state.SourceClusterID = *sourceClusterId
	}
	if destinationClusterId != nil {
		state.DestinationClusterID = *destinationClusterId
	}
}

func waitForRestoreReadyFunc(
	ctx context.Context, clusterID string, restoreID string, cl client.Service, restore *client.Restore,
) retry.RetryFunc {
	return func() *retry.RetryError {
		traceAPICall("GetRestore")
		res, httpResp, err := cl.GetRestore(ctx, clusterID, restoreID)
		if err != nil {
			if httpResp != nil && httpResp.StatusCode < http.StatusInternalServerError {
				return retry.NonRetryableError(fmt.Errorf("error getting restore: %s", formatAPIErrorMessage(err)))
			} else {
				return retry.RetryableError(errors.New("encountered a server error while reading restore status - trying again"))
			}
		}
		*restore = *res

		switch status := restore.GetStatus(); status {
		case client.RESTORESTATUSTYPE_PENDING:
			return retry.RetryableError(errCreateRestorePending)
		case client.RESTORESTATUSTYPE_FAILED:
			if restore.ClientErrorMessage != nil && *restore.ClientErrorMessage != "" && restore.ClientErrorCode != nil {
				return retry.NonRetryableError(fmt.Errorf("restore job failed: %s (code: %d)", *restore.ClientErrorMessage, *restore.ClientErrorCode))
			}
			return retry.NonRetryableError(errors.New("restore job failed"))
		case client.RESTORESTATUSTYPE_SUCCESS:
			// The CompletedAt field is set by the API *after* the restore job is marked as successful, so we should check
			// that it is set before considering the restore ready.
			if restore.CompletedAt == nil || restore.CompletedAt.IsZero() {
				return retry.RetryableError(errors.New("restore succeeded but completed_at not yet set"))
			}
			return nil
		default:
			return nil
		}
	}
}
