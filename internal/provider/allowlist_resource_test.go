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
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v2/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccDedicatedAllowlistEntryResource attempts to create, check, and destroy
// a real dedicated cluster and allowlist entry. It will be skipped if TF_ACC
// isn't set.
func TestAccDedicatedAllowlistEntryResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-networking-%s", tfTestPrefix, GenerateRandomString(2))
	entryName := "default-allow-list"
	entry := client.AllowlistEntry{
		Name:     &entryName,
		CidrIp:   "192.168.3.2",
		CidrMask: 32,
		Sql:      true,
		Ui:       true,
	}
	newEntryName := "update-test"
	newEntry := client.AllowlistEntry{
		Name:     &newEntryName,
		CidrIp:   "192.168.3.2",
		CidrMask: 32,
		Sql:      false,
		Ui:       false,
	}
	testAllowlistEntryResource(t, clusterName, entry, newEntry, false /* useMock */, false /* isServerless */)
}

// TestAccServerlessAllowlistEntryResource attempts to create, check, and
// destroy a real serverless cluster and allowlist entry. It will be skipped if
// TF_ACC isn't set.
func TestAccServerlessAllowlistEntryResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-networking-%s", tfTestPrefix, GenerateRandomString(2))
	entryName := "default-allow-list"
	entry := client.AllowlistEntry{
		Name:     &entryName,
		CidrIp:   "192.168.3.42",
		CidrMask: 32,
		Sql:      true,
		Ui:       false,
	}
	newEntryName := "update-test"
	newEntry := client.AllowlistEntry{
		Name:     &newEntryName,
		CidrIp:   "192.168.3.42",
		CidrMask: 32,
		Sql:      false,
		Ui:       false,
	}
	testAllowlistEntryResource(t, clusterName, entry, newEntry, false /* useMock */, true /* isServerless */)
}

