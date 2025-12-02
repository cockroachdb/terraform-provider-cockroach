package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccClusterRestoreResource attempts to create and check a real cluster
// restore. It will be skipped if TF_ACC isn't set.
func TestAccClusterRestoreResource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := testAccProvider.(*provider)
	p.service = NewService(cl)
	clusterName := fmt.Sprintf("%s-cluster-to-restore-%s", tfTestPrefix, GenerateRandomString(4))

	testRestoreResource(t, false, clusterName, "CLUSTER", "DATABASE", nil, nil, p.service, ctx, false)
}

// TestIntegrationClusterRestoreResource attempts to create and check a real
// cluster restore, but uses a mocked API service.
func TestIntegrationClusterRestoreResource(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	clusterName := fmt.Sprintf("%s-cluster-to-restore-%s", tfTestPrefix, GenerateRandomString(4))
	clusterID := "12345678-1234-1234-1234-123456789012"
	restoreID := "87654321-4321-4321-4321-210987654321"
	backupID := "11111111-2222-3333-4444-555555555555"
	cluster := &client.Cluster{
		Id:    clusterID,
		Name:  clusterName,
		State: client.CLUSTERSTATETYPE_CREATED,
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{
				UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
				UsageLimits: &client.UsageLimits{
					ProvisionedVirtualCpus: ptr(int64(2)),
				},
			},
		},
		Regions: []client.Region{
			{
				Name: "us-central1",
			},
		},
		CloudProvider: client.CLOUDPROVIDERTYPE_GCP,
		Plan:          client.PLANTYPE_STANDARD,
	}
	backupConfig := &client.BackupConfiguration{
		Enabled:          true,
		FrequencyMinutes: 5,
		RetentionDays:    30,
	}

	createRestoreReq := &client.CockroachCloudCreateRestoreRequest{
		Type:            client.RESTORETYPETYPE_CLUSTER,
		SourceClusterId: &clusterID,
	}

	now := time.Now()
	backupEndTime := now.Add(-1 * time.Hour)

	restore := &client.Restore{
		Id:                     restoreID,
		BackupId:               backupID,
		Type:                   client.RESTORETYPETYPE_CLUSTER,
		Status:                 client.RESTORESTATUSTYPE_PENDING,
		CreatedAt:              now,
		CompletionPercent:      0.0,
		SourceClusterName:      clusterName,
		DestinationClusterName: clusterName,
		BackupEndTime:          backupEndTime,
		CrdbJobId:              ptr("12345"),
	}

	completedAt := now.Add(30 * time.Minute)
	restoreSuccess := &client.Restore{
		Id:                     restoreID,
		BackupId:               backupID,
		Type:                   client.RESTORETYPETYPE_CLUSTER,
		Status:                 client.RESTORESTATUSTYPE_SUCCESS,
		CreatedAt:              now,
		CompletionPercent:      1.0,
		SourceClusterName:      clusterName,
		DestinationClusterName: clusterName,
		BackupEndTime:          backupEndTime,
		CompletedAt:            &completedAt,
		CrdbJobId:              ptr("12345"),
	}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(cluster, nil, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(backupConfig, nil, nil).AnyTimes()
	s.EXPECT().UpdateBackupConfiguration(gomock.Any(), clusterID, gomock.Any()).Return(backupConfig, nil, nil)
	s.EXPECT().CreateRestore(gomock.Any(), clusterID, createRestoreReq).Return(restore, nil, nil)
	// Initial calls return PENDING status (to mimic polling).
	s.EXPECT().GetRestore(gomock.Any(), clusterID, restoreID).Return(restore, nil, nil).Times(3)
	// GetRestore finally returns SUCCESS status.
	s.EXPECT().GetRestore(gomock.Any(), clusterID, restoreID).Return(restoreSuccess, nil, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(nil, nil, nil)

	testRestoreResource(t, true, clusterName, "CLUSTER", "DATABASE", nil, nil, s, context.Background(), false)
}

// TestIntegrationDatabaseRestoreResource attempts to create and check a real
// database restore, but uses a mocked API service.
func TestIntegrationDatabaseRestoreResource(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	objects := []RestoreItem{
		{
			Database: types.StringValue("test_db"),
		},
	}

	opts := &RestoreOpts{
		NewDBName: types.StringValue("new_test_db"),
	}

	clusterName := fmt.Sprintf("%s-cluster-to-restore-%s", tfTestPrefix, GenerateRandomString(4))
	clusterID := "12345678-1234-1234-1234-123456789012"
	restoreID := "87654321-4321-4321-4321-210987654321"
	backupID := "11111111-2222-3333-4444-555555555555"
	cluster := &client.Cluster{
		Id:    clusterID,
		Name:  clusterName,
		State: client.CLUSTERSTATETYPE_CREATED,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				StorageGib:     15,
				NumVirtualCpus: 4,
				MachineType:    "n2-standard-4",
				MemoryGib:      16.0,
				DiskIops:       3000,
			},
		},
		Regions: []client.Region{
			{
				Name:      "us-central1",
				NodeCount: 1,
			},
		},
		CidrRange:        "172.28.0.0/16",
		CloudProvider:    client.CLOUDPROVIDERTYPE_GCP,
		CockroachVersion: "v25.2",
		Plan:             client.PLANTYPE_ADVANCED,
	}
	backupConfig := &client.BackupConfiguration{
		Enabled:          true,
		FrequencyMinutes: 5,
		RetentionDays:    30,
	}

	createRestoreReq := &client.CockroachCloudCreateRestoreRequest{
		Type:            client.RESTORETYPETYPE_DATABASE,
		SourceClusterId: &clusterID,
		Objects:         &[]client.RestoreItem{dbRestoreItem("test_db")},
		RestoreOpts:     &client.RestoreOpts{NewDbName: ptr("new_test_db")},
	}

	now := time.Now()
	backupEndTime := now.Add(-1 * time.Hour)

	restore := &client.Restore{
		Id:                     restoreID,
		BackupId:               backupID,
		Type:                   client.RESTORETYPETYPE_DATABASE,
		Status:                 client.RESTORESTATUSTYPE_PENDING,
		CreatedAt:              now,
		CompletionPercent:      0.0,
		SourceClusterName:      clusterName,
		DestinationClusterName: clusterName,
		BackupEndTime:          backupEndTime,
		CrdbJobId:              ptr("12345"),
		Objects:                &[]client.RestoreItem{dbRestoreItem("test_db")},
		RestoreOpts:            &client.RestoreOpts{NewDbName: ptr("new_test_db")},
	}

	completedAt := now.Add(30 * time.Minute)
	restoreSuccess := &client.Restore{
		Id:                     restoreID,
		BackupId:               backupID,
		Type:                   client.RESTORETYPETYPE_DATABASE,
		Status:                 client.RESTORESTATUSTYPE_SUCCESS,
		CreatedAt:              now,
		CompletionPercent:      1.0,
		SourceClusterName:      clusterName,
		DestinationClusterName: clusterName,
		BackupEndTime:          backupEndTime,
		CompletedAt:            &completedAt,
		CrdbJobId:              ptr("12345"),
		Objects:                &[]client.RestoreItem{dbRestoreItem("test_db")},
		RestoreOpts:            &client.RestoreOpts{NewDbName: ptr("new_test_db")},
	}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(cluster, nil, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(backupConfig, nil, nil).AnyTimes()
	s.EXPECT().CreateRestore(gomock.Any(), clusterID, createRestoreReq).Return(restore, nil, nil)
	// Initial calls return PENDING status (to mimic polling).
	s.EXPECT().GetRestore(gomock.Any(), clusterID, restoreID).Return(restore, nil, nil).Times(3)
	// GetRestore finally returns SUCCESS status.
	s.EXPECT().GetRestore(gomock.Any(), clusterID, restoreID).Return(restoreSuccess, nil, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(nil, nil, nil)
	testRestoreResource(t, true, clusterName, "DATABASE", "TABLE", objects, opts, s, context.Background(), true)
}

// TestIntegrationTableRestoreResource attempts to create and check a real table
// restore, but uses a mocked API service.
func TestIntegrationTableRestoreResource(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	objects := []RestoreItem{
		{
			Database: types.StringValue("test_db"),
			Schema:   types.StringValue("public"),
			Tables:   types.ListValueMust(types.StringType, []attr.Value{types.StringValue("table1"), types.StringValue("table2")}),
		},
		{
			Database: types.StringValue("test_db2"),
			Schema:   types.StringValue("public"),
			Tables:   types.ListValueMust(types.StringType, []attr.Value{types.StringValue("table3")}),
		},
	}

	opts := &RestoreOpts{
		IntoDB: types.StringValue("target_db"),
	}

	clusterName := fmt.Sprintf("%s-cluster-to-restore-%s", tfTestPrefix, GenerateRandomString(4))
	clusterID := "12345678-1234-1234-1234-123456789012"
	restoreID := "87654321-4321-4321-4321-210987654321"
	backupID := "11111111-2222-3333-4444-555555555555"
	cluster := &client.Cluster{
		Id:    clusterID,
		Name:  clusterName,
		State: client.CLUSTERSTATETYPE_CREATED,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				StorageGib:     15,
				NumVirtualCpus: 4,
				MachineType:    "n2-standard-4",
				MemoryGib:      16.0,
				DiskIops:       3000,
			},
		},
		Regions: []client.Region{
			{
				Name:      "us-central1",
				NodeCount: 1,
			},
		},
		CidrRange:        "172.28.0.0/16",
		CloudProvider:    client.CLOUDPROVIDERTYPE_GCP,
		CockroachVersion: "v25.2",
		Plan:             client.PLANTYPE_ADVANCED,
	}
	backupConfig := &client.BackupConfiguration{
		Enabled:          true,
		FrequencyMinutes: 5,
		RetentionDays:    30,
	}

	var restoreItems []client.RestoreItem
	restoreItems = append(restoreItems, tablesRestoreItem("test_db", "public", []string{"table1", "table2"})...)
	restoreItems = append(restoreItems, tablesRestoreItem("test_db2", "public", []string{"table3"})...)

	createRestoreReq := &client.CockroachCloudCreateRestoreRequest{
		Type:            client.RESTORETYPETYPE_TABLE,
		SourceClusterId: &clusterID,
		Objects:         &restoreItems,
		RestoreOpts:     &client.RestoreOpts{IntoDb: ptr("target_db")},
	}

	now := time.Now()
	backupEndTime := now.Add(-1 * time.Hour)

	restore := &client.Restore{
		Id:                     restoreID,
		BackupId:               backupID,
		Type:                   client.RESTORETYPETYPE_TABLE,
		Status:                 client.RESTORESTATUSTYPE_PENDING,
		CreatedAt:              now,
		CompletionPercent:      0.0,
		SourceClusterName:      clusterName,
		DestinationClusterName: clusterName,
		BackupEndTime:          backupEndTime,
		CrdbJobId:              ptr("12345"),
		Objects:                &restoreItems,
		RestoreOpts:            &client.RestoreOpts{IntoDb: ptr("target_db")},
	}

	completedAt := now.Add(30 * time.Minute)
	restoreSuccess := &client.Restore{
		Id:                     restoreID,
		BackupId:               backupID,
		Type:                   client.RESTORETYPETYPE_TABLE,
		Status:                 client.RESTORESTATUSTYPE_SUCCESS,
		CreatedAt:              now,
		CompletionPercent:      1.0,
		SourceClusterName:      clusterName,
		DestinationClusterName: clusterName,
		BackupEndTime:          backupEndTime,
		CompletedAt:            &completedAt,
		CrdbJobId:              ptr("12345"),
		Objects:                &restoreItems,
		RestoreOpts:            &client.RestoreOpts{IntoDb: ptr("target_db")},
	}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(cluster, nil, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(backupConfig, nil, nil).AnyTimes()
	s.EXPECT().CreateRestore(gomock.Any(), clusterID, createRestoreReq).Return(restore, nil, nil)
	// Initial calls return PENDING status (to mimic polling).
	s.EXPECT().GetRestore(gomock.Any(), clusterID, restoreID).Return(restore, nil, nil).Times(3)
	// GetRestore finally returns SUCCESS status.
	s.EXPECT().GetRestore(gomock.Any(), clusterID, restoreID).Return(restoreSuccess, nil, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(nil, nil, nil)
	testRestoreResource(t, true, clusterName, "TABLE", "DATABASE", objects, opts, s, context.Background(), true)
}

func testRestoreResource(
	t *testing.T,
	useMock bool,
	clusterName string,
	restoreType string,
	newRestoreType string,
	objects []RestoreItem,
	opts *RestoreOpts,
	service client.Service,
	ctx context.Context,
	isAdvancedCluster bool,
) {
	var (
		restoreResourceName = "cockroach_restore.test_restore"
		clusterResourceName = "cockroach_cluster.test_cluster"
	)
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					traceMessageStep("creating cluster and waiting for backups to become available")
				},
				Config: getTestRestoreResourceConfig(clusterName, false /*includeRestore*/, restoreType, objects, opts, isAdvancedCluster),
				Check:  testWaitForBackupReadyFunc(t, useMock, ctx, service),
			},
			{
				PreConfig: func() {
					traceMessageStep("kicking off the restore job")
				},
				Config: getTestRestoreResourceConfig(clusterName, true /*includeRestore*/, restoreType, objects, opts, isAdvancedCluster),
				Check:  testCheckRestoreResource(restoreResourceName, clusterResourceName, restoreType),
			},
			{
				PreConfig: func() {
					traceMessageStep("attempting to update the restore resource")
				},
				Config:      getTestRestoreResourceConfig(clusterName, true /*includeRestore*/, newRestoreType, objects, opts, isAdvancedCluster),
				ExpectError: regexp.MustCompile("Immutable resource update attempted"),
			},
			{
				PreConfig: func() {
					traceMessageStep("attempting to import state")
				},
				ResourceName: restoreResourceName,
				ImportState:  true,
				ExpectError:  regexp.MustCompile("Import not supported"),
			},
		},
	})
}

