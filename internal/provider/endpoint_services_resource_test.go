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
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestEndpointServicesResource(t *testing.T) {
	t.Parallel()
	var (
		clusterResourceName = "cockroach_cluster.dedicated"
		resourceName        = "cockroach_endpoint_services"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testEndpointServicesResource(clusterResourceName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "TBD"),
					resource.TestCheckResourceAttrSet(resourceName, "services"),
				),
			},
		},
	})
}

func testEndpointServicesResource(name string) string {
	clusterName := fmt.Sprintf("endpoint-services-%s", GenerateRandomString(3))
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
 resource "cockroach_endpoint_services" "services" {
    id = cockroach_cluster.dedicated.id
}
`, clusterName)
}
