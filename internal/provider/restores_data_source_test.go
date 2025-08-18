package provider

import (
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

func TestIntegrationRestoresDataSource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-cluster-with-restores-%s", tfTestPrefix, GenerateRandomString(4))
	clusterId := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	// Mock cluster creation.
	cluster := testGetStandardCluster(clusterId, clusterName)

	// Mock backup configuration.
	backupConfig := &client.BackupConfiguration{
		Enabled:          true,
		FrequencyMinutes: 5, // Most frequent backup interval.
		RetentionDays:    30,
	}

	endTime := time.Now().Truncate(time.Second).UTC()
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

	// Mock restores response.
	restores := &client.ListRestoresResponse{
		Restores: []client.Restore{
			{
				Id:                "00000000-0000-0000-0000-000000000001",
				BackupId:          "00000000-0000-0000-0000-000000000002",
				Status:            "COMPLETED",
				CreatedAt:         time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Type:              "TABLE",
				CompletionPercent: 1.0,
			},
			{
				Id:                "00000000-0000-0000-0000-000000000003",
				BackupId:          "00000000-0000-0000-0000-000000000004",
				Status:            "PENDING",
				CreatedAt:         time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC),
				Type:              "DATABASE",
				CompletionPercent: 0.5,
			},
		},
	}

	httpOkResponse := &http.Response{Status: http.StatusText(http.StatusOK)}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterId).
		Return(cluster, httpOkResponse, nil).AnyTimes()
	s.EXPECT().UpdateBackupConfiguration(gomock.Any(), clusterId, gomock.Any()).
		Return(backupConfig, httpOkResponse, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterId).
		Return(backupConfig, httpOkResponse, nil).AnyTimes()
	// Calling ListRestores with dates.
	s.EXPECT().ListRestores(gomock.Any(), clusterId, listRestoresOptionsWithDates).
		Return(restores, httpOkResponse, nil).Times(3)
	// Calling ListRestores with timestamps.
	s.EXPECT().ListRestores(gomock.Any(), clusterId, listRestoresOptionsWithTimestamp).
		Return(restores, httpOkResponse, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterId)

	testRestoresDataSource(
		t,
		clusterName,
		startTime,
		endTime,
		true,
	)
}

func testRestoresDataSource(t *testing.T, clusterName string, startTime time.Time, endTime time.Time, useMock bool) {
	restoresDataSourceName := "data.cockroach_restores.test"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					traceMessageStep("creating cluster and listing restore jobs between two dates")
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
data "cockroach_restores" "test" {
    cluster_id = cockroach_cluster.test_cluster.id
	start_time = "%s"
	end_time = "%s"
	limit = 2
	sort_order = "DESC"
}
`, testGetStandardClusterConfig(clusterName, true /*frequentBackup*/), startTime, endTime)
}
