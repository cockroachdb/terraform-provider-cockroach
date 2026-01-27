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
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// egressRuleTestFixture holds common test setup for egress rule tests.
type egressRuleTestFixture struct {
	clusterID string
	ruleID    string
	mock      *mock_client.MockService
	ctrl      *gomock.Controller
	cleanups  []func()
}

// egressRuleConfig generates a Terraform config for an egress rule.
type egressRuleConfig struct {
	ClusterID   string
	Name        string
	Description string
	Type        string
	Destination string
	Ports       []int
}

func (c egressRuleConfig) String() string {
	portsStr := ""
	if len(c.Ports) > 0 {
		portsStr = "    ports       = ["
		for i, p := range c.Ports {
			if i > 0 {
				portsStr += ", "
			}
			portsStr += fmt.Sprintf("%d", p)
		}
		portsStr += "]\n"
	}

	return fmt.Sprintf(`
resource "cockroach_egress_rule" "test" {
    cluster_id  = "%s"
    name        = "%s"
    description = "%s"
    type        = "%s"
    destination = "%s"
%s}
`, c.ClusterID, c.Name, c.Description, c.Type, c.Destination, portsStr)
}

// TestIntegrationEgressRuleResource_Create tests creating egress rules with different types.
func TestIntegrationEgressRuleResource_Create(t *testing.T) {
	testCases := []struct {
		name   string
		rule   client.EgressRule
		config egressRuleConfig
		checks []resource.TestCheckFunc
	}{
		{
			name: "FQDN with ports",
			rule: client.EgressRule{
				Name:        "test-rule",
				Description: "Test egress rule",
				Type:        "FQDN",
				Destination: "api.example.com",
				Ports:       &[]int32{443, 8443},
				State:       "ACTIVE",
				CrlManaged:  false,
			},
			config: egressRuleConfig{
				Name:        "test-rule",
				Description: "Test egress rule",
				Type:        "FQDN",
				Destination: "api.example.com",
				Ports:       []int{443, 8443},
			},
			checks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "name", "test-rule"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "description", "Test egress rule"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "type", "FQDN"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "destination", "api.example.com"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "ports.#", "2"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "ports.0", "443"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "ports.1", "8443"),
				resource.TestCheckResourceAttrSet("cockroach_egress_rule.test", "id"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "state", "ACTIVE"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "crl_managed", "false"),
			},
		},
		{
			name: "CIDR without ports",
			rule: client.EgressRule{
				Name:        "cidr-rule",
				Description: "Allow internal network",
				Type:        "CIDR",
				Destination: "10.0.0.0/8",
				State:       "ACTIVE",
				CrlManaged:  false,
			},
			config: egressRuleConfig{
				Name:        "cidr-rule",
				Description: "Allow internal network",
				Type:        "CIDR",
				Destination: "10.0.0.0/8",
			},
			checks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "name", "cidr-rule"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "type", "CIDR"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "destination", "10.0.0.0/8"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "state", "ACTIVE"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "ports.#", "0"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := setupEgressRuleTest(t)
			defer f.cleanup()

			tc.rule.Id = f.ruleID
			tc.rule.ClusterId = f.clusterID
			tc.config.ClusterID = f.clusterID

			f.setupBasicMocks(tc.rule)

			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: tc.config.String(),
						Check:  resource.ComposeTestCheckFunc(tc.checks...),
					},
				},
			})
		})
	}
}

