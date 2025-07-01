package provider

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestIntegrationPhysicalReplicationStreamResource(t *testing.T) {
	primaryClusterName := fmt.Sprintf("primary-cluster-%s", GenerateRandomString(4))
	standbyClusterName := fmt.Sprintf("standby-cluster-%s", GenerateRandomString(4))
	primaryClusterID := uuid.New().String()
	standbyClusterID := uuid.New().String()
	replicationID := uuid.New().String()

	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	primaryCluster := client.Cluster{
		Name:          primaryClusterName,
		Id:            primaryClusterID,
		CloudProvider: "GCP",
		State:         "CREATED",
		Config: client.ClusterConfig{
			Dedicated: nil,
		},
	}
	standbyCluster := client.Cluster{
		Name:          standbyClusterName,
		Id:            standbyClusterID,
		CloudProvider: "GCP",
		State:         "CREATED",
		Config: client.ClusterConfig{
			Dedicated: nil,
		},
	}
	replication := client.PhysicalReplicationStream{
		Id:               replicationID,
		PrimaryClusterId: primaryClusterID,
		StandbyClusterId: standbyClusterID,
		CreatedAt:        time.Now(),
		Status:           client.REPLICATIONSTREAMSTATUSTYPE_REPLICATING,
	}

	// Create
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&primaryCluster, nil, nil)
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&standbyCluster, nil, nil)
	s.EXPECT().CreatePhysicalReplicationStream(gomock.Any(), &client.CreatePhysicalReplicationStreamRequest{
		PrimaryClusterId: primaryClusterID,
		StandbyClusterId: standbyClusterID,
	}).Return(&replication, nil, nil)

	// Read
	s.EXPECT().GetPhysicalReplicationStream(gomock.Any(), replicationID).Return(&replication, nil, nil).Times(2)

	// Delete
	s.EXPECT().DeleteCluster(gomock.Any(), primaryClusterID)
	s.EXPECT().DeleteCluster(gomock.Any(), standbyClusterID)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestPhysicalReplicationStreamResourceConfig(primaryClusterName, standbyClusterName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_physical_replication_stream.replication", "primary_cluster_id", primaryClusterID),
					resource.TestCheckResourceAttr("cockroach_physical_replication_stream.replication", "standby_cluster_id", standbyClusterID),
				),
			},
			{
				ResourceName:      "cockroach_physical_replication_stream.replication",
				ImportState:       true,
				ImportStateId:     replicationID,
				ImportStateVerify: true,
			},
		},
	})
}

func getTestPhysicalReplicationStreamResourceConfig(primaryClusterName, standbyClusterName string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "primary" {
    name           = "%s"
    cloud_provider = "GCP"
    dedicated = {
        storage_gib = 15
        num_virtual_cpus = 2
    }
    regions = [{
        name = "us-central1"
    }]
}

resource "cockroach_cluster" "standby" {
    name           = "%s"
    cloud_provider = "GCP"
    dedicated = {
        storage_gib = 15
        num_virtual_cpus = 2
    }
    regions = [{
        name = "us-east1"
    }]
}

resource "cockroach_physical_replication_stream" "replication" {
  primary_cluster_id = cockroach_cluster.primary.id
  standby_cluster_id = cockroach_cluster.standby.id
}
`, primaryClusterName, standbyClusterName)
}