func getTestRestoreResourceConfig(
	clusterName string,
	includeRestore bool,
	restoreType string,
	objects []RestoreItem,
	opts *RestoreOpts,
	isAdvancedCluster bool,
) string {
	resources := fmt.Sprintf(`
resource "cockroach_cluster" "test_cluster" {
	name           = "%s"
	cloud_provider = "GCP"
	plan           = "STANDARD"
	serverless = {
		usage_limits = {
			provisioned_virtual_cpus = 2
		}
		upgrade_type = "AUTOMATIC"
	}
	regions = [{
		name: "us-central1"
	}]
	backup_config = {
		enabled           = true
		frequency_minutes = 5
		retention_days    = 30
	}
}
`, clusterName)

	// Using an advanced plan clusters in cases where standard plan clusters are not supported (e.g. TABLE and DATABASE restores).
	if isAdvancedCluster {
		resources = fmt.Sprintf(`
resource "cockroach_cluster" "test_cluster" {
	name           = "%s"
	cloud_provider = "GCP"
	cockroach_version = "%s"
	dedicated = {
		storage_gib = 15
		num_virtual_cpus = 4
		cidr_range = "172.28.0.0/16"
	}
	regions = [{
		name: "us-central1"
		node_count: 1
	}]
}
`, clusterName, latestClusterMajorVersion)
	}

	if includeRestore {
		resources += fmt.Sprintf(`
resource "cockroach_restore" "test_restore" {
	destination_cluster_id = cockroach_cluster.test_cluster.id
	type = "%s"
	source_cluster_id = cockroach_cluster.test_cluster.id
	%s
	%s
}
`, restoreType, constructRestoreItems(objects), constructRestoreOpts(opts))
	}
	return resources
}

