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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v2/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccConnectionStringDataSource attempts to retrieve a cluster's
// connection string. It will be skipped if TF_ACC isn't set.
func TestAccConnectionStringDataSource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-connect-%s", tfTestPrefix, GenerateRandomString(4))

	testConnectionStringDataSource(t, clusterName, false)
}

func TestIntegrationConnectionStringDataSource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-connect-%s", tfTestPrefix, GenerateRandomString(4))
	clusterId := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	cluster := &client.Cluster{
		Id:            clusterId,
		Name:          clusterName,
		CloudProvider: "GCP",
		State:         "CREATED",
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{},
		},
		Regions: []client.Region{
			{
				Name: "us-central1",
			},
		},
	}

	db, sqlUser, os := "testdb", "test", "MAC"
	connectStringOptions := &client.GetConnectionStringOptions{
		Database: &db,
		SqlUser:  &sqlUser,
		Os:       &os,
	}
	username := "test"
	connectionStringResponse := &client.GetConnectionStringResponse{
		ConnectionString: "postgresql://test@fake.cockroachlabs.cloud:26257/testdb",
		Params: client.ConnectionStringParameters{
			Database: "testdb",
			Host:     "fake.cockroachlabs.cloud",
			Port:     "26257",
			Username: &username,
		},
	}

	httpOkResponse := &http.Response{Status: http.StatusText(http.StatusOK)}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterId).
		Return(cluster, httpOkResponse, nil).Times(2)
	s.EXPECT().GetConnectionString(gomock.Any(), clusterId, connectStringOptions).
		Return(connectionStringResponse, httpOkResponse, nil).Times(3)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterId)

	testConnectionStringDataSource(t, clusterName, true)
}

func testConnectionStringDataSource(t *testing.T, clusterName string, useMock bool) {
	connectStrDataSourceName := "data.cockroach_connection_string.test"
	sqlPassword := "hunter2"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestConnectionStringResourceConfig(clusterName, sqlPassword),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(connectStrDataSourceName, "connection_string"),
					resource.TestCheckResourceAttrSet(connectStrDataSourceName, "connection_params.host"),
					resource.TestCheckResourceAttrSet(connectStrDataSourceName, "connection_params.port"),
					resource.TestCheckResourceAttr(connectStrDataSourceName, "connection_params.database", "testdb"),
					resource.TestCheckResourceAttr(connectStrDataSourceName, "connection_params.username", "test"),
					resource.TestCheckResourceAttr(connectStrDataSourceName, "connection_params.password", sqlPassword),
					resource.TestCheckResourceAttrWith(connectStrDataSourceName, "connection_string", func(value string) error {
						connectURL, err := url.Parse(value)
						if err != nil {
							return err
						}
						if _, ok := connectURL.User.Password(); !ok {
							return errors.New("password missing from connection string")
						}
						return nil
					}),
				),
			},
		},
	})
}

func getTestConnectionStringResourceConfig(name, sqlPassword string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
	name           = "%s"
	cloud_provider = "GCP"
	serverless = {}
	regions = [{
		name = "us-central1"
	}]
}

data "cockroach_connection_string" "test" {
	id = cockroach_cluster.test.id
	os = "MAC"
	database = "testdb"
	sql_user = "test"
	password = "%s"
}
`, name, sqlPassword)
}
