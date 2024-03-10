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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDedicatedPrivateEndpointConnectionResource(t *testing.T) {
	t.Skip("Skipping until we can either integrate the AWS provider " +
		"or import a permanent test fixture.")
	t.Parallel()
	clusterName := fmt.Sprintf("aws-connection-%s", GenerateRandomString(5))
	testPrivateEndpointConnectionResource(t, clusterName, false /* useMock */, false /* isServerless */)
}

func TestAccServerlessPrivateEndpointConnectionResource(t *testing.T) {
	// This test relies on a pre-created private endpoint created via the AWS
	// console. The endpoint id is vpce-0011573c1fa6afa3d and lives in the
	// staging AWS serverless cluster account.
	t.Parallel()
	clusterName := fmt.Sprintf("aws-connection-%s", GenerateRandomString(5))
	testPrivateEndpointConnectionResource(t, clusterName, false /* useMock */, true /* isServerless */)
}

func TestIntegrationPrivateEndpointConnectionResource(t *testing.T) {
	clusterName := fmt.Sprintf("private-connection-%s", GenerateRandomString(5))
	clusterID := uuid.Nil.String()
	endpointID := "vpce-0011573c1fa6afa3d"
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	services := &client.PrivateEndpointServices{
		Services: []client.PrivateEndpointService{
			{
				RegionName:          "us-east-1",
				CloudProvider:       "AWS",
				Status:              client.PRIVATEENDPOINTSERVICESTATUSTYPE_AVAILABLE,
				Name:                "service-name",
				EndpointServiceId:   "service-id",
				AvailabilityZoneIds: []string{},
				Aws: &client.AWSPrivateLinkServiceDetail{
					ServiceName:         "service-name",
					ServiceId:           "service-id",
					AvailabilityZoneIds: []string{},
				},
			},
		},
	}
	connection := client.PrivateEndpointConnection{
		RegionName:        &services.Services[0].RegionName,
		CloudProvider:     "AWS",
		Status:            client.PRIVATEENDPOINTCONNECTIONSTATUS_AVAILABLE,
		EndpointId:        endpointID,
		EndpointServiceId: "service-id",
	}
	connections := &client.PrivateEndpointConnections{
		Connections: []client.PrivateEndpointConnection{connection},
	}

	zeroSpendLimit := int32(0)
	cases := []struct {
		name         string
		finalCluster client.Cluster
	}{
		{
			"dedicated cluster",
			client.Cluster{
				Name:          clusterName,
				Id:            clusterID,
				CloudProvider: "AWS",
				Config: client.ClusterConfig{
					Dedicated: &client.DedicatedHardwareConfig{
						StorageGib:     15,
						NumVirtualCpus: 2,
					},
				},
				State: "CREATED",
				Regions: []client.Region{
					{
						Name:      "us-east-1",
						NodeCount: 1,
					},
				},
			},
		},
		{
			"serverless cluster",
			client.Cluster{
				Name:          clusterName,
				Id:            uuid.Nil.String(),
				Plan:          "SERVERLESS",
				CloudProvider: "AWS",
				State:         "CREATED",
				Config: client.ClusterConfig{
					Serverless: &client.ServerlessClusterConfig{
						SpendLimit: &zeroSpendLimit,
						RoutingId:  "routing-id",
					},
				},
				Regions: []client.Region{
					{
						Name: "us-east-1",
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
			isServerless := cluster.Config.Dedicated == nil

			s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
				Return(&cluster, nil, nil)
			s.EXPECT().GetCluster(gomock.Any(), clusterID).
				Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
				Times(4)
			if !isServerless {
				s.EXPECT().CreatePrivateEndpointServices(gomock.Any(), clusterID).
					Return(services, nil, nil)
			}
			s.EXPECT().ListPrivateEndpointServices(gomock.Any(), clusterID).
				Return(services, nil, nil).
				Times(2)
			s.EXPECT().AddPrivateEndpointConnection(
				gomock.Any(),
				clusterID,
				&client.AddPrivateEndpointConnectionRequest{EndpointId: endpointID}).
				Return(&connection, nil, nil)
			s.EXPECT().ListPrivateEndpointConnections(gomock.Any(), clusterID).
				Return(connections, nil, nil).
				Times(3)
			s.EXPECT().DeletePrivateEndpointConnection(
				gomock.Any(),
				clusterID,
				endpointID).
				Return(nil, nil)
			s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

			testPrivateEndpointConnectionResource(
				t,
				clusterName,
				true, /* useMock */
				isServerless,
			)
		})
	}
}

func testPrivateEndpointConnectionResource(
	t *testing.T, clusterName string, useMock bool, isServerless bool,
) {
	const resourceName = "cockroach_private_endpoint_connection.connection"
	var privateEndpointConnectionResourceConfigFn func(string) string
	if isServerless {
		privateEndpointConnectionResourceConfigFn = getTestPrivateEndpointConnectionResourceConfigForServerless
	} else {
		privateEndpointConnectionResourceConfigFn = getTestPrivateEndpointConnectionResourceConfigForDedicated
	}
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: privateEndpointConnectionResourceConfigFn(clusterName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "endpoint_id", "vpce-0011573c1fa6afa3d"),
					resource.TestCheckResourceAttr(resourceName, "region_name", "us-east-1"),
					resource.TestCheckResourceAttr(resourceName, "cloud_provider", "AWS"),
					resource.TestCheckResourceAttrSet(resourceName, "service_id"),
					resource.TestCheckResourceAttrSet(resourceName, "cluster_id"),
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

func getTestPrivateEndpointConnectionResourceConfigForDedicated(clusterName string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "dedicated" {
    name           = "%s"
    cloud_provider = "AWS"
    dedicated = {
        storage_gib = 15
        num_virtual_cpus = 2
    }
    regions = [{
        name: "us-east-1"
        node_count: 1
    }]
}
resource "cockroach_private_endpoint_services" "services" {
    cluster_id = cockroach_cluster.dedicated.id
}

resource "cockroach_private_endpoint_connection" "connection" {
    cluster_id = cockroach_cluster.dedicated.id
    endpoint_id = "vpce-0011573c1fa6afa3d"
}
`, clusterName)
}

func getTestPrivateEndpointConnectionResourceConfigForServerless(clusterName string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "serverless" {
    name           = "%s"
    cloud_provider = "AWS"
    serverless = {
        spend_limit = 0
    }
    regions = [{
        name = "us-east-1"
    }]
}
resource "cockroach_private_endpoint_services" "services" {
    cluster_id = cockroach_cluster.serverless.id
}

resource "cockroach_private_endpoint_connection" "connection" {
    cluster_id = cockroach_cluster.serverless.id
    endpoint_id = "vpce-0011573c1fa6afa3d"
}
`, clusterName)
}
