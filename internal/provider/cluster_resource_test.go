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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	framework_resource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"
)

const (
	// The patch versions are just for mocks. They don't need to be the actual
	// latest available patch versions; they just need to resolve to the correct
	// major versions.
	minSupportedClusterMajorVersion = "v23.1"
	minSupportedClusterPatchVersion = "v23.1.0"
	latestClusterMajorVersion       = "v23.2"
	latestClusterPatchVersion       = "v23.2.0"
)

// TestAccClusterResource attempts to create, check, update, and destroy
// a real cluster. It will be skipped if TF_ACC isn't set.
func TestAccServerlessClusterResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-serverless-%s", tfTestPrefix, GenerateRandomString(2))
	resource.Test(t, resource.TestCase{
		IsUnitTest:               false,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			serverlessClusterWithSpendLimit(clusterName),
			serverlessClusterWithResourceLimits(clusterName),
			serverlessClusterWithNoLimits(clusterName),
			serverlessClusterWithZeroSpendLimit(clusterName),
		},
	})
}

// TestAccMultiRegionServerlessClusterResource attempts to create, update, check, and destroy
// a real multi-region serverless cluster. It will be skipped if TF_ACC isn't set.
func TestAccMultiRegionServerlessClusterResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-multi-region-serverless-%s", tfTestPrefix, GenerateRandomString(2))
	resource.Test(t, resource.TestCase{
		IsUnitTest:               false,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			multiRegionServerlessClusterResource(clusterName),
			multiRegionServerlessClusterResourceRegionUpdate(clusterName),
		},
	})
}

