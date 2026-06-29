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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v8/pkg/client"
	"github.com/cockroachdb/terraform-provider-cockroach/internal/validators"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	minSupportedClusterMajorVersion = "v25.1"
	minSupportedClusterPatchVersion = "v25.1.0"
	latestClusterMajorVersion       = "v25.2"
	latestClusterPatchVersion       = "v25.2.0"

	serverlessResourceName   = "cockroach_cluster.test"
	serverlessDataSourceName = "data.cockroach_cluster.test"

	defaultPlanType = ""

	// CockroachHeterogeneousEnabled gates acceptance tests for per-region
	// (heterogeneous) machine types, which require a limited-access feature flag on
	// the test organization.
	CockroachHeterogeneousEnabled string = "COCKROACH_HETEROGENEOUS_ENABLED"
)

var httpOk = &http.Response{Status: http.StatusText(http.StatusOK)}
var httpFail = &http.Response{Status: http.StatusText(http.StatusBadRequest)}

// TestAccServerlessClusterResource attempts to create, check, update, and
// destroy a real cluster. It will be skipped if TF_ACC isn't set.
func TestAccServerlessClusterResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-serverless-%s", tfTestPrefix, GenerateRandomString(2))
	resource.Test(t, resource.TestCase{
		IsUnitTest:               false,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			onDemandSingleRegionClusterWithLimitsStep(clusterName, defaultPlanType, 10_000_000_000, 102_400),
			onDemandSingleRegionClusterWithLimitsStep(clusterName, "BASIC", 1_000_000, 1024),
			onDemandSingleRegionClusterNoLimitsStep(clusterName, defaultPlanType),
			onDemandSingleRegionClusterWithLimitsStep(clusterName, "BASIC", 10_000_000_000, 102_400),
			onDemandSingleRegionClusterWithUnlimitedStep(clusterName, defaultPlanType),
			onDemandSingleRegionClusterNoLimitsStep(clusterName, "BASIC"),
			legacyServerlessClusterWithSpendLimitStep(clusterName, 10_00),
			onDemandSingleRegionClusterWithUnlimitedStep(clusterName, defaultPlanType),
			// Upgrade to STANDARD.
			provisionedSingleRegionClusterStep(clusterName, "STANDARD", 6),
			// Downgrade to BASIC.
			onDemandSingleRegionClusterWithUnlimitedStep(clusterName, "BASIC"),
		},
	})
}

// TestAccServerlessUpgradeType is an acceptance test focused only on testing
// upgrade_type.  Since the serverless tests are relatively quick, no
// integration test version of this is added.  It will be skipped if TF_ACC
// isn't set.
func TestAccServerlessUpgradeType(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-serverless-%s", tfTestPrefix, GenerateRandomString(2))
	checkUpgradeTypeResources := func(upgradeType client.UpgradeTypeType) resource.TestCheckFunc {
		return resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.upgrade_type", string(upgradeType)),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.upgrade_type", string(upgradeType)),
		)
	}
	resource.Test(t, resource.TestCase{
		IsUnitTest:               false,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create a provisioned cluster with the default value for upgrade_type
			{
				Config: serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6)}).Config,
				Check:  checkUpgradeTypeResources(client.UPGRADETYPETYPE_AUTOMATIC),
			},
			// Explicitly updating the value to MANUAL performs the update
			{
				Config: serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{
					vcpus:       ptr(6),
					upgradeType: ptr(client.UPGRADETYPETYPE_MANUAL),
				}).Config,
				Check: checkUpgradeTypeResources(client.UPGRADETYPETYPE_MANUAL),
			},
			// Removal of the optional value from the config makes no change
			{
				Config: serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6)}).Config,
				Check:  checkUpgradeTypeResources(client.UPGRADETYPETYPE_MANUAL),
			},
			// Change it back to automatic so we can downgrade the cluster to
			// BASIC.  Currently the ccapi doesn't allow downgrading to BASIC
			// unless upgrade_type is AUTOMATIC already.
			{
				Config: serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{
					vcpus:       ptr(6),
					upgradeType: ptr(client.UPGRADETYPETYPE_AUTOMATIC),
				}).Config,
				Check: checkUpgradeTypeResources(client.UPGRADETYPETYPE_AUTOMATIC),
			},
			// Downgrade to Basic, the upgrade_type remains AUTOMATIC
			{
				Config: serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{}).Config,
				Check:  checkUpgradeTypeResources(client.UPGRADETYPETYPE_AUTOMATIC),
			},
			// Setting the value to MANUAL is not allowed for Basic
			{
				Config:      serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{upgradeType: ptr(client.UPGRADETYPETYPE_MANUAL)}).Config,
				ExpectError: regexp.MustCompile("plan type BASIC does not allow upgrade_type MANUAL"),
				Check:       checkUpgradeTypeResources(client.UPGRADETYPETYPE_AUTOMATIC),
			},
			// Setting completely invalid value for upgrade_type
			{
				Config:      serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{upgradeType: ptr(client.UpgradeTypeType("hi"))}).Config,
				ExpectError: regexp.MustCompile("Attribute serverless.upgrade_type value must be one of"),
				Check:       checkUpgradeTypeResources(client.UPGRADETYPETYPE_AUTOMATIC),
			},
			// Basic clusters can also accept a value of AUTOMATIC.
			{
				Config: serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{upgradeType: ptr(client.UPGRADETYPETYPE_AUTOMATIC)}).Config,
				Check:  checkUpgradeTypeResources(client.UPGRADETYPETYPE_AUTOMATIC),
			},
			// Destroy the cluster so we can create it again in the next step
			{
				Config:  " ",
				Destroy: true,
			},
			// Basic clusters can also be created with a value of AUTOMATIC
			{
				Config: serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{upgradeType: ptr(client.UPGRADETYPETYPE_AUTOMATIC)}).Config,
				Check:  checkUpgradeTypeResources(client.UPGRADETYPETYPE_AUTOMATIC),
			},
		},
	})
}

// TestAccServerlessWithEmptyIpAllowlist is an acceptance test focused only on
// testing with_empty_ip_allowlist. It will be skipped if TF_ACC isn't set.
func TestAccServerlessWithEmptyIpAllowlist(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-serverless-%s", tfTestPrefix, GenerateRandomString(2))
	resource.Test(t, resource.TestCase{
		IsUnitTest:               false,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{
					withEmptyIpAllowlist: ptr(true),
				}).Config,
				Check: resource.TestCheckResourceAttr(
					serverlessResourceName, "serverless.with_empty_ip_allowlist", "true",
				),
			},
			{
				Config: serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{}).Config,
				Check: resource.TestCheckResourceAttr(
					serverlessResourceName, "serverless.with_empty_ip_allowlist", "true",
				),
			},
			{
				Config:  " ",
				Destroy: true,
			},
			{
				Config: serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{}).Config,
				Check: resource.TestCheckNoResourceAttr(
					serverlessResourceName, "serverless.with_empty_ip_allowlist",
				),
			},
		},
	})
}

// Shared Test objects
var initialBackupConfig = &client.BackupConfiguration{
	Enabled:          true,
	FrequencyMinutes: 60,
	RetentionDays:    30,
}

var initialBackupConfigDisabled = &client.BackupConfiguration{
	Enabled:          false,
	FrequencyMinutes: initialBackupConfig.FrequencyMinutes,
	RetentionDays:    initialBackupConfig.RetentionDays,
}

var updatedBackupConfig = &client.BackupConfiguration{
	Enabled:          true,
	RetentionDays:    7,
	FrequencyMinutes: 5,
}

var secondUpdatedBackupConfig = &client.BackupConfiguration{
	Enabled:          updatedBackupConfig.Enabled,
	RetentionDays:    updatedBackupConfig.RetentionDays,
	FrequencyMinutes: 1440,
}

// TestAccClusterWithBackupConfig is an acceptance test focused only on testing
// the backup_config parameter. It will be skipped if TF_ACC isn't set.
func TestAccClusterWithBackupConfig(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-serverless-%s", tfTestPrefix, GenerateRandomString(2))
	testClusterWithBackupConfig(t, clusterName, false /* useMock */)
}

// TestIntegrationClusterWithBackupConfig is an integration test focused only on
// testing the backup_config parameter.
func TestIntegrationClusterWithBackupConfig(t *testing.T) {
	clusterName := fmt.Sprintf("%s-serverless-%s", tfTestPrefix, GenerateRandomString(2))
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
		Id:               uuid.Nil.String(),
		Name:             clusterName,
		CockroachVersion: latestClusterPatchVersion,
		CloudProvider:    "GCP",
		State:            "CREATED",
		Plan:             "STANDARD",
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{
				UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
				UsageLimits: &client.UsageLimits{
					ProvisionedVirtualCpus: ptr(int64(2)),
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

	expectUpdateSequence := func(update *client.UpdateBackupConfigurationSpec, before, after *client.BackupConfiguration) {
		// Pre-update read
		s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(before, httpOk, nil)
		// Actual update
		s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).Return(&cluster, httpOk, nil)
		s.EXPECT().UpdateBackupConfiguration(gomock.Any(), clusterID, update).Return(after, httpOk, nil)
		// checkBackupConfig
		s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(after, httpOk, nil)
		// post update read
		s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(after, httpOk, nil)
	}

	expectStateMatchesServerSequence := func(backupConfig *client.BackupConfiguration) {
		s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(backupConfig, httpOk, nil).Times(3)
	}

	expectErrorSequence := func(update *client.UpdateBackupConfigurationSpec, before *client.BackupConfiguration, err error) {
		// Pre-update read
		s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(before, httpOk, nil)
		// Actual update
		s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).Return(&cluster, httpOk, nil)
		s.EXPECT().UpdateBackupConfiguration(gomock.Any(), clusterID, update).Return(before, httpFail, err)
	}

	// Our cluster never changes so to avoid complexity, we'll just return it
	// as many times as its asked for.
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&cluster, httpOk, nil).AnyTimes()

	// Step: a cluster without a backup config block still has a default config
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&cluster, nil, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(initialBackupConfig, httpOk, nil).Times(3)

	// Step: a backup config with an empty block
	expectStateMatchesServerSequence(initialBackupConfig)

	// Step: a backup config with just the enabled field
	expectStateMatchesServerSequence(initialBackupConfig)

	// Step: disabling backups without specifying any other fields
	expectUpdateSequence(&client.UpdateBackupConfigurationSpec{
		Enabled: ptr(false),
	}, initialBackupConfig, initialBackupConfigDisabled)

	// Step: reenable passing in the default values
	expectUpdateSequence(&client.UpdateBackupConfigurationSpec{
		Enabled: ptr(true),
	}, initialBackupConfigDisabled, initialBackupConfig)

	// Step: update frequency and retention
	expectUpdateSequence(&client.UpdateBackupConfigurationSpec{
		RetentionDays:    ptr(updatedBackupConfig.RetentionDays),
		FrequencyMinutes: ptr(updatedBackupConfig.FrequencyMinutes),
	}, initialBackupConfig, updatedBackupConfig)

	// Step: error case: invalid retention_days
	expectErrorSequence(&client.UpdateBackupConfigurationSpec{
		RetentionDays: ptr(int32(12345)),
	}, updatedBackupConfig, fmt.Errorf("retention_days must be one of []"))

	// Step: error case: invalid frequency_minutes
	expectErrorSequence(&client.UpdateBackupConfigurationSpec{
		FrequencyMinutes: ptr(int32(12345)),
	}, updatedBackupConfig, fmt.Errorf("frequency_minutes must be one of []"))

	// Step: remove the backup configuration block, cluster still has the last one that was set
	expectStateMatchesServerSequence(updatedBackupConfig)

	// Step: destroy cluster in prep for next step
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(initialBackupConfig, httpOk, nil)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	// Step: test setting backup config values during the create
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&cluster, nil, nil)
	s.EXPECT().UpdateBackupConfiguration(gomock.Any(), clusterID, &client.UpdateBackupConfigurationSpec{
		Enabled:          ptr(true),
		FrequencyMinutes: ptr(int32(secondUpdatedBackupConfig.FrequencyMinutes)),
		RetentionDays:    ptr(int32(secondUpdatedBackupConfig.RetentionDays)),
	}).Return(secondUpdatedBackupConfig, httpOk, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(secondUpdatedBackupConfig, httpOk, nil)

	// Delete phase
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(secondUpdatedBackupConfig, httpOk, nil)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testClusterWithBackupConfig(t, clusterName, true /* useMock */)
}

func testClusterWithBackupConfig(t *testing.T, clusterName string, useMock bool) {
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					traceMessageStep("a cluster without a backup config block still has a default config")
				},
				Config: getTestClusterWithBackupConfig(clusterName, backupCreateConfig{
					includeBackupConfig: false,
				}),
				Check: checkBackupConfig(serverlessResourceName, initialBackupConfig),
			},
			{
				PreConfig: func() {
					traceMessageStep("a backup config with an empty block")
				},
				Config: getTestClusterWithBackupConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
				}),
				Check: checkBackupConfig(serverlessResourceName, initialBackupConfig),
			},
			{
				PreConfig: func() {
					traceMessageStep("a backup config with just the enabled field")
				},
				Config: getTestClusterWithBackupConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled:             ptr(true),
				}),
				Check: checkBackupConfig(serverlessResourceName, initialBackupConfig),
			},
			{
				PreConfig: func() {
					traceMessageStep("disabling backups without specifying any other fields")
				},
				Config: getTestClusterWithBackupConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled:             ptr(false),
				}),
				Check: checkBackupConfig(serverlessResourceName, initialBackupConfigDisabled),
			},
			{
				PreConfig: func() {
					traceMessageStep("reenable passing in the default values")
				},
				Config: getTestClusterWithBackupConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled:             ptr(true),
					retention:           ptr(initialBackupConfig.RetentionDays),
					frequency:           ptr(initialBackupConfig.FrequencyMinutes),
				}),
				Check: checkBackupConfig(serverlessResourceName, initialBackupConfig),
			},
			{
				PreConfig: func() {
					traceMessageStep("update frequency and retention")
				},
				Config: getTestClusterWithBackupConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled:             ptr(true),
					retention:           ptr(updatedBackupConfig.RetentionDays),
					frequency:           ptr(updatedBackupConfig.FrequencyMinutes),
				}),
				Check: checkBackupConfig(serverlessResourceName, updatedBackupConfig),
			},
			{
				PreConfig: func() {
					traceMessageStep("error case: invalid retention_days")
				},
				Config: getTestClusterWithBackupConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled:             ptr(true),
					retention:           ptr(int32(12345)),
				}),
				Check: checkBackupConfig(serverlessResourceName, updatedBackupConfig),
				// Setting single line mode because error is broken across lines.
				ExpectError: regexp.MustCompile(`(?s)retention_days.*must.*be.*one.*of`),
			},
			{
				PreConfig: func() {
					traceMessageStep("error case: invalid frequency_minutes")
				},
				Config: getTestClusterWithBackupConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled:             ptr(true),
					frequency:           ptr(int32(12345)),
				}),
				Check: checkBackupConfig(serverlessResourceName, updatedBackupConfig),
				// Setting single line mode because error is broken across lines.
				ExpectError: regexp.MustCompile(`(?s)frequency_minutes.*must.*be.*one.*of`),
			},
			{
				PreConfig: func() {
					traceMessageStep("remove the backup configuration block, cluster still has the last one that was set")
				},
				Config: getTestClusterWithBackupConfig(clusterName, backupCreateConfig{
					includeBackupConfig: false,
				}),
				Check: checkBackupConfig(serverlessResourceName, updatedBackupConfig),
			},
			{
				PreConfig: func() {
					traceMessageStep("destroy cluster in prep for next step")
				},
				Config:  " ",
				Destroy: true,
			},
			{
				PreConfig: func() {
					traceMessageStep("test setting backup config values during the create")
				},
				Config: getTestClusterWithBackupConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled:             ptr(true),
					frequency:           ptr(secondUpdatedBackupConfig.FrequencyMinutes),
					retention:           ptr(secondUpdatedBackupConfig.RetentionDays),
				}),
				Check: resource.ComposeTestCheckFunc(
					checkBackupConfig(serverlessResourceName, secondUpdatedBackupConfig),
					traceEndOfPlan(),
				),
			},
		},
	})
}

func checkBackupConfig(
	clusterResourceName string, expected *client.BackupConfiguration,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		p := testAccProvider.(*provider)
		p.service = NewService(cl)

		clusterRs, ok := s.RootModule().Resources[clusterResourceName]
		if !ok {
			return fmt.Errorf("not found: %s", clusterResourceName)
		}

		clusterID := clusterRs.Primary.Attributes["id"]

		traceAPICall("GetBackupConfiguration")
		resp, _, err := p.service.GetBackupConfiguration(context.TODO(), clusterID)
		if err != nil {
			return fmt.Errorf("unexpected error during config lookup: %w", err)
		}

		if *resp != *expected {
			return fmt.Errorf("expected backup configuration did not match actual. expected: %v, actual %v", expected, resp)
		}

		return nil
	}
}

var notFoundResourceId = "00000000-0000-0000-0000-000000000002"

// TestAccClusterWithParentID is an acceptance test focused only on testing the
// parent_id parameter. It will be skipped if TF_ACC isn't set.
func TestAccClusterWithParentID(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-parent-c-%s", tfTestPrefix, GenerateRandomString(2))
	folderName := fmt.Sprintf("%s-parent-f-%s", tfTestPrefix, GenerateRandomString(2))
	testClusterWithParentID(t, clusterName, folderName, false /* useMock */)
}

// TestIntegrationClusterWithParentID is an integration test focused only on
// testing the parent_id parameter.
func TestIntegrationClusterWithParentID(t *testing.T) {
	clusterName := fmt.Sprintf("%s-serverless-%s", tfTestPrefix, GenerateRandomString(2))
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
		Id:               uuid.Nil.String(),
		Name:             clusterName,
		CockroachVersion: latestClusterPatchVersion,
		CloudProvider:    "GCP",
		State:            "CREATED",
		Plan:             "STANDARD",
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{
				UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
				UsageLimits: &client.UsageLimits{
					ProvisionedVirtualCpus: ptr(int64(2)),
				},
				RoutingId: "routing-id",
			},
		},
		Regions: []client.Region{
			{
				Name: "us-central1",
			},
		},
		ParentId: ptr("root"),
	}

	folder := client.FolderResource{
		Name:         "folder123",
		ResourceId:   "00000000-0000-0000-0000-000000000001",
		ParentId:     "root",
		ResourceType: client.FOLDERRESOURCETYPETYPE_FOLDER,
	}

	updatedCluster := cluster
	updatedCluster.ParentId = ptr(folder.ResourceId)

	// Step: a cluster without a backup config block still has a default config
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&cluster, nil, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&cluster, httpOk, nil).Times(3)

	// Step: updating to explicit root is a no op
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&cluster, httpOk, nil).Times(3)

	// Step: error case: values besides root and uuid are not allowed
	// No API calls expected

	// Step: error case: valid uuid but not found results in reasonable error
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&cluster, httpOk, nil)
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).Return(nil, httpFail, fmt.Errorf("invalid argument: the parent must exist"))

	// Step: move a cluster under a non-root folder
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&cluster, httpOk, nil)
	s.EXPECT().CreateFolder(gomock.Any(), gomock.Any()).Return(&folder, httpOk, nil)
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).Return(&updatedCluster, httpOk, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&updatedCluster, httpOk, nil).Times(2)
	s.EXPECT().ListFolders(gomock.Any(), gomock.Any()).Return(&client.ListFoldersResponse{
		Folders: []client.FolderResource{folder},
	}, httpOk, nil)
	s.EXPECT().GetFolder(gomock.Any(), folder.ResourceId).Return(&folder, httpOk, nil)

	// Delete phase
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&updatedCluster, httpOk, nil)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)
	s.EXPECT().DeleteFolder(gomock.Any(), folder.ResourceId)

	testClusterWithParentID(t, clusterName, folder.Name, true /* useMock */)
}

