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
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccBlackoutWindowDataSource attempts to list blackout windows for a real
// cluster. It will be skipped if TF_ACC isn't set.
func TestAccBlackoutWindowDataSource(t *testing.T) {
	t.Parallel()

	clusterName := fmt.Sprintf("%s-bwin-ds-%s", tfTestPrefix, GenerateRandomString(4))

	// First BOW starts 8 days from now and lasts 2 hours.
	// Second BOW starts 3 days after the first ends and lasts 2 hours.
	// Third BOW starts 6 days after the second ends and lasts 2 hours.
	firstStart := time.Now().AddDate(0, 0, 8).UTC()
	firstEnd := firstStart.Add(2 * time.Hour)

	secondStart := firstStart.Add(72 * time.Hour)
	secondEnd := secondStart.Add(2 * time.Hour)

	thirdStart := firstStart.Add(144 * time.Hour)
	thirdEnd := thirdStart.Add(2 * time.Hour)

	testBlackoutWindowDataSource(
		t,
		clusterName,
		firstStart, firstEnd,
		secondStart, secondEnd,
		thirdStart, thirdEnd,
		false,
	)
}

// TestIntegrationBlackoutWindowDataSource attempts to list blackout windows for
// a cluster, but uses a mocked API service. We schedule three blackout
// windows, then call the data source twice (first page with limit=2, second page
// using the returned next_page). Ensures sorting and pagination logic match the
// real API.
func TestIntegrationBlackoutWindowDataSource(t *testing.T) {

	clusterName := fmt.Sprintf("%s-bwin-ds-%s", tfTestPrefix, GenerateRandomString(4))
	clusterID := "00000000-0000-0000-0000-000000000111"
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	firstStart := time.Date(2025, 3, 15, 9, 0, 0, 0, time.UTC)
	firstEnd := firstStart.Add(2 * time.Hour)
	secondStart := firstStart.Add(72 * time.Hour)
	secondEnd := secondStart.Add(2 * time.Hour)
	thirdStart := firstStart.Add(144 * time.Hour)
	thirdEnd := thirdStart.Add(2 * time.Hour)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	cluster := &client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: "v23.1.0",
		Plan:             "ADVANCED",
		CloudProvider:    "GCP",
		State:            "CREATED",
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				MachineType:    "m5.xlarge",
				NumVirtualCpus: 4,
				StorageGib:     35,
				MemoryGib:      8,
			},
		},
		Regions: []client.Region{
			{
				Name:      "us-east1",
				NodeCount: 3,
			},
		},
	}

	createdMaintenanceWindowInfo := &client.MaintenanceWindow{
		OffsetDuration: "0s",
		WindowDuration: "21600s",
	}

	firstBlackoutWindow := &client.BlackoutWindow{
		Id:        "bw-1111",
		ClusterId: clusterID,
		StartTime: firstStart,
		EndTime:   firstEnd,
	}

	secondBlackoutWindow := &client.BlackoutWindow{
		Id:        "bw-2222",
		ClusterId: clusterID,
		StartTime: secondStart,
		EndTime:   secondEnd,
	}

	thirdBlackoutWindow := &client.BlackoutWindow{
		Id:        "bw-3333",
		ClusterId: clusterID,
		StartTime: thirdStart,
		EndTime:   thirdEnd,
	}

	// Create the cluster
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil)

	// Create maintenance window and blackout windows
	s.EXPECT().SetMaintenanceWindow(gomock.Any(), clusterID, createdMaintenanceWindowInfo).
		Return(createdMaintenanceWindowInfo, nil, nil)
	s.EXPECT().
		CreateBlackoutWindow(gomock.Any(), clusterID, &client.CreateBlackoutWindowRequest{
			StartTime: firstStart,
			EndTime:   firstEnd,
		}).
		Return(firstBlackoutWindow, nil, nil)
	s.EXPECT().
		CreateBlackoutWindow(gomock.Any(), clusterID, &client.CreateBlackoutWindowRequest{
			StartTime: secondStart,
			EndTime:   secondEnd,
		}).
		Return(secondBlackoutWindow, nil, nil)
	s.EXPECT().
		CreateBlackoutWindow(gomock.Any(), clusterID, &client.CreateBlackoutWindowRequest{
			StartTime: thirdStart,
			EndTime:   thirdEnd,
		}).
		Return(thirdBlackoutWindow, nil, nil)

	limit := int32(2)
	order := "DESC"
	nextPage := "next-page-token"

	// First read of data source - two blackout windows
	s.EXPECT().ListBlackoutWindows(gomock.Any(), clusterID, &client.ListBlackoutWindowsOptions{
		PaginationLimit:     &limit,
		PaginationSortOrder: &order,
	}).Return(&client.ListBlackoutWindowsResponse{
		BlackoutWindows: &[]client.BlackoutWindow{*thirdBlackoutWindow, *secondBlackoutWindow},
		Pagination: &client.KeysetPaginationResponse{
			NextPage: &nextPage,
		},
	}, nil, nil).Times(2)

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	// Second read of data source - one blackout window
	s.EXPECT().ListBlackoutWindows(gomock.Any(), clusterID, &client.ListBlackoutWindowsOptions{
		PaginationPage:      &nextPage,
		PaginationSortOrder: &order,
	}).Return(&client.ListBlackoutWindowsResponse{
		BlackoutWindows: &[]client.BlackoutWindow{*firstBlackoutWindow},
	}, nil, nil).Times(2)

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil)

	s.EXPECT().GetMaintenanceWindow(gomock.Any(), clusterID).
		Return(createdMaintenanceWindowInfo, nil, nil)

	s.EXPECT().
		GetBlackoutWindow(gomock.Any(), clusterID, firstBlackoutWindow.Id).
		Return(firstBlackoutWindow, nil, nil)

	s.EXPECT().
		GetBlackoutWindow(gomock.Any(), clusterID, secondBlackoutWindow.Id).
		Return(secondBlackoutWindow, nil, nil)

	s.EXPECT().
		GetBlackoutWindow(gomock.Any(), clusterID, thirdBlackoutWindow.Id).
		Return(thirdBlackoutWindow, nil, nil)

	s.EXPECT().ListBlackoutWindows(gomock.Any(), clusterID, &client.ListBlackoutWindowsOptions{
		PaginationLimit:     &limit,
		PaginationSortOrder: &order,
	}).Return(&client.ListBlackoutWindowsResponse{
		BlackoutWindows: &[]client.BlackoutWindow{*thirdBlackoutWindow, *secondBlackoutWindow},
		Pagination: &client.KeysetPaginationResponse{
			NextPage: &nextPage,
		},
	}, nil, nil)

	s.EXPECT().ListBlackoutWindows(gomock.Any(), clusterID, &client.ListBlackoutWindowsOptions{
		PaginationPage:      &nextPage,
		PaginationSortOrder: &order,
	}).Return(&client.ListBlackoutWindowsResponse{
		BlackoutWindows: &[]client.BlackoutWindow{*firstBlackoutWindow},
	}, nil, nil)

	s.EXPECT().
		DeleteBlackoutWindow(gomock.Any(), clusterID, firstBlackoutWindow.Id).
		Return(firstBlackoutWindow, nil, nil)

	s.EXPECT().
		DeleteBlackoutWindow(gomock.Any(), clusterID, secondBlackoutWindow.Id).
		Return(secondBlackoutWindow, nil, nil)

	s.EXPECT().
		DeleteBlackoutWindow(gomock.Any(), clusterID, thirdBlackoutWindow.Id).
		Return(thirdBlackoutWindow, nil, nil)

	s.EXPECT().DeleteMaintenanceWindow(gomock.Any(), clusterID)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testBlackoutWindowDataSource(
		t,
		clusterName,
		firstStart, firstEnd,
		secondStart, secondEnd,
		thirdStart, thirdEnd,
		true,
	)
}

