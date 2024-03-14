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
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccMetricExportCloudWatchConfigResource attempts to create, check, and destroy
// a real cluster. It will be skipped if TF_ACC isn't set.
func TestAccMetricExportCloudWatchConfigResource(t *testing.T) {
	t.Skip("Skipping until we can either integrate the AWS provider " +
		"or import a permanent test fixture.")
	t.Parallel()
	clusterName := fmt.Sprintf("tftest-cloudwatch-%s", GenerateRandomString(2))
	testMetricExportCloudWatchConfigResource(t, clusterName, false)
}

// TestIntegrationMetricExportCloudWatchConfigResource attempts to create, check,
// and destroy a cluster, but uses a mocked API service.
func TestIntegrationMetricExportCloudWatchConfigResource(t *testing.T) {
	clusterName := fmt.Sprintf("tftest-cloudwatch-%s", GenerateRandomString(2))
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
		Plan:             "DEDICATED",
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

	enabledStatus := client.METRICEXPORTSTATUSTYPE_ENABLED
	arn := "test-role-arn"
	logGroupName := "example"
	emptyString := ""
	updatedRegion := "us-east-1"

	createdCloudWatchClusterInfo := &client.CloudWatchMetricExportInfo{
		ClusterId:    clusterID,
		RoleArn:      arn,
		LogGroupName: &logGroupName,
		TargetRegion: &emptyString,
		Status:       &enabledStatus,
	}

	updatedCloudWatchClusterInfo := &client.CloudWatchMetricExportInfo{
		ClusterId:    clusterID,
		RoleArn:      arn,
		LogGroupName: &logGroupName,
		TargetRegion: &updatedRegion,
		Status:       &enabledStatus,
	}

	// Create
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(3)
	s.EXPECT().EnableCloudWatchMetricExport(gomock.Any(), clusterID,
		&client.EnableCloudWatchMetricExportRequest{
			RoleArn:      arn,
			LogGroupName: &logGroupName,
		}).
		Return(createdCloudWatchClusterInfo, nil, nil)
	s.EXPECT().GetCloudWatchMetricExportInfo(gomock.Any(), clusterID).
		Return(createdCloudWatchClusterInfo, nil, nil).
		Times(3)

	// Update
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, nil, nil).
		Times(2)
	s.EXPECT().GetCloudWatchMetricExportInfo(gomock.Any(), clusterID).
		Return(createdCloudWatchClusterInfo, nil, nil)
	s.EXPECT().EnableCloudWatchMetricExport(gomock.Any(), clusterID,
		&client.EnableCloudWatchMetricExportRequest{
			RoleArn:      arn,
			LogGroupName: &logGroupName,
			TargetRegion: &updatedRegion,
		}).
		Return(updatedCloudWatchClusterInfo, nil, nil)
	s.EXPECT().GetCloudWatchMetricExportInfo(gomock.Any(), clusterID).
		Return(updatedCloudWatchClusterInfo, nil, nil).
		Times(4)

	// Delete
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)
	s.EXPECT().DeleteCloudWatchMetricExport(gomock.Any(), clusterID)

	testMetricExportCloudWatchConfigResource(t, clusterName, true)
}

func testMetricExportCloudWatchConfigResource(t *testing.T, clusterName string, useMock bool) {
	var (
		clusterResourceName                      = "cockroach_cluster.test"
		metricExportCloudWatchConfigResourceName = "cockroach_metric_export_cloudwatch_config.test"
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestMetricExportCloudWatchConfigResourceCreateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testMetricExportCloudWatchConfigExists(metricExportCloudWatchConfigResourceName, clusterResourceName),
					resource.TestCheckResourceAttr(metricExportCloudWatchConfigResourceName, "role_arn", "test-role-arn"),
					resource.TestCheckResourceAttr(metricExportCloudWatchConfigResourceName, "log_group_name", "example"),
					resource.TestCheckResourceAttr(metricExportCloudWatchConfigResourceName, "target_region", ""),
				),
			},
			{
				Config: getTestMetricExportCloudWatchConfigResourceUpdateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testMetricExportCloudWatchConfigExists(metricExportCloudWatchConfigResourceName, clusterResourceName),
					resource.TestCheckResourceAttr(metricExportCloudWatchConfigResourceName, "role_arn", "test-role-arn"),
					resource.TestCheckResourceAttr(metricExportCloudWatchConfigResourceName, "log_group_name", "example"),
					resource.TestCheckResourceAttr(metricExportCloudWatchConfigResourceName, "target_region", "us-east-1"),
				),
			},
			{
				ResourceName:      metricExportCloudWatchConfigResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testMetricExportCloudWatchConfigExists(
	resourceName, clusterResourceName string,
) resource.TestCheckFunc {
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

		apiObj, _, err := p.service.GetCloudWatchMetricExportInfo(context.TODO(), clusterID)
		if err == nil {
			if string(apiObj.GetLogGroupName()) == rs.Primary.Attributes["log_group_name"] {
				return nil
			}
		}

		return fmt.Errorf("metric export CloudWatch config with log group name %s does not exist", rs.Primary.Attributes["site"])
	}
}

func getTestMetricExportCloudWatchConfigResourceCreateConfig(name string) string {
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

resource "cockroach_metric_export_cloudwatch_config" "test" {
	id      = cockroach_cluster.test.id
	role_arn       = "test-role-arn"
	log_group_name = "example"
  }
`, name)
}

func getTestMetricExportCloudWatchConfigResourceUpdateConfig(name string) string {
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

resource "cockroach_metric_export_cloudwatch_config" "test" {
	id      = cockroach_cluster.test.id
	role_arn       = "test-role-arn"
	log_group_name = "example"
	target_region  = "us-east-1"
  }
`, name)
}