func testClusterWithParentID(t *testing.T, clusterName, folderName string, useMock bool) {
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					traceMessageStep("a cluster without a parent_id is allowed and gets the default of root")
				},
				Config: getTestClusterWithParentFolder(clusterName, parentIDTestConfig{}),
				Check:  checkParentID(serverlessResourceName, "root"),
			},
			{
				PreConfig: func() {
					traceMessageStep("updating to explicit root is a no op")
				},
				Config: getTestClusterWithParentFolder(clusterName, parentIDTestConfig{folderID: ptr("root")}),
				Check:  checkParentID(serverlessResourceName, "root"),
			},
			{
				// Don't run error cases as the last step, or else it leave's the config in a bad state.
				PreConfig: func() {
					traceMessageStep("error case: values besides root and uuid are not allowed")
				},
				Config:      getTestClusterWithParentFolder(clusterName, parentIDTestConfig{folderID: ptr("123")}),
				Check:       checkParentID(serverlessResourceName, "root"),
				ExpectError: regexp.MustCompile(`Attribute parent_id value must be a UUID or the string "root", got:`),
			},
			{
				// Don't run error cases as the last step, or else it leave's the config in a bad state.
				PreConfig: func() {
					traceMessageStep("error case: valid uuid but not found results in reasonable error")
				},
				Config:      getTestClusterWithParentFolder(clusterName, parentIDTestConfig{folderID: &notFoundResourceId}),
				Check:       checkParentID(serverlessResourceName, "root"),
				ExpectError: regexp.MustCompile(`invalid argument: the parent must exist`),
			},
			{
				PreConfig: func() {
					traceMessageStep("move a cluster under a non-root folder")
				},
				Config: getTestClusterWithParentFolder(clusterName, parentIDTestConfig{folderName: &folderName}),
				Check: resource.ComposeTestCheckFunc(
					checkParentID(serverlessResourceName, folderName),
					traceEndOfPlan(),
				),
			},
		},
	})
}

func checkParentID(clusterResourceName, parentFolderName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		p := testAccProvider.(*provider)
		p.service = NewService(cl)

		clusterRs, ok := s.RootModule().Resources[clusterResourceName]
		if !ok {
			return fmt.Errorf("not found: %s", clusterResourceName)
		}

		clusterID := clusterRs.Primary.Attributes["id"]

		traceAPICall("GetCluster")
		cluster, _, err := p.service.GetCluster(context.TODO(), clusterID)
		if err != nil {
			return fmt.Errorf("unexpected error during cluster lookup: %w", err)
		}
		clusterParentID := *cluster.ParentId

		if parentFolderName == "root" {
			if clusterParentID != "root" {
				return fmt.Errorf("expect parentID to be root but was: %s", clusterParentID)
			}
			return nil
		}

		folderPath := fmt.Sprintf("/%s", parentFolderName)
		traceAPICall("ListFolders")
		foldersResp, _, err := p.service.ListFolders(context.TODO(), &client.ListFoldersOptions{
			Path: &folderPath,
		})
		if err != nil {
			return fmt.Errorf("unexpected error during folder lookup: %w", err)
		}
		if len(foldersResp.Folders) > 0 {
			folder := foldersResp.Folders[0]
			if folder.ResourceId != clusterParentID {
				return fmt.Errorf("expect parentID to be %s but was: %s", folder.ResourceId, clusterParentID)
			}
		} else {
			return fmt.Errorf("folder with path %s not found", folderPath)
		}

		return nil
	}
}

// TestAccMultiRegionServerlessClusterResource attempts to create, update, check,
// and destroy a real multi-region serverless cluster. It will be skipped if
// TF_ACC isn't set.
func TestAccMultiRegionServerlessClusterResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-multi-region-serverless-%s", tfTestPrefix, GenerateRandomString(2))
	resource.Test(t, resource.TestCase{
		IsUnitTest:               false,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			provisionedMultiRegionClusterWithLimitStep(clusterName, "STANDARD"),
			provisionedMultiRegionClusterUpdatedStep(clusterName, defaultPlanType),
		},
	})
}