func testBlackoutWindowDataSource(
	t *testing.T,
	clusterName string,
	firstStart, firstEnd, secondStart, secondEnd, thirdStart, thirdEnd time.Time,
	useMock bool,
) {
	var (
		clusterResourceName            = "cockroach_cluster.test"
		blackoutWindowDataSourceFirst  = "data.cockroach_blackout_windows.first"
		blackoutWindowDataSourceSecond = "data.cockroach_blackout_windows.second"
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getAccBlackoutWindowDataSourceConfig(
					clusterName,
					firstStart.Format(time.RFC3339), firstEnd.Format(time.RFC3339),
					secondStart.Format(time.RFC3339), secondEnd.Format(time.RFC3339),
					thirdStart.Format(time.RFC3339), thirdEnd.Format(time.RFC3339),
				),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists(clusterResourceName),
					// First page (limit 2)
					resource.TestCheckResourceAttrSet(blackoutWindowDataSourceFirst, "cluster_id"),
					resource.TestCheckResourceAttr(blackoutWindowDataSourceFirst, "blackout_windows.#", "2"),
					resource.TestCheckResourceAttrSet(blackoutWindowDataSourceFirst, "next_page"),
					resource.TestCheckResourceAttrPair(blackoutWindowDataSourceFirst, "blackout_windows.0.id", "cockroach_blackout_window.bw_third", "id"),
					resource.TestCheckResourceAttrPair(blackoutWindowDataSourceFirst, "blackout_windows.1.id", "cockroach_blackout_window.bw_second", "id"),

					// Second page (uses next_page from first)
					resource.TestCheckResourceAttrSet(blackoutWindowDataSourceSecond, "cluster_id"),
					resource.TestCheckResourceAttr(blackoutWindowDataSourceSecond, "blackout_windows.#", "1"),
					resource.TestCheckResourceAttrPair(blackoutWindowDataSourceSecond, "blackout_windows.0.id", "cockroach_blackout_window.bw_first", "id"),
					resource.TestCheckNoResourceAttr(blackoutWindowDataSourceSecond, "next_page"),
				),
			},
		},
	})
}