// TestIntegrationEgressRuleResource_Update tests updating egress rule's mutable fields.
func TestIntegrationEgressRuleResource_Update(t *testing.T) {
	testCases := []struct {
		name       string
		original   client.EgressRule
		updated    client.EgressRule
		origConfig egressRuleConfig
		updConfig  egressRuleConfig
		origChecks []resource.TestCheckFunc
		updChecks  []resource.TestCheckFunc
	}{
		{
			name: "description and ports",
			original: client.EgressRule{
				Name:        "update-test-rule",
				Description: "Original description",
				Type:        "FQDN",
				Destination: "api.example.com",
				Ports:       &[]int32{443},
				State:       "ACTIVE",
				CrlManaged:  false,
			},
			updated: client.EgressRule{
				Name:        "update-test-rule",
				Description: "Updated description",
				Type:        "FQDN",
				Destination: "api.example.com",
				Ports:       &[]int32{443, 8443, 9443},
				State:       "ACTIVE",
				CrlManaged:  false,
			},
			origConfig: egressRuleConfig{
				Name:        "update-test-rule",
				Description: "Original description",
				Type:        "FQDN",
				Destination: "api.example.com",
				Ports:       []int{443},
			},
			updConfig: egressRuleConfig{
				Name:        "update-test-rule",
				Description: "Updated description",
				Type:        "FQDN",
				Destination: "api.example.com",
				Ports:       []int{443, 8443, 9443},
			},
			origChecks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "description", "Original description"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "ports.#", "1"),
			},
			updChecks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "description", "Updated description"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "ports.#", "3"),
			},
		},
		{
			name: "type FQDN to CIDR",
			original: client.EgressRule{
				Name:        "type-change-test",
				Description: "Test type change",
				Type:        "FQDN",
				Destination: "api.example.com",
				State:       "ACTIVE",
				CrlManaged:  false,
			},
			updated: client.EgressRule{
				Name:        "type-change-test",
				Description: "Test type change",
				Type:        "CIDR",
				Destination: "10.0.0.0/8",
				State:       "ACTIVE",
				CrlManaged:  false,
			},
			origConfig: egressRuleConfig{
				Name:        "type-change-test",
				Description: "Test type change",
				Type:        "FQDN",
				Destination: "api.example.com",
			},
			updConfig: egressRuleConfig{
				Name:        "type-change-test",
				Description: "Test type change",
				Type:        "CIDR",
				Destination: "10.0.0.0/8",
			},
			origChecks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "type", "FQDN"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "destination", "api.example.com"),
			},
			updChecks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "type", "CIDR"),
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "destination", "10.0.0.0/8"),
			},
		},
		{
			name: "destination change",
			original: client.EgressRule{
				Name:        "destination-change-test",
				Description: "Test destination change",
				Type:        "FQDN",
				Destination: "api.example.com",
				State:       "ACTIVE",
				CrlManaged:  false,
			},
			updated: client.EgressRule{
				Name:        "destination-change-test",
				Description: "Test destination change",
				Type:        "FQDN",
				Destination: "api.newexample.com",
				State:       "ACTIVE",
				CrlManaged:  false,
			},
			origConfig: egressRuleConfig{
				Name:        "destination-change-test",
				Description: "Test destination change",
				Type:        "FQDN",
				Destination: "api.example.com",
			},
			updConfig: egressRuleConfig{
				Name:        "destination-change-test",
				Description: "Test destination change",
				Type:        "FQDN",
				Destination: "api.newexample.com",
			},
			origChecks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "destination", "api.example.com"),
			},
			updChecks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "destination", "api.newexample.com"),
			},
		},
		{
			name: "add ports to rule without ports",
			original: client.EgressRule{
				Name:        "add-ports-test",
				Description: "Test adding ports",
				Type:        "FQDN",
				Destination: "api.example.com",
				Ports:       nil,
				State:       "ACTIVE",
				CrlManaged:  false,
			},
			updated: client.EgressRule{
				Name:        "add-ports-test",
				Description: "Test adding ports",
				Type:        "FQDN",
				Destination: "api.example.com",
				Ports:       &[]int32{443, 8443},
				State:       "ACTIVE",
				CrlManaged:  false,
			},
			origConfig: egressRuleConfig{
				Name:        "add-ports-test",
				Description: "Test adding ports",
				Type:        "FQDN",
				Destination: "api.example.com",
			},
			updConfig: egressRuleConfig{
				Name:        "add-ports-test",
				Description: "Test adding ports",
				Type:        "FQDN",
				Destination: "api.example.com",
				Ports:       []int{443, 8443},
			},
			origChecks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "ports.#", "0"),
			},
			updChecks: []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "ports.#", "2"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := setupEgressRuleTest(t)
			defer f.cleanup()

			tc.original.Id = f.ruleID
			tc.original.ClusterId = f.clusterID
			tc.updated.Id = f.ruleID
			tc.updated.ClusterId = f.clusterID
			tc.origConfig.ClusterID = f.clusterID
			tc.updConfig.ClusterID = f.clusterID

			// Add ID check to verify in-place update
			tc.updChecks = append(tc.updChecks,
				resource.TestCheckResourceAttr("cockroach_egress_rule.test", "id", f.ruleID))

			f.setupUpdateMocks(tc.original, tc.updated)

			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: tc.origConfig.String(),
						Check:  resource.ComposeTestCheckFunc(tc.origChecks...),
					},
					{
						Config: tc.updConfig.String(),
						Check:  resource.ComposeTestCheckFunc(tc.updChecks...),
					},
				},
			})
		})
	}
}

