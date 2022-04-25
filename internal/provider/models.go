package provider

import (
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

type User struct {
	Name string `tfsdk:"name"`
}

type SqlUser struct {
	User     User   `tfsdk:"user"`
	Password string `tfsdk:"password"`
}

type SqlUserPassword struct {
	Password string `tfsdk:"password"`
}

type SqlUserName struct {
	Name string `tfsdk:"name"`
}

type SqlUsersResponse struct {
	SqlUsersName []SqlUserName `tfsdk:"users"`
}

type ClustersResponse struct {
	Cluster []Cluster `tfsdk:"clusters"`
}

type ClusterState types.String

type CloudProvider string

const (
	AWS                      CloudProvider = "CLOUD_PROVIDER_AWS"
	GCP                      CloudProvider = "CLOUD_PROVIDER_GCP"
	UnspecifiedCloudProvider CloudProvider = "CLOUD_PROVIDER_UNSPECIFIED"
)

type Cluster struct {
	ID               types.String  `tfsdk:"id"`
	Name             types.String  `tfsdk:"name"`
	CockroachVersion types.String  `tfsdk:"cockroach_version"`
	Plan             types.String  `tfsdk:"plan"`
	CloudProvider    CloudProvider `tfsdk:"cloud_provider"`
	State            ClusterState  `tfsdk:"state"`
	CreatorId        types.String  `tfsdk:"creator_id"`
	OperationStatus  types.String  `tfsdk:"operation_status"`
	Config           Config        `tfsdk:"config"`
	Regions          []Region      `tfsdk:"regions"`
	CreatedAt        time.Time     `tfsdk:"created_at"`
	UpdatedAt        time.Time     `tfsdk:"updated_at"`
	DeletedAt        time.Time     `tfsdk:"deleted_at"`
}

type Config struct {
	Dedicated  DedicatedHardwareConfig `tfsdk:"dedicated"`
	Serverless ServerlessClusterConfig `tfsdk:"serverless"`
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

type ClusterSpec struct {
	Dedicated  *DedicatedClusterSpec  `tfsdk:"dedicated"`
	Serverless *ServerlessClusterSpec `tfsdk:"serverless"`
}

type DedicatedClusterSpec struct {
	RegionNodes      types.Int64           `tfsdk:"region_nodes"`
	Hardware         DedicatedHardwareSpec `tfsdk:"hardware"`
	CockroachVersion types.String          `tfsdk:"cockroach_version"`
}

type ServerlessClusterSpec struct {
	Regions    []types.String `tfsdk:"regions"`
	SpendLimit types.Int64    `tfsdk:"spend_limit"`
}

type DedicatedHardwareSpec struct {
	MachineSpec DedicatedMachineTypeSpec `tfsdk:"machine_spec"`
	StorageGib  types.Int64              `tfsdk:"storage_gib"`
	DiskIops    types.Int64              `tfsdk:"disk_iops"`
}

type DedicatedMachineTypeSpec struct {
	MachineType    types.String `tfsdk:"machine_type"`
	NumVirtualCpus types.Int64  `tfsdk:"num_virtual_cpus"`
}

type Credential struct {
	APIKey string
}

type ClusterCertificate struct {
	FileName      string
	CaCertificate string
}

type APIErrorMessage struct {
	Code     int
	Message  string
	HttpCode int
}

type CockroachCluster struct {
	ID               types.String  `tfsdk:"id"`
	Name             types.String  `tfsdk:"name"`
	Provider         CloudProvider `tfsdk:"cloud_provider"`
	Spec             ClusterSpec   `tfsdk:"spec"`
	CockroachVersion types.String  `tfsdk:"cockroach_version"`
	Plan             types.String  `tfsdk:"plan"`
	State            types.String  `tfsdk:"state"`
	CreatorId        types.String  `tfsdk:"creator_id"`
	OperationStatus  types.String  `tfsdk:"operation_status"`
}

type CockroachClusterData struct {
	ID               types.String  `tfsdk:"id"`
	Name             types.String  `tfsdk:"name"`
	Provider         CloudProvider `tfsdk:"cloud_provider"`
	CockroachVersion types.String  `tfsdk:"cockroach_version"`
	Plan             types.String  `tfsdk:"plan"`
	State            types.String  `tfsdk:"state"`
	CreatorId        types.String  `tfsdk:"creator_id"`
	OperationStatus  types.String  `tfsdk:"operation_status"`
	Config           Config        `tfsdk:"config"`
	Regions          []Region      `tfsdk:"regions"`
}

func (e *APIErrorMessage) String() string {
	return fmt.Sprintf("%v-%v", e.Code, e.Message)
}