// TestIntegrationServerlessClusterResource attempts to create, check, and destroy a
// cluster, but uses a mocked API service.
func TestIntegrationServerlessClusterResource(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	boolPtr := func(val bool) *bool { return &val }
	int64Ptr := func(val int64) *int64 { return &val }
	clusterName := fmt.Sprintf("%s-serverless-%s", tfTestPrefix, GenerateRandomString(2))

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
					RoutingId:   "routing-id",
					UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
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
					UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
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

	provisionedSingleRegionCluster := func(planType client.PlanType, provisionedVirtualCpus int64) client.Cluster {
		return client.Cluster{
			Id:               uuid.Nil.String(),
			Name:             clusterName,
			CockroachVersion: latestClusterPatchVersion,
			CloudProvider:    "GCP",
			State:            "CREATED",
			Plan:             planType,
			Config: client.ClusterConfig{
				Serverless: &client.ServerlessClusterConfig{
					UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
					UsageLimits: &client.UsageLimits{
						ProvisionedVirtualCpus: int64Ptr(provisionedVirtualCpus),
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

	standardClusterWithVersion := func(version string) client.Cluster {
		cluster := provisionedSingleRegionCluster(client.PLANTYPE_STANDARD, 6)
		cluster.Config.Serverless.UpgradeType = client.UPGRADETYPETYPE_MANUAL
		cluster.CockroachVersion = version
		return cluster
	}

	provisionedMultiRegionCluster := func(provisionedVirtualCpus int64, primaryIndex int) client.Cluster {
		cluster := client.Cluster{
			Id:               uuid.Nil.String(),
			Name:             clusterName,
			CockroachVersion: latestClusterPatchVersion,
			CloudProvider:    "GCP",
			State:            "CREATED",
			Plan:             "STANDARD",
			Config: client.ClusterConfig{
				Serverless: &client.ServerlessClusterConfig{
					UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
					UsageLimits: &client.UsageLimits{
						ProvisionedVirtualCpus: int64Ptr(provisionedVirtualCpus),
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
		cluster.Regions[primaryIndex].Primary = boolPtr(true)
		return cluster
	}

	cases := []struct {
		name              string
		createStep        func() resource.TestStep
		validateCreate    func(req *client.CreateClusterRequest) error
		initialCluster    client.Cluster
		updateStep        func() resource.TestStep
		validateUpdate    func(spec *client.UpdateClusterSpecification) error
		finalCluster      client.Cluster
		ignoreImportPaths []string
	}{
		{
			name: "serverless rejects BYOC in Create",
			createStep: func() resource.TestStep {
				return resource.TestStep{
					Config: fmt.Sprintf(`
                        resource "cockroach_cluster" "test" {
                            name           = "%s"
                            cloud_provider = "GCP"
                            serverless = {}
                            regions = [{ name = "us-central1" }]
                            customer_cloud_account = { gcp = { service_account_email = "sa@example.iam.gserviceaccount.com" } }
                        }
                    `, clusterName),
					ExpectError: regexp.MustCompile(`These attributes cannot be configured together:\s\[serverless,customer_cloud_account\]`),
				}
			},
		},
		{
			name: "STANDARD clusters must not have a node_count",
			createStep: func() resource.TestStep {
				return resource.TestStep{
					Config: fmt.Sprintf(`
						resource "cockroach_cluster" "test" {
							name           = "%s"
							cloud_provider = "GCP"
							serverless = {}
							regions = [
								{
									name = "us-central1"
								},
								{
									name = "us-central1"
									node_count = 1
								},
							]
						}
					`, clusterName),
					ExpectError: regexp.MustCompile("node_count is supported for ADVANCED clusters only"),
				}
			},
		},
		{
			name: "single-region serverless BASIC cluster converted to unlimited resources",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimitsStep(clusterName, "BASIC", 1_000_000, 1024)
			},
			validateCreate: func(req *client.CreateClusterRequest) error {
				// Ensure that provider passes the plan type to Create.
				if req.Spec.Plan == nil || *req.Spec.Plan != client.PLANTYPE_BASIC {
					return fmt.Errorf("unexpected plan type in create request: %v", req.Spec.Plan)
				}
				return nil
			},
			initialCluster: singleRegionClusterWithLimits("BASIC", 1_000_000, 1024),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithUnlimitedStep(clusterName, "BASIC")
			},
			validateUpdate: func(spec *client.UpdateClusterSpecification) error {
				// Ensure that provider passes the plan type to Update.
				if spec.Plan == nil || *spec.Plan != client.PLANTYPE_BASIC {
					return fmt.Errorf("unexpected plan type in update request: %v", spec.Plan)
				}
				return nil
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
				return onDemandSingleRegionClusterWithLimitsStep(clusterName, defaultPlanType, 1_000_000, 1024)
			},
			initialCluster: singleRegionClusterWithLimits("BASIC", 1_000_000, 1024),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterNoLimitsStep(clusterName, defaultPlanType)
			},
			finalCluster: singleRegionClusterWithUnlimited("BASIC"),
			// When testing import, skip validating the usage limits, because the
			// server returns usage_limits = null for "unlimited", whereas the
			// TF state contains usage_limits = {}. This is a spurious failure,
			// as the two formulations are equivalent.
			ignoreImportPaths: []string{"serverless.usage_limits.%"},
		},
		{
			name: "single-region serverless BASIC cluster converted from unlimited resources",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithUnlimitedStep(clusterName, defaultPlanType)
			},
			initialCluster: singleRegionClusterWithUnlimited("BASIC"),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimitsStep(clusterName, "BASIC", 1_000_000, 1024)
			},
			finalCluster: singleRegionClusterWithLimits("BASIC", 1_000_000, 1024),
		},
		{
			name: "single-region serverless BASIC cluster converted from no limit resources",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterNoLimitsStep(clusterName, "BASIC")
			},
			initialCluster: singleRegionClusterWithUnlimited("BASIC"),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimitsStep(clusterName, defaultPlanType, 1_000_000, 1024)
			},
			finalCluster: singleRegionClusterWithLimits("BASIC", 1_000_000, 1024),
		},
		{
			name: "single-region serverless BASIC cluster with updated resource limits",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimitsStep(clusterName, defaultPlanType, 1_000_000, 1024)
			},
			initialCluster: singleRegionClusterWithLimits("BASIC", 1_000_000, 1024),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimitsStep(clusterName, "BASIC", 10_000_000_000, 102_400)
			},
			finalCluster: singleRegionClusterWithLimits("BASIC", 10_000_000_000, 102_400),
		},
		{
			name: "single-region serverless BASIC cluster upgraded to STANDARD",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithUnlimitedStep(clusterName, defaultPlanType)
			},
			initialCluster: singleRegionClusterWithUnlimited("BASIC"),
			updateStep: func() resource.TestStep {
				return provisionedSingleRegionClusterStep(clusterName, "STANDARD", 10)
			},
			validateUpdate: func(spec *client.UpdateClusterSpecification) error {
				// Ensure that provider passes the new plan type to Update.
				if spec.Plan == nil || *spec.Plan != client.PLANTYPE_STANDARD {
					return fmt.Errorf("unexpected plan type in update request: %v", spec.Plan)
				}
				return nil
			},
			finalCluster: provisionedSingleRegionCluster("STANDARD", 10),
		},
		{
			name: "single-region serverless STANDARD cluster downgraded to BASIC",
			createStep: func() resource.TestStep {
				return provisionedSingleRegionClusterStep(clusterName, defaultPlanType, 10)
			},
			initialCluster: provisionedSingleRegionCluster("STANDARD", 10),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimitsStep(clusterName, "BASIC", 10_000_000_000, 102_400)
			},
			validateUpdate: func(spec *client.UpdateClusterSpecification) error {
				// Ensure that provider passes the new plan type to Update.
				if spec.Plan == nil || *spec.Plan != client.PLANTYPE_BASIC {
					return fmt.Errorf("unexpected plan type in update request: %v", spec.Plan)
				}
				return nil
			},
			finalCluster: singleRegionClusterWithLimits("BASIC", 10_000_000_000, 102_400),
		},
		{
			name: "multi-region serverless STANDARD cluster with provisioned limit",
			createStep: func() resource.TestStep {
				return provisionedMultiRegionClusterWithLimitStep(clusterName, defaultPlanType)
			},
			initialCluster: provisionedMultiRegionCluster(6, 1),
			updateStep: func() resource.TestStep {
				return provisionedMultiRegionClusterUpdatedStep(clusterName, "STANDARD")
			},
			finalCluster: provisionedMultiRegionCluster(8, 0),
		},
		{
			name: "legacy serverless cluster from spend limit to higher spend limit",
			createStep: func() resource.TestStep {
				return legacyServerlessClusterWithSpendLimitStep(clusterName, 10_00)
			},
			initialCluster: singleRegionClusterWithLimits("BASIC", 40_000_000, 4096),
			updateStep: func() resource.TestStep {
				return legacyServerlessClusterWithSpendLimitStep(clusterName, 20_00)
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
				return legacyServerlessClusterWithSpendLimitStep(clusterName, 10_00)
			},
			initialCluster: singleRegionClusterWithLimits("BASIC", 40_000_000, 4096),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterWithLimitsStep(clusterName, "BASIC", 80_000_000, 8192)
			},
			finalCluster: singleRegionClusterWithLimits("BASIC", 80_000_000, 8192),
		},
		{
			name: "clear spend limit in legacy Serverless cluster",
			createStep: func() resource.TestStep {
				return legacyServerlessClusterWithSpendLimitStep(clusterName, 10_00)
			},
			initialCluster: singleRegionClusterWithLimits("BASIC", 40_000_000, 4096),
			updateStep: func() resource.TestStep {
				return onDemandSingleRegionClusterNoLimitsStep(clusterName, "BASIC")
			},
			finalCluster: singleRegionClusterWithUnlimited("BASIC"),
			// When testing import, skip validating the usage limits, because the
			// server returns usage_limits = null for "unlimited", whereas the
			// TF state contains usage_limits = {}. This is a spurious failure,
			// as the two formulations are equivalent.
			ignoreImportPaths: []string{"serverless.usage_limits.%"},
		},
		{
			name: "upgrade cockroach_version for STANDARD cluster",
			createStep: func() resource.TestStep {
				return serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), upgradeType: ptr(client.UPGRADETYPETYPE_MANUAL)})
			},
			initialCluster: standardClusterWithVersion(minSupportedClusterPatchVersion),
			updateStep: func() resource.TestStep {
				return serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), upgradeType: ptr(client.UPGRADETYPETYPE_MANUAL), version: ptr(latestClusterMajorVersion)})
			},
			finalCluster: standardClusterWithVersion(latestClusterPatchVersion),
		},
		{
			name: "attempt to update name",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterNoLimitsStep(clusterName, defaultPlanType)
			},
			initialCluster: singleRegionClusterWithUnlimited("BASIC"),
			updateStep: func() resource.TestStep {
				step := onDemandSingleRegionClusterNoLimitsStep("new-name", "BASIC")
				step.ExpectError = regexp.MustCompile("Cannot update cluster name")
				return step
			},
		},
		{
			name: "attempt to update cloud provider",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterNoLimitsStep(clusterName, "BASIC")
			},
			initialCluster: singleRegionClusterWithUnlimited("BASIC"),
			updateStep: func() resource.TestStep {
				step := onDemandSingleRegionClusterNoLimitsStep(clusterName, defaultPlanType)
				step.Config = strings.Replace(step.Config, "GCP", "AWS", -1)
				step.ExpectError = regexp.MustCompile("Cannot update cluster cloud provider")
				return step
			},
		},
		{
			name: "attempt to update plan type",
			createStep: func() resource.TestStep {
				return onDemandSingleRegionClusterNoLimitsStep(clusterName, defaultPlanType)
			},
			initialCluster: singleRegionClusterWithUnlimited("BASIC"),
			updateStep: func() resource.TestStep {
				return resource.TestStep{
					Config:      getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 4, nil, false),
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
									provisioned_virtual_cpus = 2
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
									provisioned_virtual_cpus = 2
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
			name: "use serverless primary region on dedicated cluster",
			createStep: func() resource.TestStep {
				config := getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 4, nil, false)
				config = strings.Replace(config, "node_count: 1", "primary: true", -1)
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile("Dedicated clusters do not support the primary attribute on regions."),
				}
			},
		},
		{
			name: "invalid plan name",
			createStep: func() resource.TestStep {
				return resource.TestStep{
					Config: `
						resource "cockroach_cluster" "test" {
							name = "foo"
							cloud_provider = "GCP"
							plan = "SERVERLESS"
							serverless = {}
							regions = [{
								name = "us-central1"
							}]
						}`,
					ExpectError: regexp.MustCompile("Invalid Attribute Value Match"),
				}
			},
		},
		{
			name: "setting cockroach_version to empty string on basic cluster creation",
			createStep: func() resource.TestStep {
				config := serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{vcpus: ptr(6), version: ptr("")}).Config
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile("cockroach_version is not supported for BASIC clusters"),
				}
			},
		},
		{
			name: "setting cockroach_version on basic cluster creation",
			createStep: func() resource.TestStep {
				config := serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{vcpus: ptr(6), version: ptr("23.1")}).Config
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile("cockroach_version is not supported for BASIC clusters"),
				}
			},
		},
		{
			name: "setting cockroach_version to empty string on standard cluster creation",
			createStep: func() resource.TestStep {
				config := serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), version: ptr("")}).Config
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile(`(?s)cockroach_version is not supported during cluster creation for STANDARD`),
				}
			},
		},
		{
			name: "setting cockroach_version on standard cluster creation",
			createStep: func() resource.TestStep {
				config := serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), version: ptr("23.1")}).Config
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile(`(?s)cockroach_version is not supported during cluster creation for STANDARD`),
				}
			},
		},
		{
			name: "setting cockroach_version to empty string on basic cluster update",
			createStep: func() resource.TestStep {
				return serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{vcpus: ptr(6)})
			},
			initialCluster: provisionedSingleRegionCluster("BASIC", 6),
			updateStep: func() resource.TestStep {
				config := serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{vcpus: ptr(6), version: ptr("")}).Config
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile(`(?s)cockroach_version is not supported for BASIC clusters`),
				}
			},
		},
		{
			name: "setting cockroach_version on basic cluster update",
			createStep: func() resource.TestStep {
				return serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{vcpus: ptr(6)})
			},
			initialCluster: provisionedSingleRegionCluster(client.PLANTYPE_BASIC, 6),
			updateStep: func() resource.TestStep {
				config := serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{vcpus: ptr(6), version: ptr("23.1")}).Config
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile(`(?s)cockroach_version is not supported for BASIC clusters`),
				}
			},
		},
		{
			name: "setting cockroach_version to empty string on basic cluster update to standard in same step",
			createStep: func() resource.TestStep {
				return serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{vcpus: ptr(6)})
			},
			initialCluster: provisionedSingleRegionCluster("BASIC", 6),
			updateStep: func() resource.TestStep {
				config := serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), version: ptr("")}).Config
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile(`(?s)plan and version must not be changed during the same terraform plan`),
				}
			},
		},
		{
			name: "setting cockroach_version on basic cluster update to standard in same step",
			createStep: func() resource.TestStep {
				return serverlessClusterStep(clusterName, client.PLANTYPE_BASIC, slsConfig{vcpus: ptr(6)})
			},
			initialCluster: provisionedSingleRegionCluster(client.PLANTYPE_BASIC, 6),
			updateStep: func() resource.TestStep {
				config := serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), version: ptr("23.1")}).Config
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile(`(?s)plan and version must not be changed during the same terraform plan`),
				}
			},
		},
		{
			name: "setting cockroach_version to empty string for standard cluster with automatic upgrade_type",
			createStep: func() resource.TestStep {
				return serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), upgradeType: ptr(client.UPGRADETYPETYPE_AUTOMATIC)})
			},
			initialCluster: provisionedSingleRegionCluster(client.PLANTYPE_STANDARD, 6),
			updateStep: func() resource.TestStep {
				config := serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), upgradeType: ptr(client.UPGRADETYPETYPE_AUTOMATIC), version: ptr("")}).Config
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile(`(?s)upgrade_type must be set to MANUAL before setting cockroach_version`),
				}
			},
		},
		{
			name: "setting cockroach_version for standard cluster with automatic upgrade_type",
			createStep: func() resource.TestStep {
				return serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), upgradeType: ptr(client.UPGRADETYPETYPE_AUTOMATIC)})
			},
			initialCluster: provisionedSingleRegionCluster(client.PLANTYPE_STANDARD, 6),
			updateStep: func() resource.TestStep {
				config := serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), upgradeType: ptr(client.UPGRADETYPETYPE_AUTOMATIC), version: ptr("23.1")}).Config
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile(`(?s)upgrade_type must be set to MANUAL before setting cockroach_version`),
				}
			},
		},
		{
			name: "setting cockroach_version to empty string for standard cluster while setting upgrade_type to MANUAL in same step",
			createStep: func() resource.TestStep {
				return serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), upgradeType: ptr(client.UPGRADETYPETYPE_AUTOMATIC)})
			},
			initialCluster: provisionedSingleRegionCluster(client.PLANTYPE_STANDARD, 6),
			updateStep: func() resource.TestStep {
				config := serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), upgradeType: ptr(client.UPGRADETYPETYPE_MANUAL), version: ptr("")}).Config
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile(`(?s)upgrade_type and version must not be changed during the same terraform plan`),
				}
			},
		},
		{
			name: "setting cockroach_version for standard cluster while setting upgrade_type to MANUAL in same step",
			createStep: func() resource.TestStep {
				return serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), upgradeType: ptr(client.UPGRADETYPETYPE_AUTOMATIC)})
			},
			initialCluster: provisionedSingleRegionCluster(client.PLANTYPE_STANDARD, 6),
			updateStep: func() resource.TestStep {
				config := serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), upgradeType: ptr(client.UPGRADETYPETYPE_MANUAL), version: ptr("23.1")}).Config
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile(`(?s)upgrade_type and version must not be changed during the same terraform plan`),
				}
			},
		},
		{
			name: "attempt to update an advanced cluster to a standard cluster in place",
			createStep: func() resource.TestStep {
				return resource.TestStep{
					Config: getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 4, nil, false),
				}
			},
			initialCluster: client.Cluster{
				Id:               uuid.Nil.String(),
				Name:             clusterName,
				CloudProvider:    "GCP",
				CockroachVersion: latestClusterMajorVersion,
				Config: client.ClusterConfig{
					Dedicated: &client.DedicatedHardwareConfig{
						NumVirtualCpus: 4,
						StorageGib:     15,
					},
				},
				Plan:      client.PLANTYPE_ADVANCED,
				State:     client.CLUSTERSTATETYPE_CREATED,
				CidrRange: "172.28.0.0/16",
				Regions:   []client.Region{{Name: "us-central1", NodeCount: 1}},
			},
			updateStep: func() resource.TestStep {
				config := serverlessClusterStep(clusterName, client.PLANTYPE_STANDARD, slsConfig{vcpus: ptr(6), version: ptr(latestClusterMajorVersion)}).Config
				return resource.TestStep{
					Config:      config,
					ExpectError: regexp.MustCompile(`Cannot update cluster plan type`),
				}
			},
		},
		{
			name: "serverless cluster with empty ip allowlist",
			createStep: func() resource.TestStep {
				return serverlessClusterStep(clusterName, "BASIC", slsConfig{
					withEmptyIpAllowlist: ptr(true),
				})
			},
			validateCreate: func(req *client.CreateClusterRequest) error {
				serverless := req.Spec.Serverless
				if serverless == nil || serverless.WithEmptyIpAllowlist == nil || !*serverless.WithEmptyIpAllowlist {
					return fmt.Errorf("expected with_empty_ip_allowlist to be true in create request")
				}
				return nil
			},
			initialCluster: singleRegionClusterWithUnlimited("BASIC"),
			updateStep: func() resource.TestStep {
				return serverlessClusterStep(clusterName, "BASIC", slsConfig{
					requestUnitLimit:     ptr(int64(1_000_000)),
					storageMibLimit:      ptr(int64(1024)),
					withEmptyIpAllowlist: ptr(true),
				})
			},
			finalCluster:      singleRegionClusterWithLimits("BASIC", 1_000_000, 1024),
			ignoreImportPaths: []string{"serverless.with_empty_ip_allowlist"},
		},
		{
			name: "attempt to add with_empty_ip_allowlist after creation",
			createStep: func() resource.TestStep {
				return serverlessClusterStep(clusterName, "BASIC", slsConfig{})
			},
			initialCluster: singleRegionClusterWithUnlimited("BASIC"),
			updateStep: func() resource.TestStep {
				step := serverlessClusterStep(clusterName, "BASIC", slsConfig{
					withEmptyIpAllowlist: ptr(true),
				})
				step.ExpectError = regexp.MustCompile("Cannot update with_empty_ip_allowlist")
				return step
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
			var versionUpdateCalled bool
			s.EXPECT().
				GetCluster(gomock.Any(), c.initialCluster.Id).
				DoAndReturn(func(context.Context, string) (*client.Cluster, *http.Response, error) {
					cluster := &c.initialCluster
					if updateCalled {
						cluster = &c.finalCluster
					} else if versionUpdateCalled {
						updatedVersionCluster := c.initialCluster
						updatedVersionCluster.CockroachVersion = c.finalCluster.CockroachVersion
						updatedVersionCluster.State = client.CLUSTERSTATETYPE_CREATED
						return ptr(updatedVersionCluster), nil, nil
					}
					return cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil
				}).AnyTimes()
			s.EXPECT().GetBackupConfiguration(gomock.Any(), c.initialCluster.Id).
				Return(initialBackupConfig, httpOk, nil).AnyTimes()

			var steps []resource.TestStep
			createStep := c.createStep()
			steps = append(steps, createStep)

			if createStep.ExpectError == nil {
				// Use DoAndReturn so that it's easy to set break points.
				s.EXPECT().
					CreateCluster(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *client.CreateClusterRequest) (*client.Cluster, *http.Response, error) {
						if c.validateCreate != nil {
							if err := c.validateCreate(req); err != nil {
								return nil, nil, err
							}
						}
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

					// Update version happens in a separate call prior to the main update
					if c.initialCluster.CockroachVersion != c.finalCluster.CockroachVersion {
						majorVersion := simplifyClusterVersion(c.finalCluster.CockroachVersion, false)
						s.EXPECT().
							UpdateCluster(gomock.Any(), c.initialCluster.Id, &client.UpdateClusterSpecification{
								CockroachVersion: ptr(majorVersion),
							}).
							DoAndReturn(func(_ context.Context, _ string, spec *client.UpdateClusterSpecification) (*client.Cluster, *http.Response, error) {
								versionUpdateCalled = true
								// This isn't what is actually returned but the
								// existing code only relies on the ID being in
								// the returned cluster. The version update is
								// expected to come back in the following
								// GetCluster calls
								return ptr(client.Cluster{Id: c.initialCluster.Id}), nil, nil
							})
					}

					s.EXPECT().
						UpdateCluster(gomock.Any(), gomock.Any(), gomock.Any()).
						DoAndReturn(func(_ context.Context, _ string, spec *client.UpdateClusterSpecification) (*client.Cluster, *http.Response, error) {
							if c.validateUpdate != nil {
								if err := c.validateUpdate(spec); err != nil {
									return nil, nil, err
								}
							}
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
func onDemandSingleRegionClusterNoLimitsStep(
	clusterName string, planType client.PlanType,
) resource.TestStep {
	return serverlessClusterStep(clusterName, planType, slsConfig{})
}

func onDemandSingleRegionClusterWithLimitsStep(
	clusterName string, planType client.PlanType, requestUnitLimit int64, storageMibLimit int64,
) resource.TestStep {
	return serverlessClusterStep(clusterName, planType, slsConfig{
		requestUnitLimit: ptr(requestUnitLimit),
		storageMibLimit:  ptr(storageMibLimit),
	})
}

func onDemandSingleRegionClusterWithUnlimitedStep(
	clusterName string, planType client.PlanType,
) resource.TestStep {
	return serverlessClusterStep(clusterName, planType, slsConfig{})
}

func provisionedSingleRegionClusterStep(
	clusterName string, planType client.PlanType, provisionedVirtualCpus int,
) resource.TestStep {
	return serverlessClusterStep(clusterName, planType, slsConfig{vcpus: ptr(provisionedVirtualCpus)})
}

type slsConfig struct {
	vcpus                *int
	requestUnitLimit     *int64
	storageMibLimit      *int64
	upgradeType          *client.UpgradeTypeType
	version              *string
	withEmptyIpAllowlist *bool
}

func serverlessClusterStep(
	clusterName string, planType client.PlanType, config slsConfig,
) resource.TestStep {
	testCheckFuncs := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(serverlessResourceName, "delete_protection", "false"),
		resource.TestCheckResourceAttr(serverlessDataSourceName, "delete_protection", "false"),
	}

	var planConfig string
	if planType != "" {
		planConfig = fmt.Sprintf("plan = \"%s\"", planType)
	}

	var provisionedVirtualCpusConfig string
	if config.vcpus != nil {
		provisionedVirtualCpusConfig = fmt.Sprintf("provisioned_virtual_cpus = %d", *config.vcpus)
		testCheckFuncs = append(
			testCheckFuncs,
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.usage_limits.provisioned_virtual_cpus", strconv.Itoa(*config.vcpus)),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.usage_limits.provisioned_virtual_cpus", strconv.Itoa(*config.vcpus)),
		)
	} else {
		testCheckFuncs = append(
			testCheckFuncs,
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.provisioned_virtual_cpus"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.provisioned_virtual_cpus"),
		)
	}

	var storageMibLimitConfig string
	if config.storageMibLimit != nil {
		storageMibLimitConfig = fmt.Sprintf("storage_mib_limit = %d", *config.storageMibLimit)
		testCheckFuncs = append(
			testCheckFuncs,
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.usage_limits.storage_mib_limit", strconv.Itoa(int(*config.storageMibLimit))),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.usage_limits.storage_mib_limit", strconv.Itoa(int(*config.storageMibLimit))),
		)
	} else {
		testCheckFuncs = append(
			testCheckFuncs,
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.storage_mib_limit"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.storage_mib_limit"),
		)
	}

	var requestUnitLimitConfig string
	if config.requestUnitLimit != nil {
		requestUnitLimitConfig = fmt.Sprintf("request_unit_limit = %d", *config.requestUnitLimit)
		testCheckFuncs = append(
			testCheckFuncs,
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.usage_limits.request_unit_limit", strconv.Itoa(int(*config.requestUnitLimit))),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.usage_limits.request_unit_limit", strconv.Itoa(int(*config.requestUnitLimit))),
		)
	} else {
		testCheckFuncs = append(
			testCheckFuncs,
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.request_unit_limit"),
		)
	}

	var upgradeTypeConfig string
	if config.upgradeType != nil {
		upgradeTypeConfig = fmt.Sprintf("upgrade_type = \"%s\"", *config.upgradeType)
		testCheckFuncs = append(
			testCheckFuncs,
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.upgrade_type", string(*config.upgradeType)),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.upgrade_type", string(*config.upgradeType)),
		)
	}

	var withEmptyIpAllowlistConfig string
	if config.withEmptyIpAllowlist != nil {
		withEmptyIpAllowlistConfig = fmt.Sprintf("with_empty_ip_allowlist = %t", *config.withEmptyIpAllowlist)
		testCheckFuncs = append(
			testCheckFuncs,
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.with_empty_ip_allowlist", strconv.FormatBool(*config.withEmptyIpAllowlist)),
		)
	}

	var versionConfig string
	if config.version != nil {

		version := *config.version
		startsWithVersionRE := regexp.MustCompile(fmt.Sprintf("^%s", version))
		versionConfig = fmt.Sprintf("cockroach_version = %q", version)
		testCheckFuncs = append(
			testCheckFuncs,
			resource.TestCheckResourceAttr(serverlessResourceName, "cockroach_version", version),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "cockroach_version", version),

			// If we check for a specific version, also check that full_version
			// starts with that version
			resource.TestMatchResourceAttr(serverlessResourceName, "full_version", startsWithVersionRE),
			resource.TestMatchResourceAttr(serverlessDataSourceName, "full_version", startsWithVersionRE),
		)
	} else {
		testCheckFuncs = append(
			testCheckFuncs,
			resource.TestCheckResourceAttrSet(serverlessResourceName, "full_version"),
		)
	}

	var usageLimitsConfig string
	if provisionedVirtualCpusConfig != "" || storageMibLimitConfig != "" || requestUnitLimitConfig != "" {
		usageLimitsConfig = fmt.Sprintf(`
					usage_limits = {
						%s
						%s
						%s
					}`, provisionedVirtualCpusConfig, requestUnitLimitConfig, storageMibLimitConfig)
	} else {
		usageLimitsConfig = `
						usage_limits = {}`
		testCheckFuncs = append(
			testCheckFuncs,
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.usage_limits.#", "0"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.usage_limits.#", "0"),
		)
	}

	return resource.TestStep{
		// Serverless cluster with provisioned resources.
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				%s
				serverless = {
					%s
					%s
					%s
				}
				regions = [{
					name = "us-central1"
				}]
				%s
			}

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.test.id
			}
			`, clusterName, planConfig, usageLimitsConfig, upgradeTypeConfig, withEmptyIpAllowlistConfig, versionConfig),
		Check: resource.ComposeTestCheckFunc(testCheckFuncs...),
	}
}

func provisionedMultiRegionClusterWithLimitStep(
	clusterName string, planType client.PlanType,
) resource.TestStep {
	var plan string
	if planType != "" {
		plan = fmt.Sprintf("plan = \"%s\"", planType)
	}

	return resource.TestStep{
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				%s
				serverless = {
					usage_limits = {
						provisioned_virtual_cpus = 6
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
						primary = false
					},
				]
			}

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.test.id
			}
			`, clusterName, plan),
		Check: resource.ComposeTestCheckFunc(
			makeDefaultServerlessResourceChecks(clusterName, "STANDARD"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.0.primary", "false"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.1.primary", "true"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.2.primary", "false"),
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.usage_limits.provisioned_virtual_cpus", "6"),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.storage_mib_limit"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.0.primary", "false"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.1.primary", "true"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.2.primary", "false"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.usage_limits.provisioned_virtual_cpus", "6"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.storage_mib_limit"),
		),
	}
}

// provisionedMultiRegionClusterUpdatedStep updates some of the fields in
// provisionedMultiRegionClusterWithLimitStep.
func provisionedMultiRegionClusterUpdatedStep(
	clusterName string, planType client.PlanType,
) resource.TestStep {
	var plan string
	if planType != "" {
		plan = fmt.Sprintf("plan = \"%s\"", planType)
	}

	return resource.TestStep{
		Config: fmt.Sprintf(`
			resource "cockroach_cluster" "test" {
				name           = "%s"
				cloud_provider = "GCP"
				%s
				serverless = {
					usage_limits = {
						provisioned_virtual_cpus = 8
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
			`, clusterName, plan),
		Check: resource.ComposeTestCheckFunc(
			makeDefaultServerlessResourceChecks(clusterName, "STANDARD"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.0.primary", "true"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.1.primary", "false"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(serverlessResourceName, "regions.2.primary", "false"),
			resource.TestCheckResourceAttr(serverlessResourceName, "serverless.usage_limits.provisioned_virtual_cpus", "8"),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(serverlessResourceName, "serverless.usage_limits.storage_mib_limit"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.#", "3"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.0.name", "europe-west1"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.0.primary", "true"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.1.name", "us-east1"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.1.primary", "false"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.2.name", "us-west2"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "regions.2.primary", "false"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "serverless.usage_limits.provisioned_virtual_cpus", "8"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.request_unit_limit"),
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.storage_mib_limit"),
			resource.TestCheckResourceAttr(serverlessDataSourceName, "delete_protection", "false"),
		),
	}
}

func legacyServerlessClusterWithSpendLimitStep(
	clusterName string, spendLimit int64,
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
			resource.TestCheckNoResourceAttr(serverlessDataSourceName, "serverless.usage_limits.provisioned_virtual_cpus"),
		),
	}
}

