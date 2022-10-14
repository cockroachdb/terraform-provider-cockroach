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

func TestAccNetworkingRulesResource(t *testing.T) {
	t.Parallel()
	var (
		clusterResourceName = "cockroach_cluster.dedicated"
		resourceName        = "cockroach_allow_list.network_list"
		networkRuleName     = "default-allow-list"
		cidrIP              = "192.168.3.2"
		cidrMask            = "32"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNetworkingRulesResource(networkRuleName, cidrIP, cidrMask),
				Check: resource.ComposeTestCheckFunc(
					testAccNetworkingRuleExists(resourceName, clusterResourceName),
					resource.TestCheckResourceAttr(resourceName, "name", networkRuleName),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_ip"),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_mask"),
					resource.TestCheckResourceAttrSet(resourceName, "ui"),
					resource.TestCheckResourceAttrSet(resourceName, "sql"),
				),
			},
		},
	})
}

func testAccNetworkingRuleExists(resourceName, clusterResourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		var networkRule client.ListAllowlistEntriesOptions
		p, _ := convertProviderType(testAccProvider)
		p.service = client.NewService(cl)

		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}
		clusterRs, ok := s.RootModule().Resources[clusterResourceName]
		if !ok {
			return fmt.Errorf("not found: %s", clusterResourceName)
		}

		if rs.Primary.ID == "" && clusterRs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		id := clusterRs.Primary.Attributes["id"]
		log.Printf("[DEBUG] clusterID: %s, name %s", clusterRs.Primary.Attributes["id"], clusterRs.Primary.Attributes["name"])

		if clusterResp, _, err := p.service.ListAllowlistEntries(context.TODO(), id, &networkRule); err == nil {
			for _, rule := range clusterResp.Allowlist {
				if rule.GetName() == rs.Primary.Attributes["name"] {
					return nil
				}
			}
		}

		return fmt.Errorf("cluster(%s:%s) does not exist", rs.Primary.Attributes["id"], rs.Primary.ID)
	}
}

func testAccNetworkingRulesResource(name, cidrIp, cidrMask string) string {
	networkClusterName := fmt.Sprintf("crdb-networking-%s", GenerateRandomString(4))
	return fmt.Sprintf(`
resource "cockroach_cluster" "dedicated" {
    name           = "%s"
    cloud_provider = "AWS"
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
 resource "cockroach_allow_list" "network_list" {
    name = "%s"
    cidr_ip = "%s"
    cidr_mask = %s
    ui = true
    sql = true
    id = cockroach_cluster.dedicated.id
}
`, networkClusterName, name, cidrIp, cidrMask)
}
