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
)

const (
	CockroachAPIKey string = "COCKROACH_API_KEY"
	APIServerURLKey string = "COCKROACH_SERVER"
	UserAgent       string = "terraform-provider-cockroach"
)

type Region struct {
	Name      types.String `tfsdk:"name"`
	SqlDns    types.String `tfsdk:"sql_dns"`
	UiDns     types.String `tfsdk:"ui_dns"`
	NodeCount types.Int64  `tfsdk:"node_count"`
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
	SpendLimit types.Int64  `tfsdk:"spend_limit"`
	RoutingId  types.String `tfsdk:"routing_id"`
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
	ClusterID types.String             `tfsdk:"cluster_id"`
	Services  []PrivateEndpointService `tfsdk:"services"`
	ID        types.String             `tfsdk:"id"`
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

type ConnectionString struct {
	ID               types.String `tfsdk:"id"`
	OS               types.String `tfsdk:"os"`
	Database         types.String `tfsdk:"database"`
	SqlUser          types.String `tfsdk:"sql_user"`
	Password         types.String `tfsdk:"password"`
	ConnectionString types.String `tfsdk:"connection_string"`
	ConnectionParams types.Map    `tfsdk:"connection_params"`
}

type LogExportGroup struct {
	LogName  types.String   `tfsdk:"log_name"`
	Channels []types.String `tfsdk:"channels"`
	MinLevel types.String   `tfsdk:"min_level"`
	Redact   types.Bool     `tfsdk:"redact"`
}

type ClusterLogExport struct {
	ID            types.String     `tfsdk:"id"`
	AuthPrincipal types.String     `tfsdk:"auth_principal"`
	LogName       types.String     `tfsdk:"log_name"`
	Type          types.String     `tfsdk:"type"`
	Redact        types.Bool       `tfsdk:"redact"`
	Region        types.String     `tfsdk:"region"`
	Groups        []LogExportGroup `tfsdk:"groups"`
	Status        types.String     `tfsdk:"status"`
	UserMessage   types.String     `tfsdk:"user_message"`
	CreatedAt     types.String     `tfsdk:"created_at"`
	UpdatedAt     types.String     `tfsdk:"updated_at"`
}

func (e *APIErrorMessage) String() string {
	return fmt.Sprintf("%v-%v", e.Code, e.Message)
}
