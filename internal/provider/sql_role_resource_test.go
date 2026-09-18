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
	"fmt"
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
)

// setupSqlRoleMockEnv mirrors setupSqlUserMockEnv but leaves the cluster reads
// unbounded. How many times Terraform refreshes the parent cluster is an
// artifact of the test harness, not behavior these tests are pinning, and an
// exhausted expectation surfaces as a hang rather than a useful failure.
func setupSqlRoleMockEnv(t *testing.T, clusterName string) (env sqlUserMockEnv, cleanup func()) {
	t.Helper()
	skipIfTerraformBelowWriteOnly(t)
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	unhook := HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})

	clusterID := uuid.Nil.String()
	cluster := client.Cluster{
		Name: clusterName, Id: clusterID, Plan: "BASIC", CloudProvider: "GCP",
		Config: client.ClusterConfig{Serverless: &client.ServerlessClusterConfig{
			RoutingId: "routing-id", UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
		}},
		State:   "CREATED",
		Regions: []client.Region{{Name: "us-central1"}},
	}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&cluster, nil, nil).Times(1)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, httpOk, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Times(1)

	return sqlUserMockEnv{ctrl: ctrl, s: s, clusterID: clusterID, cluster: cluster}, unhook
}

// sqlRoleConfig returns the HCL for a role on the shared serverless cluster.
func sqlRoleConfig(body string) string {
	return fmt.Sprintf(`
resource "cockroach_sql_role" "r" {
  cluster_id = cockroach_cluster.serverless.id
%s
}
`, body)
}

// TestAccSqlRoleResource creates, checks and destroys a real cluster and SQL
// role. Skipped unless TF_ACC is set. The pilot organization must have
// SqlRoleManagementEnabled turned on for this to pass.
func TestAccSqlRoleResource(t *testing.T) {
	skipIfTerraformBelowWriteOnly(t)
	t.Parallel()
	clusterName := fmt.Sprintf("%s-sql-role-%s", tfTestPrefix, GenerateRandomString(4))
	testSqlRoleResource(t, clusterName, false)
}

// TestIntegrationSqlRoleResource drives create, the login toggle, a password
// rotation and delete against a mocked API.
func TestIntegrationSqlRoleResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-sql-role-%s", tfTestPrefix, GenerateRandomString(4))
	env, cleanup := setupSqlRoleMockEnv(t, clusterName)
	defer cleanup()

	const (
		roleName    = "app_writer"
		password    = "role@password-1"
		rotatedPass = "role@password-2"
	)

	grouping := client.SQLRole{Name: roleName, Login: false}
	principal := client.SQLRole{Name: roleName, Login: true}

	// Step 1: a plain grouping role. login defaults to false and no password
	// is sent, so the body carries only the name and the explicit false.
	env.s.EXPECT().CreateSQLRole(
		gomock.Any(), env.clusterID,
		&client.CreateSQLRoleBody{Name: roleName, Login: ptr(false)},
	).Return(&grouping, nil, nil)
	env.s.EXPECT().GetSQLRole(gomock.Any(), env.clusterID, roleName).
		Return(&grouping, httpOk, nil).Times(2)

	// Step 2: grant login and set the password in one apply. Both fields must
	// travel, and login must not force a replacement.
	env.s.EXPECT().UpdateSQLRole(
		gomock.Any(), env.clusterID, roleName,
		&client.UpdateSQLRoleBody{Login: ptr(true), Password: ptr(password)},
	).Return(&principal, nil, nil)

	// Step 3: bump only the version. The password must be re-sent; login is
	// unchanged, so it must be absent from the body rather than sent as false.
	env.s.EXPECT().UpdateSQLRole(
		gomock.Any(), env.clusterID, roleName,
		&client.UpdateSQLRoleBody{Password: ptr(rotatedPass)},
	).Return(&principal, nil, nil)

	env.s.EXPECT().GetSQLRole(gomock.Any(), env.clusterID, roleName).
		Return(&principal, httpOk, nil).AnyTimes()
	env.s.EXPECT().DeleteSQLRole(gomock.Any(), env.clusterID, roleName).
		Return(&principal, httpOk, nil)

	testSqlRoleResource(t, clusterName, true)
}

func testSqlRoleResource(t *testing.T, clusterName string, useMock bool) {
	const (
		roleName     = "app_writer"
		resourceName = "cockroach_sql_role.r"
		password     = "role@password-1"
		rotatedPass  = "role@password-2"
	)
	clusterBlock := sqlUserClusterConfig(clusterName)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   sqlUserWriteOnlyVersionChecks(),
		Steps: []resource.TestStep{
			{
				Config: clusterBlock + sqlRoleConfig(fmt.Sprintf(`  name = "%s"`, roleName)),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", roleName),
					resource.TestCheckResourceAttr(resourceName, "login", "false"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			{
				Config: clusterBlock + sqlRoleConfig(fmt.Sprintf(`  name                = "%s"
  login               = true
  password_wo         = "%s"
  password_wo_version = 1`, roleName, password)),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "login", "true"),
					resource.TestCheckResourceAttr(resourceName, "password_wo_version", "1"),
					// The write-only value must never reach state.
					resource.TestCheckNoResourceAttr(resourceName, "password_wo"),
				),
			},
			{
				Config: clusterBlock + sqlRoleConfig(fmt.Sprintf(`  name                = "%s"
  login               = true
  password_wo         = "%s"
  password_wo_version = 2`, roleName, rotatedPass)),
				Check: resource.TestCheckResourceAttr(resourceName, "password_wo_version", "2"),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				// password_wo and its version are configuration-only, so an
				// import cannot recover them.
				ImportStateVerifyIgnore: []string{"password_wo", "password_wo_version"},
			},
		},
	})
}

