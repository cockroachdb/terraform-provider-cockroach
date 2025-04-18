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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccCMEKResource attempts to create, check, and destroy
// a real cluster and allowlist entry. It will be skipped if TF_ACC isn't set.
func TestAccCMEKResource(t *testing.T) {
	t.Skip("Skipping until we can either integrate the AWS provider " +
		"or import a permanent test fixture.")
	t.Parallel()
	clusterName := fmt.Sprintf("%s-cmek-%s", tfTestPrefix, GenerateRandomString(4))
	testCMEKResource(t, clusterName, false)
}

// TestIntegrationCMEKResource attempts to create, check, and destroy
// a cluster, but uses a mocked API service.
func TestIntegrationCMEKResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-cmek-%s", tfTestPrefix, GenerateRandomString(4))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	initialCluster := &client.Cluster{
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
				Name:      "us-central-1",
				NodeCount: 3,
			},
		},
	}
	updatedCluster := &client.Cluster{}
	*updatedCluster = *initialCluster
	updatedCluster.Regions = append(
		updatedCluster.Regions,
		[]client.Region{
			{
				Name:      "us-east-1",
				NodeCount: 3,
			},
			{
				Name:      "us-east-2",
				NodeCount: 3,
			},
		}...)

	keyType := client.CMEKKeyType("AWS_KMS")
	keyURI := "aws-kms-key-arn"
	keyPrincipal := "aws-auth-principal-arn"
	keySpec := &client.CMEKKeySpecification{
		Type:          &keyType,
		Uri:           &keyURI,
		AuthPrincipal: &keyPrincipal,
	}

	usCentral1 := "us-central-1"
	cmekCreateSpec := &client.CMEKClusterSpecification{
		RegionSpecs: []client.CMEKRegionSpecification{
			{
				Region:  &usCentral1,
				KeySpec: keySpec,
			},
		},
	}

	usEast1 := "us-east-1"
	usEast2 := "us-east-2"
	cmekUpdateRegionSpecs := []client.CMEKRegionSpecification{
		{
			Region:  &usEast1,
			KeySpec: keySpec,
		},
		{
			Region:  &usEast2,
			KeySpec: keySpec,
		},
	}
	clusterUpdateSpec := &client.UpdateClusterSpecification{
		Dedicated: &client.DedicatedClusterUpdateSpecification{
			RegionNodes: &map[string]int32{
				usCentral1: 3,
				usEast1:    3,
				usEast2:    3,
			},
			CmekRegionSpecs: &cmekUpdateRegionSpecs,
		},
	}

	cmekStatus := client.CMEKSTATUS_ENABLED
	initialCMEKInfo := &client.CMEKClusterInfo{
		Status: &cmekStatus,
		RegionInfos: &[]client.CMEKRegionInfo{
			{
				Region: &usCentral1,
				Status: &cmekStatus,
				KeyInfos: &[]client.CMEKKeyInfo{
					{
						Status: &cmekStatus,
						Spec:   keySpec,
					},
				},
			},
		},
	}
	updatedCMEKInfo := &client.CMEKClusterInfo{
		Status: &cmekStatus,
		RegionInfos: &[]client.CMEKRegionInfo{
			{
				Region: &usCentral1,
				Status: &cmekStatus,
				KeyInfos: &[]client.CMEKKeyInfo{
					{
						Status: &cmekStatus,
						Spec:   keySpec,
					},
				},
			},
			{
				Region: &usEast1,
				Status: &cmekStatus,
				KeyInfos: &[]client.CMEKKeyInfo{
					{
						Status: &cmekStatus,
						Spec:   keySpec,
					},
				},
			},
			{
				Region: &usEast2,
				Status: &cmekStatus,
				KeyInfos: &[]client.CMEKKeyInfo{
					{
						Status: &cmekStatus,
						Spec:   keySpec,
					},
				},
			},
		},
	}

	// Create
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(initialCluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(initialCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(3)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().EnableCMEKSpec(gomock.Any(), clusterID, cmekCreateSpec).
		Return(initialCMEKInfo, nil, nil)
	s.EXPECT().GetCMEKClusterInfo(gomock.Any(), clusterID).
		Return(initialCMEKInfo, nil, nil).
		Times(2)

	// Update
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(initialCluster, nil, nil).
		Times(2)
	s.EXPECT().GetCMEKClusterInfo(gomock.Any(), clusterID).
		Return(initialCMEKInfo, nil, nil).Times(2)
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, clusterUpdateSpec).
		Return(updatedCluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(updatedCluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(2)
	s.EXPECT().GetCMEKClusterInfo(gomock.Any(), clusterID).
		Return(updatedCMEKInfo, nil, nil).
		Times(2)

	// Delete
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testCMEKResource(t, clusterName, true)
}

func testCMEKResource(t *testing.T, clusterName string, useMock bool) {
	var (
		clusterResourceName = "cockroach_cluster.test"
		cmekResourceName    = "cockroach_cmek.test"
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestCMEKResourceCreateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists(clusterResourceName),
				),
			},
			{
				// Test import functionality here, because an imported CMEK resource won't have additional_regions.
				ResourceName:      cmekResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: getTestCMEKResourceUpdateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					// The original region should only show up under the cluster resource,
					// and the two additional regions should only show up under the CMEK resource.
					resource.TestCheckResourceAttr(clusterResourceName, "regions.#", "1"),
					resource.TestCheckResourceAttr(cmekResourceName, "additional_regions.#", "2"),
					resource.TestCheckResourceAttr(cmekResourceName, "regions.#", "3"),
				),
			},
		},
	})
}

func getTestCMEKResourceCreateConfig(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  dedicated = {
    storage_gib = 35
  	num_virtual_cpus = 4
  }
  regions = [{
    name = "us-central-1"
    node_count: 3
  }]
}

resource "cockroach_cmek" "test" {
	id = cockroach_cluster.test.id
	regions = [{
		region: "us-central-1"
		key: {
			auth_principal: "aws-auth-principal-arn"
			type: "AWS_KMS"
			uri: "aws-kms-key-arn"
		}
	}]
}
`, name)
}

func getTestCMEKResourceUpdateConfig(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  dedicated = {
    storage_gib = 35
  	num_virtual_cpus = 4
  }
  regions = [{
    name = "us-central-1"
    node_count: 3
  }]
}

resource "cockroach_cmek" "test" {
	id = cockroach_cluster.test.id
	regions = [
		{
			region: "us-central-1"
			key: {
				auth_principal: "aws-auth-principal-arn"
				type: "AWS_KMS"
				uri: "aws-kms-key-arn"
			}
		},
		{
			region: "us-east-1"
			key: {
				auth_principal: "aws-auth-principal-arn"
				type: "AWS_KMS"
				uri: "aws-kms-key-arn"
			}
		},
		{
			region: "us-east-2"
			key: {
				auth_principal: "aws-auth-principal-arn"
				type: "AWS_KMS"
				uri: "aws-kms-key-arn"
			}
		}
	]
	additional_regions = [
		{
			name = "us-east-1"
			node_count: 3
		},
		{
			name = "us-east-2"
			node_count: 3
		}
	]
}
`, name)
}
