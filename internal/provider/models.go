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
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	CockroachAPIKey string = "COCKROACH_API_KEY"
	APIServerURLKey string = "COCKROACH_SERVER"
	UserAgent       string = "terraform-provider-cockroach"
)

type Region struct {
	Name        types.String `tfsdk:"name"`
	SqlDns      types.String `tfsdk:"sql_dns"`
	UiDns       types.String `tfsdk:"ui_dns"`
	InternalDns types.String `tfsdk:"internal_dns"`
	NodeCount   types.Int64  `tfsdk:"node_count"`
	Primary     types.Bool   `tfsdk:"primary"`
}

type DedicatedClusterConfig struct {
	MachineType              types.String  `tfsdk:"machine_type"`
	NumVirtualCpus           types.Int64   `tfsdk:"num_virtual_cpus"`
	StorageGib               types.Int64   `tfsdk:"storage_gib"`
	MemoryGib                types.Float64 `tfsdk:"memory_gib"`
	DiskIops                 types.Int64   `tfsdk:"disk_iops"`
	PrivateNetworkVisibility types.Bool    `tfsdk:"private_network_visibility"`
}

type ServerlessClusterConfig struct {
	SpendLimit  types.Int64  `tfsdk:"spend_limit"`
	RoutingId   types.String `tfsdk:"routing_id"`
	UsageLimits *UsageLimits `tfsdk:"usage_limits"`
}

type UsageLimits struct {
	RequestUnitLimit types.Int64 `tfsdk:"request_unit_limit"`
	StorageMibLimit  types.Int64 `tfsdk:"storage_mib_limit"`
}

type SQLUser struct {
	ClusterId types.String `tfsdk:"cluster_id"`
	Name      types.String `tfsdk:"name"`
	Password  types.String `tfsdk:"password"`
	ID        types.String `tfsdk:"id"`
}

type APIErrorMessage struct {
	Code     int
	Message  string
	HttpCode int
}

// CockroachCluster is used by the cluster resource
// and the cluster data source. Changes to this model
// should be supported by both.
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
	UpgradeStatus    types.String             `tfsdk:"upgrade_status"`
	ParentId         types.String             `tfsdk:"parent_id"`
}

type AllowlistEntry struct {
	ClusterId types.String `tfsdk:"cluster_id"`
	CidrIp    types.String `tfsdk:"cidr_ip"`
	CidrMask  types.Int64  `tfsdk:"cidr_mask"`
	Ui        types.Bool   `tfsdk:"ui"`
	Sql       types.Bool   `tfsdk:"sql"`
	Name      types.String `tfsdk:"name"`
	ID        types.String `tfsdk:"id"`
}

type PrivateLinkServiceAWSDetail struct {
	ServiceName         types.String   `tfsdk:"service_name"`
	ServiceId           types.String   `tfsdk:"service_id"`
	AvailabilityZoneIds []types.String `tfsdk:"availability_zone_ids"`
}

type PrivateEndpointService struct {
	RegionName    types.String                `tfsdk:"region_name"`
	CloudProvider types.String                `tfsdk:"cloud_provider"`
	Status        types.String                `tfsdk:"status"`
	Aws           PrivateLinkServiceAWSDetail `tfsdk:"aws"`
}

type PrivateEndpointServices struct {
	ClusterID types.String `tfsdk:"cluster_id"`
	Services  types.List   `tfsdk:"services"`
	ID        types.String `tfsdk:"id"`
}

type PrivateEndpointConnection struct {
	ID            types.String `tfsdk:"id"`
	RegionName    types.String `tfsdk:"region_name"`
	CloudProvider types.String `tfsdk:"cloud_provider"`
	EndpointID    types.String `tfsdk:"endpoint_id"`
	ServiceID     types.String `tfsdk:"service_id"`
	ClusterID     types.String `tfsdk:"cluster_id"`
}

