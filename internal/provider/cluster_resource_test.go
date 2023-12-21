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
	"regexp"
	"strconv"
	"strings"
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

	serverlessResourceName   = "cockroach_cluster.test"
	serverlessDataSourceName = "data.cockroach_cluster.test"
)

// TestAccServerlessClusterResource attempts to create, check, update, and
// destroy a real cluster. It will be skipped if TF_ACC isn't set.
func TestAccServerlessClusterResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-serverless-%s", GenerateRandomString(2))
	resource.Test(t, resource.TestCase{
		IsUnitTest:               false,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			onDemandSingleRegionClusterWithLimits(clusterName, "BASIC", 10_000_000_000, 102_400),
			onDemandSingleRegionClusterWithLimits(clusterName, "BASIC", 1_000_000, 1024),
			onDemandSingleRegionClusterNoLimits(clusterName, "BASIC"),
			onDemandSingleRegionClusterWithLimits(clusterName, "BASIC", 10_000_000_000, 102_400),
			onDemandSingleRegionClusterWithUnlimited(clusterName, "BASIC"),
			onDemandSingleRegionClusterNoLimits(clusterName, "BASIC"),
			legacyServerlessClusterWithSpendLimit(clusterName, 10_00),
			onDemandSingleRegionClusterWithUnlimited(clusterName, "BASIC"),
		},
	})
}

// TestAccMultiRegionServerlessClusterResource attempts to create, update, check,
// and destroy a real multi-region serverless cluster. It will be skipped if
// TF_ACC isn't set.
func TestAccMultiRegionServerlessClusterResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-multi-region-serverless-%s", GenerateRandomString(2))
	resource.Test(t, resource.TestCase{
		IsUnitTest:               false,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			provisionedMultiRegionClusterWithLimit(clusterName),
			provisionedMultiRegionClusterUpdated(clusterName),
		},
	})
}

