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

// TestAccBackupsDataSource attempts to list backups for a real cluster.
// It will be skipped if TF_ACC isn't set.
func TestAccBackupsDataSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := testAccProvider.(*provider)
	p.service = NewService(cl)
	clusterName := fmt.Sprintf("%s-cluster-with-backups-%s", tfTestPrefix, GenerateRandomString(4))

	// Use 2 days from now as the end time so that date-only inputs (without timestamp)
	// still include all of today’s data and the test passes regardless of what time of
	// day it's run.
	endTime := time.Now().Add(2 * 24 * time.Hour).Truncate(time.Second).UTC()
	startTime := endTime.Add(-3 * 24 * time.Hour)

	testBackupsDataSource(
		t,
		clusterName,
		startTime,
		endTime,
		false,
		p.service,
		ctx,
	)
}

func TestIntegrationBackupsDataSource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-cluster-with-backups-%s", tfTestPrefix, GenerateRandomString(4))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

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

	// Use 2 days from now as the end time so that date-only inputs (without timestamp)
	// still include all of today’s data and the test passes regardless of what time of
	// day it's run.
	endTime := time.Now().Add(2 * 24 * time.Hour).Truncate(time.Second).UTC()
	startTime := endTime.Add(-3 * 24 * time.Hour)
	limit := int32(2)
	order := "DESC"

	listBackupsOptionsWithDate := &client.ListBackupsOptions{
		StartTime:           ptr(startTime.Truncate(24 * time.Hour)),
		EndTime:             ptr(endTime.Truncate(24 * time.Hour)),
		PaginationLimit:     &limit,
		PaginationSortOrder: &order,
	}
	listBackupsOptionsWithTimestamp := &client.ListBackupsOptions{
		StartTime:           &startTime,
		EndTime:             &endTime,
		PaginationLimit:     &limit,
		PaginationSortOrder: &order,
	}

	// Mock backups response.
	backups := &client.ListBackupsResponse{
		Backups: []client.BackupSummary{
			{
				Id:       "00000000-0000-0000-0000-000000000001",
				AsOfTime: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	httpOkResponse := &http.Response{Status: http.StatusText(http.StatusOK)}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(cluster, httpOkResponse, nil).AnyTimes()
	s.EXPECT().UpdateBackupConfiguration(gomock.Any(), clusterID, gomock.Any()).Return(backupConfig, httpOkResponse, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(backupConfig, httpOkResponse, nil).AnyTimes()
	s.EXPECT().ListBackups(gomock.Any(), clusterID, listBackupsOptionsWithDate).Return(backups, httpOkResponse, nil).Times(3)
	s.EXPECT().ListBackups(gomock.Any(), clusterID, listBackupsOptionsWithTimestamp).Return(backups, httpOkResponse, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	ctx := context.Background()
	testBackupsDataSource(
		t,
		clusterName,
		startTime,
		endTime,
		true,
		s,
		ctx,
	)
}

func testBackupsDataSource(
	t *testing.T,
	clusterName string,
	startTime time.Time,
	endTime time.Time,
	useMock bool,
	service client.Service,
	ctx context.Context,
) {
	backupsDataSourceName := "data.cockroach_backups.test"

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
					traceMessageStep("list backups between two dates")
				},
				Config: getTestBackupsDataSourceConfig(clusterName, startTime.Format(time.DateOnly), endTime.Format(time.DateOnly)),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(backupsDataSourceName, "cluster_id"),
					resource.TestCheckResourceAttrSet(backupsDataSourceName, "backups.0.id"),
					resource.TestCheckResourceAttrSet(backupsDataSourceName, "backups.0.as_of_time"),
				),
			},
			{
				PreConfig: func() {
					traceMessageStep("list backups between two timestamps")
				},
				Config: getTestBackupsDataSourceConfig(clusterName, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(backupsDataSourceName, "cluster_id"),
					resource.TestCheckResourceAttrSet(backupsDataSourceName, "backups.0.id"),
					resource.TestCheckResourceAttrSet(backupsDataSourceName, "backups.0.as_of_time"),
				),
			},
		},
	})
}

func getTestBackupsDataSourceConfig(clusterName string, startTime string, endTime string) string {
	return fmt.Sprintf(`
%s
data "cockroach_backups" "test" {
	cluster_id = cockroach_cluster.test_cluster.id
	start_time = "%s"
	end_time = "%s"
	limit = 2
	sort_order = "DESC"
}
`, testGetStandardClusterConfig(clusterName, true /*frequentBackup*/), startTime, endTime)
}
