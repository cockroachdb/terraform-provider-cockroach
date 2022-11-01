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
	"net/http"
	"os"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccPrivateEndpointConnectionResource(t *testing.T) {
	t.Skip("Skipping until we can either integrate the AWS provider " +
		"or import a permanent test fixture.")
	t.Parallel()
	clusterName := fmt.Sprintf("aws-connection-%s", GenerateRandomString(5))
	testPrivateEndpointConnectionResource(t, clusterName, false)
}

func TestIntegrationPrivateEndpointConnectionResource(t *testing.T) {
	clusterName := fmt.Sprintf("aws-connection-%s", GenerateRandomString(5))
	clusterID := "cluster-id"
	endpointID := "endpoint-id"
	os.Setenv("COCKROACH_API_KEY", "fake")
	defer os.Unsetenv("COCKROACH_API_KEY")

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()
	cluster := client.Cluster{
		Name:          clusterName,
		Id:            clusterID,
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
				Name:      "us-east-1",
				NodeCount: 1,
			},
		},
	}
	services := &client.PrivateEndpointServices{
		Services: []client.PrivateEndpointService{
			{
				RegionName:    "us-east-1",
				CloudProvider: "AWS",
				Status:        client.PRIVATEENDPOINTSERVICESTATUS_AVAILABLE,
				Aws: client.AWSPrivateLinkServiceDetail{
					ServiceName:         "service-name",
					ServiceId:           "service-id",
					AvailabilityZoneIds: []string{},
				},
			},
		},
	}
	connection := client.AwsEndpointConnection{
		RegionName:    "us-east-1",
		CloudProvider: "AWS",
		Status:        client.AWSENDPOINTCONNECTIONSTATUS_AVAILABLE,
		EndpointId:    endpointID,
		ServiceId:     "service-id",
	}
	connections := &client.AwsEndpointConnections{
		Connections: []client.AwsEndpointConnection{connection},
	}
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(4)
	s.EXPECT().CreatePrivateEndpointServices(gomock.Any(), clusterID, gomock.Any()).
		Return(services, nil, nil)
	s.EXPECT().ListPrivateEndpointServices(gomock.Any(), clusterID).
		Return(services, nil, nil).
		Times(2)
	available := client.AWSENDPOINTCONNECTIONSTATUS_AVAILABLE
	s.EXPECT().SetAwsEndpointConnectionState(
		gomock.Any(),
		clusterID,
		endpointID,
		&client.CockroachCloudSetAwsEndpointConnectionStateRequest{
			Status: &available,
		}).
		Return(&connection, nil, nil)
	s.EXPECT().ListAwsEndpointConnections(gomock.Any(), clusterID).
		Return(connections, nil, nil).
		Times(2)
	rejected := client.AWSENDPOINTCONNECTIONSTATUS_REJECTED
	s.EXPECT().SetAwsEndpointConnectionState(
		gomock.Any(),
		clusterID,
		endpointID,
		&client.CockroachCloudSetAwsEndpointConnectionStateRequest{
			Status: &rejected,
		}).
		Return(&connection, nil, nil)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testPrivateEndpointConnectionResource(t, clusterName, true)
}

func testPrivateEndpointConnectionResource(t *testing.T, clusterName string, useMock bool) {
	resourceName := "cockroach_private_endpoint_connection.connection"
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestPrivateEndpointConnectionResourceConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "endpoint_id", "endpoint-id"),
					resource.TestCheckResourceAttr(resourceName, "service_id", "service-id"),
					resource.TestCheckResourceAttr(resourceName, "cluster_id", "cluster-id"),
					resource.TestCheckResourceAttr(resourceName, "region_name", "us-east-1"),
					resource.TestCheckResourceAttr(resourceName, "cloud_provider", "AWS"),
				),
			},
		},
	})
}

func getTestPrivateEndpointConnectionResourceConfig(clusterName string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "dedicated" {
    name           = "%s"
    cloud_provider = "AWS"
    dedicated = {
	  storage_gib = 15
	  machine_type = "m5.large"
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
	endpoint_id = "endpoint-id"
}
`, clusterName)
}
