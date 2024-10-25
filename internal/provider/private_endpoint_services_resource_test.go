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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v4/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccDedicatedPrivateEndpointServicesResource attempts to create, check,
// and destroy a real dedicated cluster and endpoint services. It will be
// skipped if TF_ACC isn't set.
func TestAccDedicatedPrivateEndpointServicesResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-endpt-svc-%s", tfTestPrefix, GenerateRandomString(2))
	testPrivateEndpointServicesResource(t, clusterName, false /* useMock */, false /* isServerless */, client.CLOUDPROVIDERTYPE_GCP)
}

// TestAccServerlessPrivateEndpointServicesResource attempts to create, check,
// and destroy a real Serverless cluster and endpoint services. It will be
// skipped if TF_ACC isn't set.
func TestAccServerlessPrivateEndpointServicesResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-endpt-svc-%s", tfTestPrefix, GenerateRandomString(2))
	testPrivateEndpointServicesResource(t, clusterName, false /* useMock */, true /* isServerless */, client.CLOUDPROVIDERTYPE_AWS)
}

// TestIntegrationAllowlistEntryResource attempts to create, check, and destroy
// a cluster and endpoint services, but uses a mocked API service.
func TestIntegrationPrivateEndpointServicesResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-endpt-svc-%s", tfTestPrefix, GenerateRandomString(2))
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
			"dedicated GCP cluster",
			client.Cluster{
				Name:          clusterName,
				Id:            uuid.Nil.String(),
				CloudProvider: "GCP",
				Config: client.ClusterConfig{
					Dedicated: &client.DedicatedHardwareConfig{
						StorageGib:     15,
						MachineType:    "not verified",
						NumVirtualCpus: 2,
					},
				},
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
			"dedicated AWS cluster",
			client.Cluster{
				Name:          clusterName,
				Id:            uuid.Nil.String(),
				Plan:          "ADVANCED",
				CloudProvider: "AWS",
				Config: client.ClusterConfig{
					Dedicated: &client.DedicatedHardwareConfig{
						StorageGib:     15,
						MachineType:    "not verified",
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
						RoutingId:   "routing-id",
						UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
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
			s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
				Return(initialBackupConfig, httpOk, nil).AnyTimes()

			var regions []string
			for _, r := range c.finalCluster.Regions {
				regions = append(regions, r.Name)
			}

			services := &client.PrivateEndpointServices{}
			for _, region := range regions {
				svc := client.PrivateEndpointService{
					RegionName:          region,
					CloudProvider:       c.finalCluster.CloudProvider,
					Status:              client.PRIVATEENDPOINTSERVICESTATUSTYPE_AVAILABLE,
					Name:                "finalService-name",
					EndpointServiceId:   "finalService-id",
					AvailabilityZoneIds: []string{"az1", "az2", "az3"},
				}

				if c.finalCluster.CloudProvider == client.CLOUDPROVIDERTYPE_AWS {
					svc.Aws = &client.AWSPrivateLinkServiceDetail{
						ServiceName:         "finalService-name",
						ServiceId:           "finalService-id",
						AvailabilityZoneIds: []string{"az1", "az2", "az3"},
					}
				}

				services.Services = append(services.Services, svc)
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
				c.finalCluster.CloudProvider,
			)
		})
	}
}

func testPrivateEndpointServicesResource(
	t *testing.T, clusterName string, useMock bool, isServerless bool, cloud client.CloudProviderType,
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
		numExpectedServices = 1
		switch cloud {
		case client.CLOUDPROVIDERTYPE_AWS:
			privateEndpointServicesResourceConfigFn = getTestPrivateEndpointServicesResourceConfigForDedicatedAWS
		case client.CLOUDPROVIDERTYPE_GCP:
			privateEndpointServicesResourceConfigFn = getTestPrivateEndpointServicesResourceConfigForDedicatedGCP
		}
	}

	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(clusterResourceName, "name", clusterName),
		resource.TestCheckResourceAttr(resourceName, "services.#", strconv.Itoa(numExpectedServices)),
		resource.TestCheckResourceAttr(resourceName, "services_map.%", strconv.Itoa(numExpectedServices)),
	}
	for i := 0; i < numExpectedServices; i++ {
		svc := fmt.Sprintf("services.%d", i)
		checks = append(checks,
			resource.TestCheckResourceAttr(resourceName, svc+".status", string(client.PRIVATEENDPOINTSERVICESTATUSTYPE_AVAILABLE)),
		)

		checks = append(checks,
			resource.TestCheckResourceAttrPair(resourceName, svc, resourceName, "services_map.us-east-1"),
		)

		if cloud == client.CLOUDPROVIDERTYPE_AWS {
			checks = append(checks, []resource.TestCheckFunc{
				resource.TestCheckResourceAttrPair(resourceName, svc+".name", resourceName, svc+".aws.service_name"),
				resource.TestCheckResourceAttrPair(resourceName, svc+".endpoint_service_id", resourceName, svc+".aws.service_id"),
				resource.TestCheckResourceAttrPair(resourceName, svc+".availability_zone_ids", resourceName, svc+".aws.availability_zone_ids"),
			}...)
		} else {
			checks = append(checks, resource.TestCheckNoResourceAttr(resourceName, svc+".aws.service_name"))
		}
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

func getTestPrivateEndpointServicesResourceConfigForDedicatedGCP(clusterName string) string {
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
resource "cockroach_private_endpoint_services" "services" {
    cluster_id = cockroach_cluster.dedicated.id
}
`, clusterName)
}

func getTestPrivateEndpointServicesResourceConfigForDedicatedAWS(clusterName string) string {
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
