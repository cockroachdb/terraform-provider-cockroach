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
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v4/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDedicatedPrivateEndpointTrustedOwnerResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-trstd-own-%s", tfTestPrefix, GenerateRandomString(2))
	testPrivateEndpointTrustedOwnerResource(t, clusterName, false /* useMock */)
}

func TestIntegrationPrivateEndpointTrustedOwnerResource(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	var (
		clusterID = uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")).String()
		ownerID   = uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000002")).String()
	)

	// States in the server.
	cluster := client.Cluster{
		Name:          fmt.Sprintf("%s-trstd-own-%s", tfTestPrefix, GenerateRandomString(2)),
		Id:            clusterID,
		CloudProvider: "AWS",
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				StorageGib:     15,
				NumVirtualCpus: 2,
			},
		},
		State: "CREATED",
		Regions: []client.Region{
			{
				Name:      "us-east-1",
				NodeCount: 1,
			},
		},
	}
	trustedOwner := client.PrivateEndpointTrustedOwner{
		Id:              ownerID,
		ClusterId:       cluster.Id,
		CreatedAt:       time.Now(),
		ExternalOwnerId: "012301230123",
		Type:            client.PRIVATEENDPOINTTRUSTEDOWNERTYPETYPE_AWS_ACCOUNT_ID,
	}

	// Create and read cluster.
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), cluster.Id).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(2)

	// Create owner entry.
	s.EXPECT().AddPrivateEndpointTrustedOwner(
		gomock.Any(),
		cluster.Id,
		&client.AddPrivateEndpointTrustedOwnerRequest{
			Type:            client.PRIVATEENDPOINTTRUSTEDOWNERTYPETYPE_AWS_ACCOUNT_ID,
			ExternalOwnerId: trustedOwner.ExternalOwnerId,
		},
	).Return(&client.AddPrivateEndpointTrustedOwnerResponse{
		TrustedOwner: trustedOwner,
	}, nil, nil)

	// Read and import owner entry.
	s.EXPECT().GetPrivateEndpointTrustedOwner(
		gomock.Any(), cluster.Id, trustedOwner.Id,
	).Return(&client.GetPrivateEndpointTrustedOwnerResponse{
		TrustedOwner: trustedOwner,
	}, nil, nil).Times(2)

	// Delete both cluster and owner entry.
	s.EXPECT().RemovePrivateEndpointTrustedOwner(
		gomock.Any(), cluster.Id, trustedOwner.Id,
	).Return(&client.RemovePrivateEndpointTrustedOwnerResponse{
		TrustedOwner: trustedOwner,
	}, nil, nil)
	s.EXPECT().DeleteCluster(gomock.Any(), cluster.Id)

	testPrivateEndpointTrustedOwnerResource(t, cluster.Name, true /* useMock */)
}

// TODO(jaylim-crl): Update this to also test serverless once we support this
// feature for isServerless clusters within CockroachCloud.
func testPrivateEndpointTrustedOwnerResource(t *testing.T, clusterName string, useMock bool) {
	const resourceName = "cockroach_private_endpoint_trusted_owner.owner"
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestPrivateEndpointTrustedOwnerResourceConfigForDedicated(clusterName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "external_owner_id", "012301230123"),
					resource.TestCheckResourceAttrSet(resourceName, "cluster_id"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func getTestPrivateEndpointTrustedOwnerResourceConfigForDedicated(clusterName string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "dedicated" {
    name           = "%s"
    cloud_provider = "AWS"
    dedicated = {
        storage_gib = 15
        num_virtual_cpus = 2
    }
    regions = [{
        name: "us-east-1"
        node_count: 1
    }]
}
resource "cockroach_private_endpoint_trusted_owner" "owner" {
    cluster_id        = cockroach_cluster.dedicated.id
    type              = "AWS_ACCOUNT_ID"
    external_owner_id = "012301230123"
}
`, clusterName)
}