// TestIntegrationEgressRuleResource_Import tests importing an existing egress rule.
func TestIntegrationEgressRuleResource_Import(t *testing.T) {
	f := setupEgressRuleTest(t)
	defer f.cleanup()

	rule := client.EgressRule{
		Id:          f.ruleID,
		ClusterId:   f.clusterID,
		Name:        "import-test-rule",
		Description: "Rule for import test",
		Type:        "FQDN",
		Destination: "api.example.com",
		Ports:       &[]int32{443},
		State:       "ACTIVE",
		CrlManaged:  false,
	}

	f.setupBasicMocks(rule)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: egressRuleConfig{
					ClusterID:   f.clusterID,
					Name:        "import-test-rule",
					Description: "Rule for import test",
					Type:        "FQDN",
					Destination: "api.example.com",
					Ports:       []int{443},
				}.String(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("cockroach_egress_rule.test", "id"),
				),
			},
			{
				ResourceName:  "cockroach_egress_rule.test",
				ImportState:   true,
				ImportStateId: fmt.Sprintf("%s:%s", f.clusterID, f.ruleID),
			},
		},
	})
}

// TestIntegrationEgressRuleResource_ImportErrors tests importing with invalid ID formats.
func TestIntegrationEgressRuleResource_ImportErrors(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	testCases := []struct {
		name        string
		importID    string
		expectError string
	}{
		{
			name:        "missing colon separator",
			importID:    "invalid-id-without-colon",
			expectError: `Invalid egress rule ID format`,
		},
		{
			name:        "empty rule_id",
			importID:    "cluster-id:",
			expectError: `Both cluster_id and rule_id must be non-empty`,
		},
		{
			name:        "empty cluster_id",
			importID:    ":rule-id",
			expectError: `Both cluster_id and rule_id must be non-empty`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:        `resource "cockroach_egress_rule" "test" {}`,
						ResourceName:  "cockroach_egress_rule.test",
						ImportState:   true,
						ImportStateId: tc.importID,
						ExpectError:   regexp.MustCompile(tc.expectError),
					},
				},
			})
		})
	}
}

// retryTestCase defines a test case for retry behavior testing.
type retryTestCase struct {
	name        string
	statusCode  int
	errorMsg    string
	expectRetry bool
	expectNil   bool
}

var (
	baseRetryTestCases = []retryTestCase{
		{name: "retries on 503", statusCode: http.StatusServiceUnavailable, errorMsg: "service unavailable", expectRetry: true},
		{name: "retries on lock error", statusCode: http.StatusConflict, errorMsg: "cluster lock is held", expectRetry: true},
		{name: "fails fast on 400", statusCode: http.StatusBadRequest, errorMsg: "invalid request", expectRetry: false},
		{name: "retries on 500", statusCode: http.StatusInternalServerError, errorMsg: "internal error", expectRetry: true},
	}
	policyUpdatingCase = retryTestCase{name: "retries on POLICY_UPDATING", statusCode: http.StatusConflict, errorMsg: "POLICY_UPDATING: policy updating", expectRetry: true}
)

func assertRetryBehavior(t *testing.T, tc retryTestCase, retryErr *retry.RetryError) {
	t.Helper()
	if tc.expectNil {
		if retryErr != nil {
			t.Errorf("expected nil error, got: %v", retryErr)
		}
		return
	}
	if tc.expectRetry {
		if retryErr == nil || !retryErr.Retryable {
			t.Errorf("expected retryable error, got: %v", retryErr)
		}
	} else {
		if retryErr == nil || retryErr.Retryable {
			t.Errorf("expected non-retryable error, got: %v", retryErr)
		}
	}
}

type waitForStateTestCase struct {
	name        string
	expectNil   bool
	expectRetry bool
}

