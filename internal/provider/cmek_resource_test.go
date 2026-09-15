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
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v10/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/require"
)

// TestAccCMEKResource attempts to create, check, and destroy
// a real cluster and allowlist entry. It will be skipped if TF_ACC isn't set.
func TestAccCMEKResource(t *testing.T) {
	t.Skip("Skipping until we can either integrate the AWS provider " +
		"or import a permanent test fixture.")
	t.Parallel()
	clusterName := fmt.Sprintf("%s-cmek-%s", tfTestPrefix, GenerateRandomString(4))
	testCMEKResource(t, clusterName, false, false)
}

// TestIntegrationCMEKResource attempts to create, check, and destroy
// a cluster, but uses a mocked API service.
func TestIntegrationCMEKResource(t *testing.T) {
	testIntegrationCMEKResource(t, false)
}

// TestIntegrationCMEKWithTimeouts verifies that a configured `timeouts` block on
// cockroach_cmek is accepted and round-trips into state. Timeouts are config-only
// so they don't change any API calls; the default (no block) is covered above.
func TestIntegrationCMEKWithTimeouts(t *testing.T) {
	testIntegrationCMEKResource(t, true)
}

func testIntegrationCMEKResource(t *testing.T, includeTimeouts bool) {
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
		Plan:             ptr(client.PLANTYPE_ADVANCED),
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
	cmekCreateSpec := &client.EnableCMEKSpecBody{
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
		DoAndReturn(func(ctx context.Context, _ string, _ *client.EnableCMEKSpecBody) (*client.CMEKClusterInfo, *http.Response, error) {
			// The end-to-end deadline is opt-in: EnableCMEKSpec's context carries
			// a deadline only when timeouts.create is explicitly configured.
			if _, hasDeadline := ctx.Deadline(); hasDeadline != includeTimeouts {
				t.Errorf("EnableCMEKSpec ctx deadline present=%v, want %v", hasDeadline, includeTimeouts)
			}
			return initialCMEKInfo, nil, nil
		})
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

	testCMEKResource(t, clusterName, true, includeTimeouts)
}

func testCMEKResource(t *testing.T, clusterName string, useMock, includeTimeouts bool) {
	var (
		clusterResourceName = "cockroach_cluster.test"
		cmekResourceName    = "cockroach_cmek.test"
	)

	createChecks := []resource.TestCheckFunc{
		testCheckCockroachClusterExists(clusterResourceName),
	}
	updateChecks := []resource.TestCheckFunc{
		// The original region should only show up under the cluster resource,
		// and the two additional regions should only show up under the CMEK resource.
		resource.TestCheckResourceAttr(clusterResourceName, "regions.#", "1"),
		resource.TestCheckResourceAttr(cmekResourceName, "additional_regions.#", "2"),
		resource.TestCheckResourceAttr(cmekResourceName, "regions.#", "3"),
	}
	var importStateVerifyIgnore []string
	if includeTimeouts {
		createChecks = append(createChecks,
			resource.TestCheckResourceAttr(cmekResourceName, "timeouts.create", "3h"),
			resource.TestCheckResourceAttr(cmekResourceName, "timeouts.update", "4h"),
		)
		updateChecks = append(updateChecks,
			resource.TestCheckResourceAttr(cmekResourceName, "timeouts.create", "3h"),
			resource.TestCheckResourceAttr(cmekResourceName, "timeouts.update", "4h"),
		)
		// timeouts is config-only, so an imported resource won't have it.
		importStateVerifyIgnore = []string{"timeouts"}
	}

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestCMEKResourceCreateConfig(clusterName, includeTimeouts),
				Check:  resource.ComposeTestCheckFunc(createChecks...),
			},
			{
				// Test import functionality here, because an imported CMEK resource won't have additional_regions.
				ResourceName:            cmekResourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: importStateVerifyIgnore,
			},
			{
				Config: getTestCMEKResourceUpdateConfig(clusterName, includeTimeouts),
				Check:  resource.ComposeTestCheckFunc(updateChecks...),
			},
		},
	})
}