// TestIntegrationSqlRoleResource_NoOpUpdate pins that an apply which changes
// neither login nor the password version issues no UpdateSQLRole call at all.
// Every call opens a SQL connection to the customer cluster, so a spurious one
// would fail on a paused cluster for no reason. gomock fails the test if the
// unregistered call happens.
func TestIntegrationSqlRoleResource_NoOpUpdate(t *testing.T) {
	clusterName := fmt.Sprintf("%s-sql-role-noop-%s", tfTestPrefix, GenerateRandomString(4))
	env, cleanup := setupSqlRoleMockEnv(t, clusterName)
	defer cleanup()

	const roleName = "app_writer"
	role := client.SQLRole{Name: roleName, Login: true}

	env.s.EXPECT().CreateSQLRole(
		gomock.Any(), env.clusterID,
		&client.CreateSQLRoleBody{Name: roleName, Login: ptr(true)},
	).Return(&role, nil, nil)
	env.s.EXPECT().GetSQLRole(gomock.Any(), env.clusterID, roleName).
		Return(&role, httpOk, nil).AnyTimes()
	env.s.EXPECT().DeleteSQLRole(gomock.Any(), env.clusterID, roleName).
		Return(&role, httpOk, nil)

	// The second step re-applies an identical configuration with only a
	// cosmetic change elsewhere, so Update runs with nothing to send.
	config := sqlUserClusterConfig(clusterName) + sqlRoleConfig(fmt.Sprintf(`  name  = "%s"
  login = true`, roleName))

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   sqlUserWriteOnlyVersionChecks(),
		Steps: []resource.TestStep{
			{Config: config},
			{Config: config, PlanOnly: true},
		},
	})
}

// TestIntegrationSqlRoleResource_RemovedOutsideTerraform covers the API
// reporting a role as absent. That is also how it answers for a reserved
// principal and for a principal owned by cockroach_sql_user, so this is the
// path a bad import lands on too.
func TestIntegrationSqlRoleResource_RemovedOutsideTerraform(t *testing.T) {
	clusterName := fmt.Sprintf("%s-sql-role-gone-%s", tfTestPrefix, GenerateRandomString(4))
	env, cleanup := setupSqlRoleMockEnv(t, clusterName)
	defer cleanup()

	const roleName = "app_writer"
	role := client.SQLRole{Name: roleName}
	notFound := &http.Response{
		Status: http.StatusText(http.StatusNotFound), StatusCode: http.StatusNotFound,
	}

	gomock.InOrder(
		env.s.EXPECT().CreateSQLRole(
			gomock.Any(), env.clusterID,
			&client.CreateSQLRoleBody{Name: roleName, Login: ptr(false)},
		).Return(&role, nil, nil),
		env.s.EXPECT().GetSQLRole(gomock.Any(), env.clusterID, roleName).
			Return(&role, httpOk, nil),
		// Dropped out of band: the next refresh must remove the resource from
		// state and plan a fresh create rather than error.
		env.s.EXPECT().GetSQLRole(gomock.Any(), env.clusterID, roleName).
			Return(nil, notFound, fmt.Errorf("not found")),
		env.s.EXPECT().CreateSQLRole(
			gomock.Any(), env.clusterID,
			&client.CreateSQLRoleBody{Name: roleName, Login: ptr(false)},
		).Return(&role, nil, nil),
		env.s.EXPECT().GetSQLRole(gomock.Any(), env.clusterID, roleName).
			Return(&role, httpOk, nil).AnyTimes(),
	)
	env.s.EXPECT().DeleteSQLRole(gomock.Any(), env.clusterID, roleName).
		Return(&role, httpOk, nil)

	config := sqlUserClusterConfig(clusterName) + sqlRoleConfig(fmt.Sprintf(`  name = "%s"`, roleName))

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   sqlUserWriteOnlyVersionChecks(),
		Steps: []resource.TestStep{
			{Config: config},
			{Config: config},
		},
	})
}

