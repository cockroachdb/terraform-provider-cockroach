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
	"regexp"
	"strconv"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

type databaseResource struct {
	provider *provider
}

// clusterID:name
const databaseIDFmt = "%s:%s"
const databaseNameRegex = "[A-Za-z0-9_][A-Za-z0-9\\._\\-]{0,62}"

var databaseIDRegex = regexp.MustCompile(fmt.Sprintf("^(%s):(%s)$", uuidRegex, databaseNameRegex))

func (r *databaseResource) Schema(
	_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Database",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"id": schema.StringAttribute{
				Computed: true,
			},
			"table_count": schema.Int64Attribute{
				Computed: true,
			},
		},
	}
}

func (r *databaseResource) Metadata(
	_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (r *databaseResource) Configure(
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

func (r *databaseResource) Create(
	ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var databaseSpec Database
	diags := req.Config.Get(ctx, &databaseSpec)
	resp.Diagnostics.Append(diags...)
	// Create a unique ID (required by terraform framework) by combining
	// the cluster ID and database name.
	databaseSpec.ID = types.StringValue(
		fmt.Sprintf(databaseIDFmt, databaseSpec.ClusterId.ValueString(), databaseSpec.Name.ValueString()),
	)

	if resp.Diagnostics.HasError() {
		return
	}

	_, _, err := r.provider.service.GetCluster(ctx, databaseSpec.ClusterId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting the cluster",
			fmt.Sprintf("Could not get the cluster: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var databaseRequest client.CreateDatabaseRequest
	databaseRequest.Name = databaseSpec.Name.ValueString()

	_, _, err = r.provider.service.CreateDatabase(ctx, databaseSpec.ClusterId.ValueString(), &databaseRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating database",
			fmt.Sprintf("Could not create database: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	diags = resp.State.Set(ctx, databaseSpec)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *databaseResource) Read(
	ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse,
) {
	if r.provider == nil || !r.provider.configured {
		addConfigureProviderErr(&resp.Diagnostics)
		return
	}

	var state Database
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Since the state may have come from an import, we need to retrieve
	// the actual database list and make sure this one is in there.
	apiResp, httpResp, err := r.provider.service.ListDatabases(
		ctx, state.ClusterId.ValueString(), &client.ListDatabasesOptions{},
	)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning(
				"Cluster not found",
				fmt.Sprintf(
					"Database's parent cluster with clusterID %s is not found. Removing from state.",
					state.ClusterId.ValueString(),
				),
			)
			resp.State.RemoveResource(ctx)
		} else {
			resp.Diagnostics.AddError(
				"Couldn't retrieve databases",
				fmt.Sprintf("Unexpected error retrieving databases: %s", formatAPIErrorMessage(err)),
			)
		}
		return
	}
	for _, database := range apiResp.GetDatabases() {
		if database.GetName() == state.Name.ValueString() {
			// Computed values may have changed.
			loadDatabaseToTerraformState(&database, &state, &state)
			diags = resp.State.Set(ctx, state)
			resp.Diagnostics.Append(diags...)
			return
		}
	}
	resp.Diagnostics.AddWarning(
		"Couldn't find database.",
		fmt.Sprintf(
			"This cluster doesn't have a database with name '%v'. Removing from state.",
			state.Name.ValueString(),
		),
	)
	resp.State.RemoveResource(ctx)
}

func (r *databaseResource) Update(
	ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	// Get plan values
	var plan Database
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state
	var state Database
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := client.UpdateDatabaseRequest{
		Name:    state.Name.ValueString(),
		NewName: plan.Name.ValueString(),
	}

	databaseObj, _, err := r.provider.service.EditDatabase(ctx, plan.ClusterId.ValueString(), &updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating database name",
			fmt.Sprintf("Could not update database name: %s", formatAPIErrorMessage(err)),
		)
		return
	}

	var newState Database
	loadDatabaseToTerraformState(databaseObj, &newState, &plan)
	diags = resp.State.Set(ctx, newState)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *databaseResource) Delete(
	ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse,
) {
	var state Database
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, err := r.provider.service.DeleteDatabase(
		ctx, state.ClusterId.ValueString(), state.Name.ValueString(),
	)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			// Database or cluster is already gone. Swallow the error.
		} else {
			resp.Diagnostics.AddError(
				"Error deleting database",
				fmt.Sprintf("Could not delete database: %s", formatAPIErrorMessage(err)),
			)
			return
		}
	}

	// Remove resource from state
	resp.State.RemoveResource(ctx)
}

func (r *databaseResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	// Since a database is uniquely identified by two fields, the cluster ID
	// and the name, we serialize them both into the ID field. To make import
	// work, we need to deserialize an ID back into name and cluster ID.
	matches := databaseIDRegex.FindStringSubmatch(req.ID)
	if len(matches) != 3 {
		resp.Diagnostics.AddError(
			"Invalid database ID format",
			`When importing a database, the ID field should follow the format "<cluster ID>:<database name>")`)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	database := Database{
		ClusterId: types.StringValue(matches[1]),
		Name:      types.StringValue(matches[2]),
		ID:        types.StringValue(req.ID),
	}
	resp.Diagnostics = resp.State.Set(ctx, &database)
}

func NewDatabaseResource() resource.Resource {
	return &databaseResource{}
}

func loadDatabaseToTerraformState(databaseObj *client.ApiDatabase, state, plan *Database) {
	state.ClusterId = plan.ClusterId
	state.Name = types.StringValue(databaseObj.GetName())
	// Get the unique ID (required by terraform framework) by combining
	// the cluster ID and new database name.
	state.ID = types.StringValue(fmt.Sprintf(databaseIDFmt, plan.ClusterId.ValueString(), databaseObj.GetName()))
	tableCountStr := databaseObj.GetTableCount()
	if tableCountStr != "" {
		// The API returns an int64 string for table count.
		tableCountInt, err := strconv.ParseInt(tableCountStr, 10, 64)
		if err == nil {
			state.TableCount = basetypes.NewInt64Value(tableCountInt)
		}
	}
}