var waitForActiveStateTestCases = []struct {
	waitForStateTestCase
	state string
}{
	{waitForStateTestCase{"succeeds on ACTIVE state", true, false}, "ACTIVE"},
	{waitForStateTestCase{"fails fast on FAILED state", false, false}, "FAILED"},
	{waitForStateTestCase{"fails fast on ERROR state", false, false}, "ERROR"},
	{waitForStateTestCase{"retries on PENDING state", false, true}, "PENDING"},
}

var waitForDeletedTestCases = []struct {
	waitForStateTestCase
	mockSetup func(mock *mock_client.MockService, clusterID, ruleID string)
}{
	{
		waitForStateTestCase{"succeeds on 404", true, false},
		func(mock *mock_client.MockService, clusterID, ruleID string) {
			mock.EXPECT().GetEgressRule(gomock.Any(), clusterID, ruleID).
				Return(nil, &http.Response{StatusCode: http.StatusNotFound}, fmt.Errorf("not found"))
		},
	},
	{
		waitForStateTestCase{"retries when rule still exists", false, true},
		func(mock *mock_client.MockService, clusterID, ruleID string) {
			mock.EXPECT().GetEgressRule(gomock.Any(), clusterID, ruleID).
				Return(&client.GetEgressRuleResponse{Rule: client.EgressRule{State: "DELETING"}}, nil, nil)
		},
	},
	{
		waitForStateTestCase{"retries on server error", false, true},
		func(mock *mock_client.MockService, clusterID, ruleID string) {
			mock.EXPECT().GetEgressRule(gomock.Any(), clusterID, ruleID).
				Return(nil, &http.Response{StatusCode: http.StatusInternalServerError}, fmt.Errorf("internal error"))
		},
	},
	{
		waitForStateTestCase{"fails fast on client error", false, false},
		func(mock *mock_client.MockService, clusterID, ruleID string) {
			mock.EXPECT().GetEgressRule(gomock.Any(), clusterID, ruleID).
				Return(nil, &http.Response{StatusCode: http.StatusBadRequest}, fmt.Errorf("bad request"))
		},
	},
}

// TestRetryFuncBehavior tests retry logic for all egress rule retry functions.
func TestRetryFuncBehavior(t *testing.T) {
	createPolicyTestCases := append(baseRetryTestCases, policyUpdatingCase)
	deleteTestCases := append(baseRetryTestCases,
		retryTestCase{name: "succeeds on 404", statusCode: http.StatusNotFound, errorMsg: "not found", expectNil: true},
	)
	updateTestCases := append(baseRetryTestCases,
		policyUpdatingCase,
		retryTestCase{name: "fails fast on 404", statusCode: http.StatusNotFound, errorMsg: "not found", expectRetry: false},
	)

	t.Run("SetEgressTrafficPolicy", func(t *testing.T) {
		for _, tc := range createPolicyTestCases {
			t.Run(tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockService := mock_client.NewMockService(ctrl)
				clusterID := uuid.New().String()

				mockService.EXPECT().SetEgressTrafficPolicy(gomock.Any(), clusterID, gomock.Any()).
					Return(&http.Response{StatusCode: tc.statusCode}, fmt.Errorf(tc.errorMsg))

				retryErr := setEgressTrafficPolicyFunc(context.Background(), mockService, clusterID)()
				assertRetryBehavior(t, tc, retryErr)
			})
		}
	})

	t.Run("AddEgressRule", func(t *testing.T) {
		for _, tc := range createPolicyTestCases {
			t.Run(tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockService := mock_client.NewMockService(ctrl)
				clusterID := uuid.New().String()
				addReq := &client.AddEgressRuleRequest{Name: "test", Description: "test", Type: "FQDN", Destination: "example.com"}

				mockService.EXPECT().AddEgressRule(gomock.Any(), clusterID, addReq).
					Return(nil, &http.Response{StatusCode: tc.statusCode}, fmt.Errorf(tc.errorMsg))

				var rule client.EgressRule
				retryErr := createEgressRuleFunc(context.Background(), mockService, clusterID, addReq, &rule)()
				assertRetryBehavior(t, tc, retryErr)
			})
		}
	})

	t.Run("DeleteEgressRule", func(t *testing.T) {
		for _, tc := range deleteTestCases {
			t.Run(tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockService := mock_client.NewMockService(ctrl)
				clusterID, ruleID := uuid.New().String(), uuid.New().String()

				mockService.EXPECT().DeleteEgressRule(gomock.Any(), clusterID, ruleID, gomock.Any()).
					Return(nil, &http.Response{StatusCode: tc.statusCode}, fmt.Errorf(tc.errorMsg))

				retryErr := deleteEgressRuleFunc(context.Background(), mockService, clusterID, ruleID)()
				assertRetryBehavior(t, tc, retryErr)
			})
		}
	})

	t.Run("EditEgressRule", func(t *testing.T) {
		for _, tc := range updateTestCases {
			t.Run(tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockService := mock_client.NewMockService(ctrl)
				clusterID, ruleID := uuid.New().String(), uuid.New().String()
				editReq := &client.EditEgressRuleRequest{}

				mockService.EXPECT().EditEgressRule(gomock.Any(), clusterID, ruleID, editReq).
					Return(nil, &http.Response{StatusCode: tc.statusCode}, fmt.Errorf(tc.errorMsg))

				var rule client.EgressRule
				retryErr := updateEgressRuleFunc(context.Background(), mockService, clusterID, ruleID, editReq, &rule)()
				assertRetryBehavior(t, tc, retryErr)
			})
		}
	})

	t.Run("WaitForEgressRuleActive", func(t *testing.T) {
		for _, tc := range waitForActiveStateTestCases {
			t.Run(tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockService := mock_client.NewMockService(ctrl)
				clusterID, ruleID := uuid.New().String(), uuid.New().String()

				mockService.EXPECT().GetEgressRule(gomock.Any(), clusterID, ruleID).
					Return(&client.GetEgressRuleResponse{Rule: client.EgressRule{State: tc.state}}, nil, nil)

				var rule client.EgressRule
				retryErr := waitForEgressRuleActiveFunc(context.Background(), mockService, clusterID, ruleID, &rule)()
				assertWaitResult(t, tc.waitForStateTestCase, retryErr)
			})
		}
	})

	t.Run("WaitForEgressRuleDeleted", func(t *testing.T) {
		for _, tc := range waitForDeletedTestCases {
			t.Run(tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				mockService := mock_client.NewMockService(ctrl)
				clusterID, ruleID := uuid.New().String(), uuid.New().String()

				tc.mockSetup(mockService, clusterID, ruleID)

				retryErr := waitForEgressRuleDeletedFunc(context.Background(), mockService, clusterID, ruleID)()
				assertWaitResult(t, tc.waitForStateTestCase, retryErr)
			})
		}
	})
}

