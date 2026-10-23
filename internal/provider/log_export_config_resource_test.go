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
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v10/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"
)

// TestAccLogExportConfigResource attempts to create, check, and destroy
// a real cluster. It will be skipped if TF_ACC isn't set.
func TestAccLogExportConfigResource(t *testing.T) {
	t.Skip("Skipping until we can either integrate the AWS provider " +
		"or import a permanent test fixture.")
	t.Parallel()
	clusterName := fmt.Sprintf("%s-logexp-%s", tfTestPrefix, GenerateRandomString(4))
	testLogExportConfigResource(t, clusterName, false)
}

// TestIntegrationLogExportConfigResource attempts to create, check, and destroy
// a cluster, but uses a mocked API service.
func TestIntegrationLogExportConfigResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-logexp-%s", tfTestPrefix, GenerateRandomString(4))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	cluster := &client.Cluster{
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
				Name:      "us-east-1",
				NodeCount: 3,
			},
		},
	}

	authPrincipal := "iam-role-arn"
	configType, _ := client.NewLogExportTypeFromValue("AWS_CLOUDWATCH")
	logName := "test"
	region := "us-east-1"
	trueBool := true
	minLevel, _ := client.NewLogLevelTypeFromValue("WARNING")
	createdGroups := []client.LogExportGroup{
		{LogName: "sql", Channels: []string{"SQL_SCHEMA", "SQL_EXEC"}, MinLevel: minLevel, Redact: &trueBool, EnableSendingQueue: &trueBool},
	}
	omittedChannels := []string{"SQL_PERF"}
	enabledStatus, _ := client.NewLogExportStatusFromValue("ENABLED")
	createdLogExportClusterInfo := &client.LogExportClusterInfo{
		ClusterId: &clusterID,
		Spec: &client.LogExportClusterSpecification{
			AuthPrincipal:   &authPrincipal,
			LogName:         &logName,
			Groups:          &createdGroups,
			Redact:          &trueBool,
			Type:            configType,
			Region:          &region,
			OmittedChannels: &omittedChannels,
		},
		Status: enabledStatus,
	}

	falseBool := false
	updatedGroups := []client.LogExportGroup{
		{LogName: "sql", Channels: []string{"SQL_EXEC"}, MinLevel: minLevel, Redact: &trueBool, EnableSendingQueue: &falseBool},
		{LogName: "devops", Channels: []string{"OPS", "HEALTH", "STORAGE"}, MinLevel: minLevel, Redact: &falseBool, EnableSendingQueue: &trueBool},
	}
	updatedOChannels := []string{"SQL_SCHEMA"}
	updatedExternalID := "test-external-id"
	updatedLogExportClusterInfo := &client.LogExportClusterInfo{
		ClusterId: &clusterID,
		Spec: &client.LogExportClusterSpecification{
			AuthPrincipal:   &authPrincipal,
			LogName:         &logName,
			Groups:          &updatedGroups,
			Redact:          &falseBool,
			Type:            configType,
			Region:          &region,
			OmittedChannels: &updatedOChannels,
			AwsExternalId:   &updatedExternalID,
		},
		Status: enabledStatus,
	}

	// Create
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(cluster, nil, nil)

	// Trigger a lock error from attempting to enable log export config to test retry logic.
	// This increases the number of GetCluster calls by 1 due to the retry.
	s.EXPECT().EnableLogExport(gomock.Any(), clusterID, gomock.Any()).
		Return(nil, &http.Response{Status: http.StatusText(http.StatusBadRequest)}, errors.New("couldn't lock cluster"))
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(4)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().EnableLogExport(gomock.Any(), clusterID,
		&client.EnableLogExportBody{
			Groups:          &createdGroups,
			AuthPrincipal:   &authPrincipal,
			LogName:         logName,
			Redact:          &trueBool,
			Type:            *configType,
			Region:          &region,
			OmittedChannels: &omittedChannels,
		}).
		Return(createdLogExportClusterInfo, nil, nil)
	s.EXPECT().GetLogExportInfo(gomock.Any(), clusterID).
		Return(createdLogExportClusterInfo, nil, nil).
		Times(3)

	// Update
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, nil, nil).
		Times(2)
	s.EXPECT().GetLogExportInfo(gomock.Any(), clusterID).
		Return(createdLogExportClusterInfo, nil, nil)
	s.EXPECT().EnableLogExport(gomock.Any(), clusterID,
		&client.EnableLogExportBody{
			AuthPrincipal:   &authPrincipal,
			Type:            *configType,
			LogName:         logName,
			Redact:          &falseBool,
			Groups:          &updatedGroups,
			Region:          &region,
			OmittedChannels: &updatedOChannels,
			AwsExternalId:   &updatedExternalID,
		}).
		Return(updatedLogExportClusterInfo, nil, nil)
	s.EXPECT().GetLogExportInfo(gomock.Any(), clusterID).
		Return(updatedLogExportClusterInfo, nil, nil).
		Times(4)

	// Delete
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)
	s.EXPECT().DeleteLogExport(gomock.Any(), clusterID)

	testLogExportConfigResource(t, clusterName, true)
}