// TestIntegrationServerlessClusterResource attempts to create, check, and destroy
// a cluster, but uses a mocked API service.
func TestIntegrationServerlessClusterResource(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	spendLimit := int32(1)
	zeroSpendLimit := int32(0)
	true_val := true
	initialCluster := client.Cluster{
		Id:               uuid.Nil.String(),
		Name:             fmt.Sprintf("%s-serverless-%s", tfTestPrefix, GenerateRandomString(2)),
		CockroachVersion: latestClusterPatchVersion,
		Plan:             "SERVERLESS",
		CloudProvider:    "GCP",
		State:            "CREATED",
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{
				SpendLimit: &spendLimit,
				UsageLimits: &client.UsageLimits{
					RequestUnitLimit: 1,
					StorageMibLimit:  1,
				},
				RoutingId: "routing-id",
			},
		},
		Regions: []client.Region{
			{
				Name: "us-central1",
			},
		},
	}

	cases := []struct {
		name             string
		finalCluster     client.Cluster
		testStep         func(clusterName string) resource.TestStep
		skipVerifyImport bool
	}{
		{
			"single-region serverless cluster with resource limits",
			client.Cluster{
				Id:               uuid.Nil.String(),
				Name:             initialCluster.Name,
				CockroachVersion: latestClusterPatchVersion,
				Plan:             "SERVERLESS",
				CloudProvider:    "GCP",
				State:            "CREATED",
				Config: client.ClusterConfig{
					Serverless: &client.ServerlessClusterConfig{
						UsageLimits: &client.UsageLimits{
							RequestUnitLimit: 10_000_000_000,
							StorageMibLimit:  102_400,
						},
						RoutingId: "routing-id",
					},
				},
				Regions: []client.Region{
					{
						Name: "us-central1",
					},
				},
			},
			serverlessClusterWithResourceLimits,
			false,
		},
		{
			"single-region serverless cluster with no limits",
			client.Cluster{
				Id:               uuid.Nil.String(),
				Name:             initialCluster.Name,
				CockroachVersion: latestClusterPatchVersion,
				Plan:             "SERVERLESS",
				CloudProvider:    "GCP",
				State:            "CREATED",
				Config: client.ClusterConfig{
					Serverless: &client.ServerlessClusterConfig{
						RoutingId: "routing-id",
					},
				},
				Regions: []client.Region{
					{
						Name: "us-central1",
					},
				},
			},
			serverlessClusterWithNoLimits,
			false,
		},
		{
			"single-region serverless cluster with zero spend limit",
			client.Cluster{
				Id:               uuid.Nil.String(),
				Name:             initialCluster.Name,
				CockroachVersion: latestClusterPatchVersion,
				Plan:             "SERVERLESS",
				CloudProvider:    "GCP",
				State:            "CREATED",
				Config: client.ClusterConfig{
					Serverless: &client.ServerlessClusterConfig{
						SpendLimit: &zeroSpendLimit,
						RoutingId:  "routing-id",
					},
				},
				Regions: []client.Region{
					{
						Name: "us-central1",
					},
				},
			},
			serverlessClusterWithZeroSpendLimit,
			// The plan specifies a spend limit, but import always uses usage limits.
			true,
		},
		{
			"multi-region serverless cluster",
			client.Cluster{
				Id:               uuid.Nil.String(),
				Name:             initialCluster.Name,
				CockroachVersion: latestClusterPatchVersion,
				Plan:             "SERVERLESS",
				CloudProvider:    "GCP",
				State:            "CREATED",
				Config: client.ClusterConfig{
					Serverless: &client.ServerlessClusterConfig{
						UsageLimits: client.NewUsageLimits(10000000000, 102400),
						RoutingId:   "routing-id",
					},
				},
				Regions: []client.Region{
					{
						Name: "us-west2",
					},
					{
						Name:    "us-east1",
						Primary: &true_val,
					},
					{
						Name: "europe-west1",
					},
				},
			},
			multiRegionServerlessClusterResource,
			false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s := mock_client.NewMockService(ctrl)
			defer HookGlobal(&NewService, func(c *client.Client) client.Service {
				return s
			})()

			steps := []resource.TestStep{
				// Create initial resource.
				serverlessClusterWithSpendLimit(initialCluster.Name),
				// Apply an update.
				c.testStep(initialCluster.Name),
				// Import the resource.
				{
					SkipFunc: func() (bool, error) {
						return c.skipVerifyImport, nil
					},
					ResourceName:      "cockroach_cluster.serverless",
					ImportState:       true,
					ImportStateVerify: true,
				},
			}

			s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
				Return(&initialCluster, nil, nil)
			s.EXPECT().GetCluster(gomock.Any(), c.finalCluster.Id).
				Return(&initialCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
				Times(7)
			s.EXPECT().UpdateCluster(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&c.finalCluster, nil, nil)
			s.EXPECT().GetCluster(gomock.Any(), c.finalCluster.Id).
				Return(&c.finalCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
				Times(5)
			if !c.skipVerifyImport {
				s.EXPECT().GetCluster(gomock.Any(), c.finalCluster.Id).
					Return(&c.finalCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
					Times(2)
			}
			s.EXPECT().DeleteCluster(gomock.Any(), c.finalCluster.Id)

			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    steps,
			})
		})
	}
}

func serverlessClusterWithSpendLimit(clusterName string) resource.TestStep {
	const (
		resourceName   = "cockroach_cluster.serverless"
		dataSourceName = "data.cockroach_cluster.test"
	)
	return resource.TestStep{
		// Cluster with spend limit.
		Config: fmt.Sprintf(`
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

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.serverless.id
			}
			`, clusterName),
		Check: resource.ComposeTestCheckFunc(
			testCheckCockroachClusterExists(resourceName),
			resource.TestCheckResourceAttr(resourceName, "name", clusterName),
			resource.TestCheckResourceAttrSet(resourceName, "cloud_provider"),
			resource.TestCheckResourceAttrSet(resourceName, "cockroach_version"),
			resource.TestCheckResourceAttr(resourceName, "plan", "SERVERLESS"),
			resource.TestCheckResourceAttr(resourceName, "state", string(client.CLUSTERSTATETYPE_CREATED)),
			resource.TestCheckResourceAttr(resourceName, "regions.#", "1"),
			resource.TestCheckResourceAttr(resourceName, "serverless.spend_limit", "1"),
			resource.TestCheckNoResourceAttr(resourceName, "serverless.usage_limits"),
			resource.TestCheckResourceAttr(dataSourceName, "name", clusterName),
			resource.TestCheckResourceAttrSet(dataSourceName, "cloud_provider"),
			resource.TestCheckResourceAttrSet(dataSourceName, "cockroach_version"),
			resource.TestCheckResourceAttr(dataSourceName, "plan", "SERVERLESS"),
			resource.TestCheckResourceAttr(dataSourceName, "state", string(client.CLUSTERSTATETYPE_CREATED)),
			resource.TestCheckResourceAttr(dataSourceName, "regions.#", "1"),
			resource.TestCheckNoResourceAttr(dataSourceName, "serverless.spend_limit"),
			resource.TestCheckResourceAttrSet(dataSourceName, "serverless.usage_limits.request_unit_limit"),
			resource.TestCheckResourceAttrSet(dataSourceName, "serverless.usage_limits.storage_mib_limit"),
		),
	}
}