// TestIntegrationEgressRuleResource_PortsClearingTriggersReplace tests that clearing ports
// from a rule triggers resource replacement (delete + create).
func TestIntegrationEgressRuleResource_PortsClearingTriggersReplace(t *testing.T) {
	f := setupEgressRuleTest(t)
	defer f.cleanup()

	originalRule := client.EgressRule{
		Id:          f.ruleID,
		ClusterId:   f.clusterID,
		Name:        "ports-replace-test",
		Description: "Test ports clearing triggers replace",
		Type:        "FQDN",
		Destination: "api.example.com",
		Ports:       &[]int32{443, 8443},
		State:       "ACTIVE",
		CrlManaged:  false,
	}

	replacementRule := client.EgressRule{
		ClusterId:   f.clusterID,
		Name:        "ports-replace-test",
		Description: "Test ports clearing triggers replace",
		Type:        "FQDN",
		Destination: "api.example.com",
		Ports:       nil,
		State:       "ACTIVE",
		CrlManaged:  false,
	}

	originalDeleted, _ := f.setupReplacementMocks(originalRule, replacementRule)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: egressRuleConfig{
					ClusterID:   f.clusterID,
					Name:        "ports-replace-test",
					Description: "Test ports clearing triggers replace",
					Type:        "FQDN",
					Destination: "api.example.com",
					Ports:       []int{443, 8443},
				}.String(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_egress_rule.test", "ports.#", "2"),
				),
			},
			{
				Config: egressRuleConfig{
					ClusterID:   f.clusterID,
					Name:        "ports-replace-test",
					Description: "Test ports clearing triggers replace",
					Type:        "FQDN",
					Destination: "api.example.com",
				}.String(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_egress_rule.test", "ports.#", "0"),
				),
			},
		},
	})

	if atomic.LoadInt32(originalDeleted) != 1 {
		t.Error("expected original rule to be deleted for replacement")
	}
}

