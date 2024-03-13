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
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/require"
)

// TestAccClusterCertResource attempts to download a cluster's cert.
// It will be skipped if TF_ACC isn't set.
func TestAccClusterCertDataSource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-cert-%s", GenerateRandomString(4))

	testClusterCertDataSource(t, clusterName, false)
}

func TestIntegrationClusterCertDataSource(t *testing.T) {
	clusterName := fmt.Sprintf("tftest-cert-%s", GenerateRandomString(4))
	clusterId := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()
	var numDownloads int
	defer HookGlobal(&DownloadClusterCert, func(_ *client.Cluster, _ *diag.Diagnostics) []byte {
		numDownloads++
		return []byte("Hark, a cert")
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

	httpOkResponse := &http.Response{Status: http.StatusText(http.StatusOK)}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterId).
		Return(cluster, httpOkResponse, nil).Times(5)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterId)

	testClusterCertDataSource(t, clusterName, true)

	// The expected count here (3) is the number of times the framework calls
	// Read on this resource. In that sense its somewhat meaningless. It was
	// previously 4 with the sdkv2 framework.
	require.Equal(t, 3, numDownloads)
}

func testClusterCertDataSource(t *testing.T, clusterName string, useMock bool) {
	certDataSourceName := "data.cockroach_cluster_cert.test"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestClusterCertDataSourceConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(certDataSourceName, "cert"),
				),
			},
		},
	})
}

func getTestClusterCertDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
	name           = "%s"
	cloud_provider = "GCP"
	serverless = {}
	regions = [{
		name = "us-central1"
	}]
}

data "cockroach_cluster_cert" "test" {
	id = cockroach_cluster.test.id
}
`, name)
}
