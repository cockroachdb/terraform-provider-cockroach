/*
Copyright 2025 The Cockroach Authors

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

// egressTrafficPolicyTestFixture holds common test setup for egress traffic policy tests.
type egressTrafficPolicyTestFixture struct {
	clusterID string
	mock      *mock_client.MockService
	ctrl      *gomock.Controller
	cleanups  []func()
}

// TestIntegrationEgressTrafficPolicyResource tests create, update, and import operations
// in a multi-step test following the pattern from version_deferral_test.go.
func TestIntegrationEgressTrafficPolicyResource(t *testing.T) {
	f := setupEgressTrafficPolicyTest(t)
	defer f.cleanup()

	resourceName := "cockroach_egress_traffic_policy.test"

	// Track current policy state for mock responses
	currentAllowAll := true // Start with default ALLOW_ALL

	// Step 1: Create with allow_all=false — SetEgressTrafficPolicy x1, GetCluster x3
	// Step 2: Update to allow_all=true — SetEgressTrafficPolicy x1, GetCluster x4
	// Step 3: Update to allow_all=false — SetEgressTrafficPolicy x1, GetCluster x4
	// Step 4: Import — GetCluster x1
	// Destroy: Reset to allow_all=true — SetEgressTrafficPolicy x1, GetCluster x2
	// Total: SetEgressTrafficPolicy = 4, GetCluster = 14

	// Mock GetCluster to return current policy state
	f.mock.EXPECT().GetCluster(gomock.Any(), f.clusterID).
		DoAndReturn(func(ctx context.Context, clusterID string) (*client.Cluster, *http.Response, error) {
			policy := client.EGRESSTRAFFICPOLICYTYPE_ALLOW_ALL
			if !currentAllowAll {
				policy = client.EGRESSTRAFFICPOLICYTYPE_DEFAULT_DENY
			}
			return &client.Cluster{
				Id:                  clusterID,
				EgressTrafficPolicy: &policy,
			}, nil, nil
		}).Times(14)

	// Mock SetEgressTrafficPolicy to update state
	f.mock.EXPECT().SetEgressTrafficPolicy(gomock.Any(), f.clusterID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, clusterID string, req *client.SetEgressTrafficPolicyRequest) (*http.Response, error) {
			currentAllowAll = req.AllowAll
			return &http.Response{StatusCode: http.StatusOK}, nil
		}).Times(4)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: Create with allow_all = false
			{
				Config: getEgressTrafficPolicyConfig(f.clusterID, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", f.clusterID),
					resource.TestCheckResourceAttr(resourceName, "allow_all", "false"),
				),
			},
			// Step 2: Update to allow_all = true
			{
				Config: getEgressTrafficPolicyConfig(f.clusterID, true),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", f.clusterID),
					resource.TestCheckResourceAttr(resourceName, "allow_all", "true"),
				),
			},
			// Step 3: Update back to allow_all = false
			{
				Config: getEgressTrafficPolicyConfig(f.clusterID, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", f.clusterID),
					resource.TestCheckResourceAttr(resourceName, "allow_all", "false"),
				),
			},
			// Step 4: Import
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestIntegrationEgressTrafficPolicyResource_ClusterNotFound tests that the resource
// is removed from state when the cluster returns 404 during Read.
func TestIntegrationEgressTrafficPolicyResource_ClusterNotFound(t *testing.T) {
	f := setupEgressTrafficPolicyTest(t)
	defer f.cleanup()

	resourceName := "cockroach_egress_traffic_policy.test"
	simulateClusterDeleted := false

	// Step 1: Create with allow_all=false — SetEgressTrafficPolicy x1, GetCluster x3
	// Step 2: Read returns 404, then re-create — SetEgressTrafficPolicy x1, GetCluster x4 (one returns 404)
	// Destroy: Reset to allow_all=true — SetEgressTrafficPolicy x1, GetCluster x2
	// Total: SetEgressTrafficPolicy = 3, GetCluster = 9

	// Mock GetCluster: return 404 when cluster is "deleted", otherwise succeed
	f.mock.EXPECT().GetCluster(gomock.Any(), f.clusterID).
		DoAndReturn(func(ctx context.Context, clusterID string) (*client.Cluster, *http.Response, error) {
			if simulateClusterDeleted {
				// Return 404 once, then "restore" the cluster for subsequent operations
				simulateClusterDeleted = false
				return nil, &http.Response{StatusCode: http.StatusNotFound}, fmt.Errorf("cluster not found")
			}
			policy := client.EGRESSTRAFFICPOLICYTYPE_DEFAULT_DENY
			return &client.Cluster{
				Id:                  clusterID,
				EgressTrafficPolicy: &policy,
			}, nil, nil
		}).Times(9)

	// Mock SetEgressTrafficPolicy
	f.mock.EXPECT().SetEgressTrafficPolicy(gomock.Any(), f.clusterID, gomock.Any()).
		Return(&http.Response{StatusCode: http.StatusOK}, nil).Times(3)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create the resource
			{
				Config: getEgressTrafficPolicyConfig(f.clusterID, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", f.clusterID),
				),
			},
			// Trigger cluster "deletion" - Read will get 404, remove from state,
			// then Create will run again (cluster is "restored" for this)
			{
				PreConfig: func() {
					simulateClusterDeleted = true
				},
				Config: getEgressTrafficPolicyConfig(f.clusterID, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", f.clusterID),
				),
			},
		},
	})
}

// TestEgressTrafficPolicyRetryFuncs tests retry behavior for egress traffic policy helper functions.
func TestEgressTrafficPolicyRetryFuncs(t *testing.T) {
	t.Run("setEgressTrafficPolicyRetryFunc", func(t *testing.T) {
		// Reuse retryTestCase from egress_perimeter_resource_test.go
		testCases := []retryTestCase{
			{name: "succeeds on no error", statusCode: http.StatusOK, errorMsg: "", expectNil: true},
			{name: "retries on 503", statusCode: http.StatusServiceUnavailable, errorMsg: "service unavailable", expectRetry: true},
			{name: "retries on lock error", statusCode: http.StatusConflict, errorMsg: "cluster lock is held", expectRetry: true},
			{name: "retries on POLICY_UPDATING", statusCode: http.StatusConflict, errorMsg: "POLICY_UPDATING: policy updating", expectRetry: true},
			{name: "fails fast on 400", statusCode: http.StatusBadRequest, errorMsg: "invalid request", expectRetry: false},
			{name: "retries on 500", statusCode: http.StatusInternalServerError, errorMsg: "internal error", expectRetry: true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockService := mock_client.NewMockService(ctrl)
				clusterID := uuid.New().String()

				if tc.expectNil {
					mockService.EXPECT().SetEgressTrafficPolicy(gomock.Any(), clusterID, gomock.Any()).
						Return(&http.Response{StatusCode: tc.statusCode}, nil)
				} else {
					mockService.EXPECT().SetEgressTrafficPolicy(gomock.Any(), clusterID, gomock.Any()).
						Return(&http.Response{StatusCode: tc.statusCode}, fmt.Errorf(tc.errorMsg))
				}

				retryErr := setEgressTrafficPolicyRetryFunc(context.Background(), mockService, clusterID, true)()
				assertRetryBehavior(t, tc, retryErr)
			})
		}
	})

	t.Run("waitForEgressTrafficPolicyStableFunc", func(t *testing.T) {
		stateTestCases := []struct {
			waitForStateTestCase
			policy client.EgressTrafficPolicyType
		}{
			{waitForStateTestCase{"succeeds on ALLOW_ALL", true, false}, client.EGRESSTRAFFICPOLICYTYPE_ALLOW_ALL},
			{waitForStateTestCase{"succeeds on DEFAULT_DENY", true, false}, client.EGRESSTRAFFICPOLICYTYPE_DEFAULT_DENY},
			{waitForStateTestCase{"retries on UPDATING", false, true}, client.EGRESSTRAFFICPOLICYTYPE_UPDATING},
			{waitForStateTestCase{"fails fast on ERROR", false, false}, client.EGRESSTRAFFICPOLICYTYPE_ERROR},
		}

		for _, tc := range stateTestCases {
			t.Run(tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockService := mock_client.NewMockService(ctrl)
				clusterID := uuid.New().String()

				mockService.EXPECT().GetCluster(gomock.Any(), clusterID).
					Return(&client.Cluster{
						Id:                  clusterID,
						EgressTrafficPolicy: &tc.policy,
					}, nil, nil)

				retryErr := waitForEgressTrafficPolicyStableFunc(context.Background(), mockService, clusterID)()
				assertWaitResult(t, tc.waitForStateTestCase, retryErr)
			})
		}

		// Test error handling
		errorTestCases := []struct {
			waitForStateTestCase
			statusCode int
		}{
			{waitForStateTestCase{"retries on server error", false, true}, http.StatusInternalServerError},
			{waitForStateTestCase{"fails fast on client error", false, false}, http.StatusBadRequest},
		}

		for _, tc := range errorTestCases {
			t.Run(tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockService := mock_client.NewMockService(ctrl)
				clusterID := uuid.New().String()

				mockService.EXPECT().GetCluster(gomock.Any(), clusterID).
					Return(nil, &http.Response{StatusCode: tc.statusCode}, fmt.Errorf("error"))

				retryErr := waitForEgressTrafficPolicyStableFunc(context.Background(), mockService, clusterID)()
				assertWaitResult(t, tc.waitForStateTestCase, retryErr)
			})
		}
	})
}

// setupEgressTrafficPolicyTest creates a new test fixture with common setup.
func setupEgressTrafficPolicyTest(t *testing.T) *egressTrafficPolicyTestFixture {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	fixture := &egressTrafficPolicyTestFixture{
		clusterID: uuid.Nil.String(),
	}

	fixture.cleanups = append(fixture.cleanups, HookGlobal(&egressTrafficPolicyStableTimeout, 5*time.Second))

	fixture.ctrl = gomock.NewController(t)
	fixture.mock = mock_client.NewMockService(fixture.ctrl)
	fixture.cleanups = append(fixture.cleanups, HookGlobal(&NewService, func(c *client.Client) client.Service {
		return fixture.mock
	}))

	return fixture
}

// cleanup runs all cleanup functions in reverse order.
func (f *egressTrafficPolicyTestFixture) cleanup() {
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
}

func getEgressTrafficPolicyConfig(clusterID string, allowAll bool) string {
	return fmt.Sprintf(`
resource "cockroach_egress_traffic_policy" "test" {
    id        = "%s"
    allow_all = %t
}
`, clusterID, allowAll)
}