// TestIntegrationSqlRoleResource_DeleteToleratesMissing covers a role already
// dropped out of band at destroy time. A 404 is the outcome delete wanted, so
// it must not fail the apply and strand the resource in state.
func TestIntegrationSqlRoleResource_DeleteToleratesMissing(t *testing.T) {
	clusterName := fmt.Sprintf("%s-sql-role-del-%s", tfTestPrefix, GenerateRandomString(4))
	env, cleanup := setupSqlRoleMockEnv(t, clusterName)
	defer cleanup()

	const roleName = "app_writer"
	role := client.SQLRole{Name: roleName}
	notFound := &http.Response{
		Status: http.StatusText(http.StatusNotFound), StatusCode: http.StatusNotFound,
	}

	env.s.EXPECT().CreateSQLRole(
		gomock.Any(), env.clusterID,
		&client.CreateSQLRoleBody{Name: roleName, Login: ptr(false)},
	).Return(&role, nil, nil)
	env.s.EXPECT().GetSQLRole(gomock.Any(), env.clusterID, roleName).
		Return(&role, httpOk, nil).AnyTimes()
	env.s.EXPECT().DeleteSQLRole(gomock.Any(), env.clusterID, roleName).
		Return(nil, notFound, fmt.Errorf("not found"))

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   sqlUserWriteOnlyVersionChecks(),
		Steps: []resource.TestStep{
			{Config: sqlUserClusterConfig(clusterName) +
				sqlRoleConfig(fmt.Sprintf(`  name = "%s"`, roleName))},
		},
	})
}

// TestIntegrationSqlRoleResource_PlanTimeValidation exercises the schema's
// name rule, the write-only password length bound, and the RequiredTogether
// pairing of password_wo with its version trigger.
func TestIntegrationSqlRoleResource_PlanTimeValidation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		expectErr *regexp.Regexp
	}{
		{
			name: "password_wo without version",
			body: `  name        = "app_writer"
  password_wo = "role@password-1"`,
			expectErr: regexp.MustCompile(`(?s)must be configured together.*password_wo.*password_wo_version`),
		},
		{
			name: "version without password_wo",
			body: `  name                = "app_writer"
  password_wo_version = 1`,
			expectErr: regexp.MustCompile(`(?s)must be configured together.*password_wo.*password_wo_version`),
		},
		{
			name: "password_wo too short",
			body: `  name                = "app_writer"
  password_wo         = "short@12345"
  password_wo_version = 1`,
			expectErr: regexp.MustCompile(`(?is)string length must be between 12 and 500`),
		},
		{
			name: "password_wo too long",
			body: `  name                = "app_writer"
  password_wo         = "` + strings.Repeat("a", 501) + `"
  password_wo_version = 1`,
			expectErr: regexp.MustCompile(`(?is)string length must be between 12 and 500`),
		},
		{
			name:      "name starting with a hyphen",
			body:      `  name = "-app-writer"`,
			expectErr: regexp.MustCompile(`(?is)must start with a letter, digit or underscore`),
		},
		{
			name:      "name containing a space",
			body:      `  name = "app writer"`,
			expectErr: regexp.MustCompile(`(?is)must start with a letter, digit or underscore`),
		},
		{
			name:      "name too long",
			body:      fmt.Sprintf(`  name = "%s"`, strings.Repeat("a", 64)),
			expectErr: regexp.MustCompile(`(?is)must start with a letter, digit or underscore`),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			skipIfTerraformBelowWriteOnly(t)
			clusterName := fmt.Sprintf("%s-sql-role-pv-%s", tfTestPrefix, GenerateRandomString(4))
			if os.Getenv(CockroachAPIKey) == "" {
				os.Setenv(CockroachAPIKey, "fake")
			}

			// The service is hooked but never called: every case must be
			// rejected during validation, before any request is made.
			ctrl := gomock.NewController(t)
			defer HookGlobal(&NewService, func(c *client.Client) client.Service {
				return mock_client.NewMockService(ctrl)
			})()

			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				TerraformVersionChecks:   sqlUserWriteOnlyVersionChecks(),
				Steps: []resource.TestStep{
					{
						Config:      sqlUserClusterConfig(clusterName) + sqlRoleConfig(tc.body),
						ExpectError: tc.expectErr,
					},
				},
			})
		})
	}
}

// TestSqlRoleImportStateFormat pins the ID shape that import accepts. A role
// name shares the SQL user namespace, so it may contain periods and hyphens
// but the cluster ID must still be a UUID.
func TestSqlRoleImportStateFormat(t *testing.T) {
	const clusterID = "1f69fdd2-600a-4cfc-a9ba-16995df0d77d"

	for _, tc := range []struct {
		id    string
		valid bool
	}{
		{id: clusterID + ":app_writer", valid: true},
		{id: clusterID + ":app.writer-1", valid: true},
		{id: clusterID + ":" + strings.Repeat("a", 63), valid: true},
		{id: "app_writer", valid: false},
		{id: "not-a-uuid:app_writer", valid: false},
		{id: clusterID + ":", valid: false},
		{id: clusterID + ":-app_writer", valid: false},
		{id: clusterID + ":" + strings.Repeat("a", 64), valid: false},
	} {
		t.Run(tc.id, func(t *testing.T) {
			matches := sqlRoleIDRegex.FindStringSubmatch(tc.id)
			if tc.valid {
				if len(matches) != 3 {
					t.Fatalf("expected %q to parse into cluster ID and name, got %v", tc.id, matches)
				}
				return
			}
			if len(matches) == 3 {
				t.Fatalf("expected %q to be rejected, but it parsed as %v", tc.id, matches)
			}
		})
	}
}
