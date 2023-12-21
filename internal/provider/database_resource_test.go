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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// TestAccDatabaseResource attempts to create, check, update, and destroy
// a real database and its cluster. It will be skipped if TF_ACC isn't set.
func TestAccDatabaseResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-database-%s", GenerateRandomString(4))
	databaseName := "cockroach-database"
	newDatabaseName := "cockroach-database-updated"

	testDatabaseResource(t, clusterName, databaseName, newDatabaseName, false)
}

// TestIntegrationDatabaseResource attempts to create, check, update, and
// destroy a database and its cluster, but uses a mocked API service.
func TestIntegrationDatabaseResource(t *testing.T) {
	clusterName := fmt.Sprintf("tftest-database-%s", GenerateRandomString(4))
	databaseName := "test-database"
	newDatabaseName := "test-database-updated"
	clusterID := uuid.Nil.String()
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
		Plan:          "BASIC",
		CloudProvider: "GCP",
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{
				RoutingId: "routing-id",
			},
		},
		State: "CREATED",
		Regions: []client.Region{
			{
				Name: "us-central1",
			},
		},
	}
	database := client.Database{
		Name: databaseName,
	}
	updatedDatabase := client.Database{
		Name: newDatabaseName,
	}

	// Create
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(3)
	s.EXPECT().CreateDatabase(
		gomock.Any(),
		clusterID,
		&client.CreateDatabaseRequest{Name: databaseName},
	).Return(&database, nil, nil)
	s.EXPECT().ListDatabases(gomock.Any(), clusterID, gomock.Any()).
		Return(&client.ListDatabasesResponse{Databases: []client.Database{database}}, nil, nil).
		Times(2)

	// Update
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(2)
	s.EXPECT().ListDatabases(gomock.Any(), clusterID, gomock.Any()).
		Return(&client.ListDatabasesResponse{Databases: []client.Database{database}}, nil, nil)
	s.EXPECT().EditDatabase(gomock.Any(), clusterID, databaseName, &client.UpdateDatabaseRequest1{NewName: newDatabaseName}).
		Return(&updatedDatabase, nil, nil)
	s.EXPECT().ListDatabases(gomock.Any(), clusterID, gomock.Any()).
		Return(&client.ListDatabasesResponse{Databases: []client.Database{updatedDatabase}}, nil, nil).
		Times(3)

	// Delete
	s.EXPECT().DeleteDatabase(gomock.Any(), clusterID, newDatabaseName)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testDatabaseResource(t, clusterName, databaseName, newDatabaseName, true)
}

func testDatabaseResource(
	t *testing.T, clusterName, databaseName, newDatabaseName string, useMock bool,
) {
	var (
		clusterResourceName = "cockroach_cluster.serverless"
		resourceName        = "cockroach_database.test_database"
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestDatabaseResourceConfig(clusterName, databaseName),
				Check: resource.ComposeTestCheckFunc(
					testDatabaseExists(resourceName, clusterResourceName),
					resource.TestCheckResourceAttr(resourceName, "name", databaseName),
				),
			},
			{
				Config: getTestDatabaseResourceConfig(clusterName, newDatabaseName),
				Check: resource.ComposeTestCheckFunc(
					testDatabaseExists(resourceName, clusterResourceName),
					resource.TestCheckResourceAttr(resourceName, "name", newDatabaseName),
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

func testDatabaseExists(resourceName, clusterResourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
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

		clusterResp, _, err := p.service.ListDatabases(context.TODO(), clusterID, &client.ListDatabasesOptions{})
		if err == nil {
			for _, database := range clusterResp.GetDatabases() {
				if database.GetName() == rs.Primary.Attributes["name"] {
					return nil
				}
			}
		}

		return fmt.Errorf("database %s does not exist", rs.Primary.ID)
	}
}

func getTestDatabaseResourceConfig(clusterName, databaseName string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "serverless" {
    name           = "%s"
    cloud_provider = "GCP"
    serverless = {}
    regions = [{
        name = "us-central1"
    }]
}

resource "cockroach_database" "test_database" {
  name = "%s"
  cluster_id = cockroach_cluster.serverless.id
}
`, clusterName, databaseName)
}
