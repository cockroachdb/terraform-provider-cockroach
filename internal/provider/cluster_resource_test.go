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
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccServerlessClusterResource(t *testing.T) {
	t.Parallel()
	var (
		clusterName  = fmt.Sprintf("crdb-serverless-%s", GenerateRandomString(4))
		resourceName = "cockroach_cluster.serverless"
		cluster      client.Cluster
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccServerlessClusterResource(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCockroachClusterExists(resourceName, &cluster),
					resource.TestCheckResourceAttr(resourceName, "name", clusterName),
					resource.TestCheckResourceAttrSet(resourceName, "cloud_provider"),
					resource.TestCheckResourceAttrSet(resourceName, "cockroach_version"),
					resource.TestCheckResourceAttr(resourceName, "plan", "SERVERLESS"),
				),
			},
		},
	})
}

func TestAccDedicatedClusterResource(t *testing.T) {
	t.Parallel()
	var (
		clusterName  = fmt.Sprintf("crdb-dedicated-%s", GenerateRandomString(4))
		resourceName = "cockroach_cluster.dedicated"
		cluster      client.Cluster
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDedicatedClusterResource(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCockroachClusterExists(resourceName, &cluster),
					resource.TestCheckResourceAttr(resourceName, "name", clusterName),
					resource.TestCheckResourceAttrSet(resourceName, "cloud_provider"),
					resource.TestCheckResourceAttrSet(resourceName, "cockroach_version"),
					resource.TestCheckResourceAttr(resourceName, "plan", "DEDICATED"),
				),
			},
		},
	})
}

func testAccCheckCockroachClusterExists(resourceName string, cluster *client.Cluster) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		p, _ := convertProviderType(testAccProvider)
		p.service = client.NewService(cl)
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

func testAccServerlessClusterResource(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "serverless" {
    name           = "%s"
    cloud_provider = "GCP"
    wait_for_cluster_ready = true
    create_spec = {
    serverless = {
         regions = ["us-east1"]
         spend_limit = 1
    }
   }
}
`, name)
}

func testAccDedicatedClusterResource(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "dedicated" {
    name           = "%s"
    cloud_provider = "AWS"
    wait_for_cluster_ready = true
    create_spec = {
    dedicated: {
      region_nodes = {
        "ap-south-1": 1
      }
      hardware = {
        storage_gib = 15
        machine_spec = {
          machine_type = "m5.large"
        }
      }
    }
   }
}
`, name)
}
