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
	restorePending := &client.Restore{
		Id:                restoreID,
		BackupId:          backupID,
		Type:              client.RESTORETYPETYPE_CLUSTER,
		Status:            client.RESTORESTATUSTYPE_PENDING,
		CreatedAt:         restoreTime,
		CompletionPercent: 0.0,
	}

	restoreSuccess := &client.Restore{
		Id:                restoreID,
		BackupId:          backupID,
		Type:              client.RESTORETYPETYPE_CLUSTER,
		Status:            client.RESTORESTATUSTYPE_SUCCESS,
		CreatedAt:         restoreTime,
		CompletionPercent: 1.0,
	}

	// Mock restores response.
	restores := &client.ListRestoresResponse{
		Restores: []client.Restore{
			*restoreSuccess,
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
) {
	restoresDataSourceName := "data.cockroach_restores.test"

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
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "cluster_id"),
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.id"),
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.backup_id"),
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.status"),
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.created_at"),
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.type"),
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.completion_percent"),
				),
			},
			{
				PreConfig: func() {
					traceMessageStep("listing restore jobs between two timestamps")
				},
				Config: getTestRestoresDataSourceConfig(clusterName, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "cluster_id"),
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.id"),
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.backup_id"),
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.status"),
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.created_at"),
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.type"),
					resource.TestCheckResourceAttrSet(restoresDataSourceName, "restores.0.completion_percent"),
				),
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
