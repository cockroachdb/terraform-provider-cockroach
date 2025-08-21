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
	CockroachAPIKey     string = "COCKROACH_API_KEY"
	CockroachAPIJWT     string = "COCKROACH_API_JWT"
	APIServerURLKey     string = "COCKROACH_SERVER"
	UserAgent           string = "terraform-provider-cockroach"
	CockroachVanityName string = "COCKROACH_VANITY_NAME"
	CockroachUsername   string = "COCKROACH_USERNAME"
)

type Region struct {
	Name               types.String `tfsdk:"name"`
	SqlDns             types.String `tfsdk:"sql_dns"`
	UiDns              types.String `tfsdk:"ui_dns"`
	InternalDns        types.String `tfsdk:"internal_dns"`
	PrivateEndpointDns types.String `tfsdk:"private_endpoint_dns"`
	NodeCount          types.Int64  `tfsdk:"node_count"`
	Primary            types.Bool   `tfsdk:"primary"`
}

type DedicatedClusterConfig struct {
	MachineType                   types.String  `tfsdk:"machine_type"`
	NumVirtualCpus                types.Int64   `tfsdk:"num_virtual_cpus"`
	StorageGib                    types.Int64   `tfsdk:"storage_gib"`
	MemoryGib                     types.Float64 `tfsdk:"memory_gib"`
	DiskIops                      types.Int64   `tfsdk:"disk_iops"`
	PrivateNetworkVisibility      types.Bool    `tfsdk:"private_network_visibility"`
	CidrRange                     types.String  `tfsdk:"cidr_range"`
	SupportsClusterVirtualization types.Bool    `tfsdk:"supports_cluster_virtualization"`
}

type ServerlessClusterConfig struct {
	// TODO(andyk): SpendLimit is deprecated and will be removed in a future
	// release.
	SpendLimit  types.Int64  `tfsdk:"spend_limit"`
	RoutingId   types.String `tfsdk:"routing_id"`
	UpgradeType types.String `tfsdk:"upgrade_type"`
	UsageLimits *UsageLimits `tfsdk:"usage_limits"`
}

type ClusterBackupConfig struct {
	Enabled          types.Bool  `tfsdk:"enabled"`
	RetentionDays    types.Int64 `tfsdk:"retention_days"`
	FrequencyMinutes types.Int64 `tfsdk:"frequency_minutes"`
}

