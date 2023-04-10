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
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// TestAccAllowlistEntryResource attempts to create, check, and destroy
// a real cluster and endpoint services. It will be skipped if TF_ACC isn't set.
func TestAccPrivateEndpointServicesResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("endpoint-services-%s", GenerateRandomString(2))
	testPrivateEndpointServicesResource(t, clusterName, false)
}

// TestIntegrationAllowlistEntryResource attempts to create, check, and destroy
// a cluster and endpoint services, but uses a mocked API service.
func TestIntegrationPrivateEndpointServicesResource(t *testing.T) {
	clusterName := fmt.Sprintf("endpoint-services-%s", GenerateRandomString(2))
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
	finalService := client.PrivateEndpointService{
		RegionName:    "ap-south-1",
		CloudProvider: "AWS",
		Status:        client.PRIVATEENDPOINTSERVICESTATUSTYPE_AVAILABLE,
		Aws: client.AWSPrivateLinkServiceDetail{
			ServiceName:         "finalService-name",
			ServiceId:           "finalService-id",
			AvailabilityZoneIds: []string{},
		},
	}
	initialService := finalService
	initialService.Status = client.PRIVATEENDPOINTSERVICESTATUSTYPE_CREATING
	services := &client.PrivateEndpointServices{
		Services: []client.PrivateEndpointService{
			finalService,
		},
	}
	initialServices := &client.PrivateEndpointServices{
		Services: []client.PrivateEndpointService{
			initialService,
		},
	}
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(3)
	s.EXPECT().CreatePrivateEndpointServices(gomock.Any(), clusterID).
		Return(initialServices, nil, nil)
	s.EXPECT().ListPrivateEndpointServices(gomock.Any(), clusterID).
		Return(services, nil, nil).
		Times(2)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testPrivateEndpointServicesResource(t, clusterName, true)
}

func testPrivateEndpointServicesResource(t *testing.T, clusterName string, useMock bool) {
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestPrivateEndpointServicesResourceConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.dedicated", "name", clusterName),
					resource.TestCheckResourceAttr("cockroach_private_endpoint_services.services", "services.#", "1"),
					resource.TestCheckResourceAttr("cockroach_private_endpoint_services.services", "services.0.status", string(client.PRIVATEENDPOINTSERVICESTATUSTYPE_AVAILABLE)),
				),
			},
		},
	})
}

func getTestPrivateEndpointServicesResourceConfig(clusterName string) string {
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
resource "cockroach_private_endpoint_services" "services" {
    cluster_id = cockroach_cluster.dedicated.id
}
`, clusterName)
}
