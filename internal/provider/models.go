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
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	CockroachAPIKey                  string = "COCKROACH_API_KEY"
	UserAgent                        string = "terraform-provider-cockroach"
	CLUSTERSTATETYPE_CREATED         string = "CREATED"
	CLUSTERSTATETYPE_CREATION_FAILED string = "CREATION_FAILED"
	CREATE_TIMEOUT                          = 60 * time.Minute
)

type Region struct {
	Name      types.String `tfsdk:"name"`
	SqlDns    types.String `tfsdk:"sql_dns"`
	UiDns     types.String `tfsdk:"ui_dns"`
	NodeCount types.Int64  `tfsdk:"node_count"`
}

type DedicatedClusterConfig struct {
	MachineType    types.String  `tfsdk:"machine_type"`
	NumVirtualCpus types.Int64   `tfsdk:"num_virtual_cpus"`
	StorageGib     types.Int64   `tfsdk:"storage_gib"`
	MemoryGib      types.Float64 `tfsdk:"memory_gib"`
	DiskIops       types.Int64   `tfsdk:"disk_iops"`
}

type ServerlessClusterConfig struct {
	SpendLimit types.Int64  `tfsdk:"spend_limit"`
	RoutingId  types.String `tfsdk:"routing_id"`
}

// DedicatedMachineTypeSpecification struct for DedicatedMachineTypeSpecification.
type DedicatedMachineTypeSpecification struct {
	// MachineType is the machine type identifier within the given cloud provider, ex. m5.xlarge, n2-standard-4.
	MachineType types.String `tfsdk:"machine_type"`
	// NumVirtualCPUs may be used to automatically select a machine type according to the desired number of vCPUs.
	NumVirtualCpus types.Int64 `tfsdk:"num_virtual_cpus"`
}

// SQLUserSpecification struct for SQLUserSpecification.
type SQLUserSpecification struct {
	Id       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Password types.String `tfsdk:"password"`
}

type SQLUser struct {
	Name types.String `tfsdk:"name"`
}

type APIErrorMessage struct {
	Code     int
	Message  string
	HttpCode int
}

type CockroachCluster struct {
	ID               types.String             `tfsdk:"id"`
	Name             types.String             `tfsdk:"name"`
	CloudProvider    types.String             `tfsdk:"cloud_provider"`
	AccountId        types.String             `tfsdk:"account_id"`
	DedicatedConfig  *DedicatedClusterConfig  `tfsdk:"dedicated"`
	ServerlessConfig *ServerlessClusterConfig `tfsdk:"serverless"`
	Regions          []Region                 `tfsdk:"regions"`
	CockroachVersion types.String             `tfsdk:"cockroach_version"`
	Plan             types.String             `tfsdk:"plan"`
	State            types.String             `tfsdk:"state"`
	CreatorId        types.String             `tfsdk:"creator_id"`
	OperationStatus  types.String             `tfsdk:"operation_status"`
}

// AllowlistEntry struct for AllowlistEntry.
type AllowlistEntry struct {
	Id       types.String `tfsdk:"id"`
	CidrIp   types.String `tfsdk:"cidr_ip"`
	CidrMask types.Int64  `tfsdk:"cidr_mask"`
	Ui       types.Bool   `tfsdk:"ui"`
	Sql      types.Bool   `tfsdk:"sql"`
	Name     types.String `tfsdk:"name"`
}

func (e *APIErrorMessage) String() string {
	return fmt.Sprintf("%v-%v", e.Code, e.Message)
}