func serverlessClusterWithResourceLimits(clusterName string) resource.TestStep {
	const (
		resourceName   = "cockroach_cluster.serverless"
		dataSourceName = "data.cockroach_cluster.test"
	)
	return resource.TestStep{
		// Update cluster to use resource limits.
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "serverless" {
				name           = "%s"
				cloud_provider = "GCP"
				serverless = {
					usage_limits = {
						request_unit_limit = 10000000000
						storage_mib_limit = 102400
					}
				}
				regions = [{
					name = "us-central1"
				}]
			}

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.serverless.id
			}
			`, clusterName),
		Check: resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr(resourceName, "name", clusterName),
			resource.TestCheckNoResourceAttr(resourceName, "serverless.spend_limit"),
			resource.TestCheckResourceAttr(resourceName, "serverless.usage_limits.request_unit_limit", "10000000000"),
			resource.TestCheckResourceAttr(resourceName, "serverless.usage_limits.storage_mib_limit", "102400"),
			resource.TestCheckResourceAttr(dataSourceName, "name", clusterName),
			resource.TestCheckNoResourceAttr(dataSourceName, "serverless.spend_limit"),
			resource.TestCheckResourceAttr(dataSourceName, "serverless.usage_limits.request_unit_limit", "10000000000"),
			resource.TestCheckResourceAttr(dataSourceName, "serverless.usage_limits.storage_mib_limit", "102400"),
		),
	}
}

func serverlessClusterWithNoLimits(clusterName string) resource.TestStep {
	const (
		resourceName   = "cockroach_cluster.serverless"
		dataSourceName = "data.cockroach_cluster.test"
	)
	return resource.TestStep{
		// Update cluster to have no limits.
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "serverless" {
				name           = "%s"
				cloud_provider = "GCP"
				serverless = {
					usage_limits = null
				}
				regions = [{
					name = "us-central1"
				}]
			}

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.serverless.id
			}
			`, clusterName),
		Check: resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr(resourceName, "name", clusterName),
			resource.TestCheckNoResourceAttr(resourceName, "serverless.spend_limit"),
			resource.TestCheckNoResourceAttr(resourceName, "serverless.usage_limits"),
			resource.TestCheckResourceAttr(dataSourceName, "name", clusterName),
			resource.TestCheckNoResourceAttr(dataSourceName, "serverless.spend_limit"),
			resource.TestCheckNoResourceAttr(dataSourceName, "serverless.usage_limits"),
		),
	}
}

