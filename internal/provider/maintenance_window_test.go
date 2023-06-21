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
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// TestAccMaintenanceWindowResource attempts to create, check, and destroy a
// real cluster. It will be skipped if TF_ACC isn't set.
func TestAccMaintenanceWindowResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-mwin-%s", GenerateRandomString(4))
	testMaintenanceWindowResource(t, clusterName, false)
}

// TestIntegrationMaintenanceWindowResource attempts to create, check, and
// destroy a cluster, but uses a mocked API service.
func TestIntegrationMaintenanceWindowResource(t *testing.T) {
	clusterName := fmt.Sprintf("tftest-mwin-%s", GenerateRandomString(4))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	clusterInfo := &client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: "v22.2.0",
		Plan:             "DEDICATED",
		CloudProvider:    "AWS",
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
				Name:      "us-east-1",
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

	// Create
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(clusterInfo, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(clusterInfo, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(3)
	s.EXPECT().SetMaintenanceWindow(gomock.Any(), clusterID, createdMaintenanceWindowInfo).
		Return(createdMaintenanceWindowInfo, nil, nil)
	s.EXPECT().GetMaintenanceWindow(gomock.Any(), clusterID).
		Return(createdMaintenanceWindowInfo, nil, nil)

	// Update
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(clusterInfo, nil, nil).
		Times(3)
	s.EXPECT().GetMaintenanceWindow(gomock.Any(), clusterID).
		Return(createdMaintenanceWindowInfo, nil, nil)
	s.EXPECT().SetMaintenanceWindow(gomock.Any(), clusterID, updatedMaintenanceWindowInfo).
		Return(updatedMaintenanceWindowInfo, nil, nil)
	s.EXPECT().GetMaintenanceWindow(gomock.Any(), clusterID).
		Return(updatedMaintenanceWindowInfo, nil, nil)

	// Delete
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)
	s.EXPECT().DeleteMaintenanceWindow(gomock.Any(), clusterID)

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
				Config: getTestMaintenanceWindowResourceCreateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists(clusterResourceName),
					resource.TestCheckResourceAttr(maintenanceWindowResourceName, "offset_duration_seconds", "1010"),
					resource.TestCheckResourceAttr(maintenanceWindowResourceName, "window_duration_seconds", "101010"),
				),
			},
			{
				Config: getTestMaintenanceWindowResourceUpdateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists(clusterResourceName),
					resource.TestCheckResourceAttr(maintenanceWindowResourceName, "offset_duration_seconds", "1100"),
					resource.TestCheckResourceAttr(maintenanceWindowResourceName, "window_duration_seconds", "110011"),
				),
			},
		},
	})
}

func getTestMaintenanceWindowResourceCreateConfig(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  dedicated = {
    storage_gib  = 35
  	machine_type = "m5.xlarge"
  }
  regions = [{
    name = "us-east-1"
    node_count: 3
  }]
}

resource "cockroach_maintenance_window" "test" {
  id              = cockroach_cluster.test.id
  offset_duration_seconds = 1010
  window_duration_seconds = 101010
}
`, name)
}

func getTestMaintenanceWindowResourceUpdateConfig(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  dedicated = {
    storage_gib  = 35
  	machine_type = "m5.xlarge"
  }
  regions = [{
    name = "us-east-1"
    node_count: 3
  }]
}

resource "cockroach_maintenance_window" "test" {
  id              = cockroach_cluster.test.id
  offset_duration_seconds = 1100
  window_duration_seconds = 110011
}
`, name)
}