// setupReplacementMocks sets up mocks for resource replacement (delete + create).
func (f *egressRuleTestFixture) setupReplacementMocks(originalRule, replacementRule client.EgressRule) (*int32, *int32) {
	var originalDeleted, replacementCreated, replacementDeleted int32
	replacementRuleID := uuid.New().String()

	f.mock.EXPECT().GetCluster(gomock.Any(), f.clusterID).
		Return(&client.Cluster{
			Id:                  f.clusterID,
			EgressTrafficPolicy: client.EGRESSTRAFFICPOLICYTYPE_DEFAULT_DENY.Ptr(),
		}, nil, nil).
		AnyTimes()

	f.mock.EXPECT().AddEgressRule(gomock.Any(), f.clusterID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, cID string, req *client.AddEgressRuleRequest) (*client.AddEgressRuleResponse, *http.Response, error) {
			if atomic.LoadInt32(&originalDeleted) == 1 {
				atomic.StoreInt32(&replacementCreated, 1)
				replacementRule.Id = replacementRuleID
				return &client.AddEgressRuleResponse{Rule: &replacementRule}, nil, nil
			}
			return &client.AddEgressRuleResponse{Rule: &originalRule}, nil, nil
		}).
		AnyTimes()

	f.mock.EXPECT().GetEgressRule(gomock.Any(), f.clusterID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, cID, rID string) (*client.GetEgressRuleResponse, *http.Response, error) {
			if rID == f.ruleID {
				if atomic.LoadInt32(&originalDeleted) == 1 {
					return nil, &http.Response{StatusCode: http.StatusNotFound}, fmt.Errorf("not found")
				}
				return &client.GetEgressRuleResponse{Rule: originalRule}, nil, nil
			}
			if rID == replacementRuleID {
				if atomic.LoadInt32(&replacementDeleted) == 1 {
					return nil, &http.Response{StatusCode: http.StatusNotFound}, fmt.Errorf("not found")
				}
				if atomic.LoadInt32(&replacementCreated) == 1 {
					return &client.GetEgressRuleResponse{Rule: replacementRule}, nil, nil
				}
			}
			return nil, &http.Response{StatusCode: http.StatusNotFound}, fmt.Errorf("not found")
		}).
		AnyTimes()

	f.mock.EXPECT().DeleteEgressRule(gomock.Any(), f.clusterID, gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, cID, rID string, opts *client.DeleteEgressRuleOptions) (*client.DeleteEgressRuleResponse, *http.Response, error) {
			if rID == f.ruleID {
				atomic.StoreInt32(&originalDeleted, 1)
			} else if rID == replacementRuleID {
				atomic.StoreInt32(&replacementDeleted, 1)
			}
			return &client.DeleteEgressRuleResponse{}, nil, nil
		}).
		AnyTimes()

	return &originalDeleted, &replacementCreated
}

func assertWaitResult(t *testing.T, tc waitForStateTestCase, retryErr *retry.RetryError) {
	t.Helper()
	if tc.expectNil {
		if retryErr != nil {
			t.Errorf("expected nil error, got: %v", retryErr)
		}
		return
	}
	if tc.expectRetry {
		if retryErr == nil || !retryErr.Retryable {
			t.Errorf("expected retryable error, got: %v", retryErr)
		}
	} else {
		if retryErr == nil || retryErr.Retryable {
			t.Errorf("expected non-retryable error, got: %v", retryErr)
		}
	}
}

// setupEgressRuleTest creates a new test fixture with common setup.
func setupEgressRuleTest(t *testing.T) *egressRuleTestFixture {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	fixture := &egressRuleTestFixture{
		clusterID: uuid.Nil.String(),
		ruleID:    uuid.New().String(),
	}

	fixture.cleanups = append(fixture.cleanups, HookGlobal(&egressRuleActiveTimeout, 5*time.Second))
	fixture.cleanups = append(fixture.cleanups, HookGlobal(&egressRuleDeletedTimeout, 5*time.Second))

	fixture.ctrl = gomock.NewController(t)
	fixture.mock = mock_client.NewMockService(fixture.ctrl)
	fixture.cleanups = append(fixture.cleanups, HookGlobal(&NewService, func(c *client.Client) client.Service {
		return fixture.mock
	}))

	return fixture
}

// cleanup runs all cleanup functions in reverse order.
func (f *egressRuleTestFixture) cleanup() {
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
}

