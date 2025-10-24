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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccVersionDeferralResource attempts to create, check, and destroy a
// real cluster. It will be skipped if TF_ACC isn't set.
// This test covers all deferral policy transitions in a single test to minimize resource creation.
func TestAccVersionDeferralResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-version-deferral-%s", tfTestPrefix, GenerateRandomString(4))
	policies := []string{"DEFERRAL_60_DAYS", "DEFERRAL_30_DAYS", "DEFERRAL_60_DAYS", "DEFERRAL_90_DAYS", "NOT_DEFERRED"}
	testVersionDeferralResourceMultiStep(t, clusterName, false, policies)
}

// TestIntegrationVersionDeferralResource attempts to create, check, and
// destroy a cluster, but uses a mocked API service.
// This test covers all deferral policy transitions: 60 → 30 → 60 → 90 → NOT_DEFERRED
func TestIntegrationVersionDeferralResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-deferral-%s", tfTestPrefix, GenerateRandomString(4))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	clusterInfo := getClusterInfo(clusterID, clusterName)

	policies := []string{"DEFERRAL_60_DAYS", "DEFERRAL_30_DAYS", "DEFERRAL_60_DAYS", "DEFERRAL_90_DAYS", "NOT_DEFERRED"}
	setupMockExpectationsForPolicyTransitions(s, clusterInfo, clusterID, policies)

	testVersionDeferralResourceMultiStep(t, clusterName, true, policies)
}

// Helper function to set up mock expectations for policy transitions
func setupMockExpectationsForPolicyTransitions(s *mock_client.MockService, clusterInfo *client.Cluster, clusterID string, policies []string) {
	// Map policy strings to SDK types
	policyTypeMap := versionDeferralPolicyTypeMap()
	// Allow any number of GetCluster and GetBackupConfiguration calls throughout the test
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(clusterInfo, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()

	// Create cluster - this happens first
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(clusterInfo, nil, nil).
		Times(1)

	// Set up policy transitions using InOrder to ensure sequential execution
	var calls []*gomock.Call
	for i := 0; i < len(policies); i++ {
		currentPolicy := &client.ClusterVersionDeferral{
			DeferralPolicy: policyTypeMap[policies[i]],
		}
		currentPolicyPostApply := &client.ClusterVersionDeferral{
			DeferralPolicy: policyTypeMap[policies[i]],
		}
		// Add deferred_until only if policy is not NOT_DEFERRED
		if policies[i] != "NOT_DEFERRED" {
			deferredUntil := time.Now().Add(time.Duration((i+1)*30) * 24 * time.Hour)
			currentPolicyPostApply.DeferredUntil = &deferredUntil
		}

		calls = append(calls,
			s.EXPECT().SetClusterVersionDeferral(gomock.Any(), clusterID, currentPolicy).
				Return(currentPolicyPostApply, nil, nil).
				Times(1))
		calls = append(calls,
			s.EXPECT().GetClusterVersionDeferral(gomock.Any(), clusterID).
				Return(currentPolicyPostApply, nil, nil).
				Times(2)) // Called during Read operations
	}

	gomock.InOrder(calls...)

	// Delete expectations
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Times(1)
	lastPolicy := &client.ClusterVersionDeferral{
		DeferralPolicy: policyTypeMap[policies[len(policies)-1]],
	}
	s.EXPECT().SetClusterVersionDeferral(gomock.Any(), clusterID, lastPolicy).
		Return(lastPolicy, nil, nil).
		Times(1)
}

// Helper function to test version deferral resource with multiple policy transitions
func testVersionDeferralResourceMultiStep(t *testing.T, clusterName string, useMock bool, policies []string) {
	var (
		clusterResourceName         = "cockroach_cluster.test"
		versionDeferralResourceName = "cockroach_version_deferral.test"
	)

	// Build test steps for each policy
	steps := make([]resource.TestStep, 0, len(policies)+1)
	for _, policy := range policies {
		steps = append(steps, resource.TestStep{
			Config: getTestVersionDeferralConfig(clusterName, policy),
			Check: resource.ComposeTestCheckFunc(
				testCheckCockroachClusterExists(clusterResourceName),
				resource.TestCheckResourceAttr(versionDeferralResourceName, "deferral_policy", policy),
			),
		})
	}

	// Add import state verification step at the end
	steps = append(steps, resource.TestStep{
		ResourceName:      versionDeferralResourceName,
		ImportState:       true,
		ImportStateVerify: true,
	})

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

func getClusterInfo(clusterID string, clusterName string) *client.Cluster {
	clusterInfo := &client.Cluster{
		Id:               clusterID,
		Name:             clusterName,
		CockroachVersion: "v25.2.6",
		Plan:             "ADVANCED",
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
	return clusterInfo
}

func getTestVersionDeferralConfig(name string, policy string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cockroach_version = "v25.2"
  cloud_provider = "GCP"
  dedicated = {
    storage_gib  = 35
  	num_virtual_cpus = 4
  }
  regions = [{
    name = "us-east1"
    node_count: 3
  }]
}
resource "cockroach_version_deferral" "test" {
  id              = cockroach_cluster.test.id
  deferral_policy = "%s"
}
`, name, policy)
}

func versionDeferralPolicyTypeMap() map[string]client.ClusterVersionDeferralPolicyType {
	policyMap := make(map[string]client.ClusterVersionDeferralPolicyType)
	for _, v := range client.AllowedClusterVersionDeferralPolicyTypeEnumValues {
		policyMap[string(v)] = v
	}
	return policyMap
}