func serverlessClusterWithZeroSpendLimit(clusterName string) resource.TestStep {
	const (
		resourceName   = "cockroach_cluster.serverless"
		dataSourceName = "data.cockroach_cluster.test"
	)
	return resource.TestStep{
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "serverless" {
				name           = "%s"
				cloud_provider = "GCP"
				serverless = {
					spend_limit = 0
				}
				regions = [{
					name = "us-central1"
				}]
			}

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.serverless.id
			}
			`, clusterName),
		Check: resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr(resourceName, "name", clusterName),
			resource.TestCheckResourceAttr(resourceName, "serverless.spend_limit", "0"),
			resource.TestCheckNoResourceAttr(resourceName, "serverless.usage_limits"),
			resource.TestCheckResourceAttr(dataSourceName, "name", clusterName),
			resource.TestCheckNoResourceAttr(dataSourceName, "serverless.usage_limits"),
		),
	}
}

func multiRegionServerlessClusterResource(clusterName string) resource.TestStep {
	const (
		resourceName   = "cockroach_cluster.serverless"
		dataSourceName = "data.cockroach_cluster.test"
	)
	return resource.TestStep{
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "serverless" {
				name           = "%s"
				cloud_provider = "GCP"
				serverless = {
					usage_limits = {
						request_unit_limit = 10000000000
						storage_mib_limit = 102400
					}
				}
				regions = [
					{
						name = "europe-west1"
					},
					{
						name = "us-east1"
						primary = true
					},
					{
						name = "us-west2"
					},
				]
			}

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.serverless.id
			}
			`, clusterName),
		Check: resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr(resourceName, "name", clusterName),
			resource.TestCheckResourceAttrSet(resourceName, "cloud_provider"),
			resource.TestCheckResourceAttrSet(resourceName, "cockroach_version"),
			resource.TestCheckResourceAttr(resourceName, "plan", "SERVERLESS"),
			resource.TestCheckResourceAttr(resourceName, "state", string(client.CLUSTERSTATETYPE_CREATED)),
			resource.TestCheckResourceAttr(resourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(resourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(resourceName, "regions.0.primary", "false"),
			resource.TestCheckResourceAttr(resourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(resourceName, "regions.1.primary", "true"),
			resource.TestCheckResourceAttr(resourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(resourceName, "regions.2.primary", "false"),
			resource.TestCheckNoResourceAttr(resourceName, "serverless.spend_limit"),
			resource.TestCheckResourceAttr(resourceName, "serverless.usage_limits.request_unit_limit", "10000000000"),
			resource.TestCheckResourceAttr(resourceName, "serverless.usage_limits.storage_mib_limit", "102400"),
			resource.TestCheckResourceAttr(dataSourceName, "name", clusterName),
			resource.TestCheckResourceAttrSet(dataSourceName, "cloud_provider"),
			resource.TestCheckResourceAttrSet(dataSourceName, "cockroach_version"),
			resource.TestCheckResourceAttr(dataSourceName, "plan", "SERVERLESS"),
			resource.TestCheckResourceAttr(dataSourceName, "state", string(client.CLUSTERSTATETYPE_CREATED)),
			resource.TestCheckResourceAttr(dataSourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(dataSourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(dataSourceName, "regions.0.primary", "false"),
			resource.TestCheckResourceAttr(dataSourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(dataSourceName, "regions.1.primary", "true"),
			resource.TestCheckResourceAttr(dataSourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(dataSourceName, "regions.2.primary", "false"),
			resource.TestCheckNoResourceAttr(dataSourceName, "serverless.spend_limit"),
			resource.TestCheckResourceAttr(dataSourceName, "serverless.usage_limits.request_unit_limit", "10000000000"),
			resource.TestCheckResourceAttr(dataSourceName, "serverless.usage_limits.storage_mib_limit", "102400"),
		),
	}
}

func multiRegionServerlessClusterResourceRegionUpdate(clusterName string) resource.TestStep {
	const (
		resourceName   = "cockroach_cluster.serverless"
		dataSourceName = "data.cockroach_cluster.test"
	)
	return resource.TestStep{
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "serverless" {
				name           = "%s"
				cloud_provider = "GCP"
				serverless = {
					usage_limits = {
						request_unit_limit = 10000000000
						storage_mib_limit = 102400
					}
				}
				regions = [
					{
						name = "europe-west1"
						primary = true
					},
					{
						name = "us-east1"
					},
					{
						name = "us-west2"
					},
				]
			}

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.serverless.id
			}
			`, clusterName),
		Check: resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr(resourceName, "name", clusterName),
			resource.TestCheckResourceAttrSet(resourceName, "cloud_provider"),
			resource.TestCheckResourceAttrSet(resourceName, "cockroach_version"),
			resource.TestCheckResourceAttr(resourceName, "plan", "SERVERLESS"),
			resource.TestCheckResourceAttr(resourceName, "state", string(client.CLUSTERSTATETYPE_CREATED)),
			resource.TestCheckResourceAttr(resourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(resourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(resourceName, "regions.0.primary", "true"),
			resource.TestCheckResourceAttr(resourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(resourceName, "regions.1.primary", "false"),
			resource.TestCheckResourceAttr(resourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(resourceName, "regions.2.primary", "false"),
			resource.TestCheckNoResourceAttr(resourceName, "serverless.spend_limit"),
			resource.TestCheckResourceAttr(resourceName, "serverless.usage_limits.request_unit_limit", "10000000000"),
			resource.TestCheckResourceAttr(resourceName, "serverless.usage_limits.storage_mib_limit", "102400"),
			resource.TestCheckResourceAttr(dataSourceName, "name", clusterName),
			resource.TestCheckResourceAttrSet(dataSourceName, "cloud_provider"),
			resource.TestCheckResourceAttrSet(dataSourceName, "cockroach_version"),
			resource.TestCheckResourceAttr(dataSourceName, "plan", "SERVERLESS"),
			resource.TestCheckResourceAttr(dataSourceName, "state", string(client.CLUSTERSTATETYPE_CREATED)),
			resource.TestCheckResourceAttr(dataSourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(dataSourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(dataSourceName, "regions.0.primary", "true"),
			resource.TestCheckResourceAttr(dataSourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(dataSourceName, "regions.1.primary", "false"),
			resource.TestCheckResourceAttr(dataSourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(dataSourceName, "regions.2.primary", "false"),
			resource.TestCheckNoResourceAttr(dataSourceName, "serverless.spend_limit"),
			resource.TestCheckResourceAttr(dataSourceName, "serverless.usage_limits.request_unit_limit", "10000000000"),
			resource.TestCheckResourceAttr(dataSourceName, "serverless.usage_limits.storage_mib_limit", "102400"),
		),
	}
}

func TestAccDedicatedClusterResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-dedicated-%s", tfTestPrefix, GenerateRandomString(3))
	testDedicatedClusterResource(t, clusterName, false)
}

func TestIntegrationDedicatedClusterResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-dedicated-%s", tfTestPrefix, GenerateRandomString(3))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	initialCluster := client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: minSupportedClusterPatchVersion,
		Plan:             client.PLANTYPE_DEDICATED,
		CloudProvider:    client.CLOUDPROVIDERTYPE_GCP,
		State:            client.CLUSTERSTATETYPE_CREATED,
		UpgradeStatus:    client.CLUSTERUPGRADESTATUSTYPE_UPGRADE_AVAILABLE,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				NumVirtualCpus: 2,
				StorageGib:     15,
				MemoryGib:      8,
			},
		},
		Regions: []client.Region{
			{
				Name:      "us-central1",
				NodeCount: 1,
			},
		},
	}

	upgradingCluster := initialCluster
	upgradingCluster.CockroachVersion = latestClusterPatchVersion
	upgradingCluster.UpgradeStatus = client.CLUSTERUPGRADESTATUSTYPE_MAJOR_UPGRADE_RUNNING

	pendingCluster := upgradingCluster
	pendingCluster.UpgradeStatus = client.CLUSTERUPGRADESTATUSTYPE_PENDING_FINALIZATION

	finalizedCluster := pendingCluster
	finalizedCluster.UpgradeStatus = client.CLUSTERUPGRADESTATUSTYPE_FINALIZED

	scaledCluster := finalizedCluster
	scaledCluster.Config.Dedicated = &client.DedicatedHardwareConfig{}
	*scaledCluster.Config.Dedicated = *finalizedCluster.Config.Dedicated
	scaledCluster.Config.Dedicated.NumVirtualCpus = 4

	httpOk := &http.Response{Status: http.StatusText(http.StatusOK)}

	// Creation

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&initialCluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&initialCluster, httpOk, nil).Times(7)

	// Upgrade

	s.EXPECT().ListMajorClusterVersions(gomock.Any(), gomock.Any()).Return(&client.ListMajorClusterVersionsResponse{
		Versions: []client.ClusterMajorVersion{
			{
				Version: minSupportedClusterMajorVersion,
			},
			{
				Version: latestClusterMajorVersion,
			},
		},
	}, nil, nil)
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, &client.UpdateClusterSpecification{UpgradeStatus: &upgradingCluster.UpgradeStatus}).
		DoAndReturn(
			func(context.Context, string, *client.UpdateClusterSpecification,
			) (*client.Cluster, *http.Response, error) {
				return &upgradingCluster, httpOk, nil
			},
		)

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&upgradingCluster, httpOk, nil).Times(1)

	// Scale (no-op)

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).
		DoAndReturn(func(context.Context, string, *client.UpdateClusterSpecification,
		) (*client.Cluster, *http.Response, error) {
			return &pendingCluster, httpOk, nil
		})

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&pendingCluster, httpOk, nil).Times(2)

	// Finalize

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&pendingCluster, httpOk, nil).Times(1)

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).
		DoAndReturn(func(context.Context, string, *client.UpdateClusterSpecification,
		) (*client.Cluster, *http.Response, error) {
			return &finalizedCluster, httpOk, nil
		})

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&finalizedCluster, httpOk, nil).Times(3)

	// Import state happens here

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&finalizedCluster, httpOk, nil).Times(3)

	// Scale

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).
		DoAndReturn(func(context.Context, string, *client.UpdateClusterSpecification,
		) (*client.Cluster, *http.Response, error) {
			currentCluster := &scaledCluster
			return currentCluster, httpOk, nil
		})

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&scaledCluster, httpOk, nil).Times(5)

	// Deletion

	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	scaleStep := resource.TestStep{
		Config: getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 4),
		Check:  resource.TestCheckResourceAttr("cockroach_cluster.dedicated", "dedicated.num_virtual_cpus", "4"),
	}

	testDedicatedClusterResource(t, clusterName, true, scaleStep)
}

func testDedicatedClusterResource(
	t *testing.T, clusterName string, useMock bool, additionalSteps ...resource.TestStep,
) {
	const (
		resourceName   = "cockroach_cluster.dedicated"
		dataSourceName = "data.cockroach_cluster.test"
	)

	testSteps := []resource.TestStep{
		{
			Config: getTestDedicatedClusterResourceConfig(clusterName, minSupportedClusterMajorVersion, false, 2),
			Check: resource.ComposeTestCheckFunc(
				testCheckCockroachClusterExists(resourceName),
				resource.TestCheckResourceAttr(resourceName, "name", clusterName),
				resource.TestCheckResourceAttrSet(resourceName, "cloud_provider"),
				resource.TestCheckResourceAttrSet(resourceName, "cockroach_version"),
				resource.TestCheckResourceAttr(resourceName, "plan", "DEDICATED"),
				resource.TestCheckResourceAttr(dataSourceName, "name", clusterName),
				resource.TestCheckResourceAttrSet(dataSourceName, "cloud_provider"),
				resource.TestCheckResourceAttrSet(dataSourceName, "cockroach_version"),
				resource.TestCheckResourceAttr(dataSourceName, "plan", "DEDICATED"),
			),
		},
		{
			Config: getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, true, 2),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(resourceName, "cockroach_version", latestClusterMajorVersion),
				resource.TestCheckResourceAttr(dataSourceName, "cockroach_version", latestClusterMajorVersion),
			),
		},
		{
			ResourceName: resourceName,
			ImportState:  true,
			// ImportStateVerify used to work with sdkv2 but after the update to
			// terraform-plugin-testing, it no longer works.
			// There is an error:
			//     map[string]string{
			//	 -   "upgrade_status": "PENDING_FINALIZATION",
			//	 +   "upgrade_status": "FINALIZED",
			//	   }
			// My hunch is that this is related to having a separate resource
			// for finalize_version_upgrade which keeps its own state. I've
			// traced through the code and I see the state for the cluster being
			// correctly updated to FINALIZED before the Import is called and it
			// remains so after so I'm unclear why this is failing.  For now its
			// turned off due to this reason.
			ImportStateVerify: false,
		},
	}
	testSteps = append(testSteps, additionalSteps...)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    testSteps,
	})
}

func testCheckCockroachClusterExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		p := testAccProvider.(*provider)
		p.service = NewService(cl)
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		id := rs.Primary.Attributes["id"]
		log.Printf("[DEBUG] projectID: %s, name %s", rs.Primary.Attributes["id"], rs.Primary.Attributes["name"])

		traceAPICall("GetCluster")
		if _, _, err := p.service.GetCluster(context.Background(), id); err == nil {
			return nil
		}

		return fmt.Errorf("cluster(%s:%s) does not exist", rs.Primary.Attributes["id"], rs.Primary.ID)
	}
}

func getTestDedicatedClusterResourceConfig(name, version string, finalize bool, vcpus int) string {
	config := fmt.Sprintf(`
resource "cockroach_cluster" "dedicated" {
    name           = "%s"
    cloud_provider = "GCP"
    cockroach_version = "%s"
    dedicated = {
	  storage_gib = 15
	  num_virtual_cpus = %d
    }
	regions = [{
		name: "us-central1"
		node_count: 1
	}]
}

data "cockroach_cluster" "test" {
    id = cockroach_cluster.dedicated.id
}
`, name, version, vcpus)

	if finalize {
		config += fmt.Sprintf(`
resource "cockroach_finalize_version_upgrade" "test" {
	id = cockroach_cluster.dedicated.id
	cockroach_version = "%s"
}
`, version)
	}

	return config
}

func TestSortRegionsByPlan(t *testing.T) {
	t.Run("Plan matches cluster", func(t *testing.T) {
		regions := []client.Region{
			{Name: "us-central1"},
			{Name: "us-east1"},
			{Name: "us-west2"},
		}
		plan := []Region{
			{Name: types.StringValue("us-west2")},
			{Name: types.StringValue("us-central1")},
			{Name: types.StringValue("us-east1")},
		}
		sortRegionsByPlan(&regions, plan)
		for i, region := range regions {
			require.Equal(t, plan[i].Name.ValueString(), region.Name)
		}
	})

	t.Run("More regions in cluster than plan", func(t *testing.T) {
		regions := []client.Region{
			{Name: "us-central1"},
			{Name: "us-east1"},
			{Name: "us-west2"},
		}
		plan := []Region{
			{Name: types.StringValue("us-west2")},
			{Name: types.StringValue("us-central1")},
		}
		// We really just want to make sure it doesn't panic here.
		sortRegionsByPlan(&regions, plan)
	})

	t.Run("More regions in plan than cluster", func(t *testing.T) {
		regions := []client.Region{
			{Name: "us-central1"},
			{Name: "us-east1"},
		}
		plan := []Region{
			{Name: types.StringValue("us-west2")},
			{Name: types.StringValue("us-central1")},
			{Name: types.StringValue("us-east1")},
		}
		// We really just want to make sure it doesn't panic here.
		sortRegionsByPlan(&regions, plan)
	})
}

func TestSimplifyClusterVersion(t *testing.T) {
	t.Run("Normal version", func(t *testing.T) {
		require.Equal(t, "v22.2", simplifyClusterVersion("v22.2.10", false))
	})
	t.Run("Normal version, plan uses preview", func(t *testing.T) {
		require.Equal(t, "v22.2", simplifyClusterVersion("v22.2.10", true))
	})
	t.Run("Preview version", func(t *testing.T) {
		require.Equal(t, "v23.1", simplifyClusterVersion("v23.1.0-beta1", false))
	})
	t.Run("Preview version, plan uses preview", func(t *testing.T) {
		require.Equal(t, "preview", simplifyClusterVersion("v23.1.0-beta1", true))
	})
}

// TestClusterSchemaInSync ensures that if an attribute gets added to the cluster resource,
// it also gets added to the datasource, and vice versa. The attribute properties can be different,
// but the schemas should otherwise be the same.
func TestClusterSchemaInSync(t *testing.T) {
	r := NewClusterResource()
	d := NewClusterDataSource()
	var rSchema framework_resource.SchemaResponse
	var dSchema datasource.SchemaResponse
	r.Schema(context.Background(), framework_resource.SchemaRequest{}, &rSchema)
	d.Schema(context.Background(), datasource.SchemaRequest{}, &dSchema)

	rAttrs := rSchema.Schema.Attributes
	dAttrs := dSchema.Schema.Attributes
	CheckSchemaAttributesMatch(t, rAttrs, dAttrs)
}