func getAccBlackoutWindowDataSourceConfig(
	clusterName, firstStart, firstEnd, secondStart, secondEnd, thirdStart, thirdEnd string,
) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "GCP"
  dedicated = {
    storage_gib  = 35
  	num_virtual_cpus = 4
  }
  regions = [{
    name = "us-east1"
    node_count: 3
  }]
}

resource "cockroach_maintenance_window" "mw" {
  id              = cockroach_cluster.test.id
  offset_duration = 0
  window_duration = 21600
}

resource "cockroach_blackout_window" "bw_first" {
  cluster_id = cockroach_cluster.test.id
  start_time = "%s"
  end_time   = "%s"
  depends_on = [cockroach_maintenance_window.mw]
}

resource "cockroach_blackout_window" "bw_second" {
  cluster_id = cockroach_cluster.test.id
  start_time = "%s"
  end_time   = "%s"
  depends_on = [cockroach_maintenance_window.mw]
}

resource "cockroach_blackout_window" "bw_third" {
  cluster_id = cockroach_cluster.test.id
  start_time = "%s"
  end_time   = "%s"
  depends_on = [cockroach_maintenance_window.mw]
}

data "cockroach_blackout_windows" "first" {
  cluster_id = cockroach_cluster.test.id
  limit      = 2
  sort_order = "DESC"
  depends_on = [
    cockroach_blackout_window.bw_first,
    cockroach_blackout_window.bw_second,
    cockroach_blackout_window.bw_third,
  ]
}

data "cockroach_blackout_windows" "second" {
  cluster_id = cockroach_cluster.test.id
  page       = data.cockroach_blackout_windows.first.next_page
  sort_order = "DESC"
  depends_on = [data.cockroach_blackout_windows.first]
}
`,
		clusterName,
		firstStart, firstEnd,
		secondStart, secondEnd,
		thirdStart, thirdEnd,
	)
}
