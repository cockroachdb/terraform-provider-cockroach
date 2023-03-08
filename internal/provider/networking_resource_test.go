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
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// TestAccAllowlistEntryResource attempts to create, check, and destroy
// a real cluster and allowlist entry. It will be skipped if TF_ACC isn't set.
func TestAccAllowlistEntryResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-networking-%s", GenerateRandomString(2))
	entryName := "default-allow-list"
	entry := client.AllowlistEntry{
		Name:     &entryName,
		CidrIp:   "192.168.3.2",
		CidrMask: 32,
		Sql:      true,
		Ui:       true,
	}
	testAllowlistEntryResource(t, clusterName, entry, false)
}

// TestIntegrationAllowlistEntryResource attempts to create, check, and destroy
// a cluster and allowlist entry, but uses a mocked API service.
func TestIntegrationAllowlistEntryResource(t *testing.T) {
	clusterName := fmt.Sprintf("tftest-networking-%s", GenerateRandomString(2))
	clusterID := uuid.Nil.String()
	name := "default-allow-list"
	entry := client.AllowlistEntry{
		Name:     &name,
		CidrIp:   "192.168.3.2",
		CidrMask: 32,
		Sql:      true,
		Ui:       true,
	}
	// Return another entry in the List call to make sure we're selecting
	// the right one.
	otherName := "wrong-entry"
	otherEntry := client.AllowlistEntry{
		Name:     &otherName,
		CidrIp:   "192.168.5.4",
		CidrMask: 32,
	}
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()
	cluster := client.Cluster{
		Name:          clusterName,
		Id:            uuid.Nil.String(),
		CloudProvider: "AWS",
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				StorageGib:  15,
				MachineType: "m5.large",
			},
		},
		State: "CREATED",
		Regions: []client.Region{
			{
				Name:      "ap-south-1",
				NodeCount: 1,
			},
		},
	}
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(3)
	s.EXPECT().AddAllowlistEntry(gomock.Any(), clusterID, &entry).Return(&entry, nil, nil)
	s.EXPECT().ListAllowlistEntries(gomock.Any(), clusterID, gomock.Any()).Times(2).Return(
		&client.ListAllowlistEntriesResponse{Allowlist: []client.AllowlistEntry{otherEntry, entry}}, nil, nil)
	s.EXPECT().DeleteAllowlistEntry(gomock.Any(), clusterID, entry.CidrIp, entry.CidrMask)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testAllowlistEntryResource(t, clusterName, entry, true)
}

func testAllowlistEntryResource(t *testing.T, clusterName string, entry client.AllowlistEntry, useMock bool) {
	clusterResourceName := "cockroach_cluster.dedicated"
	resourceName := "cockroach_allow_list.network_list"
	newEntryName := "update-test"
	newEntry := client.AllowlistEntry{
		Name:     &newEntryName,
		CidrIp:   "192.168.3.2",
		CidrMask: 32,
		Sql:      false,
		Ui:       false,
	}
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestAllowlistEntryResourceConfig(clusterName, *entry.Name, entry.CidrIp, fmt.Sprint(entry.CidrMask), entry.Sql, entry.Ui),
				Check: resource.ComposeTestCheckFunc(
					testAllowlistEntryExists(resourceName, clusterResourceName),
					resource.TestCheckResourceAttr(resourceName, "name", *entry.Name),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_ip"),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_mask"),
					resource.TestCheckResourceAttrSet(resourceName, "ui"),
					resource.TestCheckResourceAttrSet(resourceName, "sql"),
				),
			},
			{
				Config: getTestAllowlistEntryResourceConfig(clusterName, *newEntry.Name, newEntry.CidrIp, fmt.Sprint(newEntry.CidrMask), newEntry.Sql, newEntry.Ui),
				Check: resource.ComposeTestCheckFunc(
					testAllowlistEntryExists(resourceName, clusterResourceName),
					resource.TestCheckResourceAttr(resourceName, "name", *newEntry.Name),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_ip"),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_mask"),
					resource.TestCheckResourceAttrSet(resourceName, "ui"),
					resource.TestCheckResourceAttrSet(resourceName, "sql"),
				),
			},
		},
	})
}

func testAllowlistEntryExists(resourceName, clusterResourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		var networkRule client.ListAllowlistEntriesOptions
		p := testAccProvider.(*provider)
		p.service = NewService(cl)

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

		clusterID := clusterRs.Primary.Attributes["id"]
		log.Printf("[DEBUG] clusterID: %s, name %s", clusterRs.Primary.Attributes["id"], clusterRs.Primary.Attributes["name"])

		if clusterResp, _, err := p.service.ListAllowlistEntries(context.TODO(), clusterID, &networkRule); err == nil {
			for _, rule := range clusterResp.Allowlist {
				if rule.GetCidrIp() == rs.Primary.Attributes["cidr_ip"] &&
					fmt.Sprint(rule.GetCidrMask()) == rs.Primary.Attributes["cidr_mask"] {
					return nil
				}
			}
		}

		return fmt.Errorf("entry(%s) does not exist", rs.Primary.ID)
	}
}

func getTestAllowlistEntryResourceConfig(clusterName, entryName, cidrIp, cidrMask string, sql bool, ui bool) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "dedicated" {
    name           = "%s"
    cloud_provider = "AWS"
    dedicated = {
	  storage_gib = 15
	  machine_type = "m5.large"
    }
	regions = [{
		name: "ap-south-1"
		node_count: 1
	}]
}
 resource "cockroach_allow_list" "network_list" {
    name = "%s"
    cidr_ip = "%s"
    cidr_mask = %s
    ui = %v
    sql = %v
    cluster_id = cockroach_cluster.dedicated.id
}
`, clusterName, entryName, cidrIp, cidrMask, sql, ui)
}