type UsageLimits struct {
	RequestUnitLimit       types.Int64 `tfsdk:"request_unit_limit"`
	StorageMibLimit        types.Int64 `tfsdk:"storage_mib_limit"`
	ProvisionedVirtualCpus types.Int64 `tfsdk:"provisioned_virtual_cpus"`
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

func (e *APIErrorMessage) String() string {
	return fmt.Sprintf("%v-%v", e.Code, e.Message)
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
	FullVersion      types.String             `tfsdk:"full_version"`
	Plan             types.String             `tfsdk:"plan"`
	State            types.String             `tfsdk:"state"`
	CreatorId        types.String             `tfsdk:"creator_id"`
	OperationStatus  types.String             `tfsdk:"operation_status"`
	UpgradeStatus    types.String             `tfsdk:"upgrade_status"`
	ParentId         types.String             `tfsdk:"parent_id"`
	DeleteProtection types.Bool               `tfsdk:"delete_protection"`
	BackupConfig     types.Object             `tfsdk:"backup_config"`
	Labels           types.Map                `tfsdk:"labels"`
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
	RegionName          types.String                `tfsdk:"region_name"`
	CloudProvider       types.String                `tfsdk:"cloud_provider"`
	Status              types.String                `tfsdk:"status"`
	Name                types.String                `tfsdk:"name"`
	EndpointServiceId   types.String                `tfsdk:"endpoint_service_id"`
	AvailabilityZoneIds []types.String              `tfsdk:"availability_zone_ids"`
	Aws                 PrivateLinkServiceAWSDetail `tfsdk:"aws"`
}

type PrivateEndpointServices struct {
	ClusterID   types.String `tfsdk:"cluster_id"`
	Services    types.List   `tfsdk:"services"`
	ServicesMap types.Map    `tfsdk:"services_map"`
	ID          types.String `tfsdk:"id"`
}

type PrivateEndpointConnection struct {
	ID            types.String `tfsdk:"id"`
	RegionName    types.String `tfsdk:"region_name"`
	CloudProvider types.String `tfsdk:"cloud_provider"`
	EndpointID    types.String `tfsdk:"endpoint_id"`
	ServiceID     types.String `tfsdk:"service_id"`
	ClusterID     types.String `tfsdk:"cluster_id"`
}

type PrivateEndpointTrustedOwner struct {
	ID              types.String `tfsdk:"id"`
	ClusterID       types.String `tfsdk:"cluster_id"`
	OwnerID         types.String `tfsdk:"owner_id"`
	Type            types.String `tfsdk:"type"`
	ExternalOwnerID types.String `tfsdk:"external_owner_id"`
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
	ID              types.String      `tfsdk:"id"`
	AuthPrincipal   types.String      `tfsdk:"auth_principal"`
	LogName         types.String      `tfsdk:"log_name"`
	Type            types.String      `tfsdk:"type"`
	Redact          types.Bool        `tfsdk:"redact"`
	Region          types.String      `tfsdk:"region"`
	Groups          *[]LogExportGroup `tfsdk:"groups"`
	Status          types.String      `tfsdk:"status"`
	UserMessage     types.String      `tfsdk:"user_message"`
	CreatedAt       types.String      `tfsdk:"created_at"`
	UpdatedAt       types.String      `tfsdk:"updated_at"`
	OmittedChannels *[]types.String   `tfsdk:"omitted_channels"`
	AWSExternalID   types.String      `tfsdk:"aws_external_id"`
}

type ClusterCloudWatchMetricExportConfig struct {
	ID           types.String `tfsdk:"id"`
	TargetRegion types.String `tfsdk:"target_region"`
	LogGroupName types.String `tfsdk:"log_group_name"`
	RoleArn      types.String `tfsdk:"role_arn"`
	ExternalID   types.String `tfsdk:"external_id"`
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

type ClusterPrometheusMetricExportConfig struct {
	ID      types.String `tfsdk:"id"`
	Status  types.String `tfsdk:"status"`
	Targets types.Map    `tfsdk:"targets"`
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

type ClusterBackupConfiguration struct {
	ID               types.String `tfsdk:"id"`
	Enabled          types.Bool   `tfsdk:"enabled"`
	RetentionDays    types.Int64  `tfsdk:"retention_days"`
	FrequencyMinutes types.Int64  `tfsdk:"frequency_minutes"`
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

type UserRoleGrants struct {
	ID     types.String `tfsdk:"id"`
	UserId types.String `tfsdk:"user_id"`
	Roles  []Role       `tfsdk:"roles"`
}

type UserRoleGrant struct {
	UserID types.String `tfsdk:"user_id"`
	Role   Role         `tfsdk:"role"`
}

type Folder struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	ParentId types.String `tfsdk:"parent_id"`
	Labels   types.Map    `tfsdk:"labels"`
}

type FolderDataSourceModel struct {
	ID       types.String `tfsdk:"id"`
	Path     types.String `tfsdk:"path"`
	Name     types.String `tfsdk:"name"`
	ParentId types.String `tfsdk:"parent_id"`
	Labels   types.Map    `tfsdk:"labels"`
}

type JWTIssuer struct {
	ID          types.String        `tfsdk:"id"`
	IssuerURL   types.String        `tfsdk:"issuer_url"`
	Audience    types.String        `tfsdk:"audience"`
	Jwks        types.String        `tfsdk:"jwks"`
	Claim       types.String        `tfsdk:"claim"`
	IdentityMap *[]IdentityMapEntry `tfsdk:"identity_map"`
}

type IdentityMapEntry struct {
	TokenIdentity types.String `tfsdk:"token_identity"`
	CcIdentity    types.String `tfsdk:"cc_identity"`
}

type ServiceAccount struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	CreatedAt   types.String `tfsdk:"created_at"`
	CreatorName types.String `tfsdk:"creator_name"`
}

type APIKey struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	ServiceAccountID types.String `tfsdk:"service_account_id"`
	CreatedAt        types.String `tfsdk:"created_at"`
	Secret           types.String `tfsdk:"secret"`
}

type PhysicalReplicationStream struct {
	PrimaryClusterId      types.String `tfsdk:"primary_cluster_id"`
	StandbyClusterId      types.String `tfsdk:"standby_cluster_id"`
	ID                    types.String `tfsdk:"id"`
	CreatedAt             types.String `tfsdk:"created_at"`
	ReplicatedTime        types.String `tfsdk:"replicated_time"`
	ReplicationLagSeconds types.Int64  `tfsdk:"replication_lag_seconds"`
	RetainedTime          types.String `tfsdk:"retained_time"`
	Status                types.String `tfsdk:"status"`
	FailoverAt            types.String `tfsdk:"failover_at"`
	FailoverImmediately   types.Bool   `tfsdk:"failover_immediately"`
	ActivatedAt           types.String `tfsdk:"activated_at"`
}

type Backup struct {
	ID       types.String `tfsdk:"id"`
	AsOfTime types.String `tfsdk:"as_of_time"`
}

type Backups struct {
	ClusterID types.String `tfsdk:"cluster_id"`
	StartTime types.String `tfsdk:"start_time"`
	EndTime   types.String `tfsdk:"end_time"`
	SortOrder types.String `tfsdk:"sort_order"`
	Limit     types.Int32  `tfsdk:"limit"`
	Backups   []Backup     `tfsdk:"backups"`
}

type RestoreSummary struct {
	ID                types.String  `tfsdk:"id"`
	BackupID          types.String  `tfsdk:"backup_id"`
	Status            types.String  `tfsdk:"status"`
	CreatedAt         types.String  `tfsdk:"created_at"`
	Type              types.String  `tfsdk:"type"`
	CompletionPercent types.Float32 `tfsdk:"completion_percent"`
}

type Restores struct {
	ClusterID types.String     `tfsdk:"cluster_id"`
	StartTime types.String     `tfsdk:"start_time"`
	EndTime   types.String     `tfsdk:"end_time"`
	SortOrder types.String     `tfsdk:"sort_order"`
	Limit     types.Int32      `tfsdk:"limit"`
	Restores  []RestoreSummary `tfsdk:"restores"`
}

type RestoreItem struct {
	Database types.String `tfsdk:"database"`
	Schema   types.String `tfsdk:"schema"`
	Tables   types.List   `tfsdk:"tables"`
}

type RestoreOpts struct {
	NewDBName              types.String `tfsdk:"new_db_name"`
	IntoDB                 types.String `tfsdk:"into_db"`
	SkipLocalitiesCheck    types.Bool   `tfsdk:"skip_localities_check"`
	SkipMissingForeignKeys types.Bool   `tfsdk:"skip_missing_foreign_keys"`
	SkipMissingSequences   types.Bool   `tfsdk:"skip_missing_sequences"`
	SchemaOnly             types.Bool   `tfsdk:"schema_only"`
}

type Restore struct {
	ID                   types.String  `tfsdk:"id"`
	DestinationClusterID types.String  `tfsdk:"destination_cluster_id"`
	Type                 types.String  `tfsdk:"type"`
	BackupID             types.String  `tfsdk:"backup_id"`
	SourceClusterID      types.String  `tfsdk:"source_cluster_id"`
	Objects              []RestoreItem `tfsdk:"objects"`
	RestoreOpts          *RestoreOpts  `tfsdk:"restore_opts"`
	Status               types.String  `tfsdk:"status"`
	CreatedAt            types.String  `tfsdk:"created_at"`
	CompletionPercent    types.Float32 `tfsdk:"completion_percent"`
}

type EgressPrivateEndpoint struct {
	ID                      types.String `tfsdk:"id"`
	ClusterID               types.String `tfsdk:"cluster_id"`
	EndpointConnectionID    types.String `tfsdk:"endpoint_connection_id"`
	Region                  types.String `tfsdk:"region"`
	TargetServiceIdentifier types.String `tfsdk:"target_service_identifier"`
	TargetServiceType       types.String `tfsdk:"target_service_type"`
	EndpointAddress         types.String `tfsdk:"endpoint_address"`
	State                   types.String `tfsdk:"state"`
}