// TestIntegrationServerlessClusterResource attempts to create, check, and destroy a
// cluster, but uses a mocked API service.
func TestIntegrationServerlessClusterResource(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	int64Ptr := func(val int64) *int64 { return &val }
	clusterName := fmt.Sprintf("tftest-serverless-%s", GenerateRandomString(2))

	singleRegionClusterWithUnlimited := func(planType client.PlanType) client.Cluster {
		return client.Cluster{
			Id:               uuid.Nil.String(),
			Name:             clusterName,
			CockroachVersion: latestClusterPatchVersion,
			CloudProvider:    "GCP",
			State:            "CREATED",
			Plan:             planType,
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
		}
	}

	singleRegionClusterWithLimits := func(
		planType client.PlanType,
		ruLimit int64,
		storageLimit int64,
	) client.Cluster {
		return client.Cluster{
			Id:               uuid.Nil.String(),
			Name:             clusterName,
			CockroachVersion: latestClusterPatchVersion,
			CloudProvider:    "GCP",
			State:            "CREATED",
			Plan:             planType,
			Config: client.ClusterConfig{
				Serverless: &client.ServerlessClusterConfig{
					UsageLimits: &client.UsageLimits{
						RequestUnitLimit: int64Ptr(ruLimit),
						StorageMibLimit:  int64Ptr(storageLimit),
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
	}

	provisionedMultiRegionCluster := func(provisionedCapacity int64, primaryIndex int) client.Cluster {
		cluster := client.Cluster{
			Id:               uuid.Nil.String(),
			Name:             clusterName,
			CockroachVersion: latestClusterPatchVersion,
			CloudProvider:    "GCP",
			State:            "CREATED",
			Plan:             "STANDARD",
			Config: client.ClusterConfig{
				Serverless: &client.ServerlessClusterConfig{
					UsageLimits: &client.UsageLimits{
						ProvisionedCapacity: int64Ptr(provisionedCapacity),
					},
					RoutingId: "routing-id",
				},
			},
			Regions: []client.Region{
				{
					Name: "europe-west1",
				},
				{
					Name: "us-east1",
				},
				{
					Name: "us-west2",
				},
			},
		}
		trueVal := true
		cluster.Regions[primaryIndex].Primary = &trueVal
		return cluster
	}

	cases := []struct {
		name              string
		createStep        func() resource.TestStep
		initialCluster    client.Cluster
		updateStep        func() resource.TestStep
		finalCluster      client.Cluster
		ignoreImportPaths []string
	}{
		{
			name: "single-region serverless BASIC cluster converted to unlimited resources",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimits(clusterName, "BASIC", 1_000_000, 1024)
			},
			initialCluster: singleRegionClusterWithLimits("BASIC", 1_000_000, 1024),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithUnlimited(clusterName, "BASIC")
			},
			finalCluster: singleRegionClusterWithUnlimited("BASIC"),
			// When testing import, skip validating the usage limits, because the
			// server returns usage_limits = null for "unlimited", whereas the
			// TF state contains usage_limits = {}. This is a spurious failure,
			// as the two formulations are equivalent.
			ignoreImportPaths: []string{"serverless.usage_limits.%"},
		},
		{
			name: "single-region serverless BASIC cluster converted to no limit resources",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimits(clusterName, "BASIC", 1_000_000, 1024)
			},
			initialCluster: singleRegionClusterWithLimits("BASIC", 1_000_000, 1024),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterNoLimits(clusterName, "BASIC")
			},
			finalCluster: singleRegionClusterWithUnlimited("BASIC"),
		},
		{
			name: "single-region serverless BASIC cluster converted from unlimited resources",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithUnlimited(clusterName, "BASIC")
			},
			initialCluster: singleRegionClusterWithUnlimited("BASIC"),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimits(clusterName, "BASIC", 1_000_000, 1024)
			},
			finalCluster: singleRegionClusterWithLimits("BASIC", 1_000_000, 1024),
		},
		{
			name: "single-region serverless BASIC cluster converted from no limit resources",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterNoLimits(clusterName, "BASIC")
			},
			initialCluster: singleRegionClusterWithUnlimited("BASIC"),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimits(clusterName, "BASIC", 1_000_000, 1024)
			},
			finalCluster: singleRegionClusterWithLimits("BASIC", 1_000_000, 1024),
		},
		{
			name: "single-region serverless BASIC cluster with updated resource limits",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimits(clusterName, "BASIC", 1_000_000, 1024)
			},
			initialCluster: singleRegionClusterWithLimits("BASIC", 1_000_000, 1024),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimits(clusterName, "BASIC", 10_000_000_000, 102_400)
			},
			finalCluster: singleRegionClusterWithLimits("BASIC", 10_000_000_000, 102_400),
		},
		{
			name: "multi-region serverless STANDARD cluster with provisioned limit",
			createStep: func() resource.TestStep {
				return provisionedMultiRegionClusterWithLimit(clusterName)
			},
			initialCluster: provisionedMultiRegionCluster(3000, 1),
			updateStep: func() resource.TestStep {
				return provisionedMultiRegionClusterUpdated(clusterName)
			},
			finalCluster: provisionedMultiRegionCluster(4000, 0),
		},
		{
			name: "legacy serverless cluster from spend limit to higher spend limit",
			createStep: func() resource.TestStep {
				return legacyServerlessClusterWithSpendLimit(clusterName, 10_00)
			},
			initialCluster: singleRegionClusterWithLimits("BASIC", 40_000_000, 4096),
			updateStep: func() resource.TestStep {
				return legacyServerlessClusterWithSpendLimit(clusterName, 20_00)
			},
			finalCluster: singleRegionClusterWithLimits("BASIC", 80_000_000, 8192),
			// When testing import, skip validating the configs, because the test
			// framework compares what's returned by the server (resource limits)
			// with what was previously in the TF state (spend limit).
			ignoreImportPaths: []string{
				"serverless.usage_limits.%",
				"serverless.usage_limits.request_unit_limit",
				"serverless.usage_limits.storage_mib_limit",
				"serverless.spend_limit",
			},
		},
		{
			name: "update legacy Serverless cluster with spend limit to use resource limits",
			createStep: func() resource.TestStep {
				return legacyServerlessClusterWithSpendLimit(clusterName, 10_00)
			},
			initialCluster: singleRegionClusterWithLimits("BASIC", 40_000_000, 4096),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimits(clusterName, "BASIC", 80_000_000, 8192)
			},
			finalCluster: singleRegionClusterWithLimits("BASIC", 80_000_000, 8192),
		},
		{
			name: "clear spend limit in legacy Serverless cluster",
			createStep: func() resource.TestStep {
				return legacyServerlessClusterWithSpendLimit(clusterName, 10_00)
			},
			initialCluster: singleRegionClusterWithLimits("BASIC", 40_000_000, 4096),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterNoLimits(clusterName, "BASIC")
			},
			finalCluster: singleRegionClusterWithUnlimited("BASIC"),
		},
		{
			name: "attempt to update name",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterNoLimits(clusterName, "BASIC")
			},
			initialCluster: singleRegionClusterWithUnlimited("BASIC"),
			updateStep: func() resource.TestStep {
				step := onDemandSingleRegionClusterNoLimits("new-name", "BASIC")
				step.ExpectError = regexp.MustCompile("Cannot update cluster name")
				return step
			},
		},
		{
			name: "attempt to update cloud provider",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterNoLimits(clusterName, "BASIC")
			},
			initialCluster: singleRegionClusterWithUnlimited("BASIC"),
			updateStep: func() resource.TestStep {
				step := onDemandSingleRegionClusterNoLimits(clusterName, "BASIC")
				step.Config = strings.Replace(step.Config, "GCP", "AWS", -1)
				step.ExpectError = regexp.MustCompile("Cannot update cluster cloud provider")
				return step
			},
		},
		{
			name: "attempt to update plan type",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterNoLimits(clusterName, "BASIC")
			},
			initialCluster: singleRegionClusterWithUnlimited("BASIC"),
			updateStep: func() resource.TestStep {
				return resource.TestStep{
					Config:      getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 4),
					ExpectError: regexp.MustCompile("Cannot update cluster plan type"),
				}
			},
		},
		{
			name: "request unit limit and request unit rate limit both specified",
			createStep: func() resource.TestStep {
				return resource.TestStep{
					Config: `
						resource "cockroach_cluster" "test" {
							name = "foo"
							cloud_provider = "GCP"
							serverless = {
								usage_limits = {
									request_unit_limit = 1000000
									provisioned_capacity = 1000
								}
							}
							regions = [{
								name = "us-central1"
							}]
						}`,
					ExpectError: regexp.MustCompile("Invalid Attribute Combination"),
				}
			},
		},
		{
			name: "storage limit and request unit rate limit both specified",
			createStep: func() resource.TestStep {
				return resource.TestStep{
					Config: `
						resource "cockroach_cluster" "test" {
							name = "foo"
							cloud_provider = "GCP"
							serverless = {
								usage_limits = {
									storage_mib_limit = 1024
									provisioned_capacity = 1000
								}
							}
							regions = [{
								name = "us-central1"
							}]
						}`,
					ExpectError: regexp.MustCompile("Invalid Attribute Combination"),
				}
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

			// Return the initial cluster until Update is called, after which point
			// return the final cluster.
			var updateCalled bool
			s.EXPECT().
				GetCluster(gomock.Any(), c.initialCluster.Id).
				DoAndReturn(func(context.Context, string) (*client.Cluster, *http.Response, error) {
					cluster := &c.initialCluster
					if updateCalled {
						cluster = &c.finalCluster
					}
					return cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil
				}).AnyTimes()

			var steps []resource.TestStep
			createStep := c.createStep()
			steps = append(steps, createStep)

			if createStep.ExpectError == nil {
				// Use DoAndReturn so that it's easy to set break points.
				s.EXPECT().
					CreateCluster(gomock.Any(), gomock.Any()).
					DoAndReturn(func(context.Context, *client.CreateClusterRequest) (*client.Cluster, *http.Response, error) {
						return &c.initialCluster, nil, nil
					})
				s.EXPECT().
					DeleteCluster(gomock.Any(), c.initialCluster.Id).
					DoAndReturn(func(context.Context, string) (*client.Cluster, *http.Response, error) {
						return &c.finalCluster, nil, nil
					})

				updateStep := c.updateStep()
				steps = append(steps, updateStep)

				if updateStep.ExpectError == nil {
					s.EXPECT().
						UpdateCluster(gomock.Any(), gomock.Any(), gomock.Any()).
						DoAndReturn(func(context.Context, string, *client.UpdateClusterSpecification) (*client.Cluster, *http.Response, error) {
							updateCalled = true
							return &c.finalCluster, nil, nil
						})

					// Test import and refresh.
					steps = append(steps, resource.TestStep{
						ResourceName:            "cockroach_cluster.test",
						ImportState:             true,
						ImportStateVerify:       true,
						ImportStateVerifyIgnore: c.ignoreImportPaths,
					}, resource.TestStep{
						RefreshState: true,
					})
				}
			}

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
		// Serverless cluster with no resource limits specified, which translates
		// to unlimited on-demand resources.
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				serverless = {}
				regions = [{
					name = "us-central1"
				}]
			}

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.test.id
			}
			`, clusterName),
		Check: resource.ComposeTestCheckFunc(
			testCheckCockroachClusterExists(serverlessResourceName),
			makeDefaultServerlessResourceChecks(clusterName, planType),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.#", "1"),
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.#", "0"),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.#", "1"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.#", "0"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits"),
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
		// Serverless cluster with resource limits.
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				serverless = {
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
			makeDefaultServerlessResourceChecks(clusterName, planType),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.#", "1"),
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.usage_limits.request_unit_limit", strconv.Itoa(int(requestUnitLimit))),
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.usage_limits.storage_mib_limit", strconv.Itoa(int(storageMibLimit))),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.provisioned_capacity"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.#", "1"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.usage_limits.request_unit_limit", strconv.Itoa(int(requestUnitLimit))),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.usage_limits.storage_mib_limit", strconv.Itoa(int(storageMibLimit))),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.provisioned_capacity"),
		),
	}
}

func onDemandSingleRegionClusterWithUnlimited(
	clusterName string,
	planType client.PlanType,
) resource.TestStep {
	return resource.TestStep{
		// Serverless cluster with unlimited on-demand resources.
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				serverless = {
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
			makeDefaultServerlessResourceChecks(clusterName, planType),
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.usage_limits.#", "0"),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.provisioned_capacity"),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.storage_mib_limit"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.usage_limits.#", "0"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.provisioned_capacity"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.storage_mib_limit"),
		),
	}
}

func provisionedMultiRegionClusterWithLimit(clusterName string) resource.TestStep {
	return resource.TestStep{
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				serverless = {
					usage_limits = {
						provisioned_capacity = 3000
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
			makeDefaultServerlessResourceChecks(clusterName, "STANDARD"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.0.primary", "false"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.1.primary", "true"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.2.primary", "false"),
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.usage_limits.provisioned_capacity", "3000"),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.storage_mib_limit"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.0.primary", "false"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.1.primary", "true"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.2.primary", "false"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.usage_limits.provisioned_capacity", "3000"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.storage_mib_limit"),
		),
	}
}

// provisionedMultiRegionClusterUpdated updates some of the fields in
// provisionedMultiRegionClusterWithLimit.
func provisionedMultiRegionClusterUpdated(clusterName string) resource.TestStep {
	return resource.TestStep{
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				serverless = {
					usage_limits = {
						provisioned_capacity = 4000
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
			makeDefaultServerlessResourceChecks(clusterName, "STANDARD"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.0.primary", "true"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.1.primary", "false"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.2.primary", "false"),
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.usage_limits.provisioned_capacity", "4000"),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.storage_mib_limit"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.0.primary", "true"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.1.primary", "false"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.2.primary", "false"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.usage_limits.provisioned_capacity", "4000"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.storage_mib_limit"),
		),
	}
}

func legacyServerlessClusterWithSpendLimit(
	clusterName string,
	spendLimit int64,
) resource.TestStep {
	return resource.TestStep{
		// Serverless cluster with spend limit.
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				serverless = {
					spend_limit = %d
				}
				regions = [{
					name = "us-central1"
				}]
			}

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.test.id
			}
			`, clusterName, spendLimit),
		Check: resource.ComposeTestCheckFunc(
			testCheckCockroachClusterExists(serverlessResourceName),
			makeDefaultServerlessResourceChecks(clusterName, "BASIC"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.#", "1"),
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.#", "0"),
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.spend_limit", strconv.Itoa(int(spendLimit))),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.#", "1"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.#", "0"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.usage_limits.request_unit_limit", strconv.Itoa(int(spendLimit*5_000_000*8/10/100))),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.usage_limits.storage_mib_limit", strconv.Itoa(int(spendLimit*2*1024*2/10/100))),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.provisioned_capacity"),
		),
	}
}