// TestIntegrationAllowlistEntryResource attempts to create, check, and
// destroy a cluster and allowlist entry, but uses a mocked API service.
func TestIntegrationAllowlistEntryResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-networking-%s", tfTestPrefix, GenerateRandomString(2))
	clusterID := uuid.Nil.String()
	name := "default-allow-list"
	// Return another entry in the List call to make sure we're selecting
	// the right one.
	otherName := "wrong-entry"
	otherEntry := client.AllowlistEntry{
		Name:     &otherName,
		CidrIp:   "192.168.5.4",
		CidrMask: 32,
	}
	newEntryName := "update-test"
	newEntry := client.AllowlistEntry{
		CidrIp:   "192.168.3.2",
		CidrMask: 32,
		Name:     &newEntryName,
		Sql:      false,
		Ui:       false,
	}
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	cases := []struct {
		name         string
		entry        client.AllowlistEntry
		finalCluster client.Cluster
	}{
		{
			"dedicated cluster",
			client.AllowlistEntry{
				Name:     &name,
				CidrIp:   "192.168.3.2",
				CidrMask: 32,
				Sql:      true,
				Ui:       true,
			},
			client.Cluster{
				Name:          clusterName,
				Id:            uuid.Nil.String(),
				CloudProvider: "GCP",
				Config: client.ClusterConfig{
					Dedicated: &client.DedicatedHardwareConfig{
						StorageGib:     15,
						MachineType:    "m5.large",
						NumVirtualCpus: 2,
					},
				},
				Plan:  "ADVANCED",
				State: "CREATED",
				Regions: []client.Region{
					{
						Name:      "us-east1",
						NodeCount: 1,
					},
				},
			},
		},
		{
			"serverless cluster",
			client.AllowlistEntry{
				Name:     &name,
				CidrIp:   "192.168.3.2",
				CidrMask: 32,
				Sql:      true,
				Ui:       false,
			},
			client.Cluster{
				Name:          clusterName,
				Id:            uuid.Nil.String(),
				Plan:          "BASIC",
				CloudProvider: "GCP",
				State:         "CREATED",
				Config: client.ClusterConfig{
					Serverless: &client.ServerlessClusterConfig{
						RoutingId: "routing-id",
					},
				},
				Regions: []client.Region{
					{
						Name: "us-central1",
					},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s := mock_client.NewMockService(ctrl)
			defer HookGlobal(&NewService, func(c *client.Client) client.Service {
				return s
			})()

			cluster := c.finalCluster
			entry := c.entry

			// Create
			s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
				Return(&cluster, nil, nil)
			s.EXPECT().GetCluster(gomock.Any(), clusterID).
				Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
			s.EXPECT().AddAllowlistEntry(gomock.Any(), clusterID, &entry).Return(&entry, nil, nil)
			// Called by testAllowlistEntryExists
			s.EXPECT().ListAllowlistEntries(gomock.Any(), clusterID, gomock.Any()).Return(
				&client.ListAllowlistEntriesResponse{Allowlist: []client.AllowlistEntry{otherEntry, entry}}, nil, nil)

			// Update 1
			// The OpenAPI generator does something weird when part of an object lives in the URL
			// and the rest in the request body, and it winds up as a partial object.
			newEntryForUpdate := &client.AllowlistEntry1{
				Name: newEntry.Name,
				Sql:  newEntry.Sql,
				Ui:   newEntry.Ui,
			}
			// GetCluster and ListAllowListEntries called for each allow list entry
			s.EXPECT().GetCluster(gomock.Any(), clusterID).
				Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).Times(2)
			s.EXPECT().ListAllowlistEntries(gomock.Any(), clusterID, gomock.Any()).Return(
				&client.ListAllowlistEntriesResponse{Allowlist: []client.AllowlistEntry{otherEntry, entry}}, nil, nil).Times(2)
			// One allowlist was updated
			s.EXPECT().UpdateAllowlistEntry(gomock.Any(), clusterID, entry.CidrIp, entry.CidrMask, newEntryForUpdate).
				Return(&newEntry, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
			// Called by testAllowlistEntryExists
			s.EXPECT().ListAllowlistEntries(gomock.Any(), clusterID, gomock.Any()).
				Return(&client.ListAllowlistEntriesResponse{Allowlist: []client.AllowlistEntry{otherEntry, newEntry}}, nil, nil)

			// Update 2 - nil name (this should not make an update)
			s.EXPECT().GetCluster(gomock.Any(), clusterID).
				Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
			s.EXPECT().ListAllowlistEntries(gomock.Any(), clusterID, gomock.Any()).
				Return(&client.ListAllowlistEntriesResponse{Allowlist: []client.AllowlistEntry{otherEntry, newEntry}}, nil, nil)
			s.EXPECT().GetCluster(gomock.Any(), clusterID).
				Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
			s.EXPECT().ListAllowlistEntries(gomock.Any(), clusterID, gomock.Any()).
				Return(&client.ListAllowlistEntriesResponse{Allowlist: []client.AllowlistEntry{otherEntry, newEntry}}, nil, nil).Times(2)

			// Update 3 - add name back but make it empty string (this should an update)
			newEntry2Update := &client.AllowlistEntry1{
				Name: ptr(""),
				Sql:  newEntry.Sql,
				Ui:   newEntry.Ui,
			}
			newEntry2 := newEntry
			newEntry2.Name = ptr("")

			// Two pairs of these, one for each allowlist
			s.EXPECT().GetCluster(gomock.Any(), clusterID).
				Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).Times(2)
			s.EXPECT().ListAllowlistEntries(gomock.Any(), clusterID, gomock.Any()).
				Return(&client.ListAllowlistEntriesResponse{Allowlist: []client.AllowlistEntry{otherEntry, newEntry}}, nil, nil).Times(2)
			// One allowlist was updated
			s.EXPECT().UpdateAllowlistEntry(gomock.Any(), clusterID, entry.CidrIp, entry.CidrMask, newEntry2Update).
				Return(&newEntry2, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
			// Called by testAllowlistEntryExists
			s.EXPECT().ListAllowlistEntries(gomock.Any(), clusterID, gomock.Any()).
				Return(&client.ListAllowlistEntriesResponse{Allowlist: []client.AllowlistEntry{otherEntry, newEntry2}}, nil, nil)

			// Update 4 - Update to empty string again (no churn)
			// Two pairs of these, one for each allowlist
			s.EXPECT().GetCluster(gomock.Any(), clusterID).
				Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).Times(2)
			s.EXPECT().ListAllowlistEntries(gomock.Any(), clusterID, gomock.Any()).
				Return(&client.ListAllowlistEntriesResponse{Allowlist: []client.AllowlistEntry{otherEntry, newEntry2}}, nil, nil).Times(3)

			// Delete
			s.EXPECT().GetCluster(gomock.Any(), clusterID).
				Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
			s.EXPECT().ListAllowlistEntries(gomock.Any(), clusterID, gomock.Any()).Return(
				&client.ListAllowlistEntriesResponse{Allowlist: []client.AllowlistEntry{otherEntry, newEntry2}}, nil, nil).
				Times(2)
			s.EXPECT().DeleteAllowlistEntry(gomock.Any(), clusterID, entry.CidrIp, entry.CidrMask)
			s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

			testAllowlistEntryResource(t, clusterName, entry, newEntry, true /* useMock */, cluster.Config.Dedicated == nil /* isServerless */)
		})
	}
}