func makeDefaultServerlessResourceChecks(
	clusterName string, planType client.PlanType,
) resource.TestCheckFunc {
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
		Plan:             client.PLANTYPE_ADVANCED,
		CloudProvider:    client.CLOUDPROVIDERTYPE_GCP,
		State:            client.CLUSTERSTATETYPE_CREATED,
		UpgradeStatus:    client.CLUSTERUPGRADESTATUSTYPE_UPGRADE_AVAILABLE,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				NumVirtualCpus: 4,
				StorageGib:     15,
				MemoryGib:      8,
			},
		},
		Regions: []client.Region{
			{
				Name:               "us-central1",
				NodeCount:          1,
				PrivateEndpointDns: "test-private-endpoint-dns.gcp-us-central1.crdb-test.io",
			},
		},
		CidrRange: "172.28.0.0/16",
	}

	upgradingCluster := initialCluster
	upgradingCluster.CockroachVersion = latestClusterPatchVersion
	upgradingCluster.UpgradeStatus = client.CLUSTERUPGRADESTATUSTYPE_MAJOR_UPGRADE_RUNNING

	pendingCluster := upgradingCluster
	pendingCluster.UpgradeStatus = client.CLUSTERUPGRADESTATUSTYPE_PENDING_FINALIZATION

	finalizedCluster := pendingCluster
	finalizedCluster.UpgradeStatus = client.CLUSTERUPGRADESTATUSTYPE_FINALIZED

	firstUpdateCluster := finalizedCluster
	firstUpdateCluster.DeleteProtection = ptr(client.DELETEPROTECTIONSTATETYPE_ENABLED)

	secondUpdateCluster := firstUpdateCluster
	secondUpdateCluster.DeleteProtection = ptr(client.DELETEPROTECTIONSTATETYPE_DISABLED)

	scaledCluster := secondUpdateCluster
	scaledCluster.Config.Dedicated = &client.DedicatedHardwareConfig{}
	*scaledCluster.Config.Dedicated = *secondUpdateCluster.Config.Dedicated
	scaledCluster.Config.Dedicated.NumVirtualCpus = 8

	// Creation

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&initialCluster, nil, nil)
	// This test doesn't modify BackupConfiguration, so we just allow the mocked
	// to be called AnyTimes for simplicity.
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&initialCluster, httpOk, nil).Times(7)

	// Private endpoint services

	privateEndpointServices := client.PrivateEndpointServices{
		Services: []client.PrivateEndpointService{
			{RegionName: "us-central1", Status: client.PRIVATEENDPOINTSERVICESTATUSTYPE_AVAILABLE},
		},
	}
	s.EXPECT().CreatePrivateEndpointServices(gomock.Any(), clusterID).
		Return(&privateEndpointServices, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&initialCluster, httpOk, nil)
	s.EXPECT().ListPrivateEndpointServices(gomock.Any(), clusterID).
		Return(&privateEndpointServices, nil, nil).AnyTimes()

	// Upgrade

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, &client.UpdateClusterSpecification{
		CockroachVersion: ptr(latestClusterMajorVersion),
	}).DoAndReturn(
		func(context.Context, string, *client.UpdateClusterSpecification,
		) (*client.Cluster, *http.Response, error) {
			return &upgradingCluster, httpOk, nil
		},
	)

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&upgradingCluster, httpOk, nil)

	// Scale (no-op)

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).
		DoAndReturn(func(context.Context, string, *client.UpdateClusterSpecification,
		) (*client.Cluster, *http.Response, error) {
			return &pendingCluster, httpOk, nil
		})

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&pendingCluster, httpOk, nil).Times(3)

	// Finalize

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, &client.UpdateClusterSpecification{
		UpgradeStatus: ptr(client.CLUSTERUPGRADESTATUSTYPE_FINALIZED),
	}).DoAndReturn(func(context.Context, string, *client.UpdateClusterSpecification,
	) (*client.Cluster, *http.Response, error) {
		return &finalizedCluster, httpOk, nil
	})

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&finalizedCluster, httpOk, nil).Times(6)

	// Import state happens here

	// First Update

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).
		DoAndReturn(func(context.Context, string, *client.UpdateClusterSpecification,
		) (*client.Cluster, *http.Response, error) {
			currentCluster := &firstUpdateCluster
			return currentCluster, httpOk, nil
		})

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&firstUpdateCluster, httpOk, nil).Times(7)

	// Failed Delete Attempt

	// Second Update

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).
		DoAndReturn(func(context.Context, string, *client.UpdateClusterSpecification,
		) (*client.Cluster, *http.Response, error) {
			currentCluster := &secondUpdateCluster
			return currentCluster, httpOk, nil
		})

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&secondUpdateCluster, httpOk, nil).Times(6)

	// Scale

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).
		DoAndReturn(func(context.Context, string, *client.UpdateClusterSpecification,
		) (*client.Cluster, *http.Response, error) {
			currentCluster := &scaledCluster
			return currentCluster, httpOk, nil
		})

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&scaledCluster, httpOk, nil).AnyTimes()

	// Deletion

	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	scaleStep := resource.TestStep{
		PreConfig: func() {
			traceMessageStep("Scale the cluster")
		},
		Config: getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 8, nil, true),
		Check:  resource.TestCheckResourceAttr("cockroach_cluster.test", "dedicated.num_virtual_cpus", "8"),
	}

	testDedicatedClusterResource(t, clusterName, true, scaleStep)
}

// TestIntegrationDedicatedClusterMachineTypeMigration validates that when the
// server converts instance families with the same vCPU count, the provider
// preserves the user's configured value in state rather than showing drift.
func TestIntegrationDedicatedClusterMachineTypeMigration(t *testing.T) {
	// The API returns a different instance family than what the user configured.
	migratedCluster := client.Cluster{
		Id:               uuid.Nil.String(),
		Name:             "test-cluster",
		CockroachVersion: minSupportedClusterPatchVersion,
		Plan:             client.PLANTYPE_ADVANCED,
		CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
		State:            client.CLUSTERSTATETYPE_CREATED,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				MachineType:    "m7g.xlarge",
				NumVirtualCpus: 4,
				StorageGib:     15,
				MemoryGib:      8,
			},
		},
		Regions: []client.Region{
			{
				Name:      "us-east-1",
				NodeCount: 1,
			},
		},
	}

	t.Run("drift suppressed when vCPU count matches", func(t *testing.T) {
		clusterName := fmt.Sprintf("%s-dedicated-mt-%s", tfTestPrefix, GenerateRandomString(3))
		clusterID := migratedCluster.Id
		migratedCluster.Name = clusterName
		if os.Getenv(CockroachAPIKey) == "" {
			os.Setenv(CockroachAPIKey, "fake")
		}

		ctrl := gomock.NewController(t)
		s := mock_client.NewMockService(ctrl)
		defer HookGlobal(&NewService, func(c *client.Client) client.Service {
			return s
		})()

		const resourceName = "cockroach_cluster.test"

		s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
			Return(&migratedCluster, nil, nil)
		s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
			Return(initialBackupConfig, httpOk, nil).AnyTimes()
		s.EXPECT().GetCluster(gomock.Any(), clusterID).
			Return(&migratedCluster, httpOk, nil).AnyTimes()
		s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).
			Return(&migratedCluster, httpOk, nil)

		// Deletion
		s.EXPECT().DeleteCluster(gomock.Any(), clusterID).
			Return(nil, httpOk, nil)

		configWithMachineType := func(machineType string) string {
			return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
    name           = "%s"
    cloud_provider = "AWS"
    cockroach_version = "%s"
    dedicated = {
        machine_type = "%s"
        storage_gib  = 15
    }
    regions = [{
        name       = "us-east-1"
        node_count = 1
    }]
}
`, clusterName, minSupportedClusterMajorVersion, machineType)
		}

		resource.Test(t, resource.TestCase{
			IsUnitTest:               true,
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					PreConfig: func() {
						traceMessageStep("create with m6i.xlarge, server returns m7g.xlarge")
					},
					Config: configWithMachineType("m6i.xlarge"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr(resourceName, "dedicated.machine_type", "m6i.xlarge"),
						resource.TestCheckResourceAttr(resourceName, "dedicated.num_virtual_cpus", "4"),
					),
				},
				{
					PreConfig: func() {
						traceMessageStep("re-apply same config, no changes expected")
					},
					Config: configWithMachineType("m6i.xlarge"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr(resourceName, "dedicated.machine_type", "m6i.xlarge"),
					),
				},
				{
					PreConfig: func() {
						traceMessageStep("user updates config to m7g.xlarge")
					},
					Config: configWithMachineType("m7g.xlarge"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr(resourceName, "dedicated.machine_type", "m7g.xlarge"),
						resource.TestCheckResourceAttr(resourceName, "dedicated.num_virtual_cpus", "4"),
					),
				},
			},
		})

		// Verify the warning diagnostic directly. The terraform test framework
		// doesn't expose warnings, so we call loadClusterToTerraformState
		// to check the content.
		ctx := context.Background()
		plan := &CockroachCluster{
			DedicatedConfig: &DedicatedClusterConfig{
				MachineType: types.StringValue("m6i.xlarge"),
			},
		}
		var state CockroachCluster
		diags := loadClusterToTerraformState(ctx, &migratedCluster, nil, &state, plan)

		require.False(t, diags.HasError(), "expected no errors, got: %v", diags.Errors())
		require.True(t, diags.WarningsCount() > 0, "expected a warning diagnostic for machine type drift")

		found := false
		for _, d := range diags {
			if d.Summary() == "Machine type converted by server" {
				found = true
				require.Contains(t, d.Detail(), "m7g.xlarge")
				require.Contains(t, d.Detail(), "m6i.xlarge")
				require.Contains(t, d.Detail(), "same vCPU count: 4")
				break
			}
		}
		require.True(t, found, "expected warning with summary 'Machine type converted by server'")
		require.Equal(t, "m6i.xlarge", state.DedicatedConfig.MachineType.ValueString())
	})

	t.Run("drift not suppressed when vCPU counts differ", func(t *testing.T) {
		clusterName := fmt.Sprintf("%s-dedicated-mt-err-%s", tfTestPrefix, GenerateRandomString(3))
		clusterID := migratedCluster.Id
		if os.Getenv(CockroachAPIKey) == "" {
			os.Setenv(CockroachAPIKey, "fake")
		}

		ctrl := gomock.NewController(t)
		s := mock_client.NewMockService(ctrl)
		defer HookGlobal(&NewService, func(c *client.Client) client.Service {
			return s
		})()

		// Server returns a different vCPU count than configured.
		differentVCPUCluster := client.Cluster{
			Id:               clusterID,
			Name:             clusterName,
			CockroachVersion: minSupportedClusterPatchVersion,
			Plan:             client.PLANTYPE_ADVANCED,
			CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
			State:            client.CLUSTERSTATETYPE_CREATED,
			Config: client.ClusterConfig{
				Dedicated: &client.DedicatedHardwareConfig{
					MachineType:    "m7g.2xlarge",
					NumVirtualCpus: 8,
					StorageGib:     15,
					MemoryGib:      16,
				},
			},
			Regions: []client.Region{
				{
					Name:      "us-east-1",
					NodeCount: 1,
				},
			},
		}

		s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
			Return(&differentVCPUCluster, nil, nil)
		s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
			Return(initialBackupConfig, httpOk, nil).AnyTimes()
		s.EXPECT().GetCluster(gomock.Any(), clusterID).
			Return(&differentVCPUCluster, httpOk, nil).AnyTimes()
		s.EXPECT().DeleteCluster(gomock.Any(), clusterID).
			Return(nil, httpOk, nil)

		resource.Test(t, resource.TestCase{
			IsUnitTest:               true,
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					PreConfig: func() {
						traceMessageStep("create with m6i.xlarge, server returns m7g.2xlarge (different vCPUs)")
					},
					Config: fmt.Sprintf(`
resource "cockroach_cluster" "test" {
    name           = "%s"
    cloud_provider = "AWS"
    cockroach_version = "%s"
    dedicated = {
        machine_type = "m6i.xlarge"
        storage_gib  = 15
    }
    regions = [{
        name       = "us-east-1"
        node_count = 1
    }]
}
`, clusterName, minSupportedClusterMajorVersion),
					ExpectError: regexp.MustCompile(`Provider produced inconsistent result after apply`),
				},
			},
		})
	})
}

// TestAccDedicatedAWSClusterS3VpcEndpointId is an acceptance test that creates
// a real AWS Advanced cluster and verifies that s3_vpc_endpoint_id is populated.
func TestAccDedicatedAWSClusterS3VpcEndpointId(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-aws-vpce-%s", tfTestPrefix, GenerateRandomString(3))

	cfg := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  plan           = "ADVANCED"
  dedicated = {
    storage_gib      = 35
    num_virtual_cpus = 4
  }
  regions = [{
    name       = "us-east-1"
    node_count = 3
  }]
}

data "cockroach_cluster" "test" {
  id = cockroach_cluster.test.id
}
`, clusterName)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               false,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists("cockroach_cluster.test"),
					resource.TestMatchResourceAttr("cockroach_cluster.test", "regions.0.s3_vpc_endpoint_id", regexp.MustCompile(`^vpce-`)),
					resource.TestMatchResourceAttr("data.cockroach_cluster.test", "regions.0.s3_vpc_endpoint_id", regexp.MustCompile(`^vpce-`)),
				),
			},
		},
	})
}

// TestIntegrationDedicatedAWSClusterS3VpcEndpointId validates that
// s3_vpc_endpoint_id is populated for AWS Advanced clusters and
// accessible on both the resource and data source.
func TestIntegrationDedicatedAWSClusterS3VpcEndpointId(t *testing.T) {
	clusterName := fmt.Sprintf("%s-aws-vpce-%s", tfTestPrefix, GenerateRandomString(3))
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
		CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
		State:            client.CLUSTERSTATETYPE_CREATED,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				NumVirtualCpus: 4,
				StorageGib:     35,
				MemoryGib:      16,
			},
		},
		Regions: []client.Region{
			{
				Name:            "us-east-1",
				SqlDns:          "test.aws-us-east-1.crdb.io",
				UiDns:           "admin-test.aws-us-east-1.crdb.io",
				InternalDns:     "internal-test.aws-us-east-1.crdb.io",
				NodeCount:       3,
				S3VpcEndpointId: ptr("vpce-0abc123def456"),
			},
		},
	}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&cluster, nil, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	cfg := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  plan           = "ADVANCED"
  dedicated = {
    storage_gib     = 35
    num_virtual_cpus = 4
  }
  regions = [{
    name       = "us-east-1"
    node_count = 3
  }]
}

data "cockroach_cluster" "test" {
  id = cockroach_cluster.test.id
}
`, clusterName)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.s3_vpc_endpoint_id", "vpce-0abc123def456"),
					resource.TestCheckResourceAttr("data.cockroach_cluster.test", "regions.0.s3_vpc_endpoint_id", "vpce-0abc123def456"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "cloud_provider", "AWS"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "plan", "ADVANCED"),
				),
			},
		},
	})
}

// TestIntegrationDedicatedClusterBYOC validates that BYOC details are passed in
// the create request and are reflected in state.
func TestIntegrationDedicatedClusterBYOC(t *testing.T) {
	clusterName := fmt.Sprintf("%s-dedicated-byoc-%s", tfTestPrefix, GenerateRandomString(3))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	arn := "arn:aws:iam::123456789012:role/CRDB_Test"

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	initialCluster := client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: minSupportedClusterPatchVersion,
		Plan:             client.PLANTYPE_ADVANCED,
		CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
		State:            client.CLUSTERSTATETYPE_CREATED,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				NumVirtualCpus: 4,
				StorageGib:     15,
				MemoryGib:      8,
			},
		},
		Regions: []client.Region{{Name: "us-east-1", NodeCount: 1}},
		CustomerCloudAccount: &client.CustomerCloudAccount{
			Aws: &client.AwsCustomerCloudAccount{Arn: arn},
		},
	}

	// Validate CreateCluster receives BYOC details
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req *client.CreateClusterRequest) (*client.Cluster, *http.Response, error) {
			if req.Spec.CustomerCloudAccount == nil || req.Spec.CustomerCloudAccount.Aws == nil {
				return nil, nil, fmt.Errorf("missing BYOC details in create request")
			}
			if req.Spec.CustomerCloudAccount.Aws.Arn != arn {
				return nil, nil, fmt.Errorf("unexpected arn: %s", req.Spec.CustomerCloudAccount.Aws.Arn)
			}
			return &initialCluster, httpOk, nil
		},
	)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&initialCluster, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	cfg := `
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  dedicated = {
    storage_gib = 15
    num_virtual_cpus = 4
  }
  regions = [{
    name = "us-east-1"
    node_count = 1
  }]
  customer_cloud_account = {
    %s
  }
}

data "cockroach_cluster" "test" {
  id = cockroach_cluster.test.id
}
`

	// Test that a spec with multiple clouds is rejected.
	multipleCloudsSpec := fmt.Sprintf(`
	aws = { arn = "%s" }
	gcp = { service_account_email = "%s" }
`, arn, "test@example.com")
	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(cfg, clusterName, multipleCloudsSpec),
				ExpectError: regexp.MustCompile(`These attributes cannot be configured together:\s\[customer_cloud_account.aws,customer_cloud_account.gcp\]`),
			},
		},
	})

	// Test that a spec with one cloud is accepted.
	oneCloudSpec := fmt.Sprintf(`aws = { arn = "%s" }`, arn)
	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(cfg, clusterName, oneCloudSpec),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "customer_cloud_account.aws.arn", arn),
					resource.TestCheckResourceAttr("data.cockroach_cluster.test", "customer_cloud_account.aws.arn", arn),
				),
			},
		},
	})
}

// skipUnlessHeterogeneousEnabled skips heterogeneous machine type acceptance tests
// unless COCKROACH_HETEROGENEOUS_ENABLED is set, since the feature is limited-access
// and only usable in a flag-enabled organization.
func skipUnlessHeterogeneousEnabled(t *testing.T) {
	if os.Getenv(CockroachHeterogeneousEnabled) == "" {
		t.Skip("COCKROACH_HETEROGENEOUS_ENABLED is not set; skipping heterogeneous machine type acceptance test")
	}
}

// TestAccDedicatedClusterHeterogeneous creates a heterogeneous Advanced cluster
// (per-region num_virtual_cpus) against the real API and verifies the values
// round-trip with no drift on re-plan.
func TestAccDedicatedClusterHeterogeneous(t *testing.T) {
	skipUnlessHeterogeneousEnabled(t)
	t.Parallel()
	clusterName := fmt.Sprintf("%s-hetero-%s", tfTestPrefix, GenerateRandomString(3))

	cfg := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "GCP"
  plan           = "ADVANCED"
  dedicated = {
    storage_gib = 15
  }
  regions = [
    { name = "us-central1", node_count = 3, num_virtual_cpus = 4 },
    { name = "us-east1", node_count = 3, num_virtual_cpus = 8 },
    { name = "us-west1", node_count = 3, num_virtual_cpus = 8 },
  ]
}

data "cockroach_cluster" "test" {
  id = cockroach_cluster.test.id
}
`, clusterName)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               false,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists("cockroach_cluster.test"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.num_virtual_cpus", "4"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.1.num_virtual_cpus", "8"),
					// machine_type is derived and populated from the API even though
					// the user configured only num_virtual_cpus.
					resource.TestCheckResourceAttrSet("cockroach_cluster.test", "regions.0.machine_type"),
					resource.TestCheckResourceAttr("data.cockroach_cluster.test", "regions.0.num_virtual_cpus", "4"),
				),
			},
		},
	})
}

// TestAccDedicatedClusterHomogeneousToHeterogeneous creates a homogeneous Advanced
// cluster and then switches it to per-region (heterogeneous) sizing, exercising the
// real resize path where the cluster-wide num_virtual_cpus/machine_type aggregates
// shift.
func TestAccDedicatedClusterHomogeneousToHeterogeneous(t *testing.T) {
	skipUnlessHeterogeneousEnabled(t)
	t.Parallel()
	clusterName := fmt.Sprintf("%s-homo2het-%s", tfTestPrefix, GenerateRandomString(3))

	homogeneousCfg := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "GCP"
  plan           = "ADVANCED"
  dedicated = {
    storage_gib      = 15
    num_virtual_cpus = 4
  }
  regions = [
    { name = "us-central1", node_count = 3 },
    { name = "us-east1", node_count = 3 },
    { name = "us-west1", node_count = 3 },
  ]
}
`, clusterName)

	heterogeneousCfg := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "GCP"
  plan           = "ADVANCED"
  dedicated = {
    storage_gib = 15
  }
  regions = [
    { name = "us-central1", node_count = 3, num_virtual_cpus = 4 },
    { name = "us-east1", node_count = 3, num_virtual_cpus = 8 },
    { name = "us-west1", node_count = 3, num_virtual_cpus = 8 },
  ]
}
`, clusterName)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               false,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: homogeneousCfg,
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists("cockroach_cluster.test"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "dedicated.num_virtual_cpus", "4"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.#", "3"),
				),
			},
			{
				Config: heterogeneousCfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.1.num_virtual_cpus", "8"),
					resource.TestCheckResourceAttrSet("cockroach_cluster.test", "regions.1.machine_type"),
				),
			},
		},
	})
}

