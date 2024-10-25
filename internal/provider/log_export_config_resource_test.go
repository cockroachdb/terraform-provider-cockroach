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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v4/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccLogExportConfigResource attempts to create, check, and destroy
// a real cluster. It will be skipped if TF_ACC isn't set.
func TestAccLogExportConfigResource(t *testing.T) {
	t.Skip("Skipping until we can either integrate the AWS provider " +
		"or import a permanent test fixture.")
	t.Parallel()
	clusterName := fmt.Sprintf("%s-logexp-%s", tfTestPrefix, GenerateRandomString(4))
	testLogExportConfigResource(t, clusterName, false)
}

// TestIntegrationLogExportConfigResource attempts to create, check, and destroy
// a cluster, but uses a mocked API service.
func TestIntegrationLogExportConfigResource(t *testing.T) {
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

	cluster := &client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: "v22.2.0",
		Plan:             "ADVANCED",
		CloudProvider:    "AWS",
		State:            "CREATED",
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				MachineType:    "m5.xlarge",
				NumVirtualCpus: 4,
				StorageGib:     35,
				MemoryGib:      8,
			},
		},
		Regions: []client.Region{
			{
				Name:      "us-east-1",
				NodeCount: 3,
			},
		},
	}

	authPrincipal := "iam-role-arn"
	configType, _ := client.NewLogExportTypeFromValue("AWS_CLOUDWATCH")
	logName := "test"
	region := "us-east-1"
	trueBool := true
	minLevel, _ := client.NewLogLevelTypeFromValue("WARNING")
	createdGroups := []client.LogExportGroup{
		{LogName: "sql", Channels: []string{"SQL_SCHEMA", "SQL_EXEC"}, MinLevel: minLevel, Redact: &trueBool},
	}
	omittedChannels := []string{"SQL_PERF"}
	enabledStatus, _ := client.NewLogExportStatusFromValue("ENABLED")
	createdLogExportClusterInfo := &client.LogExportClusterInfo{
		ClusterId: &clusterID,
		Spec: &client.LogExportClusterSpecification{
			AuthPrincipal:   &authPrincipal,
			LogName:         &logName,
			Groups:          &createdGroups,
			Redact:          &trueBool,
			Type:            configType,
			Region:          &region,
			OmittedChannels: &omittedChannels,
		},
		Status: enabledStatus,
	}

	falseBool := false
	updatedGroups := []client.LogExportGroup{
		{LogName: "sql", Channels: []string{"SQL_EXEC"}, MinLevel: minLevel, Redact: &trueBool},
		{LogName: "devops", Channels: []string{"OPS", "HEALTH", "STORAGE"}, MinLevel: minLevel, Redact: &falseBool},
	}
	updatedOChannels := []string{"SQL_SCHEMA"}
	updatedLogExportClusterInfo := &client.LogExportClusterInfo{
		ClusterId: &clusterID,
		Spec: &client.LogExportClusterSpecification{
			AuthPrincipal:   &authPrincipal,
			LogName:         &logName,
			Groups:          &updatedGroups,
			Redact:          &falseBool,
			Type:            configType,
			Region:          &region,
			OmittedChannels: &updatedOChannels,
		},
		Status: enabledStatus,
	}

	// Create
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(3)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().EnableLogExport(gomock.Any(), clusterID,
		&client.EnableLogExportRequest{
			Groups:          &createdGroups,
			AuthPrincipal:   authPrincipal,
			LogName:         logName,
			Redact:          &trueBool,
			Type:            *configType,
			Region:          &region,
			OmittedChannels: &omittedChannels,
		}).
		Return(createdLogExportClusterInfo, nil, nil)
	s.EXPECT().GetLogExportInfo(gomock.Any(), clusterID).
		Return(createdLogExportClusterInfo, nil, nil).
		Times(3)

	// Update
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, nil, nil).
		Times(2)
	s.EXPECT().GetLogExportInfo(gomock.Any(), clusterID).
		Return(createdLogExportClusterInfo, nil, nil)
	s.EXPECT().EnableLogExport(gomock.Any(), clusterID,
		&client.EnableLogExportRequest{
			AuthPrincipal:   authPrincipal,
			Type:            *configType,
			LogName:         logName,
			Redact:          &falseBool,
			Groups:          &updatedGroups,
			Region:          &region,
			OmittedChannels: &updatedOChannels,
		}).
		Return(updatedLogExportClusterInfo, nil, nil)
	s.EXPECT().GetLogExportInfo(gomock.Any(), clusterID).
		Return(updatedLogExportClusterInfo, nil, nil).
		Times(4)

	// Delete
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)
	s.EXPECT().DeleteLogExport(gomock.Any(), clusterID)

	testLogExportConfigResource(t, clusterName, true)
}

