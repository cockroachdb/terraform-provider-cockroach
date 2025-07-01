package provider

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

type clusterReqMatcher struct {
	ExpectedName string
}

func (m clusterReqMatcher) Matches(x any) bool {
	req, ok := x.(*client.CreateClusterRequest)
	if !ok {
		return false
	}
	return req.Name == m.ExpectedName
}

func (m clusterReqMatcher) String() string {
	return fmt.Sprintf("clusterReqMatcher %s", m.ExpectedName)
}

func TestIntegrationPhysicalReplicationStreamResource(t *testing.T) {
	primaryClusterName := fmt.Sprintf("primary-cluster-%s", GenerateRandomString(4))
	standbyClusterName := fmt.Sprintf("standby-cluster-%s", GenerateRandomString(4))
	primaryClusterID := uuid.New().String()
	standbyClusterID := uuid.New().String()
	replicationID := uuid.New().String()

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
			Dedicated: &client.DedicatedHardwareConfig{
				StorageGib:     15,
				NumVirtualCpus: 2,
			},
		},
		Regions: []client.Region{
			{
				Name:      "us-central1",
				NodeCount: 3,
			},
		},
	}

	standbyCluster := client.Cluster{
		Name:          standbyClusterName,
		Id:            standbyClusterID,
		CloudProvider: "GCP",
		State:         "CREATED",
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				StorageGib:     15,
				NumVirtualCpus: 2,
			},
		},
		Regions: []client.Region{
			{
				Name:      "us-east1",
				NodeCount: 3,
			},
		},
	}

	now := time.Now()
	replicatedTime := now.Add(-1 * time.Hour)
	retainedTime := now.Add(-24 * time.Hour)
	statuslessReplicationStream := client.PhysicalReplicationStream{
		Id:                    replicationID,
		PrimaryClusterId:      primaryClusterID,
		StandbyClusterId:      standbyClusterID,
		CreatedAt:             now,
		ReplicatedTime:        &replicatedTime,
		ReplicationLagSeconds: ptr(int32(30)),
		RetainedTime:          &retainedTime,
	}

	// One create call for each resource.
	s.EXPECT().CreateCluster(gomock.Any(), clusterReqMatcher{ExpectedName: primaryClusterName}).Return(&primaryCluster, nil, nil).Times(1)
	s.EXPECT().CreateCluster(gomock.Any(), clusterReqMatcher{ExpectedName: standbyClusterName}).Return(&standbyCluster, nil, nil).Times(1)

	replicatingReplicationStream := statuslessReplicationStream
	replicatingReplicationStream.Status = client.REPLICATIONSTREAMSTATUSTYPE_REPLICATING
	s.EXPECT().CreatePhysicalReplicationStream(gomock.Any(), gomock.Any()).Return(&replicatingReplicationStream, nil, nil).Times(1)

	// The Get functions are called a number of times for various reasons.
	// Keeping track of the exact count isn't interesting.
	s.EXPECT().GetCluster(gomock.Any(), primaryClusterID).Return(&primaryCluster, nil, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), standbyClusterID).Return(&standbyCluster, nil, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), primaryClusterID).Return(initialBackupConfig, nil, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), standbyClusterID).Return(initialBackupConfig, nil, nil).AnyTimes()

	// We need to change what this call is returning in between steps
	// using the PreConfig hook.
	replicationStatus := client.REPLICATIONSTREAMSTATUSTYPE_REPLICATING
	s.EXPECT().GetPhysicalReplicationStream(gomock.Any(), replicationID).DoAndReturn(
		func(context.Context, string) (*client.PhysicalReplicationStream, *http.Response, error) {
			replicationStream := statuslessReplicationStream
			replicationStream.Status = replicationStatus
			return &replicationStream, nil, nil
		}).AnyTimes()

	// One update call for failover.
	completedStream := statuslessReplicationStream
	completedStream.Status = client.REPLICATIONSTREAMSTATUSTYPE_COMPLETED
	s.EXPECT().UpdatePhysicalReplicationStream(gomock.Any(), replicationID, gomock.Any()).Return(&completedStream, nil, nil).Times(1)

	s.EXPECT().DeleteCluster(gomock.Any(), primaryClusterID)
	s.EXPECT().DeleteCluster(gomock.Any(), standbyClusterID)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestPhysicalReplicationStreamResourceConfigCreate(primaryClusterName, standbyClusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists("cockroach_cluster.primary"),
					testCheckCockroachClusterExists("cockroach_cluster.standby"),
					resource.TestCheckResourceAttr("cockroach_physical_replication_stream.stream", "primary_cluster_id", primaryClusterID),
					resource.TestCheckResourceAttr("cockroach_physical_replication_stream.stream", "standby_cluster_id", standbyClusterID),
					resource.TestCheckResourceAttr("cockroach_physical_replication_stream.stream", "status", string(client.REPLICATIONSTREAMSTATUSTYPE_REPLICATING)),
				),
			},
			{
				PreConfig: func() { replicationStatus = client.REPLICATIONSTREAMSTATUSTYPE_COMPLETED },
				Config:    getTestPhysicalReplicationStreamResourceConfigFailover(primaryClusterName, standbyClusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists("cockroach_cluster.primary"),
					testCheckCockroachClusterExists("cockroach_cluster.standby"),
					resource.TestCheckResourceAttr("cockroach_physical_replication_stream.stream", "primary_cluster_id", primaryClusterID),
					resource.TestCheckResourceAttr("cockroach_physical_replication_stream.stream", "standby_cluster_id", standbyClusterID),
					resource.TestCheckResourceAttr("cockroach_physical_replication_stream.stream", "status", string(client.REPLICATIONSTREAMSTATUSTYPE_COMPLETED)),
				),
			},
		},
	})
}

func getTestPhysicalReplicationStreamResourceConfigCreate(primaryClusterName, standbyClusterName string) string {
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
				node_count = 3
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
				node_count = 3
    }]
}

resource "cockroach_physical_replication_stream" "stream" {
  primary_cluster_id = cockroach_cluster.primary.id
  standby_cluster_id = cockroach_cluster.standby.id
}
`, primaryClusterName, standbyClusterName)
}

func getTestPhysicalReplicationStreamResourceConfigFailover(primaryClusterName, standbyClusterName string) string {
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
				node_count = 3
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
				node_count = 3
    }]
}

resource "cockroach_physical_replication_stream" "stream" {
  primary_cluster_id = cockroach_cluster.primary.id
  standby_cluster_id = cockroach_cluster.standby.id
  failover_immediately = true
}
`, primaryClusterName, standbyClusterName)
}
