/*
 Copyright 2025 The Cockroach Authors

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
	"regexp"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccEgressPrivateEndpointResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-epe-%s", tfTestPrefix, GenerateRandomString(2))

	cluster := client.Cluster{
		Name: clusterName,
	}

	endpoint := client.EgressPrivateEndpoint{
		Region:                  "us-east-2",
		TargetServiceIdentifier: "com.amazonaws.us-east-2.logs",
		TargetServiceType:       client.EGRESSPRIVATEENDPOINTTARGETSERVICETYPETYPE_PRIVATE_SERVICE,
	}

	testEgressPrivateEndpointResource(t, cluster, endpoint, "", false)
}

func TestIntegrationEgressPrivateEndpointResource(t *testing.T) {
	clusterID := uuid.Nil.String()
	clusterName := fmt.Sprintf("%s-epe-%s", tfTestPrefix, GenerateRandomString(2))
	endpointID := uuid.Nil.String()
	endpointRegion := "us-east-2"
	targetServiceType := client.EGRESSPRIVATEENDPOINTTARGETSERVICETYPETYPE_PRIVATE_SERVICE
	targetServiceIdentifier := "com.amazonaws.vpce.us-east-1.vpce-svc-01234567890abcdef"

	defaultAWSCluster := &client.Cluster{
		Id:            clusterID,
		Name:          clusterName,
		State:         "CREATED",
		Plan:          "ADVANCED",
		CloudProvider: "AWS",
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				StorageGib:     15,
				NumVirtualCpus: 2,
			},
		},
		Regions: []client.Region{
			{
				Name:      endpointRegion,
				NodeCount: 1,
			},
		},
	}

	defaultEndpoint := client.EgressPrivateEndpoint{
		Id:                      endpointID,
		Region:                  endpointRegion,
		TargetServiceIdentifier: targetServiceIdentifier,
		TargetServiceType:       client.EgressPrivateEndpointTargetServiceTypeType(targetServiceType),

		EndpointConnectionId: "generatedID",
		EndpointAddress:      "generatedAddress",
		State:                client.EGRESSPRIVATEENDPOINTSTATETYPE_AVAILABLE,
	}

	testCases := []struct {
		description  string
		setupCluster func() client.Cluster
		endpoint     client.EgressPrivateEndpoint

		expectError string
	}{
		{
			description: "valid cluster",
			setupCluster: func() client.Cluster {
				return *defaultAWSCluster
			},
			endpoint: defaultEndpoint,
		},
		{
			description: "serverless cluster",
			expectError: "Egress private endpoints cannot be created on serverless clusters",

			setupCluster: func() client.Cluster {
				serverlessCluster := *defaultAWSCluster
				serverlessCluster.Plan = "STANDARD"
				serverlessCluster.Config = client.ClusterConfig{
					Serverless: &client.ServerlessClusterConfig{
						RoutingId:   "routing-id",
						UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
					},
				}
				return serverlessCluster
			},
			endpoint: defaultEndpoint,
		},
		{
			description: "azure cluster",
			expectError: "Egress private endpoints cannot be created on Azure clusters",

			setupCluster: func() client.Cluster {
				azureCluster := *defaultAWSCluster
				azureCluster.CloudProvider = "AZURE"
				return azureCluster
			},
			endpoint: defaultEndpoint,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s := mock_client.NewMockService(ctrl)
			defer ctrl.Finish()
			defer HookGlobal(&NewService, func(c *client.Client) client.Service {
				return s
			})()

			cluster := testCase.setupCluster()

			s.EXPECT().GetCluster(gomock.Any(), clusterID).
				Return(&cluster, nil, nil)

			if testCase.expectError == "" {
				s.EXPECT().CreateEgressPrivateEndpoint(gomock.Any(), clusterID, gomock.Any()).
					Return(&testCase.endpoint, nil, nil)

				call := s.EXPECT().GetEgressPrivateEndpoint(gomock.Any(), clusterID, gomock.Any()).
					Return(&testCase.endpoint, nil, nil).
					Times(3)

				s.EXPECT().GetEgressPrivateEndpoint(gomock.Any(), clusterID, gomock.Any()).
					Return(nil, nil, nil).After(call)

				s.EXPECT().DeleteEgressPrivateEndpoint(
					gomock.Any(), cluster.Id, testCase.endpoint.Id).
					Return(nil, nil)
			}

			testEgressPrivateEndpointResource(
				t,
				cluster,
				testCase.endpoint,
				testCase.expectError,
				true,
			)
		})
	}
}

func testEgressPrivateEndpointResource(
	t *testing.T,
	cluster client.Cluster,
	endpoint client.EgressPrivateEndpoint,
	expectError string,
	useMock bool,
) {
	resourceName := "cockroach_egress_private_endpoint.test"

	getConfigFn := getTestEgressPrivateEndpointResourceConfigWithCluster
	if useMock {
		getConfigFn = getTestEgressPrivateEndpointResourceConfig
	}

	steps := []resource.TestStep{
		{
			Config: getConfigFn(cluster, endpoint),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(
					resourceName, "region", endpoint.Region,
				),
				resource.TestCheckResourceAttr(
					resourceName, "target_service_identifier", endpoint.TargetServiceIdentifier,
				),
				resource.TestCheckResourceAttr(
					resourceName, "target_service_type", string(endpoint.TargetServiceType),
				),
				resource.TestCheckResourceAttrSet(
					resourceName, "id",
				),
				resource.TestCheckResourceAttrSet(
					resourceName, "cluster_id",
				),
				resource.TestCheckResourceAttrSet(
					resourceName, "endpoint_connection_id",
				),
				resource.TestCheckResourceAttrSet(
					resourceName, "endpoint_address",
				),
				resource.TestCheckResourceAttrSet(
					resourceName, "state",
				),
			),
		},
	}
	if useMock {
		steps = append(steps, resource.TestStep{
			ResourceName:      resourceName,
			ImportState:       true,
			ImportStateVerify: true,
			ImportStateId:     fmt.Sprintf("%s:%s", cluster.Id, endpoint.Id),
		})
	}

	if expectError != "" {
		steps = []resource.TestStep{
			{
				Config:      getConfigFn(cluster, endpoint),
				ExpectError: regexp.MustCompile(expectError),
			},
		}
	}

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

func getTestEgressPrivateEndpointResourceConfig(cluster client.Cluster, endpoint client.EgressPrivateEndpoint) string {
	clusterID := fmt.Sprintf(`"%s"`, cluster.Id)
	if cluster.Id == "" {
		clusterID = "cockroach_cluster.dedicated.id"
	}

	endpointConfig := fmt.Sprintf(`
resource "cockroach_egress_private_endpoint" "test" {
  cluster_id                = %s
  region                    = "%s"
  target_service_type       = "%s"
  target_service_identifier = "%s"
}
`, clusterID, endpoint.Region, string(endpoint.TargetServiceType), endpoint.TargetServiceIdentifier)
	return endpointConfig
}

func getTestEgressPrivateEndpointResourceConfigWithCluster(cluster client.Cluster, endpoint client.EgressPrivateEndpoint) string {
	clusterConfig := fmt.Sprintf(`
resource "cockroach_cluster" "dedicated" {
    name           = "%s"
    cloud_provider = "AWS"
    dedicated = {
        storage_gib = 15
        num_virtual_cpus = 2
    }
    regions = [{
        name: "%s"
        node_count: 1
    }]
}
`, cluster.Name, endpoint.Region)
	return clusterConfig + getTestEgressPrivateEndpointResourceConfig(cluster, endpoint)
}