func testLogExportConfigResource(t *testing.T, clusterName string, useMock bool) {
	var (
		clusterResourceName         = "cockroach_cluster.test"
		logExportConfigResourceName = "cockroach_log_export_config.test"
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestLogExportConfigResourceCreateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testLogExportConfigExists(logExportConfigResourceName, clusterResourceName),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "redact", "true"),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "groups.#", "1"),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "groups.0.channels.#", "2"),
				),
			},
			{
				Config: getTestLogExportConfigResourceUpdateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testLogExportConfigExists(logExportConfigResourceName, clusterResourceName),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "redact", "false"),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "groups.#", "2"),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "groups.0.channels.#", "1"),
				),
			},
			{
				ResourceName:      logExportConfigResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testLogExportConfigExists(resourceName, clusterResourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		p := testAccProvider.(*provider)
		p.service = NewService(cl)

		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}
		clusterRs, ok := s.RootModule().Resources[clusterResourceName]
		if !ok {
			return fmt.Errorf("not found: %s", clusterResourceName)
		}

		clusterID := clusterRs.Primary.Attributes["id"]
		log.Printf("[DEBUG] clusterID: %s, name %s", clusterRs.Primary.Attributes["id"], clusterRs.Primary.Attributes["name"])

		traceAPICall("GetLogExportInfo")
		logExportConfigResponse, _, err := p.service.GetLogExportInfo(context.TODO(), clusterID)
		if err == nil {
			if logExportConfigResponse.Spec.GetLogName() == rs.Primary.Attributes["log_name"] {
				return nil
			}
		}

		return fmt.Errorf("log export config with log name %s does not exist", rs.Primary.Attributes["log_name"])
	}
}

func getTestLogExportConfigResourceCreateConfig(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  dedicated = {
    storage_gib = 35
  	num_virtual_cpus = 4
  }
  regions = [{
    name = "us-east-1"
    node_count: 3
  }]
}

resource "cockroach_log_export_config" "test" {
  id             = cockroach_cluster.test.id
  auth_principal = "iam-role-arn"
  log_name       = "test"
  type           = "AWS_CLOUDWATCH"
  redact         = true
  region 		 = "us-east-1"
  groups = [
    {
      log_name = "sql",
      channels = ["SQL_SCHEMA", "SQL_EXEC"],
      min_level = "WARNING",
	  redact = true
    }
  ]
  omitted_channels = ["SQL_PERF"]
}
`, name)
}

func getTestLogExportConfigResourceUpdateConfig(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  dedicated = {
    storage_gib = 35
  	num_virtual_cpus = 4
  }
  regions = [{
    name = "us-east-1"
    node_count: 3
  }]
}

resource "cockroach_log_export_config" "test" {
  id             = cockroach_cluster.test.id
  auth_principal = "iam-role-arn"
  log_name       = "test"
  type           = "AWS_CLOUDWATCH"
  redact         = false
  region 		 = "us-east-1"
  groups = [
    {
      log_name = "sql",
      channels = ["SQL_EXEC"],
      min_level = "WARNING",
	  redact: true
    },
    {
      log_name = "devops",
      channels = ["OPS", "HEALTH", "STORAGE"],
      min_level = "WARNING",
	  redact = false
    }
  ]
  omitted_channels = ["SQL_SCHEMA"]
}
`, name)
}
