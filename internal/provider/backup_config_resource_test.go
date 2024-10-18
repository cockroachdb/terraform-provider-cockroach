/*
Copyright 2024 The Cockroach Authors

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
	"net/http"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v4/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccBackupConfigResource attempts to check, and update a
// real backup configuration. It will be skipped if TF_ACC isn't set.
func TestAccBackupConfigResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-backup-%s", tfTestPrefix, GenerateRandomString(4))
	testBackupConfigResource(t, clusterName, false /* useMock */)
}

// Shared Test objects
var initialBackupConfig = &client.BackupConfiguration{
	Enabled: true,
	FrequencyMinutes: 60,
	RetentionDays: 30,
}

var initialBackupConfigDisabled = &client.BackupConfiguration{
	Enabled: false,
	FrequencyMinutes: initialBackupConfig.FrequencyMinutes,
	RetentionDays: initialBackupConfig.RetentionDays,
}

var updatedBackupConfig = &client.BackupConfiguration{
	Enabled: true,
	RetentionDays: 7,
	FrequencyMinutes: 5,
}

var secondUpdatedBackupConfig = &client.BackupConfiguration{
	Enabled: updatedBackupConfig.Enabled,
	RetentionDays: updatedBackupConfig.RetentionDays,
	FrequencyMinutes: 1440,
}

// TestIntegrationBackupConfigResource attempts to check, and update a
// backup configuration, but uses a mocked API service.
func TestIntegrationBackupConfigResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-logexp-%s", tfTestPrefix, GenerateRandomString(4))
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

	httpOk := &http.Response{Status: http.StatusText(http.StatusOK)}
	httpFail := &http.Response{Status: http.StatusText(http.StatusBadRequest)}

	expectUpdateSequence := func(update *client.UpdateBackupConfigurationSpec, before, after *client.BackupConfiguration) {
		s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(before, httpOk, nil)
		s.EXPECT().UpdateBackupConfiguration(gomock.Any(), clusterID, update).Return(after, httpOk, nil)
		s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(after, httpOk, nil).Times(2)
	}

	expectErrorSequence := func(update *client.UpdateBackupConfigurationSpec, before *client.BackupConfiguration, err error) {
		s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(before, httpOk, nil)
		s.EXPECT().UpdateBackupConfiguration(gomock.Any(), clusterID, update).Return(before, httpFail, err)
	}

	// Our cluster never changes so to avoid complexity, we'll just return it
	// as many times as its asked for.
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, httpOk, nil).AnyTimes()

	// Step: a cluster without a backup config resource still has a default config
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&cluster, nil, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(initialBackupConfig, httpOk, nil)

	// Step: a resource with just the mandatory enabled field
	expectUpdateSequence(&client.UpdateBackupConfigurationSpec{
		Enabled: ptr(true),
	}, initialBackupConfig, initialBackupConfig)

	// Step: disabling backups without specifying any other fields
	expectUpdateSequence(&client.UpdateBackupConfigurationSpec{
		Enabled: ptr(false),
	}, initialBackupConfig, initialBackupConfigDisabled)

	// Step: reenable passing in the default values
	expectUpdateSequence(&client.UpdateBackupConfigurationSpec{
		Enabled: ptr(true),
		RetentionDays: ptr(initialBackupConfig.RetentionDays),
		FrequencyMinutes: ptr(initialBackupConfig.FrequencyMinutes),
	}, initialBackupConfigDisabled, initialBackupConfig)

	// Step: update frequency and retention
	expectUpdateSequence(&client.UpdateBackupConfigurationSpec{
		Enabled: ptr(true),
		RetentionDays: ptr(updatedBackupConfig.RetentionDays),
		FrequencyMinutes: ptr(updatedBackupConfig.FrequencyMinutes),
	}, initialBackupConfig, updatedBackupConfig)

	// Step: error case: invalid retention_days
	expectErrorSequence(&client.UpdateBackupConfigurationSpec{
		Enabled: ptr(true),
		RetentionDays: ptr(int32(12345)),
	}, updatedBackupConfig, fmt.Errorf("retention_days must be one of []"))

	// Step: error case: invalid frequency_minutes
	expectErrorSequence(&client.UpdateBackupConfigurationSpec{
		Enabled: ptr(true),
		FrequencyMinutes: ptr(int32(12345)),
	}, updatedBackupConfig, fmt.Errorf("frequency_minutes must be one of []"))

	// Step: remove the backup configuration resource, cluster still has the last one that was set
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(updatedBackupConfig, httpOk, nil).Times(2)

	// Step: test updating values during the create
	expectUpdateSequence(&client.UpdateBackupConfigurationSpec{
		Enabled: ptr(true),
		FrequencyMinutes: ptr(int32(secondUpdatedBackupConfig.FrequencyMinutes)),
	}, updatedBackupConfig, secondUpdatedBackupConfig)

	// Delete phase
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).Return(secondUpdatedBackupConfig, httpOk, nil)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testBackupConfigResource(t, clusterName, true /* useMock */)
}

