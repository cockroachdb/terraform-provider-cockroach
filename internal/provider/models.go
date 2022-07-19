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
	"github.com/hashicorp/terraform-plugin-framework/types"
	"time"
)

type ClusterState types.String

// ApiCloudProvider  - GCP: The Google Cloud Platform cloud provider.  - AWS: The Amazon Web Services cloud provider.
type ApiCloudProvider string

const (
	CockroachAPIKey                  string = "COCKROACH_API_KEY"
	UserAgent                        string = "terraform-provider-cockroach"
	CLUSTERSTATETYPE_CREATED         string = "CREATED"
	CLUSTERSTATETYPE_CREATION_FAILED string = "CREATION_FAILED"
	CREATE_TIMEOUT                          = 60 * time.Minute
)

// ClusterConfig struct for ClusterConfig.
type ClusterConfig struct {
	Dedicated  *DedicatedHardwareConfig `tfsdk:"dedicated"`
	Serverless *ServerlessClusterConfig `tfsdk:"serverless"`
}

type Region struct {
	Name      types.String `tfsdk:"name"`
	SqlDns    types.String `tfsdk:"sql_dns"`
	UiDns     types.String `tfsdk:"ui_dns"`
	NodeCount types.Int64  `tfsdk:"node_count"`
}

type DedicatedHardwareConfig struct {
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

// CreateClusterSpecification struct for CreateClusterSpecification.
type CreateClusterSpecification struct {
	Dedicated  *DedicatedClusterCreateSpecification  `tfsdk:"dedicated"`
	Serverless *ServerlessClusterCreateSpecification `tfsdk:"serverless"`
}

// ServerlessClusterCreateSpecification struct for ServerlessClusterCreateSpecification.
type ServerlessClusterCreateSpecification struct {
	// Region values should match the cloud provider's zone code. For example, for Oregon, set region_name to \"us-west2\" for GCP and \"us-west-2\" for AWS.
	Regions    []types.String `tfsdk:"regions"`
	SpendLimit types.Int64    `tfsdk:"spend_limit"`
}

// DedicatedClusterCreateSpecification struct for DedicatedClusterCreateSpecification.
type DedicatedClusterCreateSpecification struct {
	// Region keys should match the cloud provider's zone code. For example, for Oregon, set region_name to \"us-west2\" for GCP and \"us-west-2\" for AWS. Values represent the node count.
	RegionNodes *map[string]int32                     `tfsdk:"region_nodes"`
	Hardware    *DedicatedHardwareCreateSpecification `tfsdk:"hardware"`
	// The CockroachDB version for the cluster. The current version is used if omitted.
	CockroachVersion types.String `tfsdk:"cockroach_version"`
}

// DedicatedHardwareCreateSpecification struct for DedicatedHardwareCreateSpecification.
type DedicatedHardwareCreateSpecification struct {
	MachineSpec *DedicatedMachineTypeSpecification `tfsdk:"machine_spec"`
	// StorageGiB is the number of storage GiB per node in the cluster. Zero indicates default to the lowest storage GiB available given machine specs.
	StorageGib types.Int64 `tfsdk:"storage_gib"`
	// DiskIOPs is the number of disk I/O operations per second that are permitted on each node in the cluster. Zero indicates the cloud provider-specific default. Only available for AWS clusters.
	DiskIops types.Int64 `tfsdk:"disk_iops"`
}

// DedicatedMachineTypeSpecification struct for DedicatedMachineTypeSpecification.
type DedicatedMachineTypeSpecification struct {
	// MachineType is the machine type identifier within the given cloud provider, ex. m5.xlarge, n2-standard-4.
	MachineType types.String `tfsdk:"machine_type"`
	// NumVirtualCPUs may be used to automatically select a machine type according to the desired number of vCPUs.
	NumVirtualCpus types.Int64 `tfsdk:"num_virtual_cpus"`
}

type ServerlessClusterSpec struct {
	Regions    []types.String `tfsdk:"regions"`
	SpendLimit types.Int64    `tfsdk:"spend_limit"`
}

// ServerlessClusterUpdateSpecification struct for ServerlessClusterUpdateSpecification.
type ServerlessClusterUpdateSpecification struct {
	SpendLimit types.Int64 `tfsdk:"spend_limit"`
}

// DedicatedClusterUpdateSpecification struct for DedicatedClusterUpdateSpecification.
type DedicatedClusterUpdateSpecification struct {
	// Region keys should match the cloud provider's zone code. For example, for Oregon, set region_name to \"us-west2\" for GCP and \"us-west-2\" for AWS. Values represent the node count.
	RegionNodes *map[string]int32                     `tfsdk:"region_nodes"`
	Hardware    *DedicatedHardwareUpdateSpecification `tfsdk:"hardware"`
}

// DedicatedHardwareUpdateSpecification struct for DedicatedHardwareUpdateSpecification.
type DedicatedHardwareUpdateSpecification struct {
	MachineSpec *DedicatedMachineTypeSpecification `tfsdk:"machine_spec"`
	// StorageGiB is the number of storage GiB per node in the cluster.
	StorageGib types.Int64 `tfsdk:"storage_gib"`
	// DiskIOPs is the number of disk I/O operations per second that are permitted on each node in the cluster. Zero indicates the cloud provider-specific default. Only available for AWS clusters.
	DiskIops types.Int64 `tfsdk:"disk_iops"`
}

// UpdateClusterSpecification struct for UpdateClusterSpecification.
type UpdateClusterSpecification struct {
	Dedicated  *DedicatedClusterUpdateSpecification  `tfsdk:"dedicated"`
	Serverless *ServerlessClusterUpdateSpecification `tfsdk:"serverless"`
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
	ID                  types.String                `tfsdk:"id"`
	Name                types.String                `tfsdk:"name"`
	CloudProvider       ApiCloudProvider            `tfsdk:"cloud_provider"`
	AccountId           types.String                `tfsdk:"account_id"`
	CreateSpec          *CreateClusterSpecification `tfsdk:"create_spec"`
	UpdateSpec          *UpdateClusterSpecification `tfsdk:"update_spec"`
	Config              *ClusterConfig              `tfsdk:"config"`
	Regions             []Region                    `tfsdk:"regions"`
	CockroachVersion    types.String                `tfsdk:"cockroach_version"`
	Plan                types.String                `tfsdk:"plan"`
	State               types.String                `tfsdk:"state"`
	CreatorId           types.String                `tfsdk:"creator_id"`
	OperationStatus     types.String                `tfsdk:"operation_status"`
	WaitForClusterReady types.Bool                  `tfsdk:"wait_for_cluster_ready"`
}

type CockroachClusterData struct {
	ID               types.String     `tfsdk:"id"`
	Name             types.String     `tfsdk:"name"`
	CloudProvider    ApiCloudProvider `tfsdk:"cloud_provider"`
	AccountId        types.String     `tfsdk:"account_id"`
	CockroachVersion types.String     `tfsdk:"cockroach_version"`
	Plan             types.String     `tfsdk:"plan"`
	State            types.String     `tfsdk:"state"`
	CreatorId        types.String     `tfsdk:"creator_id"`
	OperationStatus  types.String     `tfsdk:"operation_status"`
	Config           ClusterConfig    `tfsdk:"config"`
	Regions          []Region         `tfsdk:"regions"`
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