// setupBasicMocks sets up common mock expectations for create/read/delete operations.
func (f *egressRuleTestFixture) setupBasicMocks(rule client.EgressRule) *int32 {
	var deleted int32

	f.mock.EXPECT().GetCluster(gomock.Any(), f.clusterID).
		Return(&client.Cluster{
			Id:                  f.clusterID,
			EgressTrafficPolicy: client.EGRESSTRAFFICPOLICYTYPE_ALLOW_ALL.Ptr(),
		}, nil, nil).
		AnyTimes()

	f.mock.EXPECT().SetEgressTrafficPolicy(gomock.Any(), f.clusterID, gomock.Any()).
		Return(&http.Response{StatusCode: http.StatusOK}, nil).
		AnyTimes()

	f.mock.EXPECT().AddEgressRule(gomock.Any(), f.clusterID, gomock.Any()).
		Return(&client.AddEgressRuleResponse{Rule: &rule}, nil, nil)

	f.mock.EXPECT().GetEgressRule(gomock.Any(), f.clusterID, f.ruleID).
		DoAndReturn(func(ctx context.Context, cID, rID string) (*client.GetEgressRuleResponse, *http.Response, error) {
			if atomic.LoadInt32(&deleted) == 1 {
				return nil, &http.Response{StatusCode: http.StatusNotFound}, fmt.Errorf("not found")
			}
			return &client.GetEgressRuleResponse{Rule: rule}, nil, nil
		}).
		AnyTimes()

	f.mock.EXPECT().DeleteEgressRule(gomock.Any(), f.clusterID, f.ruleID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, cID, rID string, opts *client.DeleteEgressRuleOptions) (*client.DeleteEgressRuleResponse, *http.Response, error) {
			atomic.StoreInt32(&deleted, 1)
			return &client.DeleteEgressRuleResponse{}, nil, nil
		})

	return &deleted
}

// setupUpdateMocks sets up mock expectations for update operations.
func (f *egressRuleTestFixture) setupUpdateMocks(originalRule, updatedRule client.EgressRule) *int32 {
	var state int32

	f.mock.EXPECT().GetCluster(gomock.Any(), f.clusterID).
		Return(&client.Cluster{
			Id:                  f.clusterID,
			EgressTrafficPolicy: client.EGRESSTRAFFICPOLICYTYPE_ALLOW_ALL.Ptr(),
		}, nil, nil).
		AnyTimes()

	f.mock.EXPECT().SetEgressTrafficPolicy(gomock.Any(), f.clusterID, gomock.Any()).
		Return(&http.Response{StatusCode: http.StatusOK}, nil).
		AnyTimes()

	f.mock.EXPECT().AddEgressRule(gomock.Any(), f.clusterID, gomock.Any()).
		Return(&client.AddEgressRuleResponse{Rule: &originalRule}, nil, nil)

	f.mock.EXPECT().GetEgressRule(gomock.Any(), f.clusterID, f.ruleID).
		DoAndReturn(func(ctx context.Context, cID, rID string) (*client.GetEgressRuleResponse, *http.Response, error) {
			currentState := atomic.LoadInt32(&state)
			if currentState == 2 {
				return nil, &http.Response{StatusCode: http.StatusNotFound}, fmt.Errorf("not found")
			}
			if currentState == 1 {
				return &client.GetEgressRuleResponse{Rule: updatedRule}, nil, nil
			}
			return &client.GetEgressRuleResponse{Rule: originalRule}, nil, nil
		}).
		AnyTimes()

	f.mock.EXPECT().EditEgressRule(gomock.Any(), f.clusterID, f.ruleID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, cID, rID string, req *client.EditEgressRuleRequest) (*client.EditEgressRuleResponse, *http.Response, error) {
			atomic.StoreInt32(&state, 1)
			return &client.EditEgressRuleResponse{Rule: &updatedRule}, nil, nil
		})

	f.mock.EXPECT().DeleteEgressRule(gomock.Any(), f.clusterID, f.ruleID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, cID, rID string, opts *client.DeleteEgressRuleOptions) (*client.DeleteEgressRuleResponse, *http.Response, error) {
			atomic.StoreInt32(&state, 2)
			return &client.DeleteEgressRuleResponse{}, nil, nil
		})

	return &state
}
