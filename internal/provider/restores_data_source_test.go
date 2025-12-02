package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRestoresDataSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := testAccProvider.(*provider)
	p.service = NewService(cl)
	clusterName := fmt.Sprintf("%s-cluster-with-restores-%s", tfTestPrefix, GenerateRandomString(4))

	// Use 2 days from now as the end time so that date-only inputs (without timestamp)
	// still include all of today’s data and the test passes regardless of what time of
	// day it's run.
	endTime := time.Now().Add(2 * 24 * time.Hour).Truncate(time.Second).UTC()
	startTime := endTime.Add(-3 * 24 * time.Hour)

	testRestoresDataSource(
		t,
		clusterName,
		startTime,
		endTime,
		false,
		p.service,
		ctx,
		nil,
	)
}

func TestIntegrationRestoresDataSource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-cluster-with-restores-%s", tfTestPrefix, GenerateRandomString(4))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	// Mock cluster creation.
	cluster := testGetStandardCluster(clusterID, clusterName)

	// Mock backup configuration.
	backupConfig := &client.BackupConfiguration{
		Enabled:          true,
		FrequencyMinutes: 5, // Most frequent backup interval.
		RetentionDays:    30,
	}

	// Use tomorrow as the end time so that date-only inputs (without timestamp)
	// still include all of today’s data.
	restoreTime := time.Now()
	endTime := restoreTime.Add(1 * 24 * time.Hour).Truncate(time.Second).UTC()
	startTime := endTime.Add(-3 * 24 * time.Hour)
	limit := int32(2)
	order := "DESC"

	listRestoresOptionsWithDates := &client.ListRestoresOptions{
		StartTime:           ptr(startTime.Truncate(24 * time.Hour)),
		EndTime:             ptr(endTime.Truncate(24 * time.Hour)),
		PaginationLimit:     &limit,
		PaginationSortOrder: &order,
	}
	listRestoresOptionsWithTimestamp := &client.ListRestoresOptions{
		StartTime:           &startTime,
		EndTime:             &endTime,
		PaginationLimit:     &limit,
		PaginationSortOrder: &order,
	}

	// Mock restore for the restore resource creation
	restoreID := "00000000-0000-0000-0000-000000000005"
	backupID := "00000000-0000-0000-0000-000000000001"
	backupEndTime := restoreTime.Add(-1 * time.Hour)

	restorePending := &client.Restore{
		Id:                     restoreID,
		BackupId:               backupID,
		Type:                   client.RESTORETYPETYPE_CLUSTER,
		Status:                 client.RESTORESTATUSTYPE_PENDING,
		CreatedAt:              restoreTime,
		CompletionPercent:      0.0,
		SourceClusterName:      clusterName,
		DestinationClusterName: clusterName,
		BackupEndTime:          backupEndTime,
		CrdbJobId:              ptr("12345"),
	}

	completedAt := restoreTime.Add(30 * time.Minute)
	restoreSuccess := &client.Restore{
		Id:                     restoreID,
		BackupId:               backupID,
		Type:                   client.RESTORETYPETYPE_CLUSTER,
		Status:                 client.RESTORESTATUSTYPE_SUCCESS,
		CreatedAt:              restoreTime,
		CompletionPercent:      1.0,
		SourceClusterName:      clusterName,
		DestinationClusterName: clusterName,
		BackupEndTime:          backupEndTime,
		CompletedAt:            &completedAt,
		CrdbJobId:              ptr("12345"),
	}

	// Mock objects in lexicographically sorted order (as API returns them)
	// These will be grouped by database+schema (for failed TABLE restore)
	mockObjects := tablesRestoreItem("otherdb", "private", []string{"settings"})
	mockObjects = append(mockObjects, tablesRestoreItem("testdb", "public", []string{"orders", "products", "users"})...)

	mockRestoreOpts := &client.RestoreOpts{
		SkipLocalitiesCheck:    ptr(true),
		SkipMissingForeignKeys: ptr(false),
		SchemaOnly:             ptr(false),
		IntoDb:                 ptr("testdb"),
	}

	// Mock a failed restore to test error fields
	failedRestoreID := "00000000-0000-0000-0000-000000000006"
	errorCode := int32(500)
	errorMessage := "restore job failed due to insufficient permissions"
	restoreFailed := &client.Restore{
		Id:                     failedRestoreID,
		BackupId:               backupID,
		Type:                   client.RESTORETYPETYPE_TABLE,
		Status:                 client.RESTORESTATUSTYPE_FAILED,
		CreatedAt:              restoreTime.Add(-2 * time.Hour),
		CompletionPercent:      0.5,
		SourceClusterName:      clusterName,
		DestinationClusterName: clusterName,
		BackupEndTime:          backupEndTime,
		CompletedAt:            &completedAt,
		CrdbJobId:              ptr("67890"),
		ClientErrorCode:        &errorCode,
		ClientErrorMessage:     &errorMessage,
		Objects:                &mockObjects,
		RestoreOpts:            mockRestoreOpts,
	}

	// Mock restores response.
	restores := &client.ListRestoresResponse{
		Restores: []client.Restore{
			*restoreSuccess,
			*restoreFailed,
		},
	}

	httpOkResponse := &http.Response{Status: http.StatusText(http.StatusOK)}

	// Create cluster.
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, httpOkResponse, nil).AnyTimes()
	s.EXPECT().UpdateBackupConfiguration(gomock.Any(), clusterID, gomock.Any()).
		Return(backupConfig, httpOkResponse, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(backupConfig, httpOkResponse, nil).AnyTimes()

	// Kick off restore operation.
	s.EXPECT().CreateRestore(gomock.Any(), clusterID, gomock.Any()).Return(restorePending, httpOkResponse, nil)
	gomock.InOrder(
		// Restore operation is pending.
		s.EXPECT().GetRestore(gomock.Any(), clusterID, restoreID).Return(restorePending, httpOkResponse, nil).Times(3),
		// Restore operation is successful.
		s.EXPECT().GetRestore(gomock.Any(), clusterID, restoreID).Return(restoreSuccess, httpOkResponse, nil).Times(4),
	)

	// Calling ListRestores with dates.
	s.EXPECT().ListRestores(gomock.Any(), clusterID, listRestoresOptionsWithDates).
		Return(restores, httpOkResponse, nil).Times(3)
	// Calling ListRestores with timestamps.
	s.EXPECT().ListRestores(gomock.Any(), clusterID, listRestoresOptionsWithTimestamp).
		Return(restores, httpOkResponse, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testRestoresDataSource(
		t,
		clusterName,
		startTime,
		endTime,
		true,
		s,
		ctx,
		[]resource.TestCheckFunc{
			// Check first restore (SUCCESS CLUSTER restore)
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.0.status", "SUCCESS"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.0.type", "CLUSTER"),

			// Check second restore (FAILED TABLE restore with objects, restore_opts, and error fields)
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.status", "FAILED"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.type", "TABLE"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.client_error_code", "500"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.client_error_message", "restore job failed due to insufficient permissions"),

			// Check objects field - API returns objects in lexicographically sorted order, so we get:
			// Group 0: otherdb.private with 1 table (settings)
			// Group 1: testdb.public with 3 tables (orders, products, users)
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.objects.#", "2"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.objects.0.database", "otherdb"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.objects.0.schema", "private"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.objects.0.tables.#", "1"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.objects.0.tables.0", "settings"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.objects.1.database", "testdb"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.objects.1.schema", "public"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.objects.1.tables.#", "3"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.objects.1.tables.0", "orders"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.objects.1.tables.1", "products"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.objects.1.tables.2", "users"),

			// Check restore_opts fields
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.restore_opts.skip_localities_check", "true"),
			resource.TestCheckResourceAttr("data.cockroach_restores.test", "restores.1.restore_opts.into_db", "testdb"),
		},
	)
}

