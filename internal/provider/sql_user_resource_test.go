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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// TestAccSqlUserResource attempts to create, check, and destroy
// a real cluster and SQL user. It will be skipped if TF_ACC isn't set.
func TestAccSqlUserResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-sql-user-%s", GenerateRandomString(4))
	sqlUserName := "cockroach-user"
	sqlPassword := "cockroach@123456"

	testSqlUserResource(t, clusterName, sqlUserName, sqlPassword, false)
}

// TestIntegrationSqlUserResource attempts to create, check, and destroy
// a cluster and SQL user, but uses a mocked API service.
func TestIntegrationSqlUserResource(t *testing.T) {
	clusterName := fmt.Sprintf("tftest-sql-user-%s", GenerateRandomString(4))
	sqlUserName := "cockroach-user"
	sqlPassword := "cockroach@123456"
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
		Name:          clusterName,
		Id:            "cluster-id",
		CloudProvider: "GCP",
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{
				SpendLimit: 1,
			},
		},
		State: "CREATED",
		Regions: []client.Region{
			{
				Name: "us-central1",
			},
		},
	}
	user := client.SQLUser{
		Name: sqlUserName,
	}
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(3)
	s.EXPECT().CreateSQLUser(
		gomock.Any(),
		clusterID,
		&client.CreateSQLUserRequest{Name: sqlUserName, Password: sqlPassword},
	).Return(&user, nil, nil)
	s.EXPECT().ListSQLUsers(gomock.Any(), clusterID, gomock.Any()).Times(2).Return(
		&client.ListSQLUsersResponse{Users: []client.SQLUser{user}}, nil, nil)
	s.EXPECT().DeleteSQLUser(gomock.Any(), clusterID, user.Name)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testSqlUserResource(t, clusterName, sqlUserName, sqlPassword, true)
}

func testSqlUserResource(t *testing.T, clusterName, sqlUserName, sqlPassword string, useMock bool) {
	var (
		clusterResourceName = "cockroach_cluster.serverless"
		resourceName        = "cockroach_sql_user.sqluser"
	)
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestSqlUserResourceConfig(clusterName, sqlUserName, "cockroach@123456"),
				Check: resource.ComposeTestCheckFunc(
					testSqlUserExists(resourceName, clusterResourceName),
					resource.TestCheckResourceAttr(resourceName, "name", sqlUserName),
				),
			},
		},
	})
}

func testSqlUserExists(resourceName, clusterResourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		var listUserOptions client.ListSQLUsersOptions
		p, _ := convertProviderType(testAccProvider)
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

		clusterResp, _, err := p.service.ListSQLUsers(context.TODO(), clusterID, &listUserOptions)
		if err == nil {
			for _, user := range clusterResp.Users {
				if user.GetName() == rs.Primary.Attributes["name"] {
					return nil
				}
			}
		}

		return fmt.Errorf("user %s does not exist", rs.Primary.ID)
	}
}

func getTestSqlUserResourceConfig(clusterName, userName, password string) string {
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

resource "cockroach_sql_user" "sqluser" {
  name = "%s"
  password = "%s"
  cluster_id = cockroach_cluster.serverless.id
}
`, clusterName, userName, password)
}