func makeDefaultServerlessResourceChecks(clusterName string, planType client.PlanType) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttr(serverlessResourceName, "name", clusterName),
		resource.TestCheckResourceAttrSet(serverlessResourceName, "cloud_provider"),
		resource.TestCheckResourceAttrSet(serverlessResourceName, "cockroach_version"),
		resource.TestCheckResourceAttr(serverlessResourceName, "plan", string(planType)),
		resource.TestCheckResourceAttr(serverlessResourceName, "state", string(client.CLUSTERSTATETYPE_CREATED)),
		resource.TestCheckResourceAttr(serverlessDataSourceName, "name", clusterName),
		resource.TestCheckResourceAttrSet(serverlessDataSourceName, "cloud_provider"),
		resource.TestCheckResourceAttrSet(serverlessDataSourceName, "cockroach_version"),
		resource.TestCheckResourceAttr(serverlessDataSourceName, "plan", string(planType)),
		resource.TestCheckResourceAttr(serverlessDataSourceName, "state", string(client.CLUSTERSTATETYPE_CREATED)),
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
		Check:  resource.TestCheckResourceAttr("cockroach_cluster.test", "dedicated.num_virtual_cpus", "4"),
	}

	testDedicatedClusterResource(t, clusterName, true, scaleStep)
}

func testDedicatedClusterResource(t *testing.T, clusterName string, useMock bool, additionalSteps ...resource.TestStep) {
	const (
		resourceName   = "cockroach_cluster.test"
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
resource "cockroach_cluster" "test" {
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
    id = cockroach_cluster.test.id
}
`, name, version, vcpus)

	if finalize {
		config += fmt.Sprintf(`
resource "cockroach_finalize_version_upgrade" "test" {
	id = cockroach_cluster.test.id
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