func testRestoresDataSource(
	t *testing.T,
	clusterName string,
	startTime time.Time,
	endTime time.Time,
	useMock bool,
	service client.Service,
	ctx context.Context,
	additionalChecks []resource.TestCheckFunc,
) {
	restoresDataSourceName := "data.cockroach_restores.test"

	// Build check functions
	buildChecks := func() []resource.TestCheckFunc {
		// Common checks that work for both mock and real scenarios
		checks := []resource.TestCheckFunc{
			resource.TestCheckResourceAttrSet(restoresDataSourceName, "cluster_id"),
			resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.id"),
			resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.backup_id"),
			resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.status"),
			resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.created_at"),
			resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.type"),
			resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.completion_percent"),
			resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.source_cluster_name"),
			resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.destination_cluster_name"),
			resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.backup_end_time"),
			resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.completed_at"),
			resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.crdb_job_id"),
		}

		// Add any additional checks provided by caller
		if len(additionalChecks) > 0 {
			checks = append(checks, additionalChecks...)
		}

		return checks
	}

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					traceMessageStep("creating cluster and waiting for backups to become available")
				},
				Config: testGetStandardClusterConfig(clusterName, true /*frequentBackup*/),
				Check:  testWaitForBackupReadyFunc(t, useMock, ctx, service),
			},
			{
				PreConfig: func() {
					traceMessageStep("starting restore operation and listing restore jobs between two dates")
				},
				Config: getTestRestoresDataSourceConfig(clusterName, startTime.Format(time.DateOnly), endTime.Format(time.DateOnly)),
				Check:  resource.ComposeTestCheckFunc(buildChecks()...),
			},
			{
				PreConfig: func() {
					traceMessageStep("listing restore jobs between two timestamps")
				},
				Config: getTestRestoresDataSourceConfig(clusterName, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)),
				Check:  resource.ComposeTestCheckFunc(buildChecks()...),
			},
		},
	})
}

func getTestRestoresDataSourceConfig(clusterName string, startTime string, endTime string) string {
	return fmt.Sprintf(`
%s
resource "cockroach_restore" "test" {
	source_cluster_id = cockroach_cluster.test_cluster.id
	destination_cluster_id = cockroach_cluster.test_cluster.id
	type = "CLUSTER"
}

data "cockroach_restores" "test" {
    cluster_id = cockroach_cluster.test_cluster.id
	start_time = "%s"
	end_time = "%s"
	limit = 2
	sort_order = "DESC"
	depends_on = [cockroach_restore.test]
}
`, testGetStandardClusterConfig(clusterName, true /*frequentBackup*/), startTime, endTime)
}