// TestIntegrationDedicatedClusterPerRegionMachineType validates that a
// heterogeneous Advanced cluster sends per-region machine specs
// (region_machine_specs) in the create request and omits the cluster-wide
// hardware.machine_spec, and that the per-region values round-trip into state.
func TestIntegrationDedicatedClusterPerRegionMachineType(t *testing.T) {
	clusterName := fmt.Sprintf("%s-dedicated-hetero-%s", tfTestPrefix, GenerateRandomString(3))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	vcpu4 := int32(4)
	vcpu8 := int32(8)
	// The API echoes a machine_type per region (as a real heterogeneous cluster
	// does), even though the user only configured num_virtual_cpus.
	mtSmall := "m6i.xlarge"
	mtLarge := "m6i.2xlarge"
	cluster := client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: minSupportedClusterPatchVersion,
		Plan:             client.PLANTYPE_ADVANCED,
		CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
		State:            client.CLUSTERSTATETYPE_CREATED,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				NumVirtualCpus: 4,
				StorageGib:     15,
				MemoryGib:      8,
			},
		},
		Regions: []client.Region{
			{Name: "us-east-1", NodeCount: 1, NumVirtualCpus: &vcpu4, MachineType: &mtSmall},
			{Name: "us-east-2", NodeCount: 1, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
			{Name: "us-west-2", NodeCount: 1, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
		},
	}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req *client.CreateClusterRequest) (*client.Cluster, *http.Response, error) {
			ded := req.Spec.Dedicated
			if ded == nil {
				return nil, nil, fmt.Errorf("missing dedicated spec")
			}
			if ded.RegionMachineSpecs == nil {
				return nil, nil, fmt.Errorf("expected region_machine_specs to be set")
			}
			specs := *ded.RegionMachineSpecs
			if specs["us-east-1"].NumVirtualCpus == nil || *specs["us-east-1"].NumVirtualCpus != 4 {
				return nil, nil, fmt.Errorf("unexpected us-east-1 spec: %+v", specs["us-east-1"])
			}
			if specs["us-west-2"].NumVirtualCpus == nil || *specs["us-west-2"].NumVirtualCpus != 8 {
				return nil, nil, fmt.Errorf("unexpected us-west-2 spec: %+v", specs["us-west-2"])
			}
			if len(specs) != 3 {
				return nil, nil, fmt.Errorf("expected 3 region specs, got %d", len(specs))
			}
			if ded.Hardware.MachineSpec != nil {
				return nil, nil, fmt.Errorf("expected hardware.machine_spec to be nil in heterogeneous mode")
			}
			return &cluster, httpOk, nil
		},
	)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(nil, httpOk, nil)

	cfg := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name              = "%s"
  cloud_provider    = "AWS"
  cockroach_version = "%s"
  dedicated = {
    storage_gib = 15
  }
  regions = [
    { name = "us-east-1", node_count = 1, num_virtual_cpus = 4 },
    { name = "us-east-2", node_count = 1, num_virtual_cpus = 8 },
    { name = "us-west-2", node_count = 1, num_virtual_cpus = 8 },
  ]
}
`, clusterName, minSupportedClusterMajorVersion)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.num_virtual_cpus", "4"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.1.num_virtual_cpus", "8"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.2.num_virtual_cpus", "8"),
					// machine_type is populated from the API even though the user
					// only set num_virtual_cpus, mirroring the cluster-wide path.
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.machine_type", "m6i.xlarge"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.1.machine_type", "m6i.2xlarge"),
				),
			},
			{
				// Re-applying the identical config must produce an empty plan. This
				// guards against the per-region machine_type showing a perpetual
				// "(known after apply)" for num_virtual_cpus users.
				Config:   cfg,
				PlanOnly: true,
			},
		},
	})
}

// TestIntegrationDedicatedClusterHeterogeneousToHomogeneous switches a
// heterogeneous Advanced cluster back to cluster-wide (homogeneous) sizing and
// asserts the resulting update request.
func TestIntegrationDedicatedClusterHeterogeneousToHomogeneous(t *testing.T) {
	clusterName := fmt.Sprintf("%s-het2homo-%s", tfTestPrefix, GenerateRandomString(3))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	vcpu4 := int32(4)
	vcpu8 := int32(8)
	mtSmall := "m6i.xlarge"
	mtLarge := "m6i.2xlarge"

	// Heterogeneous starting point: per-region specs, with the API reporting a
	// cluster-wide aggregate (the most common per-region value) in Config.Dedicated.
	heteroCluster := client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: minSupportedClusterPatchVersion,
		Plan:             client.PLANTYPE_ADVANCED,
		CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
		State:            client.CLUSTERSTATETYPE_CREATED,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{NumVirtualCpus: 8, StorageGib: 15, MemoryGib: 16},
		},
		Regions: []client.Region{
			{Name: "us-east-1", NodeCount: 1, NumVirtualCpus: &vcpu4, MachineType: &mtSmall},
			{Name: "us-east-2", NodeCount: 1, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
			{Name: "us-west-2", NodeCount: 1, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
		},
	}

	// After switching to cluster-wide 8 vCPU sizing the whole cluster is uniform.
	homoCluster := heteroCluster
	homoCluster.Config.Dedicated = &client.DedicatedHardwareConfig{NumVirtualCpus: 8, StorageGib: 15, MemoryGib: 16}
	homoCluster.Regions = []client.Region{
		{Name: "us-east-1", NodeCount: 1, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
		{Name: "us-east-2", NodeCount: 1, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
		{Name: "us-west-2", NodeCount: 1, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
	}

	// current tracks what GetCluster returns; UpdateCluster flips it to homogeneous.
	current := heteroCluster
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&heteroCluster, nil, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		DoAndReturn(func(context.Context, string) (*client.Cluster, *http.Response, error) {
			c := current
			return &c, httpOk, nil
		}).AnyTimes()

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, spec *client.UpdateClusterSpecification) (*client.Cluster, *http.Response, error) {
			ded := spec.Dedicated
			if ded == nil {
				return nil, nil, fmt.Errorf("expected a dedicated update spec")
			}
			// The switch-back must not carry forward per-region specs from state.
			if ded.RegionMachineSpecs != nil {
				return nil, nil, fmt.Errorf("expected no region_machine_specs when switching to homogeneous, got %+v", *ded.RegionMachineSpecs)
			}
			if ded.Hardware == nil || ded.Hardware.MachineSpec == nil {
				return nil, nil, fmt.Errorf("expected a cluster-wide hardware.machine_spec")
			}
			ms := ded.Hardware.MachineSpec
			if ms.NumVirtualCpus == nil || *ms.NumVirtualCpus != 8 {
				return nil, nil, fmt.Errorf("expected cluster-wide machine_spec of 8 vCPUs, got %+v", *ms)
			}
			current = homoCluster
			return &homoCluster, httpOk, nil
		})
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(nil, httpOk, nil)

	heteroCfg := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name              = "%s"
  cloud_provider    = "AWS"
  cockroach_version = "%s"
  dedicated = {
    storage_gib = 15
  }
  regions = [
    { name = "us-east-1", node_count = 1, num_virtual_cpus = 4 },
    { name = "us-east-2", node_count = 1, num_virtual_cpus = 8 },
    { name = "us-west-2", node_count = 1, num_virtual_cpus = 8 },
  ]
}
`, clusterName, minSupportedClusterMajorVersion)

	homoCfg := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name              = "%s"
  cloud_provider    = "AWS"
  cockroach_version = "%s"
  dedicated = {
    storage_gib      = 15
    num_virtual_cpus = 8
  }
  regions = [
    { name = "us-east-1", node_count = 1 },
    { name = "us-east-2", node_count = 1 },
    { name = "us-west-2", node_count = 1 },
  ]
}
`, clusterName, minSupportedClusterMajorVersion)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: heteroCfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.num_virtual_cpus", "4"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.1.num_virtual_cpus", "8"),
				),
			},
			{
				Config: homoCfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "dedicated.num_virtual_cpus", "8"),
					// The per-region machine fields must clear on the switch-back.
					resource.TestCheckNoResourceAttr("cockroach_cluster.test", "regions.0.num_virtual_cpus"),
				),
			},
		},
	})
}

// TestIntegrationDedicatedClusterPerRegionUnknownValue verifies that the
// all-or-none validation does not fire falsely when a per-region machine value is
// an unresolved reference at plan time (unknown, not null). Here one region's
// num_virtual_cpus comes from a terraform_data output, which is unknown until
// apply.
func TestIntegrationDedicatedClusterPerRegionUnknownValue(t *testing.T) {
	clusterName := fmt.Sprintf("%s-hetero-unknown-%s", tfTestPrefix, GenerateRandomString(3))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	vcpu4 := int32(4)
	vcpu8 := int32(8)
	cluster := client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: minSupportedClusterPatchVersion,
		Plan:             client.PLANTYPE_ADVANCED,
		CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
		State:            client.CLUSTERSTATETYPE_CREATED,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{NumVirtualCpus: 4, StorageGib: 15, MemoryGib: 8},
		},
		Regions: []client.Region{
			{Name: "us-east-1", NodeCount: 1, NumVirtualCpus: &vcpu4},
			{Name: "us-east-2", NodeCount: 1, NumVirtualCpus: &vcpu8},
			{Name: "us-west-2", NodeCount: 1, NumVirtualCpus: &vcpu8},
		},
	}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&cluster, nil, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(nil, httpOk, nil)

	// terraform_data.cpus.output is unknown at plan time (the resource doesn't
	// exist yet), so regions[1].num_virtual_cpus is unknown during validation.
	cfg := fmt.Sprintf(`
resource "terraform_data" "cpus" {
  input = 8
}
resource "cockroach_cluster" "test" {
  name              = "%s"
  cloud_provider    = "AWS"
  cockroach_version = "%s"
  dedicated = { storage_gib = 15 }
  regions = [
    { name = "us-east-1", node_count = 1, num_virtual_cpus = 4 },
    { name = "us-east-2", node_count = 1, num_virtual_cpus = terraform_data.cpus.output },
    { name = "us-west-2", node_count = 1, num_virtual_cpus = 8 },
  ]
}
`, clusterName, minSupportedClusterMajorVersion)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.1.num_virtual_cpus", "8"),
				),
			},
		},
	})
}

// TestIntegrationDedicatedClusterPerRegionMachineTypeValidation validates the
// mutual-exclusivity and all-or-none rules for per-region machine types. These
// fail during plan/validation, before any API call.
func TestIntegrationDedicatedClusterPerRegionMachineTypeValidation(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	clusterName := fmt.Sprintf("%s-hetero-val-%s", tfTestPrefix, GenerateRandomString(3))

	cases := []struct {
		name        string
		config      string
		expectError *regexp.Regexp
	}{
		{
			name: "both num_virtual_cpus and machine_type in a region",
			config: fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  dedicated = { storage_gib = 15 }
  regions = [
    { name = "us-east-1", node_count = 1, num_virtual_cpus = 4, machine_type = "m6i.xlarge" },
  ]
}
`, clusterName),
			expectError: regexp.MustCompile(`mutually exclusive within a region`),
		},
		{
			name: "mixing cluster-wide and per-region",
			config: fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  dedicated = { storage_gib = 15, num_virtual_cpus = 4 }
  regions = [
    { name = "us-east-1", node_count = 1, num_virtual_cpus = 4 },
  ]
}
`, clusterName),
			expectError: regexp.MustCompile(`not both`),
		},
		{
			name: "partial per-region violates all-or-none",
			config: fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  dedicated = { storage_gib = 15 }
  regions = [
    { name = "us-east-1", node_count = 1, num_virtual_cpus = 4 },
    { name = "us-east-2", node_count = 1, num_virtual_cpus = 8 },
    { name = "us-west-2", node_count = 1 },
  ]
}
`, clusterName),
			expectError: regexp.MustCompile(`every region must`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      tc.config,
						ExpectError: tc.expectError,
					},
				},
			})
		})
	}
}

// TestIntegrationDedicatedClusterPerRegionToClusterWide is a regression test for
// the case where a heterogeneous cluster is switched back to cluster-wide sizing.
// Plan modifiers carry the prior per-region values forward into the plan even
// after the user removes them from config, so the switch must be decided from
// config: the update must send the cluster-wide hardware.machine_spec (not stale
// region_machine_specs), and the post-apply read must not resurrect per-region
// values (which would cause an "inconsistent result after apply" error).
func TestIntegrationDedicatedClusterPerRegionToClusterWide(t *testing.T) {
	clusterName := fmt.Sprintf("%s-hetero-switch-%s", tfTestPrefix, GenerateRandomString(3))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	vcpu4 := int32(4)
	vcpu8 := int32(8)
	heteroCluster := client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: minSupportedClusterPatchVersion,
		Plan:             client.PLANTYPE_ADVANCED,
		CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
		State:            client.CLUSTERSTATETYPE_CREATED,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{NumVirtualCpus: 4, StorageGib: 15, MemoryGib: 8},
		},
		Regions: []client.Region{
			{Name: "us-east-1", NodeCount: 1, NumVirtualCpus: &vcpu4},
			{Name: "us-east-2", NodeCount: 1, NumVirtualCpus: &vcpu8},
			{Name: "us-west-2", NodeCount: 1, NumVirtualCpus: &vcpu8},
		},
	}
	// After the switch, the cluster is homogeneous at 8 vCPUs. The API still
	// reports per-region num_virtual_cpus (all equal), which must NOT leak into
	// state because the user is no longer managing them per region.
	homoCluster := client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: minSupportedClusterPatchVersion,
		Plan:             client.PLANTYPE_ADVANCED,
		CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
		State:            client.CLUSTERSTATETYPE_CREATED,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{NumVirtualCpus: 8, StorageGib: 15, MemoryGib: 16},
		},
		Regions: []client.Region{
			{Name: "us-east-1", NodeCount: 1, NumVirtualCpus: &vcpu8},
			{Name: "us-east-2", NodeCount: 1, NumVirtualCpus: &vcpu8},
			{Name: "us-west-2", NodeCount: 1, NumVirtualCpus: &vcpu8},
		},
	}
	current := &heteroCluster

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&heteroCluster, nil, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), clusterID).DoAndReturn(
		func(_ context.Context, _ string) (*client.Cluster, *http.Response, error) {
			return current, httpOk, nil
		}).AnyTimes()
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, spec *client.UpdateClusterSpecification) (*client.Cluster, *http.Response, error) {
			ded := spec.Dedicated
			if ded == nil || ded.Hardware == nil {
				return nil, nil, fmt.Errorf("missing dedicated/hardware in update")
			}
			if ded.RegionMachineSpecs != nil {
				return nil, nil, fmt.Errorf("expected region_machine_specs to be nil when switching to cluster-wide, got %+v", *ded.RegionMachineSpecs)
			}
			if ded.Hardware.MachineSpec == nil || ded.Hardware.MachineSpec.NumVirtualCpus == nil || *ded.Hardware.MachineSpec.NumVirtualCpus != 8 {
				return nil, nil, fmt.Errorf("expected cluster-wide machine_spec num_virtual_cpus=8, got %+v", ded.Hardware.MachineSpec)
			}
			current = &homoCluster
			return &homoCluster, httpOk, nil
		})
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(nil, httpOk, nil)

	heteroCfg := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name              = "%s"
  cloud_provider    = "AWS"
  cockroach_version = "%s"
  dedicated = { storage_gib = 15 }
  regions = [
    { name = "us-east-1", node_count = 1, num_virtual_cpus = 4 },
    { name = "us-east-2", node_count = 1, num_virtual_cpus = 8 },
    { name = "us-west-2", node_count = 1, num_virtual_cpus = 8 },
  ]
}
`, clusterName, minSupportedClusterMajorVersion)

	homoCfg := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name              = "%s"
  cloud_provider    = "AWS"
  cockroach_version = "%s"
  dedicated = { storage_gib = 15, num_virtual_cpus = 8 }
  regions = [
    { name = "us-east-1", node_count = 1 },
    { name = "us-east-2", node_count = 1 },
    { name = "us-west-2", node_count = 1 },
  ]
}
`, clusterName, minSupportedClusterMajorVersion)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: heteroCfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.num_virtual_cpus", "4"),
				),
			},
			{
				Config: homoCfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "dedicated.num_virtual_cpus", "8"),
					resource.TestCheckNoResourceAttr("cockroach_cluster.test", "regions.0.num_virtual_cpus"),
				),
			},
		},
	})
}

