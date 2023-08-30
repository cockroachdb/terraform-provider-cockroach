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

// TestAccVersionDeferralResource attempts to create, check, and destroy a
// real cluster. It will be skipped if TF_ACC isn't set.
func TestAccVersionDeferralResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-version-deferral-%s", GenerateRandomString(4))
	testVersionDeferralResource(t, clusterName, false)
}

// TestIntegrationVersionDeferralResource attempts to create, check, and
// destroy a cluster, but uses a mocked API service.
func TestIntegrationVersionDeferralResource(t *testing.T) {
	clusterName := fmt.Sprintf("tftest-deferral-%s", GenerateRandomString(4))
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
	createdVersionDeferralInfo := &client.ClusterVersionDeferral{
		DeferralPolicy: client.CLUSTERVERSIONDEFERRALPOLICYTYPE_FIXED_DEFERRAL,
	}
	updatedVersionDeferralInfo := &client.ClusterVersionDeferral{
		DeferralPolicy: client.CLUSTERVERSIONDEFERRALPOLICYTYPE_NOT_DEFERRED,
	}
	deletedVersionDeferralInfo := &client.ClusterVersionDeferral{
		DeferralPolicy: client.CLUSTERVERSIONDEFERRALPOLICYTYPE_NOT_DEFERRED,
	}

	// Create
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(clusterInfo, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(clusterInfo, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(3)
	s.EXPECT().SetClusterVersionDeferral(gomock.Any(), clusterID, createdVersionDeferralInfo).
		Return(createdVersionDeferralInfo, nil, nil)
	s.EXPECT().GetClusterVersionDeferral(gomock.Any(), clusterID).
		Return(createdVersionDeferralInfo, nil, nil)

	// Update
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(clusterInfo, nil, nil).
		Times(3)
	s.EXPECT().GetClusterVersionDeferral(gomock.Any(), clusterID).
		Return(createdVersionDeferralInfo, nil, nil)
	s.EXPECT().SetClusterVersionDeferral(gomock.Any(), clusterID, updatedVersionDeferralInfo).
		Return(updatedVersionDeferralInfo, nil, nil)
	s.EXPECT().GetClusterVersionDeferral(gomock.Any(), clusterID).
		Return(updatedVersionDeferralInfo, nil, nil).
		Times(2)

	// Delete
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)
	s.EXPECT().SetClusterVersionDeferral(gomock.Any(), clusterID, deletedVersionDeferralInfo)

	testVersionDeferralResource(t, clusterName, true)
}

func testVersionDeferralResource(t *testing.T, clusterName string, useMock bool) {
	var (
		clusterResourceName         = "cockroach_cluster.test"
		versionDeferralResourceName = "cockroach_version_deferral.test"
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestVersionDeferralResourceCreateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists(clusterResourceName),
					resource.TestCheckResourceAttr(versionDeferralResourceName, "deferral_policy", "FIXED_DEFERRAL"),
				),
			},
			{
				Config: getTestVersionDeferralResourceUpdateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists(clusterResourceName),
					resource.TestCheckResourceAttr(versionDeferralResourceName, "deferral_policy", "NOT_DEFERRED"),
				),
			},
			{
				ResourceName:      versionDeferralResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func getTestVersionDeferralResourceCreateConfig(name string) string {
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
resource "cockroach_version_deferral" "test" {
  id              = cockroach_cluster.test.id
  deferral_policy = "FIXED_DEFERRAL"
}
`, name)
}

func getTestVersionDeferralResourceUpdateConfig(name string) string {
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
resource "cockroach_version_deferral" "test" {
  id              = cockroach_cluster.test.id
  deferral_policy = "NOT_DEFERRED"
}
`, name)
}
