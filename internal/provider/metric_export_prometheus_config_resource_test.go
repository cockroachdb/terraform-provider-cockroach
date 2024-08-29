package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v2/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccMetricExportPrometheusConfigResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-prometheus-%s", tfTestPrefix, GenerateRandomString(4))
	testMetricExportPrometheusConfigResource(t, clusterName, false)
}

// TestIntegrationMetricExportPrometheusConfigResource attempts to create, check,
// and destroy a cluster, but uses a mocked API service.
func TestIntegrationMetricExportPrometheusConfigResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-prometheus-%s", tfTestPrefix, GenerateRandomString(4))
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
		CloudProvider:    "GCP",
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
				Name:      "us-east1",
				NodeCount: 3,
			},
		},
	}

	enabledStatus := client.METRICEXPORTSTATUSTYPE_ENABLED

	createdPrometheusClusterInfo := &client.PrometheusMetricExportInfo{
		ClusterId: clusterID,
		Status:    &enabledStatus,
	}

	updatedPrometheusClusterInfo := &client.PrometheusMetricExportInfo{
		ClusterId: clusterID,
		Status:    &enabledStatus,
	}

	// Create
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(4)
	s.EXPECT().EnablePrometheusMetricExport(gomock.Any(), clusterID).
		Return(createdPrometheusClusterInfo, nil, nil)
	s.EXPECT().GetPrometheusMetricExportInfo(gomock.Any(), clusterID).
		Return(createdPrometheusClusterInfo, nil, nil).
		Times(3)

	// Update
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, nil, nil).
		Times(3)
	s.EXPECT().GetPrometheusMetricExportInfo(gomock.Any(), clusterID).
		Return(createdPrometheusClusterInfo, nil, nil)
	// It should not invoke EnablePrometheusMetricExport as request contains clusterID
	// which will not get changed.
	s.EXPECT().EnablePrometheusMetricExport(gomock.Any(), clusterID).
		Return(updatedPrometheusClusterInfo, nil, nil).Times(0)
	s.EXPECT().GetPrometheusMetricExportInfo(gomock.Any(), clusterID).
		Return(updatedPrometheusClusterInfo, nil, nil).
		Times(3)

	// Delete
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)
	s.EXPECT().DeletePrometheusMetricExport(gomock.Any(), clusterID)

	testMetricExportPrometheusConfigResource(t, clusterName, true)
}

func testMetricExportPrometheusConfigResource(t *testing.T, clusterName string, useMock bool) {
	var (
		clusterResourceName                      = "cockroach_cluster.test"
		metricExportPrometheusConfigResourceName = "cockroach_metric_export_prometheus_config.test"
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestMetricExportPrometheusConfigResourceCreateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testMetricExportPrometheusConfigExists(metricExportPrometheusConfigResourceName, clusterResourceName),
					resource.TestCheckResourceAttr(metricExportPrometheusConfigResourceName, "status", "ENABLED"),
				),
			},
			{
				Config: getTestMetricExportPrometheusConfigResourceUpdateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testMetricExportPrometheusConfigExists(metricExportPrometheusConfigResourceName, clusterResourceName),
					resource.TestCheckResourceAttr(metricExportPrometheusConfigResourceName, "status", "ENABLED"),
				),
			},
			{
				ResourceName:      metricExportPrometheusConfigResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testMetricExportPrometheusConfigExists(
	resourceName, clusterResourceName string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		p := testAccProvider.(*provider)
		p.service = NewService(cl)

		_, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}
		clusterRs, ok := s.RootModule().Resources[clusterResourceName]
		if !ok {
			return fmt.Errorf("not found: %s", clusterResourceName)
		}

		clusterID := clusterRs.Primary.Attributes["id"]
		config, _, err := p.service.GetPrometheusMetricExportInfo(context.TODO(), clusterID)
		if err != nil {
			return fmt.Errorf("metric export Prometheus config does not exist")
		}

		if config.GetStatus() != client.METRICEXPORTSTATUSTYPE_ENABLED {
			return fmt.Errorf("metric export Prometheus config is not enabled")
		}

		/*
			Metric export changes take cluster lock. We are cleaning resource immediately as part of test cleanup.
			This is causing failure intermittently as cluster lock might have not been released. We are checking & retrying
			cluster state before destroy.
		*/
		cluster, _, err := p.service.GetCluster(context.TODO(), clusterID)
		if err != nil {
			return fmt.Errorf("fetching cluster details failed")
		}

		if cluster.GetState() == client.CLUSTERSTATETYPE_LOCKED {
			err = retry.RetryContext(context.TODO(), metricExportEnableTimeout,
				waitForClusterReadyFunc(context.TODO(), clusterID, p.service, cluster))
			if err != nil {
				return fmt.Errorf("error in checking cluster state")
			}
		}

		return nil
	}
}

func getTestMetricExportPrometheusConfigResourceCreateConfig(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "GCP"
  dedicated = {
    storage_gib = 35
  	num_virtual_cpus = 4
  }
  regions = [{
    name = "us-east1"
    node_count: 3
  }]
}

resource "cockroach_metric_export_prometheus_config" "test" {
	id      = cockroach_cluster.test.id
  }
`, name)
}

func getTestMetricExportPrometheusConfigResourceUpdateConfig(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "GCP"
  dedicated = {
    storage_gib = 35
  	num_virtual_cpus = 4
  }
  regions = [{
    name = "us-east1"
    node_count: 3
  }]
}

resource "cockroach_metric_export_prometheus_config" "test" {
	id      = cockroach_cluster.test.id
  }
`, name)
}