func testLogExportConfigResource(t *testing.T, clusterName string, useMock bool) {
	var (
		clusterResourceName         = "cockroach_cluster.test"
		logExportConfigResourceName = "cockroach_log_export_config.test"
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestLogExportConfigResourceCreateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testLogExportConfigExists(logExportConfigResourceName, clusterResourceName),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "redact", "true"),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "groups.#", "1"),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "groups.0.channels.#", "2"),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "groups.0.enable_sending_queue", "true"),
					resource.TestCheckNoResourceAttr(logExportConfigResourceName, "aws_external_id"),
				),
			},
			{
				Config: getTestLogExportConfigResourceUpdateConfig(clusterName),
				Check: resource.ComposeTestCheckFunc(
					testLogExportConfigExists(logExportConfigResourceName, clusterResourceName),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "redact", "false"),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "groups.#", "2"),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "groups.0.channels.#", "1"),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "groups.0.enable_sending_queue", "false"),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "groups.1.enable_sending_queue", "true"),
					resource.TestCheckResourceAttr(logExportConfigResourceName, "aws_external_id", "test-external-id"),
				),
			},
			{
				ResourceName:      logExportConfigResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testLogExportConfigExists(resourceName, clusterResourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		p := testAccProvider.(*provider)
		p.service = NewService(cl)

		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}
		clusterRs, ok := s.RootModule().Resources[clusterResourceName]
		if !ok {
			return fmt.Errorf("not found: %s", clusterResourceName)
		}

		clusterID := clusterRs.Primary.Attributes["id"]
		log.Printf("[DEBUG] clusterID: %s, name %s", clusterRs.Primary.Attributes["id"], clusterRs.Primary.Attributes["name"])

		traceAPICall("GetLogExportInfo")
		logExportConfigResponse, _, err := p.service.GetLogExportInfo(context.TODO(), clusterID)
		if err == nil {
			if logExportConfigResponse.Spec.GetLogName() == rs.Primary.Attributes["log_name"] {
				return nil
			}
		}

		return fmt.Errorf("log export config with log name %s does not exist", rs.Primary.Attributes["log_name"])
	}
}

func getTestLogExportConfigResourceCreateConfig(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  dedicated = {
    storage_gib      = 35
    num_virtual_cpus = 4
  }
  regions = [{
    name = "us-east-1"
    node_count : 3
  }]
}

