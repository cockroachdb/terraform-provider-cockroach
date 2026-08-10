/*
Copyright 2026 The Cockroach Authors

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
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v9/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/require"
)

// TestAccClusterRuntimeScanningResource attempts to create, check, and destroy a
// real cluster with runtime scanning enabled. It will be skipped if TF_ACC isn't
// set. The cluster's organization must be entitled to runtime scanning (HBTD) for
// this test to pass.
func TestAccClusterRuntimeScanningResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-runtime-scan-%s", tfTestPrefix, GenerateRandomString(4))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterRuntimeScanningConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testCheckCockroachClusterExists("cockroach_cluster.test"),
					resource.TestCheckResourceAttr(runtimeScanningResourceName, "type", "WIZ"),
					resource.TestCheckResourceAttrSet(runtimeScanningResourceName, "cluster_id"),
				),
			},
			{
				ResourceName:      runtimeScanningResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestIntegrationClusterRuntimeScanningResource creates, checks, imports, and
// destroys the resource against a mocked API service. It references a pre-existing
// cluster by ID (rather than creating one) so that every mocked call has an exact,
// deterministic invocation count.
func TestIntegrationClusterRuntimeScanningResource(t *testing.T) {
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	clusterInfo := getClusterInfo(clusterID, "runtime-scan-cluster")
	enabledInfo := &client.RuntimeScanningInfo{Type: client.RUNTIMESCANNINGTYPE_WIZ}
	disabledInfo := &client.RuntimeScanningInfo{Type: client.RUNTIMESCANNINGTYPE_NONE}

	// GetCluster is called once, by Create's dedicated-cluster pre-check.
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(clusterInfo, httpOk, nil).Times(1)
	// Enable is called once, on create.
	s.EXPECT().EnableClusterRuntimeScanning(gomock.Any(), clusterID,
		client.NewEnableClusterRuntimeScanningBody(client.RUNTIMESCANNINGTYPE_WIZ)).
		Return(enabledInfo, nil, nil).Times(1)
	// Read runs on the create step's readiness wait and post-apply refresh, and on
	// import.
	s.EXPECT().GetClusterRuntimeScanning(gomock.Any(), clusterID).
		Return(enabledInfo, nil, nil).Times(3)
	// Disable is called once, on destroy.
	s.EXPECT().DisableClusterRuntimeScanning(gomock.Any(), clusterID).
		Return(disabledInfo, nil, nil).Times(1)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestClusterRuntimeScanningConfig(clusterID, "WIZ"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(runtimeScanningResourceName, "type", "WIZ"),
					resource.TestCheckResourceAttr(runtimeScanningResourceName, "cluster_id", clusterID),
					resource.TestCheckResourceAttr(runtimeScanningResourceName, "id", clusterID),
				),
			},
			{
				ResourceName:      runtimeScanningResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestIntegrationClusterRuntimeScanningDisableDrift verifies that when runtime
// scanning is disabled out-of-band (GetClusterRuntimeScanning reports type NONE),
// Read drops the resource from state and a subsequent plan wants to re-create it.
func TestIntegrationClusterRuntimeScanningDisableDrift(t *testing.T) {
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	clusterInfo := getClusterInfo(clusterID, "runtime-scan-cluster")
	enabledInfo := &client.RuntimeScanningInfo{Type: client.RUNTIMESCANNINGTYPE_WIZ}
	disabledInfo := &client.RuntimeScanningInfo{Type: client.RUNTIMESCANNINGTYPE_NONE}

	// GetCluster: once on create.
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(clusterInfo, httpOk, nil).Times(1)
	// Enable: once on create. The drift step is plan-only.
	s.EXPECT().EnableClusterRuntimeScanning(gomock.Any(), clusterID,
		client.NewEnableClusterRuntimeScanningBody(client.RUNTIMESCANNINGTYPE_WIZ)).
		Return(enabledInfo, nil, nil).Times(1)
	// The create step reads WIZ twice (readiness wait + post-apply refresh); the
	// drift step's refresh then reads NONE, which drops the resource from state and
	// makes the plan non-empty.
	gomock.InOrder(
		s.EXPECT().GetClusterRuntimeScanning(gomock.Any(), clusterID).
			Return(enabledInfo, nil, nil).Times(2),
		s.EXPECT().GetClusterRuntimeScanning(gomock.Any(), clusterID).
			Return(disabledInfo, nil, nil).Times(1),
	)
	// The plan-only drift step does not persist the removal, so the resource is
	// still in state at teardown: destroy disables scanning once.
	s.EXPECT().DisableClusterRuntimeScanning(gomock.Any(), clusterID).
		Return(disabledInfo, nil, nil).Times(1)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestClusterRuntimeScanningConfig(clusterID, "WIZ"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(runtimeScanningResourceName, "type", "WIZ"),
				),
			},
			{
				// The out-of-band disable makes the next refresh see NONE, drop the
				// resource, and plan a re-create.
				Config:             getTestClusterRuntimeScanningConfig(clusterID, "WIZ"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

const runtimeScanningResourceName = "cockroach_cluster_runtime_scanning.test"

// testAccClusterRuntimeScanningConfig returns a config that creates a real cluster
// and enables runtime scanning on it (used by the acceptance test).
func testAccClusterRuntimeScanningConfig(name string) string {
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
resource "cockroach_cluster_runtime_scanning" "test" {
  cluster_id = cockroach_cluster.test.id
  type       = "WIZ"
}
`, name)
}

// getTestClusterRuntimeScanningConfig returns a config that manages runtime
// scanning on a pre-existing cluster referenced by ID (no cluster resource).
func getTestClusterRuntimeScanningConfig(clusterID string, scanType string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster_runtime_scanning" "test" {
  cluster_id = "%s"
  type       = "%s"
}
`, clusterID, scanType)
}

func TestRetryEnableClusterRuntimeScanning_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()
	cluster := &client.Cluster{}
	body := client.NewEnableClusterRuntimeScanningBody(client.RUNTIMESCANNINGTYPE_WIZ)
	apiObj := &client.RuntimeScanningInfo{}

	s.EXPECT().EnableClusterRuntimeScanning(gomock.Any(), clusterID, body).
		Return(&client.RuntimeScanningInfo{Type: client.RUNTIMESCANNINGTYPE_WIZ},
			&http.Response{StatusCode: http.StatusOK}, nil)

	result := retryEnableClusterRuntimeScanning(context.Background(), s, clusterID, cluster, body, apiObj)()
	require.Nil(t, result, "expected nil for successful enable")
	require.Equal(t, client.RUNTIMESCANNINGTYPE_WIZ, apiObj.GetType())
}

func TestRetryEnableClusterRuntimeScanning_NonRetryable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()
	cluster := &client.Cluster{}
	body := client.NewEnableClusterRuntimeScanningBody(client.RUNTIMESCANNINGTYPE_WIZ)
	apiObj := &client.RuntimeScanningInfo{}

	s.EXPECT().EnableClusterRuntimeScanning(gomock.Any(), clusterID, body).
		Return(nil, &http.Response{StatusCode: http.StatusBadRequest}, errors.New("bad request"))

	result := retryEnableClusterRuntimeScanning(context.Background(), s, clusterID, cluster, body, apiObj)()
	require.NotNil(t, result)
	require.False(t, result.Retryable, "expected non-retryable error for a 400, got retryable")
}

// A transient 5xx (or a nil response from a transport failure) is retried rather
// than aborting the operation. Covers the shared helper's server-error branch.
func TestRetryEnableClusterRuntimeScanning_ServerErrorRetryable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()
	cluster := &client.Cluster{}
	body := client.NewEnableClusterRuntimeScanningBody(client.RUNTIMESCANNINGTYPE_WIZ)
	apiObj := &client.RuntimeScanningInfo{}

	s.EXPECT().EnableClusterRuntimeScanning(gomock.Any(), clusterID, body).
		Return(nil, &http.Response{StatusCode: http.StatusBadGateway}, errors.New("bad gateway"))

	result := retryEnableClusterRuntimeScanning(context.Background(), s, clusterID, cluster, body, apiObj)()
	require.NotNil(t, result)
	require.True(t, result.Retryable, "expected a 502 to be retried, got non-retryable")
}

func TestRetryDisableClusterRuntimeScanning_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()

	s.EXPECT().DisableClusterRuntimeScanning(gomock.Any(), clusterID).
		Return(&client.RuntimeScanningInfo{Type: client.RUNTIMESCANNINGTYPE_NONE},
			&http.Response{StatusCode: http.StatusOK}, nil)

	result := retryDisableClusterRuntimeScanning(context.Background(), s, clusterID, &client.Cluster{})()
	require.Nil(t, result, "expected nil for successful disable")
}

func TestRetryDisableClusterRuntimeScanning_NotFoundSwallowed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()

	// An already-gone config/cluster (404) is treated as a successful delete.
	s.EXPECT().DisableClusterRuntimeScanning(gomock.Any(), clusterID).
		Return(nil, &http.Response{StatusCode: http.StatusNotFound}, errors.New("not found"))

	result := retryDisableClusterRuntimeScanning(context.Background(), s, clusterID, &client.Cluster{})()
	require.Nil(t, result, "expected 404 to be swallowed as success")
}

func TestWaitForClusterRuntimeScanningReady_Ready(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()
	out := &client.RuntimeScanningInfo{}

	s.EXPECT().GetClusterRuntimeScanning(gomock.Any(), clusterID).
		Return(&client.RuntimeScanningInfo{Type: client.RUNTIMESCANNINGTYPE_WIZ},
			&http.Response{StatusCode: http.StatusOK}, nil)

	result := waitForClusterRuntimeScanningReadyFunc(
		context.Background(), clusterID, client.RUNTIMESCANNINGTYPE_WIZ, s, out)()
	require.Nil(t, result, "expected nil once the read reflects the desired type")
	require.Equal(t, client.RUNTIMESCANNINGTYPE_WIZ, out.GetType())
}

func TestWaitForClusterRuntimeScanningReady_Pending(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()
	out := &client.RuntimeScanningInfo{}

	// Read still reports NONE while enablement rolls out -> retry.
	s.EXPECT().GetClusterRuntimeScanning(gomock.Any(), clusterID).
		Return(&client.RuntimeScanningInfo{Type: client.RUNTIMESCANNINGTYPE_NONE},
			&http.Response{StatusCode: http.StatusOK}, nil)

	result := waitForClusterRuntimeScanningReadyFunc(
		context.Background(), clusterID, client.RUNTIMESCANNINGTYPE_WIZ, s, out)()
	require.NotNil(t, result)
	require.True(t, result.Retryable, "expected retryable while not yet the desired type")
}

func TestWaitForClusterRuntimeScanningReady_ClientErrorTerminal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()
	out := &client.RuntimeScanningInfo{}

	s.EXPECT().GetClusterRuntimeScanning(gomock.Any(), clusterID).
		Return(nil, &http.Response{StatusCode: http.StatusBadRequest}, errors.New("bad request"))

	result := waitForClusterRuntimeScanningReadyFunc(
		context.Background(), clusterID, client.RUNTIMESCANNINGTYPE_WIZ, s, out)()
	require.NotNil(t, result)
	require.False(t, result.Retryable, "expected client errors to be terminal")
}

func TestWaitForClusterRuntimeScanningReady_ServerErrorRetryable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()
	out := &client.RuntimeScanningInfo{}

	s.EXPECT().GetClusterRuntimeScanning(gomock.Any(), clusterID).
		Return(nil, &http.Response{StatusCode: http.StatusInternalServerError}, errors.New("server error"))

	result := waitForClusterRuntimeScanningReadyFunc(
		context.Background(), clusterID, client.RUNTIMESCANNINGTYPE_WIZ, s, out)()
	require.NotNil(t, result)
	require.True(t, result.Retryable, "expected server errors to be retryable")
}

// TestRetryEnableClusterRuntimeScanning_ClusterBusyRetryable covers the shared
// busy-handling branch (busy -> wait for readiness -> retry). It uses a
// POLICY_UPDATING error (non-503, no "lock" text) to also guard against a
// regression where the busy check stops going through isClusterBusy. Enable and
// disable share this branch, so it is only tested once.
func TestRetryEnableClusterRuntimeScanning_ClusterBusyRetryable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()
	cluster := &client.Cluster{State: client.CLUSTERSTATETYPE_CREATED}
	body := client.NewEnableClusterRuntimeScanningBody(client.RUNTIMESCANNINGTYPE_WIZ)
	apiObj := &client.RuntimeScanningInfo{}

	s.EXPECT().EnableClusterRuntimeScanning(gomock.Any(), clusterID, body).
		Return(nil, &http.Response{StatusCode: http.StatusConflict}, errors.New("cluster POLICY_UPDATING"))
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, &http.Response{StatusCode: http.StatusOK}, nil)

	result := retryEnableClusterRuntimeScanning(context.Background(), s, clusterID, cluster, body, apiObj)()
	require.NotNil(t, result)
	require.True(t, result.Retryable, "expected POLICY_UPDATING to be retryable on enable, got: %v", result.Err)
}