type CMEKKey struct {
	Status        types.String `tfsdk:"status"`
	UserMessage   types.String `tfsdk:"user_message"`
	Type          types.String `tfsdk:"type"`
	URI           types.String `tfsdk:"uri"`
	AuthPrincipal types.String `tfsdk:"auth_principal"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

type CMEKRegion struct {
	Region types.String `tfsdk:"region"`
	Status types.String `tfsdk:"status"`
	Key    CMEKKey      `tfsdk:"key"`
}

type ClusterCMEK struct {
	ID                types.String `tfsdk:"id"`
	Status            types.String `tfsdk:"status"`
	Regions           []CMEKRegion `tfsdk:"regions"`
	AdditionalRegions []Region     `tfsdk:"additional_regions"`
}

type ClusterCert struct {
	ID   types.String `tfsdk:"id"`
	Cert types.String `tfsdk:"cert"`
}

type ConnectionParams struct {
	Host     types.String `tfsdk:"host"`
	Port     types.String `tfsdk:"port"`
	Database types.String `tfsdk:"database"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

type ConnectionString struct {
	ID               types.String      `tfsdk:"id"`
	OS               types.String      `tfsdk:"os"`
	Database         types.String      `tfsdk:"database"`
	SqlUser          types.String      `tfsdk:"sql_user"`
	Password         types.String      `tfsdk:"password"`
	ConnectionString types.String      `tfsdk:"connection_string"`
	ConnectionParams *ConnectionParams `tfsdk:"connection_params"`
}

type Database struct {
	ClusterId  types.String `tfsdk:"cluster_id"`
	Name       types.String `tfsdk:"name"`
	ID         types.String `tfsdk:"id"`
	TableCount types.Int64  `tfsdk:"table_count"`
}

type FinalizeVersionUpgrade struct {
	CockroachVersion types.String `tfsdk:"cockroach_version"`
	ID               types.String `tfsdk:"id"`
}

type Organization struct {
	ID        types.String `tfsdk:"id"`
	Label     types.String `tfsdk:"label"`
	Name      types.String `tfsdk:"name"`
	CreatedAt types.String `tfsdk:"created_at"`
}

type PersonUser struct {
	ID    types.String `tfsdk:"id"`
	Email types.String `tfsdk:"email"`
}

type LogExportGroup struct {
	LogName  types.String   `tfsdk:"log_name"`
	Channels []types.String `tfsdk:"channels"`
	MinLevel types.String   `tfsdk:"min_level"`
	Redact   types.Bool     `tfsdk:"redact"`
}

type ClusterLogExport struct {
	ID            types.String      `tfsdk:"id"`
	AuthPrincipal types.String      `tfsdk:"auth_principal"`
	LogName       types.String      `tfsdk:"log_name"`
	Type          types.String      `tfsdk:"type"`
	Redact        types.Bool        `tfsdk:"redact"`
	Region        types.String      `tfsdk:"region"`
	Groups        *[]LogExportGroup `tfsdk:"groups"`
	Status        types.String      `tfsdk:"status"`
	UserMessage   types.String      `tfsdk:"user_message"`
	CreatedAt     types.String      `tfsdk:"created_at"`
	UpdatedAt     types.String      `tfsdk:"updated_at"`
}

type ClusterCloudWatchMetricExportConfig struct {
	ID           types.String `tfsdk:"id"`
	TargetRegion types.String `tfsdk:"target_region"`
	LogGroupName types.String `tfsdk:"log_group_name"`
	RoleArn      types.String `tfsdk:"role_arn"`
	UserMessage  types.String `tfsdk:"user_message"`
	Status       types.String `tfsdk:"status"`
}

type ClusterDatadogMetricExportConfig struct {
	ID          types.String `tfsdk:"id"`
	Site        types.String `tfsdk:"site"`
	ApiKey      types.String `tfsdk:"api_key"`
	UserMessage types.String `tfsdk:"user_message"`
	Status      types.String `tfsdk:"status"`
}

type ClusterMaintenanceWindow struct {
	ID             types.String `tfsdk:"id"`
	OffsetDuration types.Int64  `tfsdk:"offset_duration"`
	WindowDuration types.Int64  `tfsdk:"window_duration"`
}

type ClusterVersionDeferral struct {
	ID             types.String `tfsdk:"id"`
	DeferralPolicy types.String `tfsdk:"deferral_policy"`
}

type ClientCACertResourceModel struct {
	ID          types.String `tfsdk:"id"`
	X509PemCert types.String `tfsdk:"x509_pem_cert"`
	Status      types.String `tfsdk:"status"`
}

type Role struct {
	RoleName     types.String `tfsdk:"role_name"`
	ResourceType types.String `tfsdk:"resource_type"`
	ResourceId   types.String `tfsdk:"resource_id"`
}

type RoleGrant struct {
	ID     types.String `tfsdk:"id"`
	UserId types.String `tfsdk:"user_id"`
	Roles  []Role       `tfsdk:"roles"`
}

type Folder struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	ParentId types.String `tfsdk:"parent_id"`
}

func (e *APIErrorMessage) String() string {
	return fmt.Sprintf("%v-%v", e.Code, e.Message)
}
