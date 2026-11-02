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
	"sync"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v10/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"
)

const otlpMetricExportTestRole = "arn:aws:iam::123456789012:role/otlp-metrics"

func TestIntegrationMetricExportOTLPConfigResource(t *testing.T) {
	t.Setenv(CockroachAPIKey, "fake")
	s := mock_client.NewMockService(gomock.NewController(t))
	defer HookGlobal(&NewService, func(_ *client.Client) client.Service { return s })()
	clusterID := uuid.Nil.String()
	var mu sync.Mutex
	var current *client.OtlpMetricExportInfo
	requests := []*client.EnableOtlpMetricExportBody{
		{Cloudwatch: &client.OtlpMetricExportCloudWatch{RoleArn: otlpMetricExportTestRole}},
		{Cloudwatch: &client.OtlpMetricExportCloudWatch{
			RoleArn: otlpMetricExportTestRole, TargetRegion: ptr("us-east-1"), ExternalId: ptr("test-external-id"),
		}},
		{Cloudwatch: &client.OtlpMetricExportCloudWatch{RoleArn: otlpMetricExportTestRole}},
	}
	var calls []*gomock.Call
	for _, request := range requests {
		calls = append(calls, s.EXPECT().EnableOtlpMetricExport(gomock.Any(), clusterID, request).
			DoAndReturn(func(_ context.Context, id string, body *client.EnableOtlpMetricExportBody) (*client.OtlpMetricExportInfo, *http.Response, error) {
				mu.Lock()
				defer mu.Unlock()
				current = &client.OtlpMetricExportInfo{
					ClusterId: id, Status: client.METRICEXPORTSTATUSTYPE_ENABLED,
					Cloudwatch: &client.OtlpMetricExportCloudWatchInfo{
						RoleArn: body.Cloudwatch.RoleArn, TargetRegion: body.Cloudwatch.TargetRegion, ExternalId: body.Cloudwatch.ExternalId,
					},
				}
				return current, httpOk, nil
			}))
	}
	gomock.InOrder(calls...)
	s.EXPECT().GetOtlpMetricExportInfo(gomock.Any(), clusterID).
		DoAndReturn(func(context.Context, string) (*client.OtlpMetricExportInfo, *http.Response, error) {
			mu.Lock()
			defer mu.Unlock()
			if current == nil {
				return nil, &http.Response{StatusCode: http.StatusNotFound}, errors.New("not found")
			}
			return current, httpOk, nil
		}).AnyTimes()
	s.EXPECT().DeleteOtlpMetricExport(gomock.Any(), clusterID).
		DoAndReturn(func(context.Context, string) (*client.DeleteMetricExportResponse, *http.Response, error) {
			mu.Lock()
			defer mu.Unlock()
			current = nil
			return client.NewDeleteMetricExportResponse(clusterID), httpOk, nil
		})
	testMetricExportOTLPConfigResource(t, true, "", fmt.Sprintf("%q", clusterID), otlpMetricExportTestRole, "us-east-1", "", "test-external-id")
}

// This test creates a real AWS cluster. The role must already trust the test
// organization and allow cloudwatch:PutMetricData, and the org must have OTLP access.
func TestAccMetricExportOTLPConfigResource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set to run live acceptance tests")
	}
	roleArn := os.Getenv("COCKROACH_OTLP_METRICS_ROLE_ARN")
	if roleArn == "" {
		t.Skip("COCKROACH_OTLP_METRICS_ROLE_ARN must be set to a role that trusts the test organization")
	}
	testAccPreCheck(t)
	targetRegion := os.Getenv("COCKROACH_OTLP_METRICS_TARGET_REGION")
	if targetRegion == "" {
		targetRegion = "us-east-1"
	}
	externalID := os.Getenv("COCKROACH_OTLP_METRICS_EXTERNAL_ID")
	clusterConfig := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = %q
  cloud_provider = "AWS"
  dedicated = {
    storage_gib      = 35
    num_virtual_cpus = 4
  }
  regions = [{ name = %q, node_count = 3 }]
}
`, fmt.Sprintf("%s-otlp-%s", tfTestPrefix, GenerateRandomString(4)), targetRegion)
	testMetricExportOTLPConfigResource(t, false, clusterConfig, "cockroach_cluster.test.id", roleArn, targetRegion, externalID, externalID)
}

func testMetricExportOTLPConfigResource(
	t *testing.T, useMock bool, clusterConfig, clusterIDExpression, roleArn, targetRegion, initialExternalID, updatedExternalID string,
) {
	t.Helper()
	const name = "cockroach_metric_export_otlp_config.test"
	config := func(region, externalID string) string {
		optional := ""
		if region != "" {
			optional += fmt.Sprintf("target_region = %q\n", region)
		}
		if externalID != "" {
			optional += fmt.Sprintf("external_id = %q\n", externalID)
		}
		return clusterConfig + fmt.Sprintf(`
