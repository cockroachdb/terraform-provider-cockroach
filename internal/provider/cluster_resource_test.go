/*
 Copyright 2022 The Cockroach Authors

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
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
)

// TestAccClusterResource attempts to create, check, and destroy
// a real cluster and allowlist entry. It will be skipped if TF_ACC isn't set.
func TestAccServerlessClusterResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-serverless-%s", GenerateRandomString(2))
	testServerlessClusterResource(t, clusterName, false)
}

// TestIntegrationServerlessClusterResource attempts to create, check, and destroy
// a cluster, but uses a mocked API service.
func TestIntegrationServerlessClusterResource(t *testing.T) {
	clusterName := fmt.Sprintf("tftest-serverless-%s", GenerateRandomString(2))
	clusterID := "cluster-id"
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	finalCluster := client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: "v22.1.0",
		Plan:             "SERVERLESS",
		CloudProvider:    "GCP",
		State:            "CREATED",
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{
				SpendLimit: 1,
				RoutingId:  "routing-id",
			},
		},
		Regions: []client.Region{
			{
				Name: "us-central1",
			},
		},
	}
	initialCluster := finalCluster
	initialCluster.State = client.CLUSTERSTATETYPE_CREATING

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&initialCluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&finalCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(3)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testServerlessClusterResource(t, clusterName, true)
}

func testServerlessClusterResource(t *testing.T, clusterName string, useMock bool) {
	var (
		resourceName = "cockroach_cluster.serverless"
		cluster      client.Cluster
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestServerlessClusterResourceConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists(resourceName, &cluster),
					resource.TestCheckResourceAttr(resourceName, "name", clusterName),
					resource.TestCheckResourceAttrSet(resourceName, "cloud_provider"),
					resource.TestCheckResourceAttrSet(resourceName, "cockroach_version"),
					resource.TestCheckResourceAttr(resourceName, "plan", "SERVERLESS"),
					resource.TestCheckResourceAttr(resourceName, "state", string(client.CLUSTERSTATETYPE_CREATED)),
				),
			},
		},
	})
}

func TestAccDedicatedClusterResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-dedicated-%s", GenerateRandomString(3))
	testDedicatedClusterResource(t, clusterName, false)
}

func TestIntegrationDedicatedClusterResource(t *testing.T) {
	clusterName := fmt.Sprintf("tftest-dedicated-%s", GenerateRandomString(3))
	clusterID := "cluster-id"
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	cluster := client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: "v22.1.0",
		Plan:             "DEDICATED",
		CloudProvider:    "GCP",
		State:            "CREATED",
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				MachineType:    "n1-standard-2",
				NumVirtualCpus: 2,
				StorageGib:     15,
				MemoryGib:      8,
			},
		},
		Regions: []client.Region{
			{
				Name:      "us-central1",
				NodeCount: 1,
			},
		},
	}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(3)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testDedicatedClusterResource(t, clusterName, true)
}

func testDedicatedClusterResource(t *testing.T, clusterName string, useMock bool) {
	var (
		resourceName = "cockroach_cluster.dedicated"
		cluster      client.Cluster
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestDedicatedClusterResourceConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists(resourceName, &cluster),
					resource.TestCheckResourceAttr(resourceName, "name", clusterName),
					resource.TestCheckResourceAttrSet(resourceName, "cloud_provider"),
					resource.TestCheckResourceAttrSet(resourceName, "cockroach_version"),
					resource.TestCheckResourceAttr(resourceName, "plan", "DEDICATED"),
				),
			},
		},
	})
}

func testCheckCockroachClusterExists(resourceName string, cluster *client.Cluster) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		p := testAccProvider.(*provider)
		p.service = NewService(cl)
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		id := rs.Primary.Attributes["id"]
		log.Printf("[DEBUG] projectID: %s, name %s", rs.Primary.Attributes["id"], rs.Primary.Attributes["name"])

		if clusterResp, _, err := p.service.GetCluster(context.Background(), id); err == nil {
			*cluster = *clusterResp
			return nil
		}

		return fmt.Errorf("cluster(%s:%s) does not exist", rs.Primary.Attributes["id"], rs.Primary.ID)
	}
}

func getTestServerlessClusterResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "serverless" {
    name           = "%s"
    cloud_provider = "GCP"
    serverless = {
        spend_limit = 1
    }
	regions = [{
		name = "us-central1"
	}]
}
`, name)
}

func getTestDedicatedClusterResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "dedicated" {
    name           = "%s"
    cloud_provider = "GCP"
    cockroach_version = "v22.1"
    dedicated = {
	  storage_gib = 15
	  machine_type = "n1-standard-2"
    }
	regions = [{
		name: "us-central1"
		node_count: 1
	}]
}
`, name)
}

func TestSortRegionsByPlan(t *testing.T) {
	t.Run("Plan matches cluster", func(t *testing.T) {
		clusterObj := &client.Cluster{Regions: []client.Region{
			{Name: "us-central1"},
			{Name: "us-east1"},
			{Name: "us-west2"},
		}}
		plan := &CockroachCluster{
			Regions: []Region{
				{Name: types.StringValue("us-west2")},
				{Name: types.StringValue("us-central1")},
				{Name: types.StringValue("us-east1")},
			},
		}
		sortRegionsByPlan(clusterObj, plan)
		for i, region := range clusterObj.Regions {
			require.Equal(t, plan.Regions[i].Name.ValueString(), region.Name)
		}
	})

	t.Run("More regions in cluster than plan", func(t *testing.T) {
		clusterObj := &client.Cluster{Regions: []client.Region{
			{Name: "us-central1"},
			{Name: "us-east1"},
			{Name: "us-west2"},
		}}
		plan := &CockroachCluster{
			Regions: []Region{
				{Name: types.StringValue("us-west2")},
				{Name: types.StringValue("us-central1")},
			},
		}
		// We really just want to make sure it doesn't panic here.
		sortRegionsByPlan(clusterObj, plan)
	})

	t.Run("More regions in plan than cluster", func(t *testing.T) {
		clusterObj := &client.Cluster{Regions: []client.Region{
			{Name: "us-central1"},
			{Name: "us-east1"},
		}}
		plan := &CockroachCluster{
			Regions: []Region{
				{Name: types.StringValue("us-west2")},
				{Name: types.StringValue("us-central1")},
				{Name: types.StringValue("us-east1")},
			},
		}
		// We really just want to make sure it doesn't panic here.
		sortRegionsByPlan(clusterObj, plan)
	})
}