func cmekTimeoutsBlock(includeTimeouts bool) string {
	if !includeTimeouts {
		return ""
	}
	return `timeouts {
		create = "3h"
		update = "4h"
	}`
}

func getTestCMEKResourceCreateConfig(name string, includeTimeouts bool) string {
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
	%s
}
`, name, cmekTimeoutsBlock(includeTimeouts))
}

func getTestCMEKResourceUpdateConfig(name string, includeTimeouts bool) string {
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
	%s
}
`, name, cmekTimeoutsBlock(includeTimeouts))
}

// TestIntegrationCMEKTimeoutsBelowDefault verifies that timeout values shorter
// than the default budget (2h for both create and update) are rejected during
// validation, before any API call.
func TestIntegrationCMEKTimeoutsBelowDefault(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	clusterName := fmt.Sprintf("%s-cmek-%s", tfTestPrefix, GenerateRandomString(4))
	config := func(createTimeout, updateTimeout string) string {
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
	timeouts {
		create = "%s"
		update = "%s"
	}
}
`, clusterName, createTimeout, updateTimeout)
	}

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config("1h", "2h"),
				ExpectError: regexp.MustCompile(`must be at least 2h`),
			},
			{
				Config:      config("2h", "90m"),
				ExpectError: regexp.MustCompile(`must be at least 2h`),
			},
		},
	})
}

// TestRetryEnableCMEKSpec_Success tests that successful API calls return nil
func TestRetryEnableCMEKSpec_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()
	cmekSpec := client.NewEnableCMEKSpecBodyWithDefaults()
	cmekObj := &client.CMEKClusterInfo{}
	cluster := &client.Cluster{}

	expectedResponse := &client.CMEKClusterInfo{}

	s.EXPECT().EnableCMEKSpec(gomock.Any(), clusterID, cmekSpec).
		Return(expectedResponse, &http.Response{StatusCode: http.StatusOK}, nil)

	ctx := context.Background()
	retryFunc := retryEnableCMEKSpec(ctx, s, clusterID, cluster, clusterUpdateTimeout, cmekSpec, cmekObj)
	result := retryFunc()

	require.Nil(t, result, "Expected nil for successful call")
}

// TestRetryEnableCMEKSpec_ServiceUnavailable tests that 503 errors trigger
// cluster readiness check and retry.
func TestRetryEnableCMEKSpec_ServiceUnavailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()
	cluster := &client.Cluster{State: client.CLUSTERSTATETYPE_CREATED}
	cmekSpec := client.NewEnableCMEKSpecBodyWithDefaults()
	cmekObj := &client.CMEKClusterInfo{}

	// First call returns 503, which should trigger a cluster check and retry
	s.EXPECT().EnableCMEKSpec(gomock.Any(), clusterID, cmekSpec).
		Return(nil, &http.Response{StatusCode: http.StatusServiceUnavailable}, errors.New("service unavailable"))
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&client.Cluster{Id: clusterID, State: client.CLUSTERSTATETYPE_CREATED}, nil, nil)

	ctx := context.Background()
	retryFunc := retryEnableCMEKSpec(ctx, s, clusterID, cluster, clusterUpdateTimeout, cmekSpec, cmekObj)
	result := retryFunc()

	require.NotNil(t, result, "Expected retryable error")
	require.True(t, result.Retryable, "Expected retryable error for 503, got non-retryable: %v", result.Err)
}

// TestRetryEnableCMEKSpec_IAMRetryable tests that IAM-related errors (403)
// trigger a retryable error on first occurrence.
func TestRetryEnableCMEKSpec_IAMRetryable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()
	cluster := &client.Cluster{State: client.CLUSTERSTATETYPE_CREATED}
	cmekSpec := client.NewEnableCMEKSpecBodyWithDefaults()
	cmekObj := &client.CMEKClusterInfo{}

	// Return 403 Forbidden, which indicates IAM permission not yet propagated
	s.EXPECT().EnableCMEKSpec(gomock.Any(), clusterID, cmekSpec).
		Return(nil, &http.Response{StatusCode: http.StatusForbidden}, errors.New("access denied"))

	ctx := context.Background()
	retryFunc := retryEnableCMEKSpec(ctx, s, clusterID, cluster, clusterUpdateTimeout, cmekSpec, cmekObj)
	result := retryFunc()

	require.NotNil(t, result, "Expected retryable error for IAM error")
	require.True(t, result.Retryable, "Expected retryable error for 403, got non-retryable: %v", result.Err)
}

// Region-count drift (the original panic): Create/Update fails closed, Read
// surfaces the extra regions as drift.
func TestLoadCMEKToTerraformState_APIReturnsMoreRegionsThanPlan(t *testing.T) {
	uri1 := "aws-kms-key-arn-1"
	uri2 := "aws-kms-key-arn-2"
	uri3 := "aws-kms-key-arn-3"
	newCMEKObj := func() *client.CMEKClusterInfo {
		return &client.CMEKClusterInfo{
			Status: func() *client.CMEKStatus { s := client.CMEKSTATUS_ENABLED; return &s }(),
			RegionInfos: &[]client.CMEKRegionInfo{
				cmekRegionInfo("us-east-1", client.CMEKSTATUS_ENABLED, cmekKeyInfo(client.CMEKSTATUS_ENABLED, &uri1)),
				cmekRegionInfo("us-east-2", client.CMEKSTATUS_ENABLED, cmekKeyInfo(client.CMEKSTATUS_ENABLED, &uri2)),
				cmekRegionInfo("us-west-2", client.CMEKSTATUS_ENABLED, cmekKeyInfo(client.CMEKSTATUS_ENABLED, &uri3)),
			},
		}
	}

	t.Run("Create/Update fails closed with per-region errors", func(t *testing.T) {
		plan := &ClusterCMEK{
			ID:      types.StringValue("cluster-id"),
			Regions: []CMEKRegion{planRegion("us-east-1", uri1)},
		}
		state := &ClusterCMEK{}

		var diags diag.Diagnostics
		require.NotPanics(t, func() {
			loadCMEKToTerraformState(newCMEKObj(), state, plan, &diags, true)
		})
		require.True(t, diags.HasError())
		require.Equal(t, 2, diags.ErrorsCount())
		details := diags.Errors()[0].Detail() + diags.Errors()[1].Detail()
		require.Contains(t, details, "us-east-2")
		require.Contains(t, details, "us-west-2")
	})

	t.Run("Read surfaces out-of-band regions as drift", func(t *testing.T) {
		// On Read, plan == state, tracking only us-east-1.
		planAndState := &ClusterCMEK{
			ID:      types.StringValue("cluster-id"),
			Regions: []CMEKRegion{planRegion("us-east-1", uri1)},
		}

		var diags diag.Diagnostics
		require.NotPanics(t, func() {
			loadCMEKToTerraformState(newCMEKObj(), planAndState, planAndState, &diags, false)
		})
		require.False(t, diags.HasError())
		require.Len(t, planAndState.Regions, 3)
		require.Equal(t, "us-east-1", planAndState.Regions[0].Region.ValueString())
		require.Equal(t, uri1, planAndState.Regions[0].Key.URI.ValueString())
		require.Equal(t, uri2, planAndState.Regions[1].Key.URI.ValueString())
		require.Equal(t, uri3, planAndState.Regions[2].Key.URI.ValueString())
	})
}

// A planned URI matching no server key (e.g. key rotated out of band) falls
// back to the real enabled key rather than blank metadata.
func TestLoadCMEKToTerraformState_PlannedURIMatchesNoKey(t *testing.T) {
	oldURI := "aws-kms-key-arn-old"
	newURI := "aws-kms-key-arn-new"
	cmekObj := &client.CMEKClusterInfo{
		Status: func() *client.CMEKStatus { s := client.CMEKSTATUS_ENABLED; return &s }(),
		RegionInfos: &[]client.CMEKRegionInfo{
			cmekRegionInfo("us-east-1", client.CMEKSTATUS_ENABLED, cmekKeyInfo(client.CMEKSTATUS_ENABLED, &newURI)),
		},
	}
	planAndState := &ClusterCMEK{
		ID:      types.StringValue("cluster-id"),
		Regions: []CMEKRegion{planRegion("us-east-1", oldURI)},
	}

	var diags diag.Diagnostics
	loadCMEKToTerraformState(cmekObj, planAndState, planAndState, &diags, false)
	require.False(t, diags.HasError())
	require.Len(t, planAndState.Regions, 1)
	require.Equal(t, newURI, planAndState.Regions[0].Key.URI.ValueString())
	require.Equal(t, string(client.CMEKSTATUS_ENABLED), planAndState.Regions[0].Key.Status.ValueString())
}

// A key spec with a nil URI must not be dereferenced.
func TestLoadCMEKToTerraformState_NilKeyURI(t *testing.T) {
	uri := "aws-kms-key-arn"
	cmekObj := &client.CMEKClusterInfo{
		Status: func() *client.CMEKStatus { s := client.CMEKSTATUS_ENABLED; return &s }(),
		RegionInfos: &[]client.CMEKRegionInfo{
			cmekRegionInfo("us-east-1", client.CMEKSTATUS_ENABLED,
				cmekKeyInfo(client.CMEKSTATUS_DISABLED, nil),
				cmekKeyInfo(client.CMEKSTATUS_ENABLED, &uri),
			),
		},
	}
	plan := &ClusterCMEK{
		ID:      types.StringValue("cluster-id"),
		Regions: []CMEKRegion{planRegion("us-east-1", uri)},
	}
	state := &ClusterCMEK{}

	var diags diag.Diagnostics
	require.NotPanics(t, func() {
		loadCMEKToTerraformState(cmekObj, state, plan, &diags, true)
	})
	require.False(t, diags.HasError())
	require.Len(t, state.Regions, 1)
	require.Equal(t, uri, state.Regions[0].Key.URI.ValueString())
}

// When plan order differs from API order, each region is paired with its own
// key by name (no cross-region mis-pairing).
func TestLoadCMEKToTerraformState_OrderMismatchSameLength(t *testing.T) {
	uriA := "aws-kms-key-arn-a"
	uriB := "aws-kms-key-arn-b"
	cmekObj := &client.CMEKClusterInfo{
		Status: func() *client.CMEKStatus { s := client.CMEKSTATUS_ENABLED; return &s }(),
		RegionInfos: &[]client.CMEKRegionInfo{
			cmekRegionInfo("us-east-1", client.CMEKSTATUS_ENABLED, cmekKeyInfo(client.CMEKSTATUS_ENABLED, &uriA)),
			cmekRegionInfo("us-west-2", client.CMEKSTATUS_ENABLED, cmekKeyInfo(client.CMEKSTATUS_ENABLED, &uriB)),
		},
	}
	plan := &ClusterCMEK{
		ID: types.StringValue("cluster-id"),
		Regions: []CMEKRegion{
			planRegion("us-west-2", uriB),
			planRegion("us-east-1", uriA),
		},
	}
	state := &ClusterCMEK{}

	var diags diag.Diagnostics
	loadCMEKToTerraformState(cmekObj, state, plan, &diags, true)
	require.False(t, diags.HasError())
	require.Len(t, state.Regions, 2)
	require.Equal(t, "us-west-2", state.Regions[0].Region.ValueString())
	require.Equal(t, uriB, state.Regions[0].Key.URI.ValueString())
	require.Equal(t, "us-east-1", state.Regions[1].Region.ValueString())
	require.Equal(t, uriA, state.Regions[1].Key.URI.ValueString())
}

// Same region count as the plan, but a name mismatch (plan lists a region the
// server doesn't have; the server has one the plan doesn't). With DISTINCT
// per-region keys this proves matching is by name, not slice index: the real
// server region must get ITS OWN key, never the bogus plan slot's URI (the
// defect-c mis-pairing).
func TestLoadCMEKToTerraformState_SameCountNameMismatchNoMispairing(t *testing.T) {
	uriEast := "aws-kms-key-arn-east"
	uriWest := "aws-kms-key-arn-west"
	uriBogus := "aws-kms-key-arn-bogus"
	// Server (name-sorted) with distinct keys per region.
	cmekObj := &client.CMEKClusterInfo{
		Status: func() *client.CMEKStatus { s := client.CMEKSTATUS_ENABLED; return &s }(),
		RegionInfos: &[]client.CMEKRegionInfo{
			cmekRegionInfo("us-east-1", client.CMEKSTATUS_ENABLED, cmekKeyInfo(client.CMEKSTATUS_ENABLED, &uriEast)),
			cmekRegionInfo("us-west-2", client.CMEKSTATUS_ENABLED, cmekKeyInfo(client.CMEKSTATUS_ENABLED, &uriWest)),
		},
	}
	// Plan/state: same count (2), but "bogus-region" replaces "us-west-2".
	planAndState := &ClusterCMEK{
		ID: types.StringValue("cluster-id"),
		Regions: []CMEKRegion{
			planRegion("us-east-1", uriEast),
			planRegion("bogus-region", uriBogus),
		},
	}

	var diags diag.Diagnostics
	require.NotPanics(t, func() {
		loadCMEKToTerraformState(cmekObj, planAndState, planAndState, &diags, false)
	})
	require.False(t, diags.HasError())
	require.Len(t, planAndState.Regions, 2)

	byRegion := map[string]string{}
	for _, r := range planAndState.Regions {
		byRegion[r.Region.ValueString()] = r.Key.URI.ValueString()
	}
	// us-east-1 keeps its own key; us-west-2 (drift) gets ITS OWN key, not the
	// bogus plan slot's URI; bogus-region (not on server) is dropped.
	require.Equal(t, uriEast, byRegion["us-east-1"])
	require.Equal(t, uriWest, byRegion["us-west-2"])
	require.NotContains(t, byRegion, "bogus-region")
	require.NotEqual(t, uriBogus, byRegion["us-west-2"])
}

// A CMEK response that omits region_infos entirely must not panic (nil deref):
// sortCMEKRegionsByPlan dereferences RegionInfos, and the loader iterates it.
func TestLoadCMEKToTerraformState_NilRegionInfos(t *testing.T) {
	cmekObj := &client.CMEKClusterInfo{
		Status:      func() *client.CMEKStatus { s := client.CMEKSTATUS_ENABLED; return &s }(),
		RegionInfos: nil,
	}
	plan := &ClusterCMEK{
		ID:      types.StringValue("cluster-id"),
		Regions: []CMEKRegion{planRegion("us-east-1", "some-uri")},
	}
	state := &ClusterCMEK{}

	var diags diag.Diagnostics
	require.NotPanics(t, func() {
		sortCMEKRegionsByPlan(cmekObj, plan)
		loadCMEKToTerraformState(cmekObj, state, plan, &diags, true)
	})
	require.False(t, diags.HasError())
	require.Empty(t, state.Regions)
}

// Import path: an empty plan falls back to each region's first ENABLED key.
func TestLoadCMEKToTerraformState_ImportEmptyPlan(t *testing.T) {
	disabledURI := "disabled-uri"
	enabledURI := "enabled-uri"
	secondURI := "second-enabled-uri"
	cmekObj := &client.CMEKClusterInfo{
		Status: func() *client.CMEKStatus { s := client.CMEKSTATUS_ENABLED; return &s }(),
		RegionInfos: &[]client.CMEKRegionInfo{
			cmekRegionInfo("us-east-1", client.CMEKSTATUS_ENABLED,
				cmekKeyInfo(client.CMEKSTATUS_DISABLED, &disabledURI),
				cmekKeyInfo(client.CMEKSTATUS_ENABLED, &enabledURI),
			),
			cmekRegionInfo("us-west-2", client.CMEKSTATUS_ENABLED,
				cmekKeyInfo(client.CMEKSTATUS_ENABLED, &secondURI),
			),
		},
	}
	plan := &ClusterCMEK{ID: types.StringValue("cluster-id")}
	state := &ClusterCMEK{}

	var diags diag.Diagnostics
	loadCMEKToTerraformState(cmekObj, state, plan, &diags, true)
	require.False(t, diags.HasError())
	require.Len(t, state.Regions, 2)
	require.Equal(t, enabledURI, state.Regions[0].Key.URI.ValueString())
	require.Equal(t, secondURI, state.Regions[1].Key.URI.ValueString())
}

// Plan-absent regions sort to the end (not ordinal 0).
func TestSortCMEKRegionsByPlan_UnknownRegionsSortLast(t *testing.T) {
	cmekObj := &client.CMEKClusterInfo{
		RegionInfos: &[]client.CMEKRegionInfo{
			cmekRegionInfo("us-west-2", client.CMEKSTATUS_ENABLED),
			cmekRegionInfo("us-east-1", client.CMEKSTATUS_ENABLED),
			cmekRegionInfo("us-east-2", client.CMEKSTATUS_ENABLED),
		},
	}
	plan := &ClusterCMEK{
		Regions: []CMEKRegion{
			planRegion("us-east-1", "u1"),
			planRegion("us-east-2", "u2"),
		},
	}

	sortCMEKRegionsByPlan(cmekObj, plan)
	got := []string{}
	for _, r := range *cmekObj.RegionInfos {
		got = append(got, r.GetRegion())
	}
	require.Equal(t, []string{"us-east-1", "us-east-2", "us-west-2"}, got)
}

// Update's preflight: a region in state but not the plan is undeclared drift,
// which must be caught before any mutating API call.
func TestUndeclaredCMEKRegions(t *testing.T) {
	t.Run("state region absent from plan is flagged", func(t *testing.T) {
		state := &ClusterCMEK{Regions: []CMEKRegion{
			planRegion("us-east-1", "u1"),
			planRegion("us-east-2", "u2"),
		}}
		plan := &ClusterCMEK{Regions: []CMEKRegion{planRegion("us-east-1", "u1")}}
		require.Equal(t, []string{"us-east-2"}, undeclaredCMEKRegions(state, plan))
	})

	t.Run("no drift when state is a subset of plan", func(t *testing.T) {
		state := &ClusterCMEK{Regions: []CMEKRegion{planRegion("us-east-1", "u1")}}
		plan := &ClusterCMEK{Regions: []CMEKRegion{
			planRegion("us-east-1", "u1"),
			planRegion("us-east-2", "u2"),
		}}
		require.Empty(t, undeclaredCMEKRegions(state, plan))
	})
}

func cmekKeyInfo(status client.CMEKStatus, uri *string) client.CMEKKeyInfo {
	keyType := client.CMEKKeyType("AWS_KMS")
	principal := "aws-auth-principal-arn"
	return client.CMEKKeyInfo{
		Status: &status,
		Spec: &client.CMEKKeySpecification{
			Type:          &keyType,
			Uri:           uri,
			AuthPrincipal: &principal,
		},
	}
}

func cmekRegionInfo(region string, status client.CMEKStatus, keys ...client.CMEKKeyInfo) client.CMEKRegionInfo {
	r, s := region, status
	return client.CMEKRegionInfo{
		Region:   &r,
		Status:   &s,
		KeyInfos: &keys,
	}
}

func planRegion(region, uri string) CMEKRegion {
	return CMEKRegion{
		Region: types.StringValue(region),
		Key: CMEKKey{
			URI: types.StringValue(uri),
		},
	}
}