resource "cockroach_metric_export_otlp_config" "test" {
  id = %s
  cloudwatch = {
    role_arn = %q
    %s
  }
}
`, clusterIDExpression, roleArn, optional)
	}
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(state *terraform.State) error {
			for _, rs := range state.RootModule().Resources {
				if rs.Type != "cockroach_metric_export_otlp_config" {
					continue
				}
				info, httpResp, err := NewService(cl).GetOtlpMetricExportInfo(context.Background(), rs.Primary.ID)
				if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
					continue
				}
				if err != nil {
					return err
				}
				if info.GetStatus() != client.METRICEXPORTSTATUSTYPE_NOT_DEPLOYED {
					return fmt.Errorf("OTLP metric export still exists for cluster %s", rs.Primary.ID)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: config("", initialExternalID),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "cloudwatch.role_arn", roleArn),
					resource.TestCheckResourceAttr(name, "status", "ENABLED"),
					resource.TestCheckNoResourceAttr(name, "cloudwatch.target_region"),
				),
			},
			{
				Config: config(targetRegion, updatedExternalID),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "cloudwatch.target_region", targetRegion),
					resource.TestCheckResourceAttr(name, "status", "ENABLED"),
					func(s *terraform.State) error {
						if updatedExternalID != "" {
							return resource.TestCheckResourceAttr(name, "cloudwatch.external_id", updatedExternalID)(s)
						}
						return resource.TestCheckNoResourceAttr(name, "cloudwatch.external_id")(s)
					},
				),
			},
			{ResourceName: name, ImportState: true, ImportStateVerify: true},
			{
				Config: config("", initialExternalID),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(name, "cloudwatch.target_region"),
					resource.TestCheckResourceAttr(name, "status", "ENABLED"),
				),
			},
		},
	})
}

func TestOTLPMetricExportWait(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     client.MetricExportStatusType
		httpStatus int
		deleting   bool
		retry      bool
		failed     bool
	}{
		{name: "enabled", status: client.METRICEXPORTSTATUSTYPE_ENABLED},
		{name: "enabling", status: client.METRICEXPORTSTATUSTYPE_ENABLING, retry: true},
		{name: "not deployed yet", status: client.METRICEXPORTSTATUSTYPE_NOT_DEPLOYED, retry: true},
		{name: "disabling during enable", status: client.METRICEXPORTSTATUSTYPE_DISABLING, retry: true},
		{name: "deployment error", status: client.METRICEXPORTSTATUSTYPE_ERROR, failed: true},
		{name: "unexpected status", status: "UNKNOWN", failed: true},
		{name: "server error", httpStatus: http.StatusInternalServerError, retry: true},
		{name: "unauthorized", httpStatus: http.StatusUnauthorized, failed: true},
		{name: "not visible yet", httpStatus: http.StatusNotFound, retry: true},
		{name: "deleted", httpStatus: http.StatusNotFound, deleting: true},
		{name: "not deployed after delete", status: client.METRICEXPORTSTATUSTYPE_NOT_DEPLOYED, deleting: true},
		{name: "disabling", status: client.METRICEXPORTSTATUSTYPE_DISABLING, deleting: true, retry: true},
		{name: "still enabled after delete", status: client.METRICEXPORTSTATUSTYPE_ENABLED, deleting: true, retry: true},
		{name: "delete error", status: client.METRICEXPORTSTATUSTYPE_ERROR, deleting: true, failed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := mock_client.NewMockService(gomock.NewController(t))
			var apiErr error
			if tc.httpStatus != 0 {
				apiErr = errors.New("API error")
			}
			info := &client.OtlpMetricExportInfo{Status: tc.status, UserMessage: "check role permissions"}
			s.EXPECT().GetOtlpMetricExportInfo(gomock.Any(), "cluster").
				Return(info, &http.Response{StatusCode: tc.httpStatus}, apiErr)
			result := &client.OtlpMetricExportInfo{}
			err := waitForOTLPMetricExportFunc(context.Background(), "cluster", s, result, tc.deleting)()
			if !tc.retry && !tc.failed {
				require.Nil(t, err)
				return
			}
			require.NotNil(t, err)
			require.Equal(t, tc.retry, err.Retryable)
			if tc.status == client.METRICEXPORTSTATUSTYPE_ERROR {
				require.Contains(t, err.Err.Error(), "check role permissions")
			}
		})
	}
}

func otlpMetricExportTestState(t *testing.T) tfsdk.State {
	t.Helper()
	var resp frameworkresource.SchemaResponse
	NewMetricExportOTLPConfigResource().Schema(context.Background(), frameworkresource.SchemaRequest{}, &resp)
	state := tfsdk.State{Schema: resp.Schema}
	require.False(t, state.Set(context.Background(), &ClusterOTLPMetricExportConfig{
		ID: types.StringValue("cluster"), Status: types.StringValue("ENABLED"), UserMessage: types.StringValue(""),
		CloudWatch: &ClusterOTLPMetricExportCloudWatchConfig{
			RoleArn: types.StringValue(otlpMetricExportTestRole), TargetRegion: types.StringNull(), ExternalID: types.StringNull(),
		},
	}).HasError())
	return state
}

func TestOTLPMetricExportRead(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     client.MetricExportStatusType
		httpStatus int
		absent     bool
		failed     bool
	}{
		{name: "missing export or cluster", httpStatus: http.StatusNotFound, absent: true},
		{name: "disabled outside Terraform", status: client.METRICEXPORTSTATUSTYPE_NOT_DEPLOYED, absent: true},
		{name: "permission denied", httpStatus: http.StatusForbidden, failed: true},
		{name: "deployment error stays managed", status: client.METRICEXPORTSTATUSTYPE_ERROR},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := mock_client.NewMockService(gomock.NewController(t))
			var err error
			if tc.httpStatus != 0 {
				err = errors.New("API error")
			}
			s.EXPECT().GetOtlpMetricExportInfo(gomock.Any(), "cluster").Return(
				&client.OtlpMetricExportInfo{ClusterId: "cluster", Status: tc.status}, &http.Response{StatusCode: tc.httpStatus}, err)
			r := &metricExportOTLPConfigResource{provider: &provider{configured: true, service: s}}
			state := otlpMetricExportTestState(t)
			resp := frameworkresource.ReadResponse{State: state}
			r.Read(context.Background(), frameworkresource.ReadRequest{State: state}, &resp)
			require.Equal(t, tc.failed, resp.Diagnostics.HasError(), resp.Diagnostics)
			require.Equal(t, tc.absent, resp.State.Raw.IsNull())
			require.Equal(t, tc.absent, len(resp.Diagnostics.Warnings()) > 0, resp.Diagnostics)
		})
	}
}

func TestOTLPMetricExportCreateKeepsStateOnDeploymentError(t *testing.T) {
	s := mock_client.NewMockService(gomock.NewController(t))
	info := &client.OtlpMetricExportInfo{
		ClusterId: "cluster", Status: client.METRICEXPORTSTATUSTYPE_ENABLING,
		Cloudwatch: &client.OtlpMetricExportCloudWatchInfo{RoleArn: otlpMetricExportTestRole},
	}
	s.EXPECT().EnableOtlpMetricExport(gomock.Any(), "cluster", gomock.Any()).Return(info, httpOk, nil)
	failed := *info
	failed.Status = client.METRICEXPORTSTATUSTYPE_ERROR
	failed.UserMessage = "role cannot be assumed"
	s.EXPECT().GetOtlpMetricExportInfo(gomock.Any(), "cluster").Return(&failed, httpOk, nil)
	state := otlpMetricExportTestState(t)
	r := &metricExportOTLPConfigResource{provider: &provider{configured: true, service: s}}
	resp := frameworkresource.CreateResponse{State: tfsdk.State{Schema: state.Schema}}
	r.Create(context.Background(), frameworkresource.CreateRequest{Plan: tfsdk.Plan(state)}, &resp)
	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "role cannot be assumed")
	var saved ClusterOTLPMetricExportConfig
	require.False(t, resp.State.Get(context.Background(), &saved).HasError())
	require.Equal(t, "cluster", saved.ID.ValueString())
	require.Equal(t, "ERROR", saved.Status.ValueString())
}

func TestOTLPMetricExportUpdateKeepsStateOnDeploymentError(t *testing.T) {
	ctx := context.Background()
	state := otlpMetricExportTestState(t)
	const updatedRole = "arn:aws:iam::123456789012:role/updated-otlp-metrics"
	var planned ClusterOTLPMetricExportConfig
	require.False(t, state.Get(ctx, &planned).HasError())
	planned.CloudWatch.RoleArn = types.StringValue(updatedRole)
	planned.Status = types.StringUnknown()
	planned.UserMessage = types.StringUnknown()
	plan := tfsdk.Plan(state)
	require.False(t, plan.Set(ctx, &planned).HasError())

	s := mock_client.NewMockService(gomock.NewController(t))
	info := &client.OtlpMetricExportInfo{
		ClusterId: "cluster", Status: client.METRICEXPORTSTATUSTYPE_ENABLING,
		Cloudwatch: &client.OtlpMetricExportCloudWatchInfo{RoleArn: updatedRole},
	}
	failed := *info
	failed.Status = client.METRICEXPORTSTATUSTYPE_ERROR
	failed.UserMessage = "role cannot be assumed"
	gomock.InOrder(
		s.EXPECT().EnableOtlpMetricExport(gomock.Any(), "cluster", &client.EnableOtlpMetricExportBody{
			Cloudwatch: &client.OtlpMetricExportCloudWatch{RoleArn: updatedRole},
		}).Return(info, httpOk, nil),
		s.EXPECT().GetOtlpMetricExportInfo(gomock.Any(), "cluster").Return(&failed, httpOk, nil),
	)
	r := &metricExportOTLPConfigResource{provider: &provider{configured: true, service: s}}
	resp := frameworkresource.UpdateResponse{State: tfsdk.State(plan)}
	r.Update(ctx, frameworkresource.UpdateRequest{Plan: plan, State: state}, &resp)
	require.True(t, resp.Diagnostics.HasError())
	require.Equal(t, "Error updating OTLP metric export", resp.Diagnostics.Errors()[0].Summary())
	require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), failed.UserMessage)
	var saved ClusterOTLPMetricExportConfig
	require.False(t, resp.State.Get(ctx, &saved).HasError())
	require.Equal(t, "cluster", saved.ID.ValueString())
	require.NotNil(t, saved.CloudWatch)
	require.Equal(t, updatedRole, saved.CloudWatch.RoleArn.ValueString())
	require.Equal(t, "ERROR", saved.Status.ValueString())
	require.Equal(t, failed.UserMessage, saved.UserMessage.ValueString())
}

func TestOTLPMetricExportDeleteAlreadyGone(t *testing.T) {
	s := mock_client.NewMockService(gomock.NewController(t))
	s.EXPECT().DeleteOtlpMetricExport(gomock.Any(), "cluster").
		Return(nil, &http.Response{StatusCode: http.StatusNotFound}, errors.New("not found"))
	state := otlpMetricExportTestState(t)
	r := &metricExportOTLPConfigResource{provider: &provider{configured: true, service: s}}
	resp := frameworkresource.DeleteResponse{State: state}
	r.Delete(context.Background(), frameworkresource.DeleteRequest{State: state}, &resp)
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	require.True(t, resp.State.Raw.IsNull())
}

func TestOTLPMetricExportRetryOperation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		message    string
		httpStatus int
		retry      bool
	}{
		{name: "cluster busy", message: "busy", httpStatus: http.StatusServiceUnavailable, retry: true},
		{name: "cluster locked without HTTP response", message: "cluster lock held", retry: true},
		{name: "invalid configuration", message: "invalid role", httpStatus: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := mock_client.NewMockService(gomock.NewController(t))
			if tc.retry {
				s.EXPECT().GetCluster(gomock.Any(), "cluster").Return(&client.Cluster{State: client.CLUSTERSTATETYPE_CREATED}, httpOk, nil)
			}
			var httpResp *http.Response
			if tc.httpStatus != 0 {
				httpResp = &http.Response{StatusCode: tc.httpStatus}
			}
			err := retryOTLPMetricExportOperation(context.Background(), "cluster", s, func() (*http.Response, error) {
				return httpResp, errors.New(tc.message)
			})()
			require.NotNil(t, err)
			require.Equal(t, tc.retry, err.Retryable)
		})
	}
}
