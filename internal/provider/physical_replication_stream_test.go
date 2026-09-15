package provider

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v10/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// pcrFailoverReadyTimeout bounds the wait for the initial scan to finish. The
// test clusters are empty, so this should be reached quickly.
const pcrFailoverReadyTimeout = 15 * time.Minute

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

// TestAccServerlessClusterResource attempts to create a real physical
// replication stream between two real clusters. It will be skipped if
// TF_ACC isn't set.
func TestAccPhysicalReplicationStreamResource(t *testing.T) {
	primaryClusterName := fmt.Sprintf("primary-cluster-%s", GenerateRandomString(4))
	standbyClusterName := fmt.Sprintf("standby-cluster-%s", GenerateRandomString(4))
	testPhysicalReplicationStreamResource(t, primaryClusterName, standbyClusterName, false)
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

	primaryCluster, standbyCluster := pcrTestClusters(
		primaryClusterName, standbyClusterName, primaryClusterID, standbyClusterID)

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

	// The stream needs to start REPLICATING and then become COMPLETED.
	completedReplicationStream := statuslessReplicationStream
	completedReplicationStream.Status = client.REPLICATIONSTREAMSTATUSTYPE_COMPLETED
	s.EXPECT().GetPhysicalReplicationStream(gomock.Any(), replicationID).Return(&replicatingReplicationStream, nil, nil).Times(1)
	s.EXPECT().GetPhysicalReplicationStream(gomock.Any(), replicationID).Return(&completedReplicationStream, nil, nil).AnyTimes()

	// One update call for failover.
	completedStream := statuslessReplicationStream
	completedStream.Status = client.REPLICATIONSTREAMSTATUSTYPE_COMPLETED
	s.EXPECT().UpdatePhysicalReplicationStream(gomock.Any(), replicationID, gomock.Any()).Return(&completedStream, nil, nil).Times(1)

	s.EXPECT().DeleteCluster(gomock.Any(), primaryClusterID)
	s.EXPECT().DeleteCluster(gomock.Any(), standbyClusterID)

	testPhysicalReplicationStreamResource(t, primaryClusterName, standbyClusterName, true)
}

// TestIntegrationPhysicalReplicationStreamDestroyCancelsStream checks that
// destroying a stream that is still replicating cancels it through the API
// rather than only dropping it from state. An uncanceled stream blocks deletion
// of both clusters.
func TestIntegrationPhysicalReplicationStreamDestroyCancelsStream(t *testing.T) {
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

	primaryCluster, standbyCluster := pcrTestClusters(
		primaryClusterName, standbyClusterName, primaryClusterID, standbyClusterID)

	now := time.Now()
	replicatedTime := now.Add(-1 * time.Hour)
	retainedTime := now.Add(-24 * time.Hour)
	replicatingStream := client.PhysicalReplicationStream{
		Id:                    replicationID,
		PrimaryClusterId:      primaryClusterID,
		StandbyClusterId:      standbyClusterID,
		CreatedAt:             now,
		ReplicatedTime:        &replicatedTime,
		ReplicationLagSeconds: ptr(int32(30)),
		RetainedTime:          &retainedTime,
		Status:                client.REPLICATIONSTREAMSTATUSTYPE_REPLICATING,
	}
	canceledStream := replicatingStream
	canceledStream.Status = client.REPLICATIONSTREAMSTATUSTYPE_CANCELED

	s.EXPECT().CreateCluster(gomock.Any(), clusterReqMatcher{ExpectedName: primaryClusterName}).Return(&primaryCluster, nil, nil).Times(1)
	s.EXPECT().CreateCluster(gomock.Any(), clusterReqMatcher{ExpectedName: standbyClusterName}).Return(&standbyCluster, nil, nil).Times(1)
	s.EXPECT().CreatePhysicalReplicationStream(gomock.Any(), gomock.Any()).Return(&replicatingStream, nil, nil).Times(1)

	s.EXPECT().GetCluster(gomock.Any(), primaryClusterID).Return(&primaryCluster, nil, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), standbyClusterID).Return(&standbyCluster, nil, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), primaryClusterID).Return(initialBackupConfig, nil, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), standbyClusterID).Return(initialBackupConfig, nil, nil).AnyTimes()

	var canceled bool
	s.EXPECT().GetPhysicalReplicationStream(gomock.Any(), replicationID).DoAndReturn(
		func(_ context.Context, _ string) (*client.PhysicalReplicationStream, *http.Response, error) {
			if canceled {
				return &canceledStream, nil, nil
			}
			return &replicatingStream, nil, nil
		}).AnyTimes()

	// Destroy must send exactly one cancellation. The controller fails the test
	// if it never arrives.
	s.EXPECT().UpdatePhysicalReplicationStream(gomock.Any(), replicationID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, spec *client.UpdatePhysicalReplicationStreamSpec) (*client.PhysicalReplicationStream, *http.Response, error) {
			if spec.Status == nil || *spec.Status != client.REPLICATIONSTREAMSTATUSTYPE_CANCELED {
				return nil, nil, fmt.Errorf(
					"expected a CANCELED status update, got %q", spec.GetStatus())
			}
			canceled = true
			return &canceledStream, nil, nil
		}).Times(1)

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
					testCheckPhysicalReplicationStreamExists("cockroach_physical_replication_stream.stream"),
					resource.TestCheckResourceAttr("cockroach_physical_replication_stream.stream", "status", string(client.REPLICATIONSTREAMSTATUSTYPE_REPLICATING)),
				),
			},
		},
	})
}