resource "cockroach_log_export_config" "test" {
  id             = cockroach_cluster.test.id
  auth_principal = "iam-role-arn"
  log_name       = "test"
  type           = "AWS_CLOUDWATCH"
  redact         = true
  region         = "us-east-1"
  groups = [
    {
      log_name             = "sql",
      channels             = ["SQL_SCHEMA", "SQL_EXEC"],
      min_level            = "WARNING",
      redact               = true,
      enable_sending_queue = true
    }
  ]
  omitted_channels = ["SQL_PERF"]
}
`, name)
}

func getTestLogExportConfigResourceUpdateConfig(name string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = "%s"
  cloud_provider = "AWS"
  dedicated = {
    storage_gib      = 35
    num_virtual_cpus = 4
  }
  regions = [{
    name = "us-east-1"
    node_count : 3
  }]
}

resource "cockroach_log_export_config" "test" {
  id              = cockroach_cluster.test.id
  auth_principal  = "iam-role-arn"
  log_name        = "test"
  type            = "AWS_CLOUDWATCH"
  redact          = false
  region          = "us-east-1"
  aws_external_id = "test-external-id"
  groups = [
    {
      log_name             = "sql",
      channels             = ["SQL_EXEC"],
      min_level            = "WARNING",
      redact               = true,
      enable_sending_queue = false
    },
    {
      log_name             = "devops",
      channels             = ["OPS", "HEALTH", "STORAGE"],
      min_level            = "WARNING",
      redact               = false,
      enable_sending_queue = true
    }
  ]
  omitted_channels = ["SQL_SCHEMA"]
}
`, name)
}

// Note: IAM retry helper behavior (isRetryableCloudError, iamRetryHelper)
// is tested in utils_test.go. The tests below verify log export-specific
// retry logic integration.

// TestRetryEnableLogExport_Success tests that successful API calls return nil
func TestRetryEnableLogExport_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()
	cluster := &client.Cluster{}
	logExportRequest := client.NewEnableLogExportBodyWithDefaults()
	apiLogExportObj := &client.LogExportClusterInfo{}

	expectedResponse := &client.LogExportClusterInfo{
		ClusterId: &clusterID,
	}

	s.EXPECT().EnableLogExport(gomock.Any(), clusterID, logExportRequest).
		Return(expectedResponse, &http.Response{StatusCode: http.StatusOK}, nil)

	ctx := context.Background()
	retryFunc := retryEnableLogExport(ctx, iamPropagationTimeout, s, clusterID, cluster, logExportRequest, apiLogExportObj)
	result := retryFunc()

	require.Nil(t, result, "Expected nil for successful call")
}

// TestRetryEnableLogExport_ServiceUnavailable tests that 503 errors trigger
// cluster readiness check and retry.
func TestRetryEnableLogExport_ServiceUnavailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	s := mock_client.NewMockService(ctrl)
	clusterID := uuid.Nil.String()
	cluster := &client.Cluster{State: client.CLUSTERSTATETYPE_CREATED}
	logExportRequest := client.NewEnableLogExportBodyWithDefaults()
	apiLogExportObj := &client.LogExportClusterInfo{}

	// First call returns 503, which should trigger a cluster check and retry
	s.EXPECT().EnableLogExport(gomock.Any(), clusterID, logExportRequest).
		Return(nil, &http.Response{StatusCode: http.StatusServiceUnavailable}, errors.New("service unavailable"))

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(cluster, &http.Response{StatusCode: http.StatusOK}, nil)

	ctx := context.Background()
	retryFunc := retryEnableLogExport(ctx, iamPropagationTimeout, s, clusterID, cluster, logExportRequest, apiLogExportObj)
	result := retryFunc()

	require.NotNil(t, result, "Expected retryable error")
	require.True(t, result.Retryable, "Expected retryable error for 503, got non-retryable: %v", result.Err)
}

// TestAccOTLPLogExportConfigResource attempts to create, check, and destroy a
// real cluster with OTLP/HTTP log export. It requires OTLP/HTTP log export to be
// enabled for the organization and cluster, and a receiver configured through
// TF_VAR_otlp_endpoint and TF_VAR_otlp_headers.
func TestAccOTLPLogExportConfigResource(t *testing.T) {
	skipIfTerraformBelowWriteOnly(t)
	if os.Getenv("TF_VAR_otlp_endpoint") == "" {
		t.Skip("Set TF_VAR_otlp_endpoint and TF_VAR_otlp_headers to test against an OTLP receiver.")
	}
	clusterName := fmt.Sprintf("%s-otlp-%s", tfTestPrefix, GenerateRandomString(4))
	testOTLPLogExportConfigResource(t, clusterName, false)
}