// TestIntegrationDedicatedClusterPerRegionResize is the regression test for the
// resize inconsistency: changing one region's num_virtual_cpus must not leave a
// stale machine_type in the plan (carried by SuppressMachineTypeDrift) that the
// resize invalidates, which would produce "Provider produced inconsistent result
// after apply".
func TestIntegrationDedicatedClusterPerRegionResize(t *testing.T) {
	clusterName := fmt.Sprintf("%s-hetero-resize-%s", tfTestPrefix, GenerateRandomString(3))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	vcpu4 := int32(4)
	vcpu8 := int32(8)
	mtSmall := "m6i.xlarge"
	mtLarge := "m6i.2xlarge"
	baseCluster := func(eastVCPUs int32, eastMT string) client.Cluster {
		e := eastVCPUs
		return client.Cluster{
			Id:               clusterID,
			Name:             clusterName,
			CockroachVersion: minSupportedClusterPatchVersion,
			Plan:             client.PLANTYPE_ADVANCED,
			CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
			State:            client.CLUSTERSTATETYPE_CREATED,
			Config: client.ClusterConfig{
				Dedicated: &client.DedicatedHardwareConfig{NumVirtualCpus: eastVCPUs, StorageGib: 15, MemoryGib: 8},
			},
			Regions: []client.Region{
				{Name: "us-east-1", NodeCount: 1, NumVirtualCpus: &e, MachineType: &eastMT},
				{Name: "us-east-2", NodeCount: 1, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
				{Name: "us-west-2", NodeCount: 1, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
			},
		}
	}
	hetero := baseCluster(vcpu4, mtSmall)
	resized := baseCluster(vcpu8, mtLarge)
	// The resize changes the cluster-wide derived memory too.
	resized.Config.Dedicated.MemoryGib = 16
	current := &hetero

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&hetero, nil, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), clusterID).DoAndReturn(
		func(_ context.Context, _ string) (*client.Cluster, *http.Response, error) {
			return current, httpOk, nil
		}).AnyTimes()
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, spec *client.UpdateClusterSpecification) (*client.Cluster, *http.Response, error) {
			ded := spec.Dedicated
			if ded == nil || ded.RegionMachineSpecs == nil {
				return nil, nil, fmt.Errorf("expected region_machine_specs on resize")
			}
			specs := *ded.RegionMachineSpecs
			if specs["us-east-1"].NumVirtualCpus == nil || *specs["us-east-1"].NumVirtualCpus != 8 {
				return nil, nil, fmt.Errorf("expected us-east-1 resized to 8 vCPUs, got %+v", specs["us-east-1"])
			}
			current = &resized
			return &resized, httpOk, nil
		})
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(nil, httpOk, nil)

	cfg := func(eastVCPUs int) string {
		return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name              = "%s"
  cloud_provider    = "AWS"
  cockroach_version = "%s"
  dedicated = { storage_gib = 15 }
  regions = [
    { name = "us-east-1", node_count = 1, num_virtual_cpus = %d },
    { name = "us-east-2", node_count = 1, num_virtual_cpus = 8 },
    { name = "us-west-2", node_count = 1, num_virtual_cpus = 8 },
  ]
}
`, clusterName, minSupportedClusterMajorVersion, eastVCPUs)
	}

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(4),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.num_virtual_cpus", "4"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.machine_type", "m6i.xlarge"),
				),
			},
			{
				// Resize us-east-1 4 -> 8. Must apply cleanly (no inconsistent
				// result) and land the new machine_type in state.
				Config: cfg(8),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.num_virtual_cpus", "8"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.machine_type", "m6i.2xlarge"),
				),
			},
		},
	})
}

// TestIntegrationDedicatedClusterPerRegionRemove is the regression test for the
// region-removal path: the CC API reports the cluster-wide num_virtual_cpus/
// machine_type as the most common per-region value, so removing a region can shift
// that aggregate. ModifyPlan must treat a removal as a resize and mark the
// cluster-wide derived fields unknown, or apply fails with "inconsistent result
// after apply". Starts at 4 regions so the removal leaves 3 (never a 2-region
// cluster).
func TestIntegrationDedicatedClusterPerRegionRemove(t *testing.T) {
	clusterName := fmt.Sprintf("%s-hetero-remove-%s", tfTestPrefix, GenerateRandomString(3))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	vcpu4 := int32(4)
	vcpu8 := int32(8)
	mtSmall := "m6i.xlarge"
	mtLarge := "m6i.2xlarge"
	// Initial cluster: two 8-vCPU and two 4-vCPU regions; the API reports the
	// cluster-wide aggregate as 8 / m6i.2xlarge / 16 GiB.
	base := client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: minSupportedClusterPatchVersion,
		Plan:             client.PLANTYPE_ADVANCED,
		CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
		State:            client.CLUSTERSTATETYPE_CREATED,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{MachineType: mtLarge, NumVirtualCpus: 8, StorageGib: 15, MemoryGib: 16},
		},
		Regions: []client.Region{
			{Name: "us-east-1", NodeCount: 3, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
			{Name: "us-east-2", NodeCount: 3, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
			{Name: "us-west-1", NodeCount: 3, NumVirtualCpus: &vcpu4, MachineType: &mtSmall},
			{Name: "us-west-2", NodeCount: 3, NumVirtualCpus: &vcpu4, MachineType: &mtSmall},
		},
	}
	// After removing us-east-1 (an 8-vCPU region), the plurality flips to 4, so the
	// API's reported cluster-wide aggregate shifts to 4 / m6i.xlarge / 8 GiB.
	afterRemoval := client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: minSupportedClusterPatchVersion,
		Plan:             client.PLANTYPE_ADVANCED,
		CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
		State:            client.CLUSTERSTATETYPE_CREATED,
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{MachineType: mtSmall, NumVirtualCpus: 4, StorageGib: 15, MemoryGib: 8},
		},
		Regions: []client.Region{
			{Name: "us-east-2", NodeCount: 3, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
			{Name: "us-west-1", NodeCount: 3, NumVirtualCpus: &vcpu4, MachineType: &mtSmall},
			{Name: "us-west-2", NodeCount: 3, NumVirtualCpus: &vcpu4, MachineType: &mtSmall},
		},
	}
	current := &base

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&base, nil, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), clusterID).DoAndReturn(
		func(_ context.Context, _ string) (*client.Cluster, *http.Response, error) {
			return current, httpOk, nil
		}).AnyTimes()
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, spec *client.UpdateClusterSpecification) (*client.Cluster, *http.Response, error) {
			ded := spec.Dedicated
			if ded == nil || ded.RegionNodes == nil {
				return nil, nil, fmt.Errorf("expected region_nodes on region removal")
			}
			if _, ok := (*ded.RegionNodes)["us-east-1"]; ok {
				return nil, nil, fmt.Errorf("expected region_nodes to omit the removed region us-east-1, got %v", *ded.RegionNodes)
			}
			current = &afterRemoval
			return &afterRemoval, httpOk, nil
		})
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(nil, httpOk, nil)

	fourRegions := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name              = "%s"
  cloud_provider    = "AWS"
  cockroach_version = "%s"
  dedicated = { storage_gib = 15 }
  regions = [
    { name = "us-east-1", node_count = 3, num_virtual_cpus = 8 },
    { name = "us-east-2", node_count = 3, num_virtual_cpus = 8 },
    { name = "us-west-1", node_count = 3, num_virtual_cpus = 4 },
    { name = "us-west-2", node_count = 3, num_virtual_cpus = 4 },
  ]
}
`, clusterName, minSupportedClusterMajorVersion)
	threeRegions := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name              = "%s"
  cloud_provider    = "AWS"
  cockroach_version = "%s"
  dedicated = { storage_gib = 15 }
  regions = [
    { name = "us-east-2", node_count = 3, num_virtual_cpus = 8 },
    { name = "us-west-1", node_count = 3, num_virtual_cpus = 4 },
    { name = "us-west-2", node_count = 3, num_virtual_cpus = 4 },
  ]
}
`, clusterName, minSupportedClusterMajorVersion)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fourRegions,
				Check:  resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.#", "4"),
			},
			{
				// Removing us-east-1 shifts the cluster-wide aggregate; the apply
				// must succeed (no inconsistent result) and land 3 regions.
				Config: threeRegions,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.#", "3"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.num_virtual_cpus", "8"),
				),
			},
		},
	})
}

// TestIntegrationDedicatedClusterClusterWideResize guards the same inconsistency
// on the cluster-wide path: a num_virtual_cpus resize must recompute the
// cluster-wide machine_type rather than carry the stale value forward.
func TestIntegrationDedicatedClusterClusterWideResize(t *testing.T) {
	clusterName := fmt.Sprintf("%s-homo-resize-%s", tfTestPrefix, GenerateRandomString(3))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	homo := func(vcpus int32, mt string) client.Cluster {
		return client.Cluster{
			Id:               clusterID,
			Name:             clusterName,
			CockroachVersion: minSupportedClusterPatchVersion,
			Plan:             client.PLANTYPE_ADVANCED,
			CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
			State:            client.CLUSTERSTATETYPE_CREATED,
			Config: client.ClusterConfig{
				Dedicated: &client.DedicatedHardwareConfig{MachineType: mt, NumVirtualCpus: vcpus, StorageGib: 15, MemoryGib: 8},
			},
			Regions: []client.Region{{Name: "us-east-1", NodeCount: 1}},
		}
	}
	small := homo(4, "m6i.xlarge")
	large := homo(8, "m6i.2xlarge")
	large.Config.Dedicated.MemoryGib = 16
	current := &small

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&small, nil, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), clusterID).DoAndReturn(
		func(_ context.Context, _ string) (*client.Cluster, *http.Response, error) {
			return current, httpOk, nil
		}).AnyTimes()
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, spec *client.UpdateClusterSpecification) (*client.Cluster, *http.Response, error) {
			ded := spec.Dedicated
			if ded == nil || ded.Hardware == nil || ded.Hardware.MachineSpec == nil {
				return nil, nil, fmt.Errorf("expected cluster-wide machine_spec on resize")
			}
			if ded.Hardware.MachineSpec.NumVirtualCpus == nil || *ded.Hardware.MachineSpec.NumVirtualCpus != 8 {
				return nil, nil, fmt.Errorf("expected cluster-wide resize to 8 vCPUs, got %+v", ded.Hardware.MachineSpec)
			}
			current = &large
			return &large, httpOk, nil
		})
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(nil, httpOk, nil)

	cfg := func(vcpus int) string {
		return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name              = "%s"
  cloud_provider    = "AWS"
  cockroach_version = "%s"
  dedicated = { storage_gib = 15, num_virtual_cpus = %d }
  regions = [{ name = "us-east-1", node_count = 1 }]
}
`, clusterName, minSupportedClusterMajorVersion, vcpus)
	}

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(4),
				Check:  resource.TestCheckResourceAttr("cockroach_cluster.test", "dedicated.machine_type", "m6i.xlarge"),
			},
			{
				Config: cfg(8),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "dedicated.num_virtual_cpus", "8"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "dedicated.machine_type", "m6i.2xlarge"),
				),
			},
		},
	})
}

// TestIntegrationDedicatedClusterHeterogeneousWithCMEKRegion verifies that when a
// region managed by a separate resource (e.g. CMEK) is present on the cluster but
// not in this resource's config, an update sends region_machine_specs covering only
// the config regions while region_nodes includes the externally-managed region. The
// API contract (SDK: "unnamed regions retain their current machine type") makes the
// partial map safe.
func TestIntegrationDedicatedClusterHeterogeneousWithCMEKRegion(t *testing.T) {
	clusterName := fmt.Sprintf("%s-hetero-cmek-%s", tfTestPrefix, GenerateRandomString(3))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	vcpu4 := int32(4)
	vcpu8 := int32(8)
	mtSmall := "m6i.xlarge"
	mtLarge := "m6i.2xlarge"
	// The cluster has three config-managed regions plus us-central-1, which is
	// managed externally (CMEK) and never appears in this resource's config.
	mkCluster := func(eastNodes int32) client.Cluster {
		e := eastNodes
		return client.Cluster{
			Id:               clusterID,
			Name:             clusterName,
			CockroachVersion: minSupportedClusterPatchVersion,
			Plan:             client.PLANTYPE_ADVANCED,
			CloudProvider:    client.CLOUDPROVIDERTYPE_AWS,
			State:            client.CLUSTERSTATETYPE_CREATED,
			Config: client.ClusterConfig{
				Dedicated: &client.DedicatedHardwareConfig{NumVirtualCpus: 4, StorageGib: 15, MemoryGib: 8},
			},
			Regions: []client.Region{
				{Name: "us-east-1", NodeCount: e, NumVirtualCpus: &vcpu4, MachineType: &mtSmall},
				{Name: "us-east-2", NodeCount: 3, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
				{Name: "us-west-2", NodeCount: 3, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
				{Name: "us-central-1", NodeCount: 3, NumVirtualCpus: &vcpu8, MachineType: &mtLarge},
			},
		}
	}
	base := mkCluster(3)
	resized := mkCluster(4)
	current := &base

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&base, nil, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), clusterID).DoAndReturn(
		func(_ context.Context, _ string) (*client.Cluster, *http.Response, error) {
			return current, httpOk, nil
		}).AnyTimes()
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, spec *client.UpdateClusterSpecification) (*client.Cluster, *http.Response, error) {
			ded := spec.Dedicated
			if ded == nil || ded.RegionNodes == nil || ded.RegionMachineSpecs == nil {
				return nil, nil, fmt.Errorf("expected region_nodes and region_machine_specs to be set")
			}
			nodes := *ded.RegionNodes
			specs := *ded.RegionMachineSpecs
			if _, ok := nodes["us-central-1"]; !ok {
				return nil, nil, fmt.Errorf("expected region_nodes to include the CMEK region us-central-1, got %v", nodes)
			}
			if _, ok := specs["us-central-1"]; ok {
				return nil, nil, fmt.Errorf("expected region_machine_specs to omit the CMEK region us-central-1, got %v", specs)
			}
			if len(specs) != 3 {
				return nil, nil, fmt.Errorf("expected 3 region_machine_specs (config regions only), got %d", len(specs))
			}
			current = &resized
			return &resized, httpOk, nil
		})
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(nil, httpOk, nil)

	cfg := func(eastNodes int) string {
		return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name              = "%s"
  cloud_provider    = "AWS"
  cockroach_version = "%s"
  dedicated = { storage_gib = 15 }
  regions = [
    { name = "us-east-1", node_count = %d, num_virtual_cpus = 4 },
    { name = "us-east-2", node_count = 3, num_virtual_cpus = 8 },
    { name = "us-west-2", node_count = 3, num_virtual_cpus = 8 },
  ]
}
`, clusterName, minSupportedClusterMajorVersion, eastNodes)
	}

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(3),
				Check: resource.ComposeTestCheckFunc(
					// The externally-managed region is filtered out of this
					// resource's state.
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.#", "3"),
				),
			},
			{
				// Change us-east-1 node_count to force a region update; the CMEK
				// region flows into region_nodes but not region_machine_specs.
				Config: cfg(4),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.node_count", "4"),
				),
			},
		},
	})
}

func testDedicatedClusterResource(
	t *testing.T, clusterName string, useMock bool, additionalSteps ...resource.TestStep,
) {
	const (
		resourceName   = "cockroach_cluster.test"
		dataSourceName = "data.cockroach_cluster.test"
	)

	// We can't easily check the exact patch version here because the patch
	// version for real clusters changes too often.
	startsWithMinSupportedMajorVersionRE := regexp.MustCompile(fmt.Sprintf("^%s", minSupportedClusterMajorVersion))
	startsWithLatestMajorVersionRE := regexp.MustCompile(fmt.Sprintf("^%s", latestClusterMajorVersion))
	testSteps := []resource.TestStep{
		{
			PreConfig: func() {
				traceMessageStep("create a cluster")
			},
			Config: getTestDedicatedClusterResourceConfig(clusterName, minSupportedClusterMajorVersion, false, 4, nil, true),
			Check: resource.ComposeTestCheckFunc(
				testCheckCockroachClusterExists(resourceName),
				resource.TestCheckResourceAttr(resourceName, "name", clusterName),
				resource.TestCheckResourceAttrSet(resourceName, "cloud_provider"),
				resource.TestCheckResourceAttrSet(resourceName, "cockroach_version"),
				resource.TestCheckResourceAttr(resourceName, "plan", "ADVANCED"),
				resource.TestCheckResourceAttr(resourceName, "dedicated.cidr_range", "172.28.0.0/16"),
				resource.TestMatchResourceAttr(resourceName, "full_version", startsWithMinSupportedMajorVersionRE),
				resource.TestCheckResourceAttr(resourceName, "dedicated.supports_cluster_virtualization", "true"),
				resource.TestCheckResourceAttr(dataSourceName, "name", clusterName),
				resource.TestCheckResourceAttrSet(dataSourceName, "cloud_provider"),
				resource.TestCheckResourceAttrSet(dataSourceName, "cockroach_version"),
				resource.TestCheckResourceAttr(dataSourceName, "plan", "ADVANCED"),
				resource.TestCheckResourceAttr(dataSourceName, "dedicated.cidr_range", "172.28.0.0/16"),
				resource.TestMatchResourceAttr(dataSourceName, "full_version", startsWithMinSupportedMajorVersionRE),
				resource.TestCheckResourceAttrSet(dataSourceName, "regions.0.private_endpoint_dns"),
			),
		},
		{
			PreConfig: func() {
				traceMessageStep("update the version")
			},
			Config: getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, true, 4, nil, true),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(resourceName, "cockroach_version", latestClusterMajorVersion),
				resource.TestCheckResourceAttr(dataSourceName, "cockroach_version", latestClusterMajorVersion),
				resource.TestMatchResourceAttr(resourceName, "full_version", startsWithLatestMajorVersionRE),
				resource.TestMatchResourceAttr(dataSourceName, "full_version", startsWithLatestMajorVersionRE),

				// We check for private_endpoint_dns here because it's not available during initial cluster
				// creation. The private_endpoint_dns field is only populated after a PrivateEndpointService
				// resource is created.
				// Note: The data source check is done in the previous test step since the data source
				// has a dependency on cockroach_private_endpoint_services.test, so the read operation
				// won't happen until after the PrivateEndpointService resource is created.
				resource.TestCheckResourceAttrSet(resourceName, "regions.0.private_endpoint_dns"),
			),
		},
		{
			PreConfig: func() {
				traceMessageStep("test import")
			},
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
		{
			PreConfig: func() {
				traceMessageStep("enable delete protection")
			},
			Config: getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 4, ptr(true), true),
			Check:  resource.TestCheckResourceAttr(resourceName, "delete_protection", "true"),
		},
		{
			// Delete step that fails since delete protection is enabled.
			PreConfig: func() {
				traceMessageStep("check failure due to delete protection being enabled")
			},
			Config:      " ",
			Destroy:     true,
			ExpectError: regexp.MustCompile(".*Cannot destroy cluster with delete protection enabled*"),
		},
		{
			PreConfig: func() {
				traceMessageStep("Unset delete protection")
			},
			Config: getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 4, ptr(false), true),
			Check:  resource.TestCheckResourceAttr(resourceName, "delete_protection", "false"),
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

func getTestDedicatedClusterResourceConfig(
	name, version string,
	finalize bool,
	vcpus int,
	deleteProtectionEnabled *bool,
	privateEndpointServicesEnabled bool,
) string {
	var deleteProtectionConfig string
	if deleteProtectionEnabled != nil {
		deleteProtectionConfig = fmt.Sprintf("\ndelete_protection = %t\n", *deleteProtectionEnabled)
	}

	var privateEndpointServicesResource string
	var dataSourceDependsOn string
	if privateEndpointServicesEnabled {
		privateEndpointServicesResource =
			`resource "cockroach_private_endpoint_services" "test" {
				cluster_id = cockroach_cluster.test.id
			}`
		dataSourceDependsOn = "depends_on = [cockroach_private_endpoint_services.test]\n"
	}

	config := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
    name           = "%s"
    cloud_provider = "GCP"
    cockroach_version = "%s"
    dedicated = {
	  storage_gib = 15
	  num_virtual_cpus = %d
	  cidr_range = "172.28.0.0/16"
	  supports_cluster_virtualization = true
    }
	regions = [{
		name: "us-central1"
		node_count: 1
	}]
	%s
}

%s

data "cockroach_cluster" "test" {
    id = cockroach_cluster.test.id
	%s
}
`, name, version, vcpus, deleteProtectionConfig, privateEndpointServicesResource, dataSourceDependsOn)

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

type backupCreateConfig struct {
	includeBackupConfig bool
	enabled             *bool
	retention           *int32
	frequency           *int32
}

func getTestClusterWithBackupConfig(clusterName string, config backupCreateConfig) string {

	backupConfigString := ""
	if config.includeBackupConfig {
		backupConfigOptions := ""

		if config.enabled != nil {
			backupConfigOptions = backupConfigOptions + fmt.Sprintf(`
    enabled = %t
`, *config.enabled)
		}
		if config.frequency != nil {
			backupConfigOptions = backupConfigOptions + fmt.Sprintf(`
    frequency_minutes = %d
`, *config.frequency)
		}
		if config.retention != nil {
			backupConfigOptions = backupConfigOptions + fmt.Sprintf(`
    retention_days = %d
`, *config.retention)
		}

		backupConfigString = fmt.Sprintf(`
  backup_config = {%s}
`, backupConfigOptions)
	}

	return fmt.Sprintf(`
	resource "cockroach_cluster" "test" {
		name           = "%s"
		cloud_provider = "GCP"
		plan = "STANDARD"
		serverless = {
			usage_limits = {
				provisioned_virtual_cpus = 2
			}
		}
		regions = [{
			name = "us-central1"
		}] %s
	}
	`, clusterName, backupConfigString)
}

type parentIDTestConfig struct {
	folderID   *string
	folderName *string
}

