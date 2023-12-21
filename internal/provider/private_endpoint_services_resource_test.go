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
	"strconv"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// TestAccDedicatedPrivateEndpointServicesResource attempts to create, check,
// and destroy a real dedicated cluster and endpoint services. It will be
// skipped if TF_ACC isn't set.
func TestAccDedicatedPrivateEndpointServicesResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("endpoint-services-%s", GenerateRandomString(2))
	testPrivateEndpointServicesResource(t, clusterName, false /* useMock */, false /* isServerless */)
}

// TestAccServerlessPrivateEndpointServicesResource attempts to create, check,
// and destroy a real Serverless cluster and endpoint services. It will be
// skipped if TF_ACC isn't set.
func TestAccServerlessPrivateEndpointServicesResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("endpoint-services-%s", GenerateRandomString(2))
	testPrivateEndpointServicesResource(t, clusterName, false /* useMock */, true /* isServerless */)
}

// TestIntegrationAllowlistEntryResource attempts to create, check, and destroy
// a cluster and endpoint services, but uses a mocked API service.
func TestIntegrationPrivateEndpointServicesResource(t *testing.T) {
	clusterName := fmt.Sprintf("endpoint-services-%s", GenerateRandomString(2))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	true_val := true
	cases := []struct {
		name         string
		finalCluster client.Cluster
	}{
		{
			"dedicated cluster",
			client.Cluster{
				Name:          clusterName,
				Id:            uuid.Nil.String(),
				Plan:          "ADVANCED",
				CloudProvider: "AWS",
				Config: client.ClusterConfig{
					Dedicated: &client.DedicatedHardwareConfig{
						StorageGib:     15,
						MachineType:    "m5.large",
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
				Plan:          "BASIC",
				CloudProvider: "AWS",
				State:         "CREATED",
				Config: client.ClusterConfig{
					Serverless: &client.ServerlessClusterConfig{
						RoutingId: "routing-id",
					},
				},
				Regions: []client.Region{
					{
						Name:    "us-east-1",
						Primary: &true_val,
					},
					{
						Name: "eu-central-1",
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
				Times(3)
			var regions []string
			if isServerless {
				regions = []string{"us-east-1", "eu-central-1"}
			} else {
				regions = []string{"us-east-1"}
			}
			services := &client.PrivateEndpointServices{}
			for _, region := range regions {
				services.Services = append(services.Services, client.PrivateEndpointService{
					RegionName:    region,
					CloudProvider: "AWS",
					Status:        client.PRIVATEENDPOINTSERVICESTATUSTYPE_AVAILABLE,
					Aws: client.AWSPrivateLinkServiceDetail{
						ServiceName:         "finalService-name",
						ServiceId:           "finalService-id",
						AvailabilityZoneIds: []string{},
					},
				})
			}
			if !isServerless {
				initialServices := &client.PrivateEndpointServices{}
				for _, service := range services.Services {
					service.Status = client.PRIVATEENDPOINTSERVICESTATUSTYPE_CREATING
					initialServices.Services = append(initialServices.Services, service)
				}
				s.EXPECT().CreatePrivateEndpointServices(gomock.Any(), clusterID).
					Return(initialServices, nil, nil)
			}
			s.EXPECT().ListPrivateEndpointServices(gomock.Any(), clusterID).
				Return(services, nil, nil).
				Times(3)
			s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

			testPrivateEndpointServicesResource(
				t,
				clusterName,
				true, /* useMock */
				isServerless,
			)
		})
	}
}

func testPrivateEndpointServicesResource(
	t *testing.T, clusterName string, useMock bool, isServerless bool,
) {
	resourceName := "cockroach_private_endpoint_services.services"
	var clusterResourceName string
	var privateEndpointServicesResourceConfigFn func(string) string
	var numExpectedServices int
	if isServerless {
		clusterResourceName = "cockroach_cluster.serverless"
		privateEndpointServicesResourceConfigFn = getTestPrivateEndpointServicesResourceConfigForServerless
		numExpectedServices = 2
	} else {
		clusterResourceName = "cockroach_cluster.dedicated"
		privateEndpointServicesResourceConfigFn = getTestPrivateEndpointServicesResourceConfigForDedicated
		numExpectedServices = 1
	}

	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(clusterResourceName, "name", clusterName),
		resource.TestCheckResourceAttr(resourceName, "services.#", strconv.Itoa(numExpectedServices)),
	}
	for i := 0; i < numExpectedServices; i++ {
		checks = append(checks,
			resource.TestCheckResourceAttr(resourceName, fmt.Sprintf("services.%d.status", i), string(client.PRIVATEENDPOINTSERVICESTATUSTYPE_AVAILABLE)),
		)
	}

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: privateEndpointServicesResourceConfigFn(clusterName),
				Check:  resource.ComposeTestCheckFunc(checks...),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func getTestPrivateEndpointServicesResourceConfigForDedicated(clusterName string) string {
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
`, clusterName)
}

func getTestPrivateEndpointServicesResourceConfigForServerless(clusterName string) string {
	// Use two regions here so we end up creating the cluster on the multi-
	// region host, which has PrivateLink enabled.
	return fmt.Sprintf(`
resource "cockroach_cluster" "serverless" {
    name           = "%s"
    cloud_provider = "AWS"
    serverless = {}
    regions = [
        { name = "us-east-1", primary = true },
        { name = "eu-central-1" },
    ]
}
resource "cockroach_private_endpoint_services" "services" {
    cluster_id = cockroach_cluster.serverless.id
}
`, clusterName)
}