// TestIntegrationOTLPLogExportConfigResource attempts to create, check, and
// destroy an OTLP log export configuration, but uses a mocked API service.
func TestIntegrationOTLPLogExportConfigResource(t *testing.T) {
	skipIfTerraformBelowWriteOnly(t)
	for _, cloud := range []client.CloudProviderType{
		client.CLOUDPROVIDERTYPE_AWS, client.CLOUDPROVIDERTYPE_GCP,
	} {
		t.Run(string(cloud), func(t *testing.T) {
			endpoint := "https://logs.example.com"
			if cloud == client.CLOUDPROVIDERTYPE_AWS {
				endpoint += "/v1/logs"
			}
			t.Setenv(CockroachAPIKey, "fake")
			t.Setenv("TF_VAR_otlp_endpoint", endpoint)
			t.Setenv("TF_VAR_otlp_headers", `{"authorization":"Bearer first"}`)
			clusterName := fmt.Sprintf("%s-otlp-%s", tfTestPrefix, GenerateRandomString(4))
			clusterID := uuid.Nil.String()
			cluster := &client.Cluster{
				Id:            clusterID,
				CloudProvider: cloud,
				State:         client.CLUSTERSTATETYPE_CREATED,
				Config: client.ClusterConfig{
					Dedicated: &client.DedicatedHardwareConfig{},
				},
			}
			ctrl := gomock.NewController(t)
			s := mock_client.NewMockService(ctrl)
			defer HookGlobal(&NewService, func(c *client.Client) client.Service { return s })()

			var current *client.LogExportClusterInfo
			s.EXPECT().GetCluster(gomock.Any(), clusterID).Return(cluster, httpOk, nil).AnyTimes()
			s.EXPECT().GetLogExportInfo(gomock.Any(), clusterID).
				DoAndReturn(func(context.Context, string) (*client.LogExportClusterInfo, *http.Response, error) {
					return current, httpOk, nil
				}).AnyTimes()
			s.EXPECT().DeleteLogExport(gomock.Any(), clusterID).Return(nil, httpOk, nil)

			groups := []client.LogExportGroup{{
				LogName:  "crdb-audit",
				Channels: []string{"USER_ADMIN", "PRIVILEGES", "SENSITIVE_ACCESS"},
				MinLevel: ptr(client.LOGLEVELTYPE_UNSPECIFIED),
			}}
			expectEnable := func(endpoint string, headers *map[string]string, redact bool) {
				s.EXPECT().EnableLogExport(gomock.Any(), clusterID, &client.EnableLogExportBody{
					Type:         client.LOGEXPORTTYPE_OTLP_HTTP,
					LogName:      "logs",
					Groups:       &groups,
					OtlpEndpoint: &endpoint,
					OtlpHeaders:  headers,
					Redact:       &redact,
				}).DoAndReturn(func(_ context.Context, _ string, req *client.EnableLogExportBody) (*client.LogExportClusterInfo, *http.Response, error) {
					// The API returns header names, never their values.
					current = &client.LogExportClusterInfo{
						ClusterId: &clusterID,
						Status:    ptr(client.LOGEXPORTSTATUS_ENABLED),
						Spec: &client.LogExportClusterSpecification{
							Type:            ptr(req.Type),
							LogName:         ptr(req.LogName),
							Groups:          req.Groups,
							Redact:          req.Redact,
							OtlpEndpoint:    req.OtlpEndpoint,
							OtlpHeaderNames: ptr([]string{"authorization"}),
						},
					}
					return current, httpOk, nil
				})
			}
			firstHeaders := map[string]string{"authorization": "Bearer first"}
			expectEnable(endpoint, &firstHeaders, false)

			// Only creation depends on the cluster's cloud.
			if cloud == client.CLOUDPROVIDERTYPE_AWS {
				resource.Test(t, resource.TestCase{
					IsUnitTest:               true,
					ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
					Steps: []resource.TestStep{{
						Config: getTestOTLPLogExportConfigResourceConfig(clusterName, true),
						Check: resource.ComposeTestCheckFunc(
							resource.TestCheckResourceAttr("cockroach_log_export_config.test", "type", "OTLP_HTTP"),
							resource.TestCheckNoResourceAttr("cockroach_log_export_config.test", "otlp_headers_wo.%"),
						),
					}},
				})
				return
			}

			rotatedHeaders := map[string]string{
				"authorization":      "Bearer first",
				"x-tf-otlp-rotation": "2",
			}
			expectEnable(endpoint, nil, true)
			expectEnable(endpoint, &rotatedHeaders, false)
			expectEnable("https://other.example.com/v1/logs", &rotatedHeaders, false)
			expectEnable("https://other.example.com/v1/logs", nil, false)

			configType := client.LOGEXPORTTYPE_GCP_CLOUD_LOGGING
			s.EXPECT().EnableLogExport(gomock.Any(), clusterID, &client.EnableLogExportBody{
				Type:          configType,
				LogName:       "logs",
				AuthPrincipal: ptr("principal"),
				Groups:        &groups,
			}).DoAndReturn(func(_ context.Context, _ string, req *client.EnableLogExportBody) (*client.LogExportClusterInfo, *http.Response, error) {
				current = &client.LogExportClusterInfo{
					ClusterId: &clusterID,
					Status:    ptr(client.LOGEXPORTSTATUS_ENABLED),
					Spec: &client.LogExportClusterSpecification{
						Type:          ptr(req.Type),
						LogName:       ptr(req.LogName),
						AuthPrincipal: req.AuthPrincipal,
						Groups:        req.Groups,
					},
				}
				return current, httpOk, nil
			})
			expectEnable(endpoint, &firstHeaders, false)

			testOTLPLogExportConfigResource(t, clusterName, true)
		})
	}
}