func constructRestoreItems(objects []RestoreItem) string {
	var objectBlocks []string
	for _, o := range objects {
		var database, schema, table string

		if IsKnown(o.Database) {
			database = fmt.Sprintf(`database = "%s"`, o.Database.ValueString())
		}

		if IsKnown(o.Schema) {
			schema = fmt.Sprintf(`schema = "%s"`, o.Schema.ValueString())
		}

		if len(o.Tables.Elements()) > 0 {
			var tableElements []string
			for _, table := range o.Tables.Elements() {
				tableElements = append(tableElements, table.String())
			}
			table = fmt.Sprintf(`tables = [%s]`, strings.Join(tableElements, `, `))
		}

		objectBlocks = append(objectBlocks, fmt.Sprintf(`objects {
		%s
        %s
		%s
	}`, database, schema, table))
	}

	return strings.Join(objectBlocks, "\n\t")
}

func constructRestoreOpts(opts *RestoreOpts) string {
	var optsStr string
	if opts != nil {
		var fields []string

		if IsKnown(opts.IntoDB) {
			fields = append(fields, fmt.Sprintf(`into_db = "%s"`, opts.IntoDB.ValueString()))
		}
		if IsKnown(opts.NewDBName) {
			fields = append(fields, fmt.Sprintf(`new_db_name = "%s"`, opts.NewDBName.ValueString()))
		}
		if IsKnown(opts.SkipLocalitiesCheck) {
			fields = append(fields, fmt.Sprintf(`skip_localities_check = %t`, opts.SkipLocalitiesCheck.ValueBool()))
		}
		if IsKnown(opts.SkipMissingForeignKeys) {
			fields = append(fields, fmt.Sprintf(`skip_missing_foreign_keys = %t`, opts.SkipMissingForeignKeys.ValueBool()))
		}
		if IsKnown(opts.SkipMissingSequences) {
			fields = append(fields, fmt.Sprintf(`skip_missing_sequences = %t`, opts.SkipMissingSequences.ValueBool()))
		}
		if IsKnown(opts.SchemaOnly) {
			fields = append(fields, fmt.Sprintf(`schema_only = %t`, opts.SchemaOnly.ValueBool()))
		}

		optsStr = fmt.Sprintf(`restore_opts = {
		%s
	}`, strings.Join(fields, "\n\t\t"))
	}
	return optsStr
}

