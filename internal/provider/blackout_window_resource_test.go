package provider

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccBlackoutWindowResource attempts to create, check, and destroy a
// real blackout window. It will be skipped if TF_ACC isn't set.
func TestAccBlackoutWindowResource(t *testing.T) {
	t.Parallel()

	clusterName := fmt.Sprintf("%s-bwin-%s", tfTestPrefix, GenerateRandomString(4))
	start := time.Now().AddDate(0, 0, 8).UTC()
	end := start.Add(48 * time.Hour)
	updatedStart := start.Add(6 * time.Hour)
	updatedEnd := updatedStart.Add(36 * time.Hour)

	testBlackoutWindowResource(t, clusterName, start, end, updatedStart, updatedEnd, false)
}

// TestIntegrationBlackoutWindowResource uses a mocked service implementation
// to validate the Terraform CRUD workflow without touching real infrastructure.
func TestIntegrationBlackoutWindowResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-bwin-%s", tfTestPrefix, GenerateRandomString(4))
	standardClusterID := "00000000-0000-0000-0000-000000000111"
	advancedClusterID := "00000000-0000-0000-0000-000000000222"
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service { return s })()

	standardCluster := &client.Cluster{
		Id:               standardClusterID,
		Name:             clusterName,
		CockroachVersion: "v23.1.0",
		Plan:             "STANDARD",
		CloudProvider:    "GCP",
		State:            "CREATED",
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{
				UsageLimits: &client.UsageLimits{ProvisionedVirtualCpus: ptr(int64(2))},
			},
		},
		Regions: []client.Region{
			{
				Name:    "us-central1",
				Primary: ptr(true),
			},
		},
	}

	advancedCluster := &client.Cluster{
		Id:               advancedClusterID,
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

	initialStart := time.Date(2025, 3, 15, 9, 0, 0, 0, time.UTC)
	initialEnd := initialStart.Add(72 * time.Hour)
	updatedStart := initialStart.Add(6 * time.Hour)
	updatedEnd := updatedStart.Add(48 * time.Hour)

	createdWindow := &client.BlackoutWindow{
		Id:        "blackout-window-id",
		ClusterId: advancedClusterID,
		StartTime: initialStart,
		EndTime:   initialEnd,
	}

	createdMaintenanceWindowInfo := &client.MaintenanceWindow{
		OffsetDuration: "0s",
		WindowDuration: "21600s",
	}

	// Create standard cluster
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(standardCluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), standardClusterID).
		Return(standardCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), standardClusterID).
		Return(initialBackupConfig, httpOk, nil)
	s.EXPECT().CreateBlackoutWindow(gomock.Any(), standardClusterID, &client.CreateBlackoutWindowRequest{
		StartTime: initialStart,
		EndTime:   initialEnd,
	}).
		Return(nil, nil, fmt.Errorf("invalid argument: blackout windows are supported for advanced clusters only"))

	// Delete standard cluster
	s.EXPECT().GetCluster(gomock.Any(), standardClusterID).
		Return(standardCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), standardClusterID).
		Return(initialBackupConfig, httpOk, nil)
	s.EXPECT().DeleteCluster(gomock.Any(), standardClusterID)

	// Create advanced cluster
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(advancedCluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), advancedClusterID).
		Return(advancedCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), advancedClusterID).
		Return(initialBackupConfig, httpOk, nil)

	// Set maintenance window and create blackout window for advanced cluster
	s.EXPECT().SetMaintenanceWindow(gomock.Any(), advancedClusterID, createdMaintenanceWindowInfo).
		Return(createdMaintenanceWindowInfo, nil, nil)
	s.EXPECT().
		CreateBlackoutWindow(gomock.Any(), advancedClusterID, &client.CreateBlackoutWindowRequest{
			StartTime: initialStart,
			EndTime:   initialEnd,
		}).
		Return(createdWindow, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), advancedClusterID).
		Return(advancedCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).Times(2)

	s.EXPECT().GetBackupConfiguration(gomock.Any(), advancedClusterID).
		Return(initialBackupConfig, httpOk, nil)

	s.EXPECT().GetMaintenanceWindow(gomock.Any(), advancedClusterID).
		Return(createdMaintenanceWindowInfo, nil, nil)

	s.EXPECT().
		GetBlackoutWindow(gomock.Any(), advancedClusterID, createdWindow.Id).
		Return(createdWindow, nil, nil)

	// Update advanced cluster blackout window
	updatedWindow := &client.BlackoutWindow{
		Id:        createdWindow.Id,
		ClusterId: createdWindow.ClusterId,
		StartTime: updatedStart,
		EndTime:   updatedEnd,
	}

	s.EXPECT().GetCluster(gomock.Any(), advancedClusterID).
		Return(advancedCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	s.EXPECT().GetBackupConfiguration(gomock.Any(), advancedClusterID).
		Return(initialBackupConfig, httpOk, nil)

	s.EXPECT().GetMaintenanceWindow(gomock.Any(), advancedClusterID).
		Return(createdMaintenanceWindowInfo, nil, nil)

	s.EXPECT().
		GetBlackoutWindow(gomock.Any(), advancedClusterID, createdWindow.Id).
		Return(createdWindow, nil, nil)

	s.EXPECT().
		UpdateBlackoutWindow(gomock.Any(), advancedClusterID, createdWindow.Id, &client.UpdateBlackoutWindowRequest{
			StartTime: &updatedStart,
			EndTime:   &updatedEnd,
		}).
		Return(updatedWindow, nil, nil)

	s.EXPECT().GetCluster(gomock.Any(), advancedClusterID).
		Return(advancedCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	s.EXPECT().GetBackupConfiguration(gomock.Any(), advancedClusterID).
		Return(initialBackupConfig, httpOk, nil)

	s.EXPECT().GetMaintenanceWindow(gomock.Any(), advancedClusterID).
		Return(createdMaintenanceWindowInfo, nil, nil)

	s.EXPECT().GetBlackoutWindow(gomock.Any(), advancedClusterID, createdWindow.Id).
		Return(updatedWindow, nil, nil)

	// Import advanced cluster blackout window and delete
	s.EXPECT().GetBlackoutWindow(gomock.Any(), advancedClusterID, createdWindow.Id).
		Return(updatedWindow, nil, nil).Times(2)

	s.EXPECT().GetMaintenanceWindow(gomock.Any(), advancedClusterID).
		Return(createdMaintenanceWindowInfo, nil, nil)

	s.EXPECT().GetCluster(gomock.Any(), advancedClusterID).
		Return(advancedCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	s.EXPECT().GetBackupConfiguration(gomock.Any(), advancedClusterID).
		Return(initialBackupConfig, httpOk, nil)

	s.EXPECT().
		DeleteBlackoutWindow(gomock.Any(), advancedClusterID, createdWindow.Id).
		Return(createdWindow, nil, nil)

	s.EXPECT().DeleteMaintenanceWindow(gomock.Any(), advancedClusterID)
	s.EXPECT().DeleteCluster(gomock.Any(), advancedClusterID)

	testBlackoutWindowResource(t, clusterName, initialStart, initialEnd, updatedStart, updatedEnd, true)
}

func testBlackoutWindowResource(
	t *testing.T,
	clusterName string,
	initialStart, initialEnd, updatedStart, updatedEnd time.Time,
	useMock bool,
) {
	var (
		clusterResourceName           = "cockroach_cluster.test"
		blackoutResourceName          = "cockroach_blackout_window.test"
		maintenanceWindowResourceName = "cockroach_maintenance_window.mw"
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestBlackoutWindowConfigStandard(clusterName, initialStart, initialEnd),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists(clusterResourceName),
				),
				ExpectError: regexp.MustCompile(`invalid argument:\s+blackout windows are\s+supported for advanced clusters only`),
			},
			{
				Config:  " ",
				Destroy: true,
			},
			{
				Config: getTestBlackoutWindowConfigAdvanced(clusterName, initialStart, initialEnd),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists(clusterResourceName),
					resource.TestCheckResourceAttr(maintenanceWindowResourceName, "offset_duration", "0"),
					resource.TestCheckResourceAttr(maintenanceWindowResourceName, "window_duration", "21600"),
					resource.TestCheckResourceAttr(blackoutResourceName, "start_time", initialStart.Format(time.RFC3339)),
					resource.TestCheckResourceAttr(blackoutResourceName, "end_time", initialEnd.Format(time.RFC3339)),
				),
			},
			{
				Config: getTestBlackoutWindowConfigAdvanced(clusterName, updatedStart, updatedEnd),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(blackoutResourceName, "start_time", updatedStart.Format(time.RFC3339)),
					resource.TestCheckResourceAttr(blackoutResourceName, "end_time", updatedEnd.Format(time.RFC3339)),
				),
			},
			{
				ResourceName:      blackoutResourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources[blackoutResourceName]
					if !ok {
						return "", fmt.Errorf("resource %s not found in state", blackoutResourceName)
					}
					return fmt.Sprintf("%s:%s", rs.Primary.Attributes["cluster_id"], rs.Primary.ID), nil
				},
			},
			{
				Config:  " ",
				Destroy: true,
			},
		},
	})
}

func getTestBlackoutWindowConfigStandard(clusterName string, start, end time.Time) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
	name           = "%s"
	cloud_provider = "GCP"
	plan = "STANDARD"
	serverless = {
		usage_limits = {
			provisioned_virtual_cpus = 2
		}
	}
	regions = [{
		name = "us-central1"
	}]
}

resource "cockroach_blackout_window" "test" {
  cluster_id = cockroach_cluster.test.id
  start_time = "%s"
  end_time   = "%s"
}
`, clusterName, start.Format(time.RFC3339), end.Format(time.RFC3339))
}

func getTestBlackoutWindowConfigAdvanced(clusterName string, start, end time.Time) string {
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

resource "cockroach_blackout_window" "test" {
  cluster_id = cockroach_cluster.test.id
  start_time = "%s"
  end_time   = "%s"
  depends_on = [cockroach_maintenance_window.mw]
}
`, clusterName, start.Format(time.RFC3339), end.Format(time.RFC3339))
}
