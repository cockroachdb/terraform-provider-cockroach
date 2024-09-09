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

const testPassword = "random-password"

// TestAccSqlUserResource attempts to create, check, and destroy
// a real cluster and SQL user. It will be skipped if TF_ACC isn't set.
func TestAccSqlUserResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-sql-user-%s", tfTestPrefix, GenerateRandomString(4))
	sqlUserNameWithPass := "cockroach-user"
	sqlUserNameNoPass := "cockroach-user-nopass"

	testSqlUserResource(t, clusterName, sqlUserNameWithPass, sqlUserNameNoPass, false)
}

// TestIntegrationSqlUserResource attempts to create, check, and destroy
// a cluster and SQL user, but uses a mocked API service.
func TestIntegrationSqlUserResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-sql-user-%s", tfTestPrefix, GenerateRandomString(4))
	sqlUserNameWithPass := "cockroach-user"
	sqlUserNameNoPass := "cockroach-user-nopass"
	sqlPassword := "cockroach@123456"
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
				UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
			},
		},
		State: "CREATED",
		Regions: []client.Region{
			{
				Name: "us-central1",
			},
		},
	}
	userWithPass := client.SQLUser{
		Name: sqlUserNameWithPass,
	}
	userNoPass := client.SQLUser{
		Name: sqlUserNameNoPass,
	}
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(4)
	s.EXPECT().CreateSQLUser(
		gomock.Any(),
		clusterID,
		&client.CreateSQLUserRequest{Name: sqlUserNameWithPass, Password: sqlPassword},
	).Return(&userWithPass, nil, nil)
	s.EXPECT().CreateSQLUser(
		gomock.Any(),
		clusterID,
		&client.CreateSQLUserRequest{Name: sqlUserNameNoPass, Password: testPassword},
	).Return(&userNoPass, nil, nil)
	s.EXPECT().ListSQLUsers(gomock.Any(), clusterID, gomock.Any()).Times(5).Return(
		&client.ListSQLUsersResponse{Users: []client.SQLUser{userWithPass, userNoPass}}, nil, nil)
	s.EXPECT().DeleteSQLUser(gomock.Any(), clusterID, userWithPass.Name)
	s.EXPECT().DeleteSQLUser(gomock.Any(), clusterID, userNoPass.Name)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testSqlUserResource(t, clusterName, sqlUserNameWithPass, sqlUserNameNoPass, true)
}

func testSqlUserResource(
	t *testing.T, clusterName, sqlUserNameWithPass, sqlUserNameNoPass string, useMock bool,
) {
	var (
		clusterResourceName = "cockroach_cluster.serverless"
		resourceNamePass    = "cockroach_sql_user.with_pass"
		resourceNameNoPass  = "cockroach_sql_user.no_pass"
	)
	defer HookGlobal(&generateRandomPassword, func() (string, error) {
		return testPassword, nil
	})()
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestSqlUserResourceConfig(clusterName, sqlUserNameWithPass, sqlUserNameNoPass, "cockroach@123456"),
				Check: resource.ComposeTestCheckFunc(
					testSqlUserExists(resourceNamePass, clusterResourceName),
					testSqlUserExists(resourceNameNoPass, clusterResourceName),
					resource.TestCheckResourceAttr(resourceNamePass, "name", sqlUserNameWithPass),
					resource.TestCheckResourceAttr(resourceNameNoPass, "name", sqlUserNameNoPass),
				),
			},
			{
				ResourceName:      resourceNamePass,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"password",
				},
			},
		},
	})
}

func testSqlUserExists(resourceName, clusterResourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		var listUserOptions client.ListSQLUsersOptions
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

		traceAPICall("ListSQLUsers")
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

func getTestSqlUserResourceConfig(
	clusterName, userNamePass, userNameNoPass, password string,
) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "serverless" {
    name           = "%s"
    cloud_provider = "GCP"
    serverless = {}
	regions = [{
		name = "us-central1"
	}]
}

resource "cockroach_sql_user" "with_pass" {
  name = "%s"
  password = "%s"
  cluster_id = cockroach_cluster.serverless.id
}

resource "cockroach_sql_user" "no_pass" {
  name = "%s"
  cluster_id = cockroach_cluster.serverless.id
}
`, clusterName, userNamePass, password, userNameNoPass)
}