func testOTLPLogExportConfigResource(t *testing.T, clusterName string, useMock bool) {
	t.Helper()
	resourceName := "cockroach_log_export_config.test"
	initial := getTestOTLPLogExportConfigResourceConfig(clusterName, useMock)
	steps := []resource.TestStep{
		{
			Config: initial,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(resourceName, "type", "OTLP_HTTP"),
				resource.TestCheckNoResourceAttr(resourceName, "auth_principal"),
				resource.TestCheckNoResourceAttr(resourceName, "otlp_headers_wo.%"),
				resource.TestCheckResourceAttr(resourceName, "otlp_headers_wo_version", "1"),
			),
		},
		{
			RefreshState: true,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckNoResourceAttr(resourceName, "otlp_headers_wo.%"),
				resource.TestCheckResourceAttr(resourceName, "otlp_headers_wo_version", "1"),
			),
		},
	}
	ordinary := initial
	if useMock {
		// A header-only change must neither trigger an update nor be sent
		// when another attribute changes without a header version bump.
		ordinary = strings.Replace(initial, "otlp_headers_wo = var.otlp_headers",
			`otlp_headers_wo = merge(var.otlp_headers, { "x-tf-otlp-ignored" = "1" })`, 1)
		steps = append(steps, resource.TestStep{Config: ordinary, PlanOnly: true})
	}
	ordinary = strings.Replace(ordinary, "redact = false", "redact = true", 1)
	rotated := strings.Replace(initial, "otlp_headers_wo = var.otlp_headers",
		`otlp_headers_wo = merge(var.otlp_headers, { "x-tf-otlp-rotation" = "2" })`, 1)
	rotated = strings.Replace(rotated, "otlp_headers_wo_version = 1", "otlp_headers_wo_version = 2", 1)
	steps = append(steps,
		resource.TestStep{
			Config: ordinary,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(resourceName, "redact", "true"),
				resource.TestCheckNoResourceAttr(resourceName, "otlp_headers_wo.%"),
				resource.TestCheckResourceAttr(resourceName, "otlp_headers_wo_version", "1"),
			),
		},
		resource.TestStep{
			Config: rotated,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckNoResourceAttr(resourceName, "otlp_headers_wo.%"),
				resource.TestCheckResourceAttr(resourceName, "otlp_headers_wo_version", "2"),
			),
		},
		resource.TestStep{
			ResourceName:            resourceName,
			ImportState:             true,
			ImportStateVerify:       true,
			ImportStateVerifyIgnore: []string{"otlp_headers_wo_version"},
		},
	)

	if useMock {
		otherEndpoint := "https://other.example.com/v1/logs"
		moved := strings.Replace(rotated, "otlp_endpoint = var.otlp_endpoint",
			fmt.Sprintf("otlp_endpoint = %q", otherEndpoint), 1)
		preserved := strings.Replace(moved,
			`otlp_headers_wo = merge(var.otlp_headers, { "x-tf-otlp-rotation" = "2" })`,
			"otlp_headers_wo = null", 1)
		preserved = strings.Replace(preserved, "otlp_headers_wo_version = 2", "otlp_headers_wo_version = null", 1)
		native := fmt.Sprintf(`
resource "cockroach_log_export_config" "test" {
  id             = %q
  type           = "GCP_CLOUD_LOGGING"
  log_name       = "logs"
  auth_principal = "principal"
  groups = [{
    log_name = "crdb-audit"
    channels = ["USER_ADMIN", "PRIVILEGES", "SENSITIVE_ACCESS"]
  }]
}
`, uuid.Nil.String())
		steps = append(steps,
			resource.TestStep{
				Config: moved,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "otlp_endpoint", otherEndpoint),
					resource.TestCheckNoResourceAttr(resourceName, "otlp_headers_wo.%"),
					resource.TestCheckResourceAttr(resourceName, "otlp_headers_wo_version", "2"),
				),
			},
			resource.TestStep{
				Config: preserved,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(resourceName, "otlp_headers_wo.%"),
					resource.TestCheckNoResourceAttr(resourceName, "otlp_headers_wo_version"),
				),
			},
			resource.TestStep{Config: preserved, PlanOnly: true},
			resource.TestStep{
				Config: native,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(resourceName, "otlp_endpoint"),
					resource.TestCheckNoResourceAttr(resourceName, "otlp_headers_wo.%"),
					resource.TestCheckNoResourceAttr(resourceName, "otlp_headers_wo_version"),
				),
			},
			resource.TestStep{
				Config: initial,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(resourceName, "auth_principal"),
					resource.TestCheckResourceAttr(resourceName, "type", "OTLP_HTTP"),
					resource.TestCheckNoResourceAttr(resourceName, "otlp_headers_wo.%"),
					resource.TestCheckResourceAttr(resourceName, "otlp_headers_wo_version", "1"),
				),
			},
		)
	}

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

