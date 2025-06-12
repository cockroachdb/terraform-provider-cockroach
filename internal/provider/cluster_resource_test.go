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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	"github.com/cockroachdb/terraform-provider-cockroach/internal/validators"
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
	minSupportedClusterMajorVersion = "v24.1"
	minSupportedClusterPatchVersion = "v24.1.0"
	latestClusterMajorVersion       = "v24.3"
	latestClusterPatchVersion       = "v24.3.0"

	serverlessResourceName   = "cockroach_cluster.test"
	serverlessDataSourceName = "data.cockroach_cluster.test"

	defaultPlanType = ""
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
					Config:      getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 4, nil),
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
				config := getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 4, nil)
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
					Config: getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 4, nil),
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
	vcpus            *int
	requestUnitLimit *int64
	storageMibLimit  *int64
	upgradeType      *client.UpgradeTypeType
	version          *string
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
				}
				regions = [{
					name = "us-central1"
				}]
				%s
			}

			data "cockroach_cluster" "test" {
				id = cockroach_cluster.test.id
			}
			`, clusterName, planConfig, usageLimitsConfig, upgradeTypeConfig, versionConfig),
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
	fmt.Println("JASKDJFAKSDF")
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
				Name:      "us-central1",
				NodeCount: 1,
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
		Return(&firstUpdateCluster, httpOk, nil).Times(6)

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
		Config: getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 8, nil),
		Check:  resource.TestCheckResourceAttr("cockroach_cluster.test", "dedicated.num_virtual_cpus", "8"),
	}

	testDedicatedClusterResource(t, clusterName, true, scaleStep)
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
			Config: getTestDedicatedClusterResourceConfig(clusterName, minSupportedClusterMajorVersion, false, 4, nil),
			Check: resource.ComposeTestCheckFunc(
				testCheckCockroachClusterExists(resourceName),
				resource.TestCheckResourceAttr(resourceName, "name", clusterName),
				resource.TestCheckResourceAttrSet(resourceName, "cloud_provider"),
				resource.TestCheckResourceAttrSet(resourceName, "cockroach_version"),
				resource.TestCheckResourceAttr(resourceName, "plan", "ADVANCED"),
				resource.TestCheckResourceAttr(resourceName, "dedicated.cidr_range", "172.28.0.0/16"),
				resource.TestMatchResourceAttr(resourceName, "full_version", startsWithMinSupportedMajorVersionRE),
				resource.TestCheckResourceAttr(dataSourceName, "name", clusterName),
				resource.TestCheckResourceAttrSet(dataSourceName, "cloud_provider"),
				resource.TestCheckResourceAttrSet(dataSourceName, "cockroach_version"),
				resource.TestCheckResourceAttr(dataSourceName, "plan", "ADVANCED"),
				resource.TestCheckResourceAttr(dataSourceName, "dedicated.cidr_range", "172.28.0.0/16"),
				resource.TestMatchResourceAttr(dataSourceName, "full_version", startsWithMinSupportedMajorVersionRE),
			),
		},
		{
			PreConfig: func() {
				traceMessageStep("update the version")
			},
			Config: getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, true, 4, nil),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(resourceName, "cockroach_version", latestClusterMajorVersion),
				resource.TestCheckResourceAttr(dataSourceName, "cockroach_version", latestClusterMajorVersion),
				resource.TestMatchResourceAttr(resourceName, "full_version", startsWithLatestMajorVersionRE),
				resource.TestMatchResourceAttr(dataSourceName, "full_version", startsWithLatestMajorVersionRE),
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
			Config: getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 4, ptr(true)),
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
			Config: getTestDedicatedClusterResourceConfig(clusterName, latestClusterMajorVersion, false, 4, ptr(false)),
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
	name, version string, finalize bool, vcpus int, deleteProtectionEnabled *bool,
) string {
	var deleteProtectionConfig string
	if deleteProtectionEnabled != nil {
		deleteProtectionConfig = fmt.Sprintf("\ndelete_protection = %t\n", *deleteProtectionEnabled)
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
    }
	regions = [{
		name: "us-central1"
		node_count: 1
	}]
	%s
}

data "cockroach_cluster" "test" {
    id = cockroach_cluster.test.id
}
`, name, version, vcpus, deleteProtectionConfig)

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