func getTestClusterWithParentFolder(clusterName string, config parentIDTestConfig) string {

	parentConfigString := ""
	parentIDString := ""
	if config.folderID != nil {
		parentIDString = fmt.Sprintf(`
	parent_id = "%s"
`, *config.folderID)
	} else if config.folderName != nil {
		parentConfigString = fmt.Sprintf(`
	resource "cockroach_folder" "parent" {
		name = "%s"
		parent_id = "root"
	}
`, *config.folderName)
		parentIDString = `
	parent_id = cockroach_folder.parent.id
`
	}

	return fmt.Sprintf(`%s
	resource "cockroach_cluster" "test" {
		name           = "%s"
		cloud_provider = "GCP"
		plan = "STANDARD"
		serverless = {
			usage_limits = {
				provisioned_virtual_cpus = 2
			}
		}
		regions = [{
			name = "us-central1"
		}] %s
	}
	`, parentConfigString, clusterName, parentIDString)
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

func TestGetManagedRegionsS3VpcEndpointId(t *testing.T) {
	t.Run("AWS cluster with s3_vpc_endpoint_id", func(t *testing.T) {
		vpceid := "vpce-0abc123def456"
		regions := []client.Region{
			{
				Name:               "us-east-1",
				SqlDns:             "test.aws-us-east-1.crdb.io",
				UiDns:              "admin-test.aws-us-east-1.crdb.io",
				InternalDns:        "internal-test.aws-us-east-1.crdb.io",
				PrivateEndpointDns: "",
				NodeCount:          3,
				S3VpcEndpointId:    &vpceid,
			},
		}
		plan := []Region{
			{Name: types.StringValue("us-east-1")},
		}
		var diags diag.Diagnostics
		result := getManagedRegions(&regions, plan, client.CLOUDPROVIDERTYPE_AWS, &diags)
		require.Len(t, result, 1)
		require.Equal(t, "vpce-0abc123def456", result[0].S3VpcEndpointId.ValueString())
	})

	t.Run("GCP cluster without s3_vpc_endpoint_id", func(t *testing.T) {
		regions := []client.Region{
			{
				Name:               "us-central1",
				SqlDns:             "test.gcp-us-central1.crdb.io",
				UiDns:              "admin-test.gcp-us-central1.crdb.io",
				PrivateEndpointDns: "private-test.crdb.io",
				NodeCount:          3,
			},
		}
		plan := []Region{
			{Name: types.StringValue("us-central1")},
		}
		var diags diag.Diagnostics
		result := getManagedRegions(&regions, plan, client.CLOUDPROVIDERTYPE_GCP, &diags)
		require.Len(t, result, 1)
		require.Equal(t, "", result[0].S3VpcEndpointId.ValueString())
	})

	t.Run("Data source import with no plan", func(t *testing.T) {
		vpceid := "vpce-0def456abc789"
		regions := []client.Region{
			{
				Name:            "us-west-2",
				NodeCount:       3,
				S3VpcEndpointId: &vpceid,
			},
		}
		var diags diag.Diagnostics
		result := getManagedRegions(&regions, nil, client.CLOUDPROVIDERTYPE_AWS, &diags)
		require.Len(t, result, 1)
		require.Equal(t, "vpce-0def456abc789", result[0].S3VpcEndpointId.ValueString())
	})
}

func TestBuildRegionMachineSpecs(t *testing.T) {
	t.Run("no per-region specs returns false", func(t *testing.T) {
		regions := []Region{
			{Name: types.StringValue("us-east-1")},
			{Name: types.StringValue("us-east-2")},
			{Name: types.StringValue("us-west-2")},
		}
		specs, ok := buildRegionMachineSpecs(regions)
		require.False(t, ok)
		require.Empty(t, specs)
	})

	t.Run("num_virtual_cpus per region", func(t *testing.T) {
		regions := []Region{
			{Name: types.StringValue("us-east-1"), NumVirtualCpus: types.Int64Value(4)},
			{Name: types.StringValue("us-east-2"), NumVirtualCpus: types.Int64Value(8)},
			{Name: types.StringValue("us-west-2"), NumVirtualCpus: types.Int64Value(8)},
		}
		specs, ok := buildRegionMachineSpecs(regions)
		require.True(t, ok)
		require.Len(t, specs, 3)
		require.Equal(t, int32(4), *specs["us-east-1"].NumVirtualCpus)
		require.Nil(t, specs["us-east-1"].MachineType)
		require.Equal(t, int32(8), *specs["us-west-2"].NumVirtualCpus)
	})

	t.Run("machine_type per region", func(t *testing.T) {
		regions := []Region{
			{Name: types.StringValue("us-east-1"), MachineType: types.StringValue("m6i.xlarge")},
			{Name: types.StringValue("us-east-2"), MachineType: types.StringValue("m6i.2xlarge")},
			{Name: types.StringValue("us-west-2"), MachineType: types.StringValue("m6i.2xlarge")},
		}
		specs, ok := buildRegionMachineSpecs(regions)
		require.True(t, ok)
		require.Len(t, specs, 3)
		require.Equal(t, "m6i.xlarge", *specs["us-east-1"].MachineType)
		require.Nil(t, specs["us-east-1"].NumVirtualCpus)
	})
}

func TestGetManagedRegionsMachineType(t *testing.T) {
	vcpu4 := int32(4)
	vcpu8 := int32(8)
	mtEast := "m6i.xlarge"
	mtWest := "m6i.2xlarge"
	apiRegions := func() []client.Region {
		return []client.Region{
			{Name: "us-east-1", NodeCount: 3, NumVirtualCpus: &vcpu4, MachineType: &mtEast},
			{Name: "us-east-2", NodeCount: 3, NumVirtualCpus: &vcpu8, MachineType: &mtWest},
			{Name: "us-west-2", NodeCount: 3, NumVirtualCpus: &vcpu8, MachineType: &mtWest},
		}
	}

	t.Run("num_virtual_cpus user gets machine_type populated from API", func(t *testing.T) {
		// The recommended path: user sets only num_virtual_cpus per region. Both
		// fields must be populated from the API (mirroring the cluster-wide path)
		// so machine_type isn't left null (which would show perpetual drift).
		plan := []Region{
			{Name: types.StringValue("us-east-1"), NumVirtualCpus: types.Int64Value(4)},
			{Name: types.StringValue("us-east-2"), NumVirtualCpus: types.Int64Value(8)},
			{Name: types.StringValue("us-west-2"), NumVirtualCpus: types.Int64Value(8)},
		}
		var diags diag.Diagnostics
		regions := apiRegions()
		result := getManagedRegions(&regions, plan, client.CLOUDPROVIDERTYPE_AWS, &diags)
		require.Len(t, result, 3)
		require.Equal(t, int64(4), result[0].NumVirtualCpus.ValueInt64())
		require.Equal(t, "m6i.xlarge", result[0].MachineType.ValueString())
		require.Equal(t, int64(8), result[1].NumVirtualCpus.ValueInt64())
		require.Equal(t, "m6i.2xlarge", result[1].MachineType.ValueString())
	})

	t.Run("falls back to plan value when API omits a field", func(t *testing.T) {
		// The server echoes nil for a per-region field the user set (e.g. on a
		// feature-flag/older-server edge). State must keep the planned value so we
		// don't produce an "inconsistent result after apply".
		regions := []client.Region{
			{Name: "us-east-1", NodeCount: 3}, // NumVirtualCpus/MachineType nil
		}
		plan := []Region{
			{Name: types.StringValue("us-east-1"), NumVirtualCpus: types.Int64Value(4)},
		}
		var diags diag.Diagnostics
		result := getManagedRegions(&regions, plan, client.CLOUDPROVIDERTYPE_AWS, &diags)
		require.Len(t, result, 1)
		require.Equal(t, int64(4), result[0].NumVirtualCpus.ValueInt64())
		require.True(t, result[0].MachineType.IsNull())
	})

	t.Run("null when plan does not use per-region (homogeneous)", func(t *testing.T) {
		plan := []Region{
			{Name: types.StringValue("us-east-1")},
			{Name: types.StringValue("us-east-2")},
			{Name: types.StringValue("us-west-2")},
		}
		var diags diag.Diagnostics
		regions := apiRegions()
		result := getManagedRegions(&regions, plan, client.CLOUDPROVIDERTYPE_AWS, &diags)
		require.Len(t, result, 3)
		require.True(t, result[0].NumVirtualCpus.IsNull())
		require.True(t, result[0].MachineType.IsNull())
	})

	t.Run("datasource/import populates from API", func(t *testing.T) {
		var diags diag.Diagnostics
		regions := apiRegions()
		result := getManagedRegions(&regions, nil, client.CLOUDPROVIDERTYPE_AWS, &diags)
		require.Len(t, result, 3)
		require.Equal(t, int64(4), result[0].NumVirtualCpus.ValueInt64())
		require.Equal(t, "m6i.xlarge", result[0].MachineType.ValueString())
	})

	t.Run("machine_type drift suppressed when vCPU count matches", func(t *testing.T) {
		converted := "m7g.xlarge" // same 4 vCPUs as configured m6i.xlarge
		regions := []client.Region{
			{Name: "us-east-1", NodeCount: 3, NumVirtualCpus: &vcpu4, MachineType: &converted},
		}
		plan := []Region{
			{Name: types.StringValue("us-east-1"), MachineType: types.StringValue("m6i.xlarge")},
		}
		var diags diag.Diagnostics
		result := getManagedRegions(&regions, plan, client.CLOUDPROVIDERTYPE_AWS, &diags)
		require.Len(t, result, 1)
		require.Equal(t, "m6i.xlarge", result[0].MachineType.ValueString())
		require.True(t, diags.WarningsCount() > 0)
	})
}

func TestResolveMachinePlan(t *testing.T) {
	v4 := types.Int64Value(4)
	v8 := types.Int64Value(8)
	mtA := types.StringValue("m6i.xlarge")
	mtB := types.StringValue("m6i.2xlarge")
	nullI := types.Int64Null()
	nullS := types.StringNull()

	t.Run("num_virtual_cpus drives, unchanged -> machine_type from state", func(t *testing.T) {
		outV, outM, rz := resolveMachinePlan(v4, nullS, v4, mtA, true)
		require.Equal(t, v4, outV)
		require.Equal(t, mtA, outM)
		require.False(t, rz)
	})

	t.Run("num_virtual_cpus drives, changed -> machine_type unknown", func(t *testing.T) {
		outV, outM, rz := resolveMachinePlan(v8, nullS, v4, mtA, true)
		require.Equal(t, v8, outV)
		require.True(t, outM.IsUnknown())
		require.True(t, rz)
	})

	t.Run("num_virtual_cpus drives, unchanged -> machine_type taken by name", func(t *testing.T) {
		// The state machine_type comes from the same-named region, so a reordered
		// list resolves to the correct value (mtB here), not a neighbor's.
		outV, outM, rz := resolveMachinePlan(v8, nullS, v8, mtB, true)
		require.Equal(t, v8, outV)
		require.Equal(t, mtB, outM)
		require.False(t, rz)
	})

	t.Run("machine_type drives, unchanged -> num_virtual_cpus from state", func(t *testing.T) {
		outV, outM, rz := resolveMachinePlan(nullI, mtA, v4, mtA, true)
		require.Equal(t, v4, outV)
		require.Equal(t, mtA, outM)
		require.False(t, rz)
	})

	t.Run("machine_type drives, changed -> num_virtual_cpus unknown", func(t *testing.T) {
		outV, outM, rz := resolveMachinePlan(nullI, mtB, v4, mtA, true)
		require.True(t, outV.IsUnknown())
		require.Equal(t, mtB, outM)
		require.True(t, rz)
	})

	t.Run("new region (no state) -> sibling unknown", func(t *testing.T) {
		outV, outM, rz := resolveMachinePlan(v4, nullS, nullI, nullS, false)
		require.Equal(t, v4, outV)
		require.True(t, outM.IsUnknown())
		require.True(t, rz)
	})

	t.Run("neither set in config -> keep state", func(t *testing.T) {
		outV, outM, rz := resolveMachinePlan(nullI, nullS, v4, mtA, true)
		require.Equal(t, v4, outV)
		require.Equal(t, mtA, outM)
		require.False(t, rz)
	})
}

func TestCoordinateDedicatedMachinePlan(t *testing.T) {
	v4 := types.Int64Value(4)
	v8 := types.Int64Value(8)
	mtSmall := types.StringValue("m6i.xlarge")
	mtLarge := types.StringValue("m6i.2xlarge")
	nullI := types.Int64Null()
	nullS := types.StringNull()

	region := func(name string, vcpus types.Int64, mt types.String) Region {
		return Region{Name: types.StringValue(name), NumVirtualCpus: vcpus, MachineType: mt}
	}
	ded := func(vcpus types.Int64, mt types.String, mem float64) *DedicatedClusterConfig {
		return &DedicatedClusterConfig{NumVirtualCpus: vcpus, MachineType: mt, MemoryGib: types.Float64Value(mem)}
	}

	t.Run("homogeneous resize recomputes machine_type and memory", func(t *testing.T) {
		config := &CockroachCluster{DedicatedConfig: ded(v8, nullS, 8), Regions: []Region{region("us-east-1", nullI, nullS)}}
		state := &CockroachCluster{DedicatedConfig: ded(v4, mtSmall, 8), Regions: []Region{region("us-east-1", nullI, nullS)}}
		plan := &CockroachCluster{DedicatedConfig: ded(v8, mtSmall, 8), Regions: []Region{region("us-east-1", nullI, nullS)}}

		require.True(t, coordinateDedicatedMachinePlan(config, plan, state))
		require.Equal(t, int64(8), plan.DedicatedConfig.NumVirtualCpus.ValueInt64())
		require.True(t, plan.DedicatedConfig.MachineType.IsUnknown())
		require.True(t, plan.DedicatedConfig.MemoryGib.IsUnknown())
	})

	t.Run("homogeneous no-op leaves the plan untouched", func(t *testing.T) {
		config := &CockroachCluster{DedicatedConfig: ded(v4, nullS, 8), Regions: []Region{region("us-east-1", nullI, nullS)}}
		state := &CockroachCluster{DedicatedConfig: ded(v4, mtSmall, 8), Regions: []Region{region("us-east-1", nullI, nullS)}}
		plan := &CockroachCluster{DedicatedConfig: ded(v4, mtSmall, 8), Regions: []Region{region("us-east-1", nullI, nullS)}}

		require.False(t, coordinateDedicatedMachinePlan(config, plan, state))
		require.Equal(t, mtSmall, plan.DedicatedConfig.MachineType)
		require.Equal(t, float64(8), plan.DedicatedConfig.MemoryGib.ValueFloat64())
	})

	t.Run("heterogeneous per-region resize recomputes sibling and aggregates", func(t *testing.T) {
		config := &CockroachCluster{DedicatedConfig: ded(nullI, nullS, 8), Regions: []Region{region("us-east-1", v8, nullS), region("us-east-2", v8, nullS)}}
		state := &CockroachCluster{DedicatedConfig: ded(v4, mtSmall, 8), Regions: []Region{region("us-east-1", v4, mtSmall), region("us-east-2", v8, mtLarge)}}
		plan := &CockroachCluster{DedicatedConfig: ded(v4, mtSmall, 8), Regions: []Region{region("us-east-1", v8, mtSmall), region("us-east-2", v8, mtLarge)}}

		require.True(t, coordinateDedicatedMachinePlan(config, plan, state))
		require.True(t, plan.Regions[0].MachineType.IsUnknown()) // resized region's sibling
		require.Equal(t, mtLarge, plan.Regions[1].MachineType)   // unchanged region kept
		require.True(t, plan.DedicatedConfig.NumVirtualCpus.IsUnknown())
		require.True(t, plan.DedicatedConfig.MachineType.IsUnknown())
		require.True(t, plan.DedicatedConfig.MemoryGib.IsUnknown())
	})

	t.Run("heterogeneous reorder corrects machine_type by name", func(t *testing.T) {
		// Same region set, reordered. The index-wise plan modifiers carried the
		// neighbor's machine_type; coordination must correct each by name and NOT
		// treat this as a resize.
		config := &CockroachCluster{DedicatedConfig: ded(nullI, nullS, 8), Regions: []Region{region("us-east-2", v8, nullS), region("us-east-1", v4, nullS)}}
		state := &CockroachCluster{DedicatedConfig: ded(v4, mtSmall, 8), Regions: []Region{region("us-east-1", v4, mtSmall), region("us-east-2", v8, mtLarge)}}
		plan := &CockroachCluster{DedicatedConfig: ded(v4, mtSmall, 8), Regions: []Region{region("us-east-2", v8, mtSmall), region("us-east-1", v4, mtLarge)}}

		require.True(t, coordinateDedicatedMachinePlan(config, plan, state))
		require.Equal(t, mtLarge, plan.Regions[0].MachineType)       // us-east-2 corrected
		require.Equal(t, mtSmall, plan.Regions[1].MachineType)       // us-east-1 corrected
		require.False(t, plan.DedicatedConfig.MemoryGib.IsUnknown()) // not a resize
	})

	t.Run("heterogeneous removal recomputes cluster-wide aggregates", func(t *testing.T) {
		config := &CockroachCluster{DedicatedConfig: ded(nullI, nullS, 8), Regions: []Region{region("us-east-2", v8, nullS), region("us-west-2", v4, nullS)}}
		state := &CockroachCluster{DedicatedConfig: ded(v8, mtLarge, 16), Regions: []Region{region("us-east-1", v8, mtLarge), region("us-east-2", v8, mtLarge), region("us-west-2", v4, mtSmall)}}
		plan := &CockroachCluster{DedicatedConfig: ded(v8, mtLarge, 16), Regions: []Region{region("us-east-2", v8, mtLarge), region("us-west-2", v4, mtSmall)}}

		require.True(t, coordinateDedicatedMachinePlan(config, plan, state))
		require.True(t, plan.DedicatedConfig.NumVirtualCpus.IsUnknown())
		require.True(t, plan.DedicatedConfig.MachineType.IsUnknown())
		require.True(t, plan.DedicatedConfig.MemoryGib.IsUnknown())
	})

	t.Run("switch heterogeneous to cluster-wide clears per-region fields", func(t *testing.T) {
		config := &CockroachCluster{DedicatedConfig: ded(v8, nullS, 8), Regions: []Region{region("us-east-1", nullI, nullS), region("us-east-2", nullI, nullS)}}
		state := &CockroachCluster{DedicatedConfig: ded(v4, mtSmall, 8), Regions: []Region{region("us-east-1", v4, mtSmall), region("us-east-2", v8, mtLarge)}}
		plan := &CockroachCluster{DedicatedConfig: ded(v8, mtSmall, 8), Regions: []Region{region("us-east-1", v4, mtSmall), region("us-east-2", v8, mtLarge)}}

		require.True(t, coordinateDedicatedMachinePlan(config, plan, state))
		require.True(t, plan.Regions[0].NumVirtualCpus.IsNull())
		require.True(t, plan.Regions[0].MachineType.IsNull())
		require.True(t, plan.Regions[1].NumVirtualCpus.IsNull())
		require.True(t, plan.Regions[1].MachineType.IsNull())
		require.Equal(t, int64(8), plan.DedicatedConfig.NumVirtualCpus.ValueInt64())
		require.True(t, plan.DedicatedConfig.MachineType.IsUnknown())
		require.True(t, plan.DedicatedConfig.MemoryGib.IsUnknown())
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

func TestIsUpgrade(t *testing.T) {
	_, err := isUpgrade("v22.2", "foo")
	require.Error(t, err)

	upgrade, err := isUpgrade("v24.1", "v24.2")
	require.NoError(t, err)
	require.True(t, upgrade)

	upgrade, err = isUpgrade("v24.1", "v24.3")
	require.NoError(t, err)
	require.True(t, upgrade)

	upgrade, err = isUpgrade("v23.2", "v24.1")
	require.NoError(t, err)
	require.True(t, upgrade)

	upgrade, err = isUpgrade("v24.2", "v24.2")
	require.NoError(t, err)
	require.False(t, upgrade)

	upgrade, err = isUpgrade("v24.2", "v24.1")
	require.NoError(t, err)
	require.False(t, upgrade)

	upgrade, err = isUpgrade("v24.2", "v23.2")
	require.NoError(t, err)
	require.False(t, upgrade)
}

func TestIsDowngrade(t *testing.T) {
	_, err := isDowngrade("v22.2", "foo")
	require.Error(t, err)

	upgrade, err := isDowngrade("v24.2", "v24.1")
	require.NoError(t, err)
	require.True(t, upgrade)

	upgrade, err = isDowngrade("v24.2", "v23.2")
	require.NoError(t, err)
	require.True(t, upgrade)

	upgrade, err = isDowngrade("v24.2", "v24.2")
	require.NoError(t, err)
	require.False(t, upgrade)

	upgrade, err = isDowngrade("v24.1", "v24.2")
	require.NoError(t, err)
	require.False(t, upgrade)

	upgrade, err = isDowngrade("v24.1", "v24.3")
	require.NoError(t, err)
	require.False(t, upgrade)

	upgrade, err = isDowngrade("v23.2", "v24.1")
	require.NoError(t, err)
	require.False(t, upgrade)
}
func TestDerivePlanType(t *testing.T) {
	tests := []struct {
		name     string
		cluster  CockroachCluster
		expected client.PlanType
		err      error
	}{
		{
			name: "Explicit Plan: Advanced",
			cluster: CockroachCluster{
				Plan: types.StringValue("ADVANCED"),
			},
			expected: client.PLANTYPE_ADVANCED,
			err:      nil,
		},
		{
			name: "Explicit Plan: Standard",
			cluster: CockroachCluster{
				Plan: types.StringValue("STANDARD"),
			},
			expected: client.PLANTYPE_STANDARD,
			err:      nil,
		},
		{
			name: "Explicit Plan: BASIC",
			cluster: CockroachCluster{
				Plan: types.StringValue("BASIC"),
			},
			expected: client.PLANTYPE_BASIC,
			err:      nil,
		},
		{
			name: "Explicit Plan: Invalid",
			cluster: CockroachCluster{
				Plan: types.StringValue("asdf"),
			},
			expected: "",
			err:      fmt.Errorf(`invalid plan type "asdf"`),
		},
		{
			name: "Dedicated Config",
			cluster: CockroachCluster{
				DedicatedConfig: &DedicatedClusterConfig{},
			},
			expected: client.PLANTYPE_ADVANCED,
			err:      nil,
		},
		{
			name: "Serverless Config with Provisioned Resources",
			cluster: CockroachCluster{
				ServerlessConfig: &ServerlessClusterConfig{
					UsageLimits: &UsageLimits{
						ProvisionedVirtualCpus: types.Int64Value(4),
					},
				},
			},
			expected: client.PLANTYPE_STANDARD,
			err:      nil,
		},
		{
			name: "Serverless Config without Provisioned Resources",
			cluster: CockroachCluster{
				ServerlessConfig: &ServerlessClusterConfig{
					UsageLimits: &UsageLimits{},
				},
			},
			expected: client.PLANTYPE_BASIC,
			err:      nil,
		},
		{
			name:     "Underivable Plan",
			cluster:  CockroachCluster{},
			expected: "",
			err:      fmt.Errorf("could not derive plan type, plan must contain either a ServerlessConfig or a DedicatedConfig"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planType, err := derivePlanType(&tt.cluster)
			if tt.err != nil {
				require.Equal(t, tt.err, err)
			} else {
				require.Equal(t, tt.expected, planType)
			}
		})
	}
}

// TestAccClusterWithLabels is an acceptance test focused only on testing
// the labels parameter. It will be skipped if TF_ACC isn't set.
func TestAccClusterWithLabels(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-serverless-%s", tfTestPrefix, GenerateRandomString(2))
	folderName := fmt.Sprintf("%s-parent-f-%s", tfTestPrefix, GenerateRandomString(2))
	labels := map[string]string{
		"environment": "test",
		"cost_center": "12345",
		"status":      "active",
	}
	newLabels := map[string]string{
		"environment": "test",
		"cost_center": "23456",
		"status":      "inactive",
	}
	invalidLabels := map[string]string{
		"234environment": "prod",
		"status":         "<>!",
	}
	labelsAboveLimit := make(map[string]string)
	for i := 0; i < validators.ResourceLabelLimit+1; i++ {
		labelsAboveLimit[fmt.Sprintf("key%d", i)] = "value"
	}
	testClusterWithLabels(t, folderName, clusterName, labelsAboveLimit, invalidLabels, labels, newLabels, false /* useMock */)
}

// TestIntegrationClusterWithLabels is an integration test focused only on
// testing the labels parameter.
func TestIntegrationClusterWithLabels(t *testing.T) {
	clusterID := uuid.Nil.String()
	clusterName := fmt.Sprintf("%s-serverless-%s", tfTestPrefix, GenerateRandomString(2))
	labels := map[string]string{
		"environment": "test",
		"cost_center": "12345",
		"status":      "active",
	}
	newLabels := map[string]string{
		"environment": "test",
		"cost_center": "23456",
		"status":      "inactive",
	}
	invalidLabels := map[string]string{
		"234environment": "prod",
		"status":         "<>!",
	}
	labelsAboveLimit := make(map[string]string)
	for i := 0; i < validators.ResourceLabelLimit+1; i++ {
		labelsAboveLimit[fmt.Sprintf("key%d", i)] = "value"
	}

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
		CockroachVersion: latestClusterPatchVersion,
		CloudProvider:    "GCP",
		State:            "CREATED",
		Plan:             "STANDARD",
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{
				UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
				UsageLimits: &client.UsageLimits{
					ProvisionedVirtualCpus: ptr(int64(2)),
				},
				RoutingId: "routing-id",
			},
		},
		Regions: []client.Region{
			{
				Name: "us-central1",
			},
		},
		Labels: labels,
	}

	folder := client.FolderResource{
		Name:         "folder987",
		ResourceId:   "00000000-0000-0000-0000-000000000001",
		ParentId:     "root",
		ResourceType: client.FOLDERRESOURCETYPETYPE_FOLDER,
	}

	var regions []string
	for _, region := range cluster.Regions {
		regions = append(regions, region.Name)
	}
	primaryRegion := ""

	// Create folder. Cluster will get moved to this folder at the end of the test.
	s.EXPECT().CreateFolder(gomock.Any(), gomock.Any()).Return(&folder, httpOk, nil)

	// Create cluster with labels.
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&cluster, nil, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(initialBackupConfig, httpOk, nil).AnyTimes()

	// Clear cluster labels.
	s.EXPECT().GetFolder(gomock.Any(), folder.ResourceId).Return(&folder, httpOk, nil).Times(2)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&cluster, httpOk, nil).Times(3)

	clusterWithNoLabels := cluster
	clusterWithNoLabels.Labels = map[string]string{}
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&clusterWithNoLabels, httpOk, nil)
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, &client.UpdateClusterSpecification{
		Plan:   &cluster.Plan,
		Labels: &map[string]string{},
		Serverless: &client.ServerlessClusterUpdateSpecification{
			PrimaryRegion: &primaryRegion,
			Regions:       &regions,
			UsageLimits:   cluster.Config.Serverless.UsageLimits,
		},
	}).Return(&clusterWithNoLabels, httpOk, nil)

	// Add labels back to cluster.
	s.EXPECT().GetFolder(gomock.Any(), folder.ResourceId).Return(&folder, httpOk, nil).Times(2)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&clusterWithNoLabels, httpOk, nil).Times(2)

	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&cluster, httpOk, nil)
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, &client.UpdateClusterSpecification{
		Plan:   &cluster.Plan,
		Labels: &labels,
		Serverless: &client.ServerlessClusterUpdateSpecification{
			PrimaryRegion: &primaryRegion,
			Regions:       &regions,
			UsageLimits:   cluster.Config.Serverless.UsageLimits,
		},
	}).Return(&cluster, httpOk, nil)

	// Not passing in labels leaves labels unchanged.
	s.EXPECT().GetFolder(gomock.Any(), folder.ResourceId).Return(&folder, httpOk, nil).Times(2)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&cluster, httpOk, nil).Times(2)

	// Update cluster labels.
	s.EXPECT().GetFolder(gomock.Any(), folder.ResourceId).Return(&folder, httpOk, nil).Times(2)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&cluster, httpOk, nil).Times(2)

	clusterWithUpdatedLabels := cluster
	clusterWithUpdatedLabels.Labels = newLabels
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&clusterWithUpdatedLabels, httpOk, nil)
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, &client.UpdateClusterSpecification{
		Plan:   &cluster.Plan,
		Labels: &newLabels,
		Serverless: &client.ServerlessClusterUpdateSpecification{
			PrimaryRegion: &primaryRegion,
			Regions:       &regions,
			UsageLimits:   cluster.Config.Serverless.UsageLimits,
		},
	}).Return(&clusterWithUpdatedLabels, httpOk, nil)

	// Move cluster and leave labels unchanged.
	s.EXPECT().GetFolder(gomock.Any(), folder.ResourceId).Return(&folder, httpOk, nil).Times(2)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&clusterWithUpdatedLabels, httpOk, nil).Times(2)

	clusterWithParent := clusterWithUpdatedLabels
	clusterWithParent.ParentId = &folder.ResourceId
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&clusterWithParent, httpOk, nil)
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, &client.UpdateClusterSpecification{
		Plan:   &cluster.Plan,
		Labels: &newLabels,
		Serverless: &client.ServerlessClusterUpdateSpecification{
			PrimaryRegion: &primaryRegion,
			Regions:       &regions,
			UsageLimits:   cluster.Config.Serverless.UsageLimits,
		},
		ParentId: &folder.ResourceId,
	}).Return(&clusterWithParent, httpOk, nil)

	// Delete.
	s.EXPECT().GetFolder(gomock.Any(), folder.ResourceId).Return(&folder, httpOk, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&clusterWithParent, httpOk, nil)

	s.EXPECT().DeleteFolder(gomock.Any(), folder.ResourceId).
		Return(httpOk, nil)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).
		Return(nil, httpOk, nil)

	testClusterWithLabels(t, folder.Name, clusterName, labelsAboveLimit, invalidLabels, labels, newLabels, true /* useMock */)
}

