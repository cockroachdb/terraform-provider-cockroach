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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v3/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccMetricExportDatdogConfigResource attempts to create, check, and destroy
// a real cluster. It will be skipped if TF_ACC isn't set.
func TestAccMetricExportDatadogConfigResource(t *testing.T) {
	t.Skip("Skipping until we can either integrate the AWS provider " +
		"or import a permanent test fixture.")
	t.Parallel()
	clusterName := fmt.Sprintf("%s-datadog-%s", tfTestPrefix, GenerateRandomString(4))
	testMetricExportDatadogConfigResource(t, clusterName, false)
}

// TestIntegrationMetricExportDatadogConfigResource attempts to create, check,
// and destroy a cluster, but uses a mocked API service.
func TestIntegrationMetricExportDatadogConfigResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-datadog-%s", tfTestPrefix, GenerateRandomString(4))
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
	apiKey := "test-api-key"
	apiKeyTrunc := apiKey[len(apiKey)-4:]
	site := client.DATADOGSITETYPE_US1
	updatedApiKey := "test-api-key-updated"
	updatedApiKeyTrunc := updatedApiKey[len(updatedApiKey)-4:]

	createdDatadogClusterInfo := &client.DatadogMetricExportInfo{
		ClusterId: clusterID,
		ApiKey:    &apiKeyTrunc,
		Site:      site,
		Status:    &enabledStatus,
	}

	updatedDatadogClusterInfo := &client.DatadogMetricExportInfo{
		ClusterId: clusterID,
		ApiKey:    &updatedApiKeyTrunc,
		Site:      site,
		Status:    &enabledStatus,
	}

	// Create
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(3)
	s.EXPECT().EnableDatadogMetricExport(gomock.Any(), clusterID,
		&client.EnableDatadogMetricExportRequest{
			ApiKey: apiKey,
			Site:   site,
		}).
		Return(createdDatadogClusterInfo, nil, nil)
	s.EXPECT().GetDatadogMetricExportInfo(gomock.Any(), clusterID).
		Return(createdDatadogClusterInfo, nil, nil).
		Times(3)

	// Update
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, nil, nil).
		Times(2)
	s.EXPECT().GetDatadogMetricExportInfo(gomock.Any(), clusterID).
		Return(createdDatadogClusterInfo, nil, nil)
	s.EXPECT().EnableDatadogMetricExport(gomock.Any(), clusterID,
		&client.EnableDatadogMetricExportRequest{
			ApiKey: updatedApiKey,
			Site:   site,
		}).
		Return(updatedDatadogClusterInfo, nil, nil)
	s.EXPECT().GetDatadogMetricExportInfo(gomock.Any(), clusterID).
		Return(updatedDatadogClusterInfo, nil, nil).
		Times(4)

	// Delete
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)
	s.EXPECT().DeleteDatadogMetricExport(gomock.Any(), clusterID)

	testMetricExportDatadogConfigResource(t, clusterName, true)
}

func testMetricExportDatadogConfigResource(t *testing.T, clusterName string, useMock bool) {
	var (
		clusterResourceName                   = "cockroach_cluster.test"
		metricExportDatadogConfigResourceName = "cockroach_metric_export_datadog_config.test"
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestMetricExportDatadogConfigResourceCreateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testMetricExportDatadogConfigExists(metricExportDatadogConfigResourceName, clusterResourceName),
					resource.TestCheckResourceAttr(metricExportDatadogConfigResourceName, "site", "US1"),
					resource.TestCheckResourceAttr(metricExportDatadogConfigResourceName, "api_key", "test-api-key"),
					resource.TestCheckResourceAttr(metricExportDatadogConfigResourceName, "status", "ENABLED"),
				),
			},
			{
				Config: getTestMetricExportDatadogConfigResourceUpdateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testMetricExportDatadogConfigExists(metricExportDatadogConfigResourceName, clusterResourceName),
					resource.TestCheckResourceAttr(metricExportDatadogConfigResourceName, "site", "US1"),
					resource.TestCheckResourceAttr(metricExportDatadogConfigResourceName, "api_key", "test-api-key-updated"),
					resource.TestCheckResourceAttr(metricExportDatadogConfigResourceName, "status", "ENABLED"),
				),
			},
			{
				ResourceName:      metricExportDatadogConfigResourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					// The API key gets redacted and truncated by the API server.
					"api_key",
				},
			},
		},
	})
}

func testMetricExportDatadogConfigExists(
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

		traceAPICall("GetDatadogMetricExportInfo")
		apiObj, _, err := p.service.GetDatadogMetricExportInfo(context.TODO(), clusterID)
		if err == nil {
			if string(apiObj.GetSite()) == rs.Primary.Attributes["site"] {
				return nil
			}
		}

		return fmt.Errorf("metric export Datadog config with site %s does not exist", rs.Primary.Attributes["site"])
	}
}

func getTestMetricExportDatadogConfigResourceCreateConfig(name string) string {
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

resource "cockroach_metric_export_datadog_config" "test" {
	id      = cockroach_cluster.test.id
	site    = "US1"
	api_key = "test-api-key"
  }
`, name)
}

func getTestMetricExportDatadogConfigResourceUpdateConfig(name string) string {
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

resource "cockroach_metric_export_datadog_config" "test" {
	id      = cockroach_cluster.test.id
	site    = "US1"
	api_key = "test-api-key-updated"
  }
`, name)
}