func testCheckRestoreResource(restoreResourceName string, clusterResourceName string, restoreType string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		p := testAccProvider.(*provider)
		p.service = NewService(cl)

		// Get the restore resource from state.
		restore := s.RootModule().Resources[restoreResourceName]
		if restore == nil {
			return fmt.Errorf("restore resource not found in state")
		}

		// Get the cluster resource from state.
		cluster := s.RootModule().Resources[clusterResourceName]
		if cluster == nil {
			return fmt.Errorf("cluster resource not found in state")
		}
		clusterID := cluster.Primary.Attributes["id"]

		destinationClusterID := restore.Primary.Attributes["destination_cluster_id"]
		if destinationClusterID != clusterID {
			return fmt.Errorf("expected destination_cluster_id to be %s, got %s", clusterID, destinationClusterID)
		}

		sourceClusterID := restore.Primary.Attributes["source_cluster_id"]
		if sourceClusterID != clusterID {
			return fmt.Errorf("expected source_cluster_id to be %s, got %s", clusterID, sourceClusterID)
		}

		restoreTypeAttr := restore.Primary.Attributes["type"]
		if restoreTypeAttr != restoreType {
			return fmt.Errorf("expected type to be %s, got %s", restoreType, restoreTypeAttr)
		}

		backupID := restore.Primary.Attributes["backup_id"]
		if backupID == "" {
			return fmt.Errorf("expected backup_id to be set, got empty string")
		}

		restoreID := restore.Primary.Attributes["id"]
		if restoreID == "" {
			return fmt.Errorf("expected id to be set, got empty string")
		}

		createdAt := restore.Primary.Attributes["created_at"]
		if createdAt == "" {
			return fmt.Errorf("expected created_at to be set, got empty string")
		}

		status := restore.Primary.Attributes["status"]
		if status == "" {
			return fmt.Errorf("expected status to be set, got empty string")
		}

		sourceClusterName := restore.Primary.Attributes["source_cluster_name"]
		if sourceClusterName == "" {
			return fmt.Errorf("expected source_cluster_name to be set, got empty string")
		}

		destinationClusterName := restore.Primary.Attributes["destination_cluster_name"]
		if destinationClusterName == "" {
			return fmt.Errorf("expected destination_cluster_name to be set, got empty string")
		}

		backupEndTime := restore.Primary.Attributes["backup_end_time"]
		if backupEndTime == "" {
			return fmt.Errorf("expected backup_end_time to be set, got empty string")
		}

		if status == "SUCCESS" {
			// completed_at is only set when restore completes, so we check it's set if the restore was successful
			completedAt := restore.Primary.Attributes["completed_at"]
			if completedAt == "" {
				return fmt.Errorf("expected completed_at to be set for completed restore, got empty string")
			}
			// crdb_job_id is set while the restore is running, so we can only guarantee it's set if the restore was successful
			crdbJobID := restore.Primary.Attributes["crdb_job_id"]
			if crdbJobID == "" {
				return fmt.Errorf("expected crdb_job_id to be set, got empty string")
			}
		}

		return nil
	}
}
