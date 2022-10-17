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

func TestAWSEndpointConnectionResource(t *testing.T) {
	t.Parallel()
	var (
		clusterResourceName = "cockroach_cluster.dedicated"
		resourceName        = "cockroach_aws_endpoint_connection"
		endpointId          = "aws_endpoint_connection_id_test_value"
		serviceId           = "aws_endpoint_service_id_test_value"
		status              = "ENDPOINT_PENDING"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAWSEndpointConnectionResource(clusterResourceName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "TBD"),
					resource.TestCheckResourceAttrSet(endpointId, "endpoint_id"),
					resource.TestCheckResourceAttrSet(serviceId, "service_id"),
					resource.TestCheckResourceAttrSet(status, "status"),
				),
			},
		},
	})
}

func testAWSEndpointConnectionResource(name string) string {
	clusterName := fmt.Sprintf("aws-connection-%s", GenerateRandomString(3))
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
 resource "cockroach_aws_endpoint_connection" "connection" {
    id = cockroach_cluster.dedicated.id
	cloud_provider = "AWS"
	region = "us-east-1"
	endpoint_id = cockroach_aws_endpoint_connection.endpoint_id
	status = "ENDPOINT_AVAILABLE"
}
`, clusterName)
}