func testClusterWithLabels(
	t *testing.T,
	folderName, clusterName string,
	labelsAboveLimit, invalidLabels, labels, newLabels map[string]string,
	useMock bool,
) {
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					traceMessageStep("create cluster with too many labels")
				},
				Config:      getTestClusterWithLabels(folderName, clusterName, &labelsAboveLimit, false /*moveToFolder*/),
				ExpectError: regexp.MustCompile("must contain at most 50 key-value pairs"),
			},
			{
				PreConfig: func() {
					traceMessageStep("create cluster with invalid labels")
				},
				Config:      getTestClusterWithLabels(folderName, clusterName, &invalidLabels, false /*moveToFolder*/),
				ExpectError: regexp.MustCompile("consist of pairs of keys and optional values"),
			},
			{
				PreConfig: func() {
					traceMessageStep("create cluster with labels")
				},
				Config: getTestClusterWithLabels(folderName, clusterName, &labels, false /*moveToFolder*/),
				Check:  testCheckLabels(serverlessResourceName, labels),
			},
			{
				PreConfig: func() {
					traceMessageStep("clear cluster labels")
				},
				Config: getTestClusterWithLabels(folderName, clusterName, &map[string]string{}, false /*moveToFolder*/),
				Check:  testCheckLabels(serverlessResourceName, nil),
			},
			{
				PreConfig: func() {
					traceMessageStep("add labels back to cluster")
				},
				Config: getTestClusterWithLabels(folderName, clusterName, &labels, false /*moveToFolder*/),
				Check:  testCheckLabels(serverlessResourceName, labels),
			},
			{
				PreConfig: func() {
					traceMessageStep("not passing labels leaves labels unchanged")
				},
				Config: getTestClusterWithLabels(folderName, clusterName, nil, false /*moveToFolder*/),
				Check:  testCheckLabels(serverlessResourceName, labels),
			},
			{
				PreConfig: func() {
					traceMessageStep("update cluster labels")
				},
				Config: getTestClusterWithLabels(folderName, clusterName, &newLabels, false /*moveToFolder*/),
				Check:  testCheckLabels(serverlessResourceName, newLabels),
			},
			{
				PreConfig: func() {
					traceMessageStep("move cluster and leave labels unchanged")
				},
				Config: getTestClusterWithLabels(folderName, clusterName, &newLabels, true /*moveToFolder*/),
				Check:  testCheckLabels(serverlessResourceName, newLabels),
			},
		},
	})
}

func getTestClusterWithLabels(
	folderName, clusterName string, labels *map[string]string, moveToFolder bool,
) string {
	labelsString := ""
	if labels != nil {
		var labelStrings []string
		for k, v := range *labels {
			labelStrings = append(labelStrings, fmt.Sprintf("\"%s\" = \"%s\"", k, v))
		}
		labelsString = fmt.Sprintf(`
		labels = {
			%s
		}
		`, strings.Join(labelStrings, "\n\t\t\t"))
	}

	parentString := ""
	if moveToFolder {
		parentString = "parent_id = cockroach_folder.test_folder.id"
	}

	return fmt.Sprintf(`
	resource "cockroach_folder" "test_folder" {
		name = "%s"
		parent_id = "root"
	}

	resource "cockroach_cluster" "test" {
		name           = "%s"
		cloud_provider = "GCP"
		plan = "STANDARD"
		serverless = {
			usage_limits = {
				provisioned_virtual_cpus = 2
			}
		}
		regions = [{
			name = "us-central1"
		}] %s%s
	}
	`, folderName, clusterName, labelsString, parentString)
}

// TestIntegrationClusterExternalStateChange tests that the cluster resource
// gracefully handles external state changes without throwing errors.
func TestIntegrationClusterExternalStateChange(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	clusterName := fmt.Sprintf("%s-status-change-%s", tfTestPrefix, GenerateRandomString(4))
	clusterID := uuid.Nil.String()

	// Create initial cluster with CREATED status.
	initialCluster := &client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: latestClusterPatchVersion,
		CloudProvider:    "GCP",
		State:            client.CLUSTERSTATETYPE_CREATED,
		Plan:             client.PLANTYPE_STANDARD,
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{
				UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
				UsageLimits: &client.UsageLimits{
					ProvisionedVirtualCpus: ptr(int64(2)),
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

	// Create cluster with changed status (simulating external change).
	lockedCluster := *initialCluster
	lockedCluster.State = client.CLUSTERSTATETYPE_LOCKED

	// Update the cluster's region in the second test step. This simulates a Terraform
	// update that triggers a read on the resource, where a computed field has drifted
	// from the Terraform state. The test ensures no error is thrown in this scenario.
	updatedCluster := *initialCluster
	updatedCluster.Regions = []client.Region{
		{
			Name: "us-east1",
		},
	}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(initialCluster, nil, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(initialBackupConfig, httpOk, nil).AnyTimes()

	// First few calls return CREATED state, then return LOCKED state to simulate external change.
	// Eventually, the cluster is back to CREATED state.
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(initialCluster, httpOk, nil).Times(2)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&lockedCluster, httpOk, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(initialCluster, httpOk, nil).Times(2)

	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).Return(&updatedCluster, httpOk, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(&updatedCluster, httpOk, nil).AnyTimes()

	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(nil, nil, nil)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					traceMessageStep("create cluster with us-central1 region")
				},
				Config: fmt.Sprintf(`
					resource "cockroach_cluster" "test" {
						name           = "%s"
						cloud_provider = "GCP"
						plan           = "STANDARD"
						serverless = {
							usage_limits = {
								provisioned_virtual_cpus = 2
							}
						}
						regions = [{
							name = "us-central1"
						}]
					}
				`, clusterName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "name", clusterName),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.name", "us-central1"),
				),
			},
			{
				PreConfig: func() {
					traceMessageStep("update region to trigger read that detects external cluster status drift")
				},
				Config: fmt.Sprintf(`
					resource "cockroach_cluster" "test" {
						name           = "%s"
						cloud_provider = "GCP"
						plan           = "STANDARD"
						serverless = {
							usage_limits = {
								provisioned_virtual_cpus = 2
							}
						}
						regions = [{
							name = "us-east1"
						}]
					}
				`, clusterName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "name", clusterName),
					// Verify the region was successfully updated.
					resource.TestCheckResourceAttr("cockroach_cluster.test", "regions.0.name", "us-east1"),
				),
			},
		},
	})
}

func TestByocEqual(t *testing.T) {
	tests := []struct {
		name string
		a    *CustomerCloudAccount
		b    *CustomerCloudAccount
		want bool
	}{
		{
			name: "aws equal",
			a:    &CustomerCloudAccount{Aws: &AwsCustomerCloudAccount{Arn: types.StringValue("arn:aws:iam::123456789012:role/CRDB_Test")}},
			b:    &CustomerCloudAccount{Aws: &AwsCustomerCloudAccount{Arn: types.StringValue("arn:aws:iam::123456789012:role/CRDB_Test")}},
			want: true,
		},
		{
			name: "aws not equal",
			a:    &CustomerCloudAccount{Aws: &AwsCustomerCloudAccount{Arn: types.StringValue("arn:aws:iam::123456789012:role/CRDB_Test")}},
			b:    &CustomerCloudAccount{Aws: &AwsCustomerCloudAccount{Arn: types.StringValue("arn:aws:iam::123456789012:role/CRDB_Test2")}},
			want: false,
		},
		{
			name: "gcp equal",
			a:    &CustomerCloudAccount{Gcp: &GcpCustomerCloudAccount{ServiceAccountEmail: types.StringValue("test@example.com")}},
			b:    &CustomerCloudAccount{Gcp: &GcpCustomerCloudAccount{ServiceAccountEmail: types.StringValue("test@example.com")}},
			want: true,
		},
		{
			name: "gcp not equal",
			a:    &CustomerCloudAccount{Gcp: &GcpCustomerCloudAccount{ServiceAccountEmail: types.StringValue("test@example.com")}},
			b:    &CustomerCloudAccount{Gcp: &GcpCustomerCloudAccount{ServiceAccountEmail: types.StringValue("test@example.com2")}},
			want: false,
		},
		{
			name: "azure equal",
			a:    &CustomerCloudAccount{Azure: &AzureCustomerCloudAccount{SubscriptionId: types.StringValue("123456789012"), TenantId: types.StringValue("123456789012")}},
			b:    &CustomerCloudAccount{Azure: &AzureCustomerCloudAccount{SubscriptionId: types.StringValue("123456789012"), TenantId: types.StringValue("123456789012")}},
			want: true,
		},
		{
			name: "azure not equal",
			a:    &CustomerCloudAccount{Azure: &AzureCustomerCloudAccount{SubscriptionId: types.StringValue("123456789012"), TenantId: types.StringValue("123456789012")}},
			b:    &CustomerCloudAccount{Azure: &AzureCustomerCloudAccount{SubscriptionId: types.StringValue("123456789012"), TenantId: types.StringValue("1234567890122")}},
			want: false,
		},
		{
			name: "equal nil",
			a:    &CustomerCloudAccount{},
			b:    &CustomerCloudAccount{},
			want: true,
		},
		{
			name: "not equal nil",
			a:    &CustomerCloudAccount{},
			b:    &CustomerCloudAccount{Aws: &AwsCustomerCloudAccount{Arn: types.StringValue("arn:aws:iam::123456789012:role/CRDB_Test")}},
			want: false,
		},
		{
			name: "not equal different clouds",
			a:    &CustomerCloudAccount{Aws: &AwsCustomerCloudAccount{Arn: types.StringValue("arn:aws:iam::123456789012:role/CRDB_Test")}},
			b:    &CustomerCloudAccount{Gcp: &GcpCustomerCloudAccount{ServiceAccountEmail: types.StringValue("test@example.com")}},
			want: false,
		},
		{
			name: "both unset",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "one unset",
			a:    nil,
			b:    &CustomerCloudAccount{Aws: &AwsCustomerCloudAccount{Arn: types.StringValue("arn:aws:iam::123456789012:role/CRDB_Test")}},
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			equal := byocEqual(test.a, test.b)
			require.Equal(t, test.want, equal)
		})
	}
}