func testAllowlistEntryResource(
	t *testing.T,
	clusterName string,
	entry, newEntry client.AllowlistEntry,
	useMock bool,
	isServerless bool,
) {
	const (
		dedicatedClusterResourceName  = "cockroach_cluster.dedicated"
		serverlessClusterResourceName = "cockroach_cluster.serverless"
		resourceName                  = "cockroach_allow_list.network_list"
	)
	var clusterResourceName string
	var allowlistEntryResourceConfigFn func(string, *client.AllowlistEntry) string
	var uiVal string
	entryWithNilName := newEntry
	entryWithNilName.Name = nil
	entryWithEmptyStringName := newEntry
	entryWithEmptyStringName.Name = ptr("")

	if isServerless {
		clusterResourceName = serverlessClusterResourceName
		allowlistEntryResourceConfigFn = getTestAllowlistEntryResourceConfigForServerless
		uiVal = "false"
	} else {
		clusterResourceName = dedicatedClusterResourceName
		allowlistEntryResourceConfigFn = getTestAllowlistEntryResourceConfigForDedicated
		uiVal = "true"
	}
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: allowlistEntryResourceConfigFn(clusterName, &entry),
				Check: resource.ComposeTestCheckFunc(
					testAllowlistEntryExists(resourceName, clusterResourceName),
					resource.TestCheckResourceAttr(resourceName, "name", *entry.Name),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_ip"),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_mask"),
					resource.TestCheckResourceAttr(resourceName, "ui", uiVal),
					resource.TestCheckResourceAttr(resourceName, "sql", "true"),
				),
			},
			{
				Config: allowlistEntryResourceConfigFn(clusterName, &newEntry),
				Check: resource.ComposeTestCheckFunc(
					testAllowlistEntryExists(resourceName, clusterResourceName),
					resource.TestCheckResourceAttr(resourceName, "name", *newEntry.Name),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_ip"),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_mask"),
					resource.TestCheckResourceAttr(resourceName, "ui", "false"),
					resource.TestCheckResourceAttr(resourceName, "sql", "false"),
				),
			},
			// Update that removes name from the resource
			{
				Config: allowlistEntryResourceConfigFn(clusterName, &entryWithNilName),
				Check: resource.ComposeTestCheckFunc(
					testAllowlistEntryExists(resourceName, clusterResourceName),
					// Expect that the server name is not affected if the name is not explicitly set
					resource.TestCheckResourceAttr(resourceName, "name", *newEntry.Name),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_ip"),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_mask"),
					resource.TestCheckResourceAttr(resourceName, "ui", "false"),
					resource.TestCheckResourceAttr(resourceName, "sql", "false"),
				),
			},
			// Update to empty string name
			{
				Config: allowlistEntryResourceConfigFn(clusterName, &entryWithEmptyStringName),
				Check: resource.ComposeTestCheckFunc(
					testAllowlistEntryExists(resourceName, clusterResourceName),
					// Expect that the server name is not affected if the name is not explicitly set
					resource.TestCheckResourceAttr(resourceName, "name", ""),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_ip"),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_mask"),
					resource.TestCheckResourceAttr(resourceName, "ui", "false"),
					resource.TestCheckResourceAttr(resourceName, "sql", "false"),
				),
			},
			// Update to empty string name again to show no churn
			{
				Config: allowlistEntryResourceConfigFn(clusterName, &entryWithEmptyStringName),
				Check: resource.ComposeTestCheckFunc(
					testAllowlistEntryExists(resourceName, clusterResourceName),
					// Expect that the server name is not affected if the name is not explicitly set
					resource.TestCheckResourceAttr(resourceName, "name", ""),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_ip"),
					resource.TestCheckResourceAttrSet(resourceName, "cidr_mask"),
					resource.TestCheckResourceAttr(resourceName, "ui", "false"),
					resource.TestCheckResourceAttr(resourceName, "sql", "false"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
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

		traceAPICall("ListAllowlistEntries")
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

func getTestAllowlistEntryResourceConfigForDedicated(
	clusterName string, entry *client.AllowlistEntry,
) string {
	nameAttribute := ""
	if entry.Name != nil {
		nameAttribute = fmt.Sprintf("name = \"%s\"\n", *entry.Name)
	}

	return fmt.Sprintf(`
resource "cockroach_cluster" "dedicated" {
    name           = "%s"
    cloud_provider = "GCP"
    dedicated = {
        storage_gib = 15
        num_virtual_cpus = 2
    }
    regions = [{
        name: "us-east1"
        node_count: 1
    }]
}
resource "cockroach_allow_list" "network_list" {
    %s
    cidr_ip = "%s"
    cidr_mask = %d
    sql = %v
    ui = %v
    cluster_id = cockroach_cluster.dedicated.id
}
`, clusterName, nameAttribute, entry.CidrIp, entry.CidrMask, entry.Sql, entry.Ui)
}

func getTestAllowlistEntryResourceConfigForServerless(
	clusterName string, entry *client.AllowlistEntry,
) string {
	nameAttribute := ""
	if entry.Name != nil {
		nameAttribute = fmt.Sprintf("name = \"%s\"\n", *entry.Name)
	}

	return fmt.Sprintf(`
resource "cockroach_cluster" "serverless" {
    name           = "%s"
    cloud_provider = "GCP"
    serverless = {}
    regions = [{
        name = "us-central1"
    }]
}
resource "cockroach_allow_list" "network_list" {
    %s
    cidr_ip = "%s"
    cidr_mask = %d
    sql = %v
    ui = %v
    cluster_id = cockroach_cluster.serverless.id
}
`, clusterName, nameAttribute, entry.CidrIp, entry.CidrMask, entry.Sql, entry.Ui)
}