func pcrTestClusters(
	primaryClusterName, standbyClusterName, primaryClusterID, standbyClusterID string,
) (primary, standby client.Cluster) {
	primary = client.Cluster{
		Name:          primaryClusterName,
		Id:            primaryClusterID,
		CloudProvider: "GCP",
		State:         "CREATED",
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				StorageGib:     15,
				NumVirtualCpus: 4,
			},
		},
		Regions: []client.Region{
			{
				Name:      "us-central1",
				NodeCount: 3,
			},
		},
		CidrRange: "172.28.0.0/19",
	}

	standby = client.Cluster{
		Name:          standbyClusterName,
		Id:            standbyClusterID,
		CloudProvider: "GCP",
		State:         "CREATED",
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				StorageGib:     15,
				NumVirtualCpus: 4,
			},
		},
		Regions: []client.Region{
			{
				Name:      "us-east1",
				NodeCount: 3,
			},
		},
		CidrRange: "172.29.0.0/19",
	}

	return primary, standby
}

func testPhysicalReplicationStreamResource(
	t *testing.T,
	primaryClusterName, standbyClusterName string,
	useMock bool) {
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestPhysicalReplicationStreamResourceConfigCreate(primaryClusterName, standbyClusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists("cockroach_cluster.primary"),
					testCheckCockroachClusterExists("cockroach_cluster.standby"),
					testCheckPhysicalReplicationStreamExists("cockroach_physical_replication_stream.stream"),
					resource.TestCheckResourceAttr("cockroach_physical_replication_stream.stream", "status", string(client.REPLICATIONSTREAMSTATUSTYPE_REPLICATING)),
					testWaitForPhysicalReplicationStreamFailoverReady("cockroach_physical_replication_stream.stream"),
				),
			},
			{
				Config: getTestPhysicalReplicationStreamResourceConfigFailover(primaryClusterName, standbyClusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists("cockroach_cluster.primary"),
					testCheckCockroachClusterExists("cockroach_cluster.standby"),
					testCheckPhysicalReplicationStreamExists("cockroach_physical_replication_stream.stream"),
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
        num_virtual_cpus = 4
        cidr_range = "172.28.0.0/19"
        supports_cluster_virtualization = true
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
        num_virtual_cpus = 4
        cidr_range = "172.29.0.0/19"
        supports_cluster_virtualization = true
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
        num_virtual_cpus = 4
        cidr_range = "172.28.0.0/19"
        supports_cluster_virtualization = true
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
        num_virtual_cpus = 4
        cidr_range = "172.29.0.0/19"
        supports_cluster_virtualization = true
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

// testWaitForPhysicalReplicationStreamFailoverReady blocks until the stream
// reports a replicated time. A stream reports REPLICATING as soon as its
// ingestion job starts, but the API rejects failover until the initial scan
// finishes and produces a replicated time.
func testWaitForPhysicalReplicationStreamFailoverReady(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		p := testAccProvider.(*provider)
		p.service = NewService(cl)
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}
		id := rs.Primary.Attributes["id"]

		ctx := context.Background()
		return retry.RetryContext(ctx, pcrFailoverReadyTimeout, func() *retry.RetryError {
			traceAPICall("GetPhysicalReplicationStream")
			stream, _, err := p.service.GetPhysicalReplicationStream(ctx, id)
			if err != nil {
				return retry.NonRetryableError(err)
			}
			// A stream with no replicated time serializes it as the zero
			// timestamp rather than omitting the field.
			if stream.ReplicatedTime == nil || stream.ReplicatedTime.IsZero() {
				return retry.RetryableError(
					fmt.Errorf("replication stream %s has not replicated any data yet", id))
			}
			return nil
		})
	}
}

func testCheckPhysicalReplicationStreamExists(resourceName string) resource.TestCheckFunc {
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

		traceAPICall("GetPhysicalReplicationStream")
		if _, _, err := p.service.GetPhysicalReplicationStream(context.Background(), id); err == nil {
			return nil
		}

		return fmt.Errorf("cluster(%s:%s) does not exist", rs.Primary.Attributes["id"], rs.Primary.ID)
	}
}
