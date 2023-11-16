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
	"strconv"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	framework_resource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
)

const (
	// The patch versions are just for mocks. They don't need to be the actual
	// latest available patch versions; they just need to resolve to the correct
	// major versions.
	minSupportedClusterMajorVersion = "v22.2"
	minSupportedClusterPatchVersion = "v22.2.0"
	latestClusterMajorVersion       = "v23.1"
	latestClusterPatchVersion       = "v23.1.0"

	sharedResourceName   = "cockroach_cluster.test"
	sharedDataSourceName = "data.cockroach_cluster.test"
)

// TestAccSharedClusterResource attempts to create, check, update, and destroy
// a real cluster. It will be skipped if TF_ACC isn't set.
func TestAccSharedClusterResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-shared-%s", GenerateRandomString(2))
	resource.Test(t, resource.TestCase{
		IsUnitTest:               false,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			onDemandSingleRegionClusterWithLimits(clusterName, "BASIC", 10000000000, 102400),
			onDemandSingleRegionClusterWithLimits(clusterName, "BASIC", 1000000, 1024),
			onDemandSingleRegionClusterNoLimits(clusterName, "BASIC"),
			onDemandSingleRegionClusterWithLimits(clusterName, "BASIC", 10000000000, 102400),
			onDemandSingleRegionClusterWithUnlimited(clusterName, "BASIC"),
			onDemandSingleRegionClusterNoLimits(clusterName, "BASIC"),
		},
	})
}

// TestAccMultiRegionSharedClusterResource attempts to create, update, check,
// and destroy a real multi-region shared cluster. It will be skipped if TF_ACC
// isn't set.
func TestAccMultiRegionSharedClusterResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-multi-region-shared-%s", GenerateRandomString(2))
	resource.Test(t, resource.TestCase{
		IsUnitTest:               false,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			provisionedMultiRegionClusterWithLimit(clusterName, "STANDARD"),
			provisionedMultiRegionClusterUpdated(clusterName, "STANDARD"),
		},
	})
}

