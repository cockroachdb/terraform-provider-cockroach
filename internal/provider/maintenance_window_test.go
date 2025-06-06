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
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccMaintenanceWindowResource attempts to create, check, and destroy a
// real cluster. It will be skipped if TF_ACC isn't set.
func TestAccMaintenanceWindowResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-mwin-%s", tfTestPrefix, GenerateRandomString(4))
	testMaintenanceWindowResource(t, clusterName, false)
}

// TestIntegrationMaintenanceWindowResource attempts to create, check, and
// destroy a cluster, but uses a mocked API service.
func TestIntegrationMaintenanceWindowResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-mwin-%s", tfTestPrefix, GenerateRandomString(4))
	standardClusterID := "00000000-0000-0000-0000-000000000001"
	advancedClusterID := "00000000-0000-0000-0000-000000000002"
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	standardCluster := &client.Cluster{
		Id:               standardClusterID,
		Name:             clusterName,
		CockroachVersion: "v22.2.0",
		Plan:             "STANDARD",
		CloudProvider:    "GCP",
		State:            "CREATED",
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{
				UsageLimits: &client.UsageLimits{
					ProvisionedVirtualCpus: ptr(int64(2)),
				},
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
		CockroachVersion: "v22.2.0",
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
		OffsetDuration: "1010s",
		WindowDuration: "101010s",
	}
	updatedMaintenanceWindowInfo := &client.MaintenanceWindow{
		OffsetDuration: "1100s",
		WindowDuration: "110011s",
	}

	// Create Standard Cluster
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(standardCluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), standardClusterID).
		Return(standardCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), standardClusterID).
		Return(initialBackupConfig, httpOk, nil).Times(2)
	s.EXPECT().SetMaintenanceWindow(gomock.Any(), standardClusterID, createdMaintenanceWindowInfo).
		Return(nil, nil, fmt.Errorf("maintenance windows are supported for advanced clusters only"))

	// Delete Standard Cluster
	s.EXPECT().GetCluster(gomock.Any(), standardClusterID).
		Return(standardCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
	s.EXPECT().DeleteCluster(gomock.Any(), standardClusterID)

	// Create Advanced Cluster
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(advancedCluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), advancedClusterID).
		Return(advancedCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(3)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), advancedClusterID).
		Return(initialBackupConfig, httpOk, nil).Times(3)
	s.EXPECT().SetMaintenanceWindow(gomock.Any(), advancedClusterID, createdMaintenanceWindowInfo).
		Return(createdMaintenanceWindowInfo, nil, nil)
	s.EXPECT().GetMaintenanceWindow(gomock.Any(), advancedClusterID).
		Return(createdMaintenanceWindowInfo, nil, nil)

	// Update
	s.EXPECT().GetCluster(gomock.Any(), advancedClusterID).
		Return(advancedCluster, nil, nil).
		Times(3)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), advancedClusterID).
		Return(initialBackupConfig, httpOk, nil)
	s.EXPECT().GetMaintenanceWindow(gomock.Any(), advancedClusterID).
		Return(createdMaintenanceWindowInfo, nil, nil)
	s.EXPECT().SetMaintenanceWindow(gomock.Any(), advancedClusterID, updatedMaintenanceWindowInfo).
		Return(updatedMaintenanceWindowInfo, nil, nil)
	s.EXPECT().GetMaintenanceWindow(gomock.Any(), advancedClusterID).
		Return(updatedMaintenanceWindowInfo, nil, nil).
		Times(2)

	// Delete
	s.EXPECT().DeleteCluster(gomock.Any(), advancedClusterID)
	s.EXPECT().DeleteMaintenanceWindow(gomock.Any(), advancedClusterID)

	testMaintenanceWindowResource(t, clusterName, true)
}

func testMaintenanceWindowResource(t *testing.T, clusterName string, useMock bool) {
	var (
		clusterResourceName           = "cockroach_cluster.test"
		maintenanceWindowResourceName = "cockroach_maintenance_window.test"
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestMaintenanceWindowResourceCreateConfigWithStandardCluster(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists(clusterResourceName),
					resource.TestCheckResourceAttr(maintenanceWindowResourceName, "offset_duration", "1010"),
					resource.TestCheckResourceAttr(maintenanceWindowResourceName, "window_duration", "101010"),
				),
				ExpectError: regexp.MustCompile("maintenance windows are supported for advanced clusters only"),
			},
			// Tear down the serverless cluster
			{
				Config:  " ",
				Destroy: true,
			},
			{
				Config: getTestMaintenanceWindowResourceCreateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists(clusterResourceName),
					resource.TestCheckResourceAttr(maintenanceWindowResourceName, "offset_duration", "1010"),
					resource.TestCheckResourceAttr(maintenanceWindowResourceName, "window_duration", "101010"),
				),
			},
			{
				Config: getTestMaintenanceWindowResourceUpdateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists(clusterResourceName),
					resource.TestCheckResourceAttr(maintenanceWindowResourceName, "offset_duration", "1100"),
					resource.TestCheckResourceAttr(maintenanceWindowResourceName, "window_duration", "110011"),
				),
			},
			{
				ResourceName:      maintenanceWindowResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func getTestMaintenanceWindowResourceCreateConfigWithStandardCluster(name string) string {
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

resource "cockroach_maintenance_window" "teststandard" {
  id              = cockroach_cluster.test.id
  offset_duration = 1010
  window_duration = 101010
}
`, name)
}

func getTestMaintenanceWindowResourceCreateConfig(name string) string {
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

resource "cockroach_maintenance_window" "test" {
  id              = cockroach_cluster.test.id
  offset_duration = 1010
  window_duration = 101010
}
`, name)
}

func getTestMaintenanceWindowResourceUpdateConfig(name string) string {
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

resource "cockroach_maintenance_window" "test" {
  id              = cockroach_cluster.test.id
  offset_duration = 1100
  window_duration = 110011
}
`, name)
}