func testBackupConfigResource(t *testing.T, clusterName string, useMock bool) {
	var (
		clusterResourceName      = "cockroach_cluster.test"
		backupConfigResourceName = "cockroach_backup_config.test"
	)

 	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() {
					traceMessageStep("a cluster without a backup config resource still has a default config")
				},
				Config: getTestBackupConfigCreateConfig(clusterName, backupCreateConfig{
					includeBackupConfig: false,
				}),
				Check:  checkBackupConfig(clusterResourceName, initialBackupConfig),
			},
			{
				PreConfig:   func() {
					traceMessageStep("a resource with just the mandatory enabled field")
				},
				Config: getTestBackupConfigCreateConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled: true,
				}),
				Check:  checkBackupConfig(clusterResourceName, initialBackupConfig),
			},
			{
				PreConfig:   func() {
					traceMessageStep("disabling backups without specifying any other fields")
				},
				Config: getTestBackupConfigCreateConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled: false,
				}),
				Check:  checkBackupConfig(clusterResourceName, initialBackupConfigDisabled),
			},
			{
				PreConfig:   func() {
					traceMessageStep("reenable passing in the default values")
				},
				Config: getTestBackupConfigCreateConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled: true,
					retention: ptr(initialBackupConfig.RetentionDays),
					frequency: ptr(initialBackupConfig.FrequencyMinutes),
				}),
				Check:  checkBackupConfig(clusterResourceName, initialBackupConfig),
			},
			{
				PreConfig:   func() {
					traceMessageStep("update frequency and retention")
				},
				Config: getTestBackupConfigCreateConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled: true,
					retention: ptr(updatedBackupConfig.RetentionDays),
					frequency: ptr(updatedBackupConfig.FrequencyMinutes),
				}),
				Check:  checkBackupConfig(clusterResourceName, updatedBackupConfig),
			},
			{
				PreConfig:   func() {
					traceMessageStep("error case: invalid retention_days")
				},
				Config: getTestBackupConfigCreateConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled: true,
					retention: ptr(int32(12345)),
				}),
				Check:  checkBackupConfig(clusterResourceName, updatedBackupConfig),
				// Setting single line mode because error is broken across lines.
				ExpectError: regexp.MustCompile(`(?s)retention_days.*must.*be.*one.*of`),
			},
			{
				PreConfig:   func() {
					traceMessageStep("error case: invalid frequency_minutes")
				},
				Config: getTestBackupConfigCreateConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled: true,
					frequency: ptr(int32(12345)),
				}),
				Check:  checkBackupConfig(clusterResourceName, updatedBackupConfig),
				// Setting single line mode because error is broken across lines.
				ExpectError: regexp.MustCompile(`(?s)frequency_minutes.*must.*be.*one.*of`),
			},
			{
				PreConfig:   func() {
					traceMessageStep("remove the backup configuration resource, cluster still has the last one that was set")
				},
				Config: getTestBackupConfigCreateConfig(clusterName, backupCreateConfig{
					includeBackupConfig: false,
				}),
				Check:  checkBackupConfig(clusterResourceName, updatedBackupConfig),
			},
			{
				PreConfig:   func() {
					traceMessageStep("test updating values during the create")
				},
				Config: getTestBackupConfigCreateConfig(clusterName, backupCreateConfig{
					includeBackupConfig: true,
					enabled: true,
					frequency: ptr(secondUpdatedBackupConfig.FrequencyMinutes),
				}),
				Check:  resource.ComposeTestCheckFunc(
					checkBackupConfig(clusterResourceName, secondUpdatedBackupConfig),
					traceEndOfPlan(),
				),
			},
			{
				ResourceName:      backupConfigResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func checkBackupConfig(clusterResourceName string, expected *client.BackupConfiguration) resource.TestCheckFunc {
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

func getTestBackupConfigClusterConfig(name string) string {
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
	}]
}
`, name)
}

type backupCreateConfig struct {
	includeBackupConfig bool
	enabled             bool
	retention           *int32
	frequency           *int32
}

func getTestBackupConfigCreateConfig(name string, config backupCreateConfig) string {

	tfConfig := getTestBackupConfigClusterConfig(name)

	if config.includeBackupConfig {
		retentionConfig := ""
		if config.retention != nil {
			retentionConfig = fmt.Sprintf(`retention_days = %d`, *config.retention)
		}

		frequencyConfig := ""
		if config.frequency != nil {
			frequencyConfig = fmt.Sprintf(`frequency_minutes = %d`, *config.frequency)
		}

		tfConfig = fmt.Sprintf(`
%s		
resource "cockroach_backup_config" "test" {
id = cockroach_cluster.test.id
enabled = %s
%s
%s
}
`, tfConfig, strconv.FormatBool(config.enabled), retentionConfig, frequencyConfig)
	}
	return tfConfig
}