// TestIntegrationSharedClusterResource attempts to create, check, and destroy a
// cluster, but uses a mocked API service.
func TestIntegrationSharedClusterResource(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	boolPtr := func(val bool) *bool { return &val }
	int64Ptr := func(val int64) *int64 { return &val }

	initialCluster := client.Cluster{
		Id:               uuid.Nil.String(),
		Name:             fmt.Sprintf("tftest-shared-%s", GenerateRandomString(2)),
		CockroachVersion: latestClusterPatchVersion,
		CloudProvider:    "GCP",
		State:            "CREATED",
		Config: client.ClusterConfig{
			Shared: &client.SharedClusterConfig{
				UsageLimits: &client.UsageLimits{
					RequestUnitLimit: int64Ptr(1_000_000),
					StorageMibLimit:  int64Ptr(1024),
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
		name         string
		testStep     func(clusterName string, planType client.PlanType) resource.TestStep
		finalCluster client.Cluster
	}{
		{
			"single-region shared BASIC cluster with updated resource limits",
			func(clusterName string, planType client.PlanType) resource.TestStep {
				return onDemandSingleRegionClusterWithLimits(
					clusterName, "BASIC", 10000000000, 102400)
			},
			client.Cluster{
				Id:               uuid.Nil.String(),
				Name:             initialCluster.Name,
				CockroachVersion: latestClusterPatchVersion,
				Plan:             "BASIC",
				CloudProvider:    "GCP",
				State:            "CREATED",
				Config: client.ClusterConfig{
					Shared: &client.SharedClusterConfig{
						UsageLimits: &client.UsageLimits{
							RequestUnitLimit: int64Ptr(10_000_000_000),
							StorageMibLimit:  int64Ptr(102_400),
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
		},
		{
			"single-region shared BASIC cluster with unlimited resources",
			onDemandSingleRegionClusterWithUnlimited,
			client.Cluster{
				Id:               uuid.Nil.String(),
				Name:             initialCluster.Name,
				CockroachVersion: latestClusterPatchVersion,
				Plan:             "BASIC",
				CloudProvider:    "GCP",
				State:            "CREATED",
				Config: client.ClusterConfig{
					Shared: &client.SharedClusterConfig{
						RoutingId:   "routing-id",
						UsageLimits: &client.UsageLimits{},
					},
				},
				Regions: []client.Region{
					{
						Name: "us-central1",
					},
				},
			},
		},
		{
			"single-region shared BASIC cluster with no limits specified",
			onDemandSingleRegionClusterNoLimits,
			client.Cluster{
				Id:               uuid.Nil.String(),
				Name:             initialCluster.Name,
				CockroachVersion: latestClusterPatchVersion,
				Plan:             "BASIC",
				CloudProvider:    "GCP",
				State:            "CREATED",
				Config: client.ClusterConfig{
					Shared: &client.SharedClusterConfig{
						RoutingId: "routing-id",
					},
				},
				Regions: []client.Region{
					{
						Name: "us-central1",
					},
				},
			},
		},
		{
			"multi-region shared STANDARD cluster with provisioned limit",
			provisionedMultiRegionClusterWithLimit,
			client.Cluster{
				Id:               uuid.Nil.String(),
				Name:             initialCluster.Name,
				CockroachVersion: latestClusterPatchVersion,
				Plan:             "STANDARD",
				CloudProvider:    "GCP",
				State:            "CREATED",
				Config: client.ClusterConfig{
					Shared: &client.SharedClusterConfig{
						UsageLimits: &client.UsageLimits{
							RequestUnitRateLimit: int64Ptr(3000),
						},
						RoutingId: "routing-id",
					},
				},
				Regions: []client.Region{
					{
						Name: "us-west2",
					},
					{
						Name:    "us-east1",
						Primary: boolPtr(true),
					},
					{
						Name: "europe-west1",
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

			initialCluster.Plan = c.finalCluster.Plan

			steps := []resource.TestStep{
				// Create initial resource.
				onDemandSingleRegionClusterWithLimits(
					initialCluster.Name, initialCluster.Plan, 1000000, 1024),
				// Apply an update.
				c.testStep(initialCluster.Name, c.finalCluster.Plan),
				// Import the resource.
				{
					ResourceName:      "cockroach_cluster.test",
					ImportState:       true,
					ImportStateVerify: true,
				},
			}

			// Use DoAndReturn so that it's easy to set break points.
			s.EXPECT().
				CreateCluster(gomock.Any(), gomock.Any()).
				DoAndReturn(func(context.Context, *client.CreateClusterRequest) (*client.Cluster, *http.Response, error) {
					return &initialCluster, nil, nil
				})

			// GetCluster calls during cluster creation.
			s.EXPECT().
				GetCluster(gomock.Any(), c.finalCluster.Id).
				DoAndReturn(func(context.Context, string) (*client.Cluster, *http.Response, error) {
					return &initialCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil
				}).Times(7)

			s.EXPECT().
				UpdateCluster(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(context.Context, string, *client.UpdateClusterSpecification) (*client.Cluster, *http.Response, error) {
					return &c.finalCluster, nil, nil
				})

			// GetCluster calls during cluster update.
			s.EXPECT().GetCluster(gomock.Any(), c.finalCluster.Id).DoAndReturn(func(context.Context, string) (*client.Cluster, *http.Response, error) {
				return &c.finalCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil
			}).Times(6)

			// GetCluster calls during cluster import.
			s.EXPECT().
				GetCluster(gomock.Any(), c.finalCluster.Id).
				DoAndReturn(func(context.Context, string) (*client.Cluster, *http.Response, error) {
					return &c.finalCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil
				}).MinTimes(2).MaxTimes(3)

			s.EXPECT().
				DeleteCluster(gomock.Any(), c.finalCluster.Id).
				DoAndReturn(func(context.Context, string) (*client.Cluster, *http.Response, error) {
					return &c.finalCluster, nil, nil
				})

			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    steps,
			})
		})
	}
}

func onDemandSingleRegionClusterNoLimits(
	clusterName string,
	planType client.PlanType,
) resource.TestStep {
	return resource.TestStep{
		// Basic cluster with no resource limits specified, which translates to
		// unlimited on-demand resources.
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				shared = {}
				regions = [{
					name = "us-central1"
				}]
			}

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.test.id
			}
			`, clusterName),
		Check: resource.ComposeTestCheckFunc(
			testCheckCockroachClusterExists(sharedResourceName),
			makeDefaultSharedResourceChecks(clusterName, planType),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.#", "1"),
			resource.TestCheckResourceAttr(sharedResourceName, "shared.#", "0"),
			resource.TestCheckNoResourceAttr(sharedResourceName, "shared.usage_limits"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.#", "1"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "shared.#", "0"),
			resource.TestCheckNoResourceAttr(sharedDataSourceName, "shared.usage_limits"),
		),
	}
}

func onDemandSingleRegionClusterWithLimits(
	clusterName string,
	planType client.PlanType,
	requestUnitLimit int64,
	storageMibLimit int64,
) resource.TestStep {
	return resource.TestStep{
		// Basic cluster with resource limits.
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				shared = {
					usage_limits = {
						request_unit_limit = %d
						storage_mib_limit = %d
					}
				}
				regions = [{
					name = "us-central1"
				}]
			}

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.test.id
			}
			`, clusterName, requestUnitLimit, storageMibLimit),
		Check: resource.ComposeTestCheckFunc(
			makeDefaultSharedResourceChecks(clusterName, planType),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.#", "1"),
			resource.TestCheckResourceAttr(sharedResourceName, "shared.usage_limits.request_unit_limit", strconv.Itoa(int(requestUnitLimit))),
			resource.TestCheckResourceAttr(sharedResourceName, "shared.usage_limits.storage_mib_limit", strconv.Itoa(int(storageMibLimit))),
			resource.TestCheckNoResourceAttr(sharedResourceName, "shared.usage_limits.request_unit_rate_limit"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.#", "1"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "shared.usage_limits.request_unit_limit", strconv.Itoa(int(requestUnitLimit))),
			resource.TestCheckResourceAttr(sharedDataSourceName, "shared.usage_limits.storage_mib_limit", strconv.Itoa(int(storageMibLimit))),
			resource.TestCheckNoResourceAttr(sharedDataSourceName, "shared.usage_limits.request_unit_rate_limit"),
		),
	}
}

func onDemandSingleRegionClusterWithUnlimited(
	clusterName string,
	planType client.PlanType,
) resource.TestStep {
	return resource.TestStep{
		// Basic cluster with unlimited on-demand resources.
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				shared = {
					usage_limits = {}
				}
				regions = [{
					name = "us-central1"
				}]
			}

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.test.id
			}
			`, clusterName),
		Check: resource.ComposeTestCheckFunc(
			makeDefaultSharedResourceChecks(clusterName, planType),
			resource.TestCheckResourceAttr(sharedResourceName, "shared.usage_limits.#", "0"),
			resource.TestCheckNoResourceAttr(sharedResourceName, "shared.usage_limits.request_unit_rate_limit"),
			resource.TestCheckNoResourceAttr(sharedResourceName, "shared.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(sharedResourceName, "shared.usage_limits.storage_mib_limit"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "shared.usage_limits.#", "0"),
			resource.TestCheckNoResourceAttr(sharedDataSourceName, "shared.usage_limits.request_unit_rate_limit"),
			resource.TestCheckNoResourceAttr(sharedDataSourceName, "shared.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(sharedDataSourceName, "shared.usage_limits.storage_mib_limit"),
		),
	}
}

func provisionedMultiRegionClusterWithLimit(
	clusterName string,
	planType client.PlanType,
) resource.TestStep {
	return resource.TestStep{
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				shared = {
					usage_limits = {
						request_unit_rate_limit = 3000
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
				id = cockroach_cluster.test.id
			}
			`, clusterName),
		Check: resource.ComposeTestCheckFunc(
			makeDefaultSharedResourceChecks(clusterName, planType),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.0.primary", "false"),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.1.primary", "true"),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.2.primary", "false"),
			resource.TestCheckResourceAttr(sharedResourceName, "shared.usage_limits.request_unit_rate_limit", "3000"),
			resource.TestCheckNoResourceAttr(sharedResourceName, "shared.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(sharedResourceName, "shared.usage_limits.storage_mib_limit"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.0.primary", "false"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.1.primary", "true"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.2.primary", "false"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "shared.usage_limits.request_unit_rate_limit", "3000"),
			resource.TestCheckNoResourceAttr(sharedDataSourceName, "shared.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(sharedDataSourceName, "shared.usage_limits.storage_mib_limit"),
		),
	}
}

// provisionedMultiRegionClusterUpdated updates some of the fields in
// provisionedMultiRegionClusterWithLimit.
func provisionedMultiRegionClusterUpdated(
	clusterName string,
	planType client.PlanType,
) resource.TestStep {
	return resource.TestStep{
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				shared = {
					usage_limits = {
						request_unit_rate_limit = 4000
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
				id = cockroach_cluster.test.id
			}
			`, clusterName),
		Check: resource.ComposeTestCheckFunc(
			makeDefaultSharedResourceChecks(clusterName, planType),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.0.primary", "true"),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.1.primary", "false"),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(sharedResourceName, "regions.2.primary", "false"),
			resource.TestCheckResourceAttr(sharedResourceName, "shared.usage_limits.request_unit_rate_limit", "4000"),
			resource.TestCheckNoResourceAttr(sharedResourceName, "shared.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(sharedResourceName, "shared.usage_limits.storage_mib_limit"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.0.primary", "true"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.1.primary", "false"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "regions.2.primary", "false"),
			resource.TestCheckResourceAttr(sharedDataSourceName, "shared.usage_limits.request_unit_rate_limit", "4000"),
			resource.TestCheckNoResourceAttr(sharedDataSourceName, "shared.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(sharedDataSourceName, "shared.usage_limits.storage_mib_limit"),
		),
	}
}

func makeDefaultSharedResourceChecks(clusterName string, planType client.PlanType) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttr(sharedResourceName, "name", clusterName),
		resource.TestCheckResourceAttrSet(sharedResourceName, "cloud_provider"),
		resource.TestCheckResourceAttrSet(sharedResourceName, "cockroach_version"),
		resource.TestCheckResourceAttr(sharedResourceName, "plan", string(planType)),
		resource.TestCheckResourceAttr(sharedResourceName, "state", string(client.CLUSTERSTATETYPE_CREATED)),
		resource.TestCheckResourceAttr(sharedDataSourceName, "name", clusterName),
		resource.TestCheckResourceAttrSet(sharedDataSourceName, "cloud_provider"),
		resource.TestCheckResourceAttrSet(sharedDataSourceName, "cockroach_version"),
		resource.TestCheckResourceAttr(sharedDataSourceName, "plan", string(planType)),
		resource.TestCheckResourceAttr(sharedDataSourceName, "state", string(client.CLUSTERSTATETYPE_CREATED)),
	)
}

func TestAccDedicatedClusterResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-dedicated-%s", GenerateRandomString(3))
	testDedicatedClusterResource(t, clusterName, false)
}

func TestIntegrationDedicatedClusterResource(t *testing.T) {
	clusterName := fmt.Sprintf("tftest-dedicated-%s", GenerateRandomString(3))
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
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: minSupportedClusterPatchVersion,
		Plan:             client.PLANTYPE_ADVANCED,
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

	upgradingCluster := cluster
	upgradingCluster.CockroachVersion = latestClusterPatchVersion
	upgradingCluster.UpgradeStatus = client.CLUSTERUPGRADESTATUSTYPE_MAJOR_UPGRADE_RUNNING

	pendingCluster := upgradingCluster
	pendingCluster.UpgradeStatus = client.CLUSTERUPGRADESTATUSTYPE_PENDING_FINALIZATION

	finalizedCluster := upgradingCluster
	finalizedCluster.UpgradeStatus = client.CLUSTERUPGRADESTATUSTYPE_FINALIZED

	scaledCluster := finalizedCluster
	scaledCluster.Config.Dedicated = &client.DedicatedHardwareConfig{}
	*scaledCluster.Config.Dedicated = *finalizedCluster.Config.Dedicated
	scaledCluster.Config.Dedicated.NumVirtualCpus = 4

	httpOk := &http.Response{Status: http.StatusText(http.StatusOK)}

	// Creation

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, httpOk, nil).AnyTimes()

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
				cluster = upgradingCluster
				return &cluster, httpOk, nil
			},
		)

	// Scale (no-op)

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).
		DoAndReturn(func(context.Context, string, *client.UpdateClusterSpecification,
		) (*client.Cluster, *http.Response, error) {
			cluster = pendingCluster
			return &cluster, httpOk, nil
		})

	// Finalize

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, &client.UpdateClusterSpecification{UpgradeStatus: &finalizedCluster.UpgradeStatus}).
		DoAndReturn(func(context.Context, string, *client.UpdateClusterSpecification,
		) (*client.Cluster, *http.Response, error) {
			cluster = finalizedCluster
			return &cluster, httpOk, nil
		})

	// Scale

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).
		DoAndReturn(func(context.Context, string, *client.UpdateClusterSpecification,
		) (*client.Cluster, *http.Response, error) {
			cluster = scaledCluster
			return &cluster, httpOk, nil
		})
	// Deletion

	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	scaleStep := resource.TestStep{
		Config: getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 4),
		Check:  resource.TestCheckResourceAttr("cockroach_cluster.dedicated", "dedicated.num_virtual_cpus", "4"),
	}

	testDedicatedClusterResource(t, clusterName, true, scaleStep)
}

func testDedicatedClusterResource(t *testing.T, clusterName string, useMock bool, additionalSteps ...resource.TestStep) {
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
				resource.TestCheckResourceAttr(resourceName, "plan", "ADVANCED"),
				resource.TestCheckResourceAttr(dataSourceName, "name", clusterName),
				resource.TestCheckResourceAttrSet(dataSourceName, "cloud_provider"),
				resource.TestCheckResourceAttrSet(dataSourceName, "cockroach_version"),
				resource.TestCheckResourceAttr(dataSourceName, "plan", "ADVANCED"),
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
			ResourceName:      resourceName,
			ImportState:       true,
			ImportStateVerify: true,
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