func getTestOTLPLogExportConfigResourceConfig(clusterName string, useMock bool) string {
	clusterConfig := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
  name           = %q
  cloud_provider = "GCP"
  plan           = "ADVANCED"
  dedicated = {
    num_virtual_cpus = 4
    storage_gib     = 15
  }
  regions = [{ name = %q, node_count = 3 }]
}
`, clusterName, testRegion)
	clusterID := "cockroach_cluster.test.id"
	if useMock {
		clusterConfig = ""
		clusterID = fmt.Sprintf("%q", uuid.Nil.String())
	}
	return fmt.Sprintf(`
variable "otlp_endpoint" {
  type = string
}
variable "otlp_headers" {
  type      = map(string)
  sensitive = true
  ephemeral = true
  default   = {}
}
%s
resource "cockroach_log_export_config" "test" {
  id = %s
  type = "OTLP_HTTP"
  log_name = "logs"
  otlp_endpoint = var.otlp_endpoint
  otlp_headers_wo = var.otlp_headers
  otlp_headers_wo_version = 1
  redact = false
  groups = [{
    log_name = "crdb-audit"
    channels = ["USER_ADMIN", "PRIVILEGES", "SENSITIVE_ACCESS"]
  }]
}
`, clusterConfig, clusterID)
}

// TestIntegrationLogExportConfigValidation checks that invalid sink settings
// are rejected before apply, using a mocked API service.
func TestIntegrationLogExportConfigValidation(t *testing.T) {
	skipIfTerraformBelowWriteOnly(t)
	t.Setenv(CockroachAPIKey, "fake")
	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service { return s })()

	for _, tc := range []struct {
		name        string
		configType  string
		attributes  string
		expectError string
	}{
		{
			name:        "missing OTLP endpoint",
			configType:  "OTLP_HTTP",
			expectError: "otlp_endpoint is required when type is OTLP_HTTP",
		},
		{
			name:        "blank OTLP endpoint",
			configType:  "OTLP_HTTP",
			attributes:  `otlp_endpoint = " "`,
			expectError: "otlp_endpoint is required when type is OTLP_HTTP",
		},
		{
			name:        "OTLP auth principal",
			configType:  "OTLP_HTTP",
			attributes:  "otlp_endpoint = \"https://logs.example.com\"\nauth_principal = \"principal\"",
			expectError: "OTLP_HTTP authenticates through otlp_headers_wo",
		},
		{
			name:        "empty OTLP auth principal",
			configType:  "OTLP_HTTP",
			attributes:  "otlp_endpoint = \"https://logs.example.com\"\nauth_principal = \"\"",
			expectError: "OTLP_HTTP authenticates through otlp_headers_wo",
		},
		{
			name:        "null OTLP header value",
			configType:  "OTLP_HTTP",
			attributes:  "otlp_endpoint = \"https://logs.example.com\"\notlp_headers_wo = { authorization = null }\notlp_headers_wo_version = 1",
			expectError: "otlp_headers_wo values must not be null",
		},
		{
			name:        "OTLP headers without version",
			configType:  "OTLP_HTTP",
			attributes:  "otlp_endpoint = \"https://logs.example.com\"\notlp_headers_wo = { authorization = \"Bearer test\" }",
			expectError: "must be configured together",
		},
		{
			name:        "OTLP version without headers",
			configType:  "OTLP_HTTP",
			attributes:  "otlp_endpoint = \"https://logs.example.com\"\notlp_headers_wo_version = 1",
			expectError: "must be configured together",
		},
		{
			name:        "endpoint on CloudWatch",
			configType:  "AWS_CLOUDWATCH",
			attributes:  "auth_principal = \"principal\"\notlp_endpoint = \"https://logs.example.com\"",
			expectError: "otlp_endpoint can only be configured when type is OTLP_HTTP",
		},
		{
			name:        "headers on Cloud Logging",
			configType:  "GCP_CLOUD_LOGGING",
			attributes:  "auth_principal = \"principal\"\notlp_headers_wo = { authorization = \"Bearer test\" }\notlp_headers_wo_version = 1",
			expectError: "otlp_headers_wo can only be configured when type is OTLP_HTTP",
		},
		{
			name:        "headers version on Cloud Logging",
			configType:  "GCP_CLOUD_LOGGING",
			attributes:  "auth_principal = \"principal\"\notlp_headers_wo_version = 1",
			expectError: "otlp_headers_wo_version can only be configured when type is OTLP_HTTP",
		},
		{
			name:        "OTLP sending queue",
			configType:  "OTLP_HTTP",
			attributes:  "otlp_endpoint = \"https://logs.example.com\"\ngroups = [{ log_name = \"audit\", channels = [\"USER_ADMIN\"], enable_sending_queue = true }]",
			expectError: "enable_sending_queue is not supported for OTLP_HTTP",
		},
		{
			name:        "missing CloudWatch principal",
			configType:  "AWS_CLOUDWATCH",
			expectError: "auth_principal is required",
		},
		{
			name:        "missing Cloud Logging principal",
			configType:  "GCP_CLOUD_LOGGING",
			expectError: "auth_principal is required",
		},
		{
			name:        "missing Azure principal",
			configType:  "AZURE_LOG_ANALYTICS",
			expectError: "auth_principal is required",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config: fmt.Sprintf(`
resource "cockroach_log_export_config" "test" {
  id       = %q
  type     = %q
  log_name = "logs"
  %s
}
`, uuid.Nil.String(), tc.configType, tc.attributes),
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(tc.expectError),
				}},
			})
		})
	}
}
