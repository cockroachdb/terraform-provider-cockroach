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
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v8/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

const testPassword = "random-password"

var minTerraformVersionForWriteOnly = version.Must(version.NewVersion("1.11.0"))
var terraformVersionRegex = regexp.MustCompile(`Terraform v(\d+\.\d+\.\d+(?:-[\w.]+)?)`)

type sqlUserMockEnv struct {
	ctrl      *gomock.Controller
	s         *mock_client.MockService
	clusterID string
	cluster   client.Cluster
}

type clusterReadCounts struct {
	getCluster      int
	getBackupConfig int
}

// TestAccSqlUserResource attempts to create, check, and destroy
// a real cluster and SQL user. It will be skipped if TF_ACC isn't set.
func TestAccSqlUserResource(t *testing.T) {
	skipIfTerraformBelowWriteOnly(t)
	t.Parallel()
	clusterName := fmt.Sprintf("%s-sql-user-%s", tfTestPrefix, GenerateRandomString(4))
	sqlUserNamePass := "cockroach-user"
	sqlUserNameWO := "cockroach-user-wo"
	sqlUserNameNoPass := "cockroach-user-nopass"

	testSqlUserResource(t, clusterName, sqlUserNamePass, sqlUserNameWO, sqlUserNameNoPass, false)
}

// TestIntegrationSqlUserResource attempts to create, check, and destroy
// a cluster and SQL users, but uses a mocked API service. It exercises
// the legacy `password` attribute, the write-only `password_wo` attribute
// (with `password_wo_version` rotation), and the random-password path.
func TestIntegrationSqlUserResource(t *testing.T) {
	clusterName := fmt.Sprintf("%s-sql-user-%s", tfTestPrefix, GenerateRandomString(4))
	sqlUserNamePass := "cockroach-user"
	sqlUserNameWO := "cockroach-user-wo"
	sqlUserNameNoPass := "cockroach-user-nopass"
	sqlPasswordLegacy := "cockroach@legacy-1"
	sqlPasswordLegacyRotated := "cockroach@legacy-2"
	sqlPasswordWO := "cockroach@wo-1"
	sqlPasswordWORotated := "cockroach@wo-2"
	env, cleanup := setupSqlUserMockEnv(t, clusterName, clusterReadCounts{getCluster: 9, getBackupConfig: 6})
	defer cleanup()

	userPass := client.SQLUser{Name: sqlUserNamePass}
	userWO := client.SQLUser{Name: sqlUserNameWO}
	userNoPass := client.SQLUser{Name: sqlUserNameNoPass}

	// Step 1 creates: legacy password, write-only password, and random.
	env.s.EXPECT().CreateSQLUser(
		gomock.Any(), env.clusterID,
		&client.CreateSQLUserBody{Name: sqlUserNamePass, Password: sqlPasswordLegacy},
	).Return(&userPass, nil, nil)
	env.s.EXPECT().CreateSQLUser(
		gomock.Any(), env.clusterID,
		&client.CreateSQLUserBody{Name: sqlUserNameWO, Password: sqlPasswordWO},
	).Return(&userWO, nil, nil)
	env.s.EXPECT().CreateSQLUser(
		gomock.Any(), env.clusterID,
		&client.CreateSQLUserBody{Name: sqlUserNameNoPass, Password: testPassword},
	).Return(&userNoPass, nil, nil)

	// Step 2 rotations: legacy password edited; password_wo_version bumped
	// with a new password_wo value in config.
	env.s.EXPECT().UpdateSQLUserPassword(
		gomock.Any(), env.clusterID, sqlUserNamePass,
		&client.UpdateSQLUserPasswordBody{Password: sqlPasswordLegacyRotated},
	).Return(nil, nil, nil)
	env.s.EXPECT().UpdateSQLUserPassword(
		gomock.Any(), env.clusterID, sqlUserNameWO,
		&client.UpdateSQLUserPasswordBody{Password: sqlPasswordWORotated},
	).Return(nil, nil, nil)

	env.s.EXPECT().ListSQLUsers(gomock.Any(), env.clusterID, gomock.Any()).Times(20).Return(
		&client.ListSQLUsersResponse{Users: []client.SQLUser{userPass, userWO, userNoPass}}, nil, nil)
	env.s.EXPECT().DeleteSQLUser(gomock.Any(), env.clusterID, userPass.Name)
	env.s.EXPECT().DeleteSQLUser(gomock.Any(), env.clusterID, userWO.Name)
	env.s.EXPECT().DeleteSQLUser(gomock.Any(), env.clusterID, userNoPass.Name)

	testSqlUserResource(t, clusterName, sqlUserNamePass, sqlUserNameWO, sqlUserNameNoPass, true)
}

// TestIntegrationSqlUserResource_PlanTimeValidation exercises the schema's
// config validators (Conflicting / RequiredTogether) and the LengthBetween
// validators on `password` / `password_wo`.
func TestIntegrationSqlUserResource_PlanTimeValidation(t *testing.T) {
	const clusterBlock = `
resource "cockroach_cluster" "serverless" {
  name           = "%s"
  cloud_provider = "GCP"
  serverless     = {}
  regions        = [{ name = "us-central1" }]
}
`
	longPw := strings.Repeat("a", 501)

	for _, tc := range []struct {
		name      string
		tag       string
		resource  string
		expectErr *regexp.Regexp
	}{
		{
			name: "password_wo without version",
			tag:  "rtwo",
			resource: `
resource "cockroach_sql_user" "bad" {
  name        = "bad-user"
  password_wo = "wo-password-12chars"
  cluster_id  = cockroach_cluster.serverless.id
}
`,
			expectErr: regexp.MustCompile(`(?s)must be configured together.*password_wo.*password_wo_version`),
		},
		{
			name: "version without password_wo",
			tag:  "rt",
			resource: `
resource "cockroach_sql_user" "bad" {
  name                = "bad-user"
  password_wo_version = 1
  cluster_id          = cockroach_cluster.serverless.id
}
`,
			expectErr: regexp.MustCompile(`(?s)must be configured together.*password_wo.*password_wo_version`),
		},
		{
			name: "password conflicts with password_wo",
			tag:  "pwc",
			resource: `
resource "cockroach_sql_user" "bad" {
  name                = "bad-user"
  password            = "legacy-password-1"
  password_wo         = "wo-password-12chars"
  password_wo_version = 1
  cluster_id          = cockroach_cluster.serverless.id
}
`,
			expectErr: regexp.MustCompile(`(?s)cannot be configured together.*password.*password_wo`),
		},
		{
			name: "password conflicts with password_wo_version",
			tag:  "pvc",
			resource: `
resource "cockroach_sql_user" "bad" {
  name                = "bad-user"
  password            = "legacy-password-1"
  password_wo_version = 1
  cluster_id          = cockroach_cluster.serverless.id
}
`,
			expectErr: regexp.MustCompile(`(?s)cannot be configured together.*password.*password_wo_version`),
		},
		{
			name: "password too short",
			tag:  "pshort",
			resource: `
resource "cockroach_sql_user" "bad" {
  name       = "bad-user"
  password   = "short@12345"
  cluster_id = cockroach_cluster.serverless.id
}
`,
			expectErr: regexp.MustCompile(`(?is)string length must be between 12 and 500`),
		},
		{
			name: "password too long",
			tag:  "plong",
			resource: fmt.Sprintf(`
resource "cockroach_sql_user" "bad" {
  name       = "bad-user"
  password   = "%s"
  cluster_id = cockroach_cluster.serverless.id
}
`, longPw),
			expectErr: regexp.MustCompile(`(?is)string length must be between 12 and 500`),
		},
		{
			name: "password_wo too short",
			tag:  "pwoshort",
			resource: `
resource "cockroach_sql_user" "bad" {
  name                = "bad-user"
  password_wo         = "short@12345"
  password_wo_version = 1
  cluster_id          = cockroach_cluster.serverless.id
}
`,
			expectErr: regexp.MustCompile(`(?is)string length must be between 12 and 500`),
		},
		{
			name: "password_wo too long",
			tag:  "pwolong",
			resource: fmt.Sprintf(`
resource "cockroach_sql_user" "bad" {
  name                = "bad-user"
  password_wo         = "%s"
  password_wo_version = 1
  cluster_id          = cockroach_cluster.serverless.id
}
`, longPw),
			expectErr: regexp.MustCompile(`(?is)string length must be between 12 and 500`),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runSqlUserPlanTimeRejectTest(t, tc.tag, clusterBlock+tc.resource, tc.expectErr)
		})
	}
}

// TestIntegrationSqlUserResource_LegacyPasswordTransitions pins both one-edit
// exits from a legacy-`password`-populated state: clearing `password` (which
// must not touch the credential or loop) and migrating to `password_wo` in a
// single apply (which must rotate).
func TestIntegrationSqlUserResource_LegacyPasswordTransitions(t *testing.T) {
	const (
		sqlUserName       = "migrating-user"
		legacyPassword    = "legacy@cleartext-1"
		writeOnlyPassword = "write-only@rotated-1"
	)

	for _, tc := range []struct {
		name string
		// step2Config is the sql_user block after dropping `password`; it has a
		// single %s slot for the user name.
		step2Config string
		// wantUpdatePassword, when non-empty, is the exact password the
		// migration must push. Empty means step 2 must fire no
		// UpdateSQLUserPassword call at all.
		wantUpdatePassword string
		step2Check         resource.TestCheckFunc
	}{
		{
			name: "clear legacy password does not loop",
			// Clearing `password` leaves the credential untouched. The
			// framework's post-apply refresh-plan asserts an empty plan here:
			// if Update returned early on the warning and skipped State.Set,
			// state would still hold the legacy value and the plan would diff
			// again — failing the test.
			step2Config: `
resource "cockroach_sql_user" "u" {
  name       = "%s"
  cluster_id = cockroach_cluster.serverless.id
}
`,
			step2Check: resource.TestCheckNoResourceAttr("cockroach_sql_user.u", "password"),
		},
		{
			name: "migrate to write-only in one apply",
			// Drop `password` and add `password_wo` + version in one edit. The
			// write-only value must be pushed; otherwise the legacy-clear
			// warning branch would swallow the rotation and silently keep the
			// old password on the server while the plan appeared clean.
			step2Config: `
resource "cockroach_sql_user" "u" {
  name                = "%s"
  password_wo         = "` + writeOnlyPassword + `"
  password_wo_version = 1
  cluster_id          = cockroach_cluster.serverless.id
}
`,
			wantUpdatePassword: writeOnlyPassword,
			step2Check: resource.ComposeTestCheckFunc(
				resource.TestCheckNoResourceAttr("cockroach_sql_user.u", "password"),
				resource.TestCheckResourceAttr("cockroach_sql_user.u", "password_wo_version", "1"),
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clusterName := fmt.Sprintf("%s-sql-user-legacy-%s", tfTestPrefix, GenerateRandomString(4))
			env, cleanup := setupSqlUserMockEnv(t, clusterName, clusterReadCounts{getCluster: 5, getBackupConfig: 4})
			defer cleanup()

			user := client.SQLUser{Name: sqlUserName}

			// Step 1: create with the legacy password.
			env.s.EXPECT().CreateSQLUser(
				gomock.Any(), env.clusterID,
				&client.CreateSQLUserBody{Name: sqlUserName, Password: legacyPassword},
			).Return(&user, nil, nil)

			// Step 2: register the rotation expectation only for the migrate
			// case. The clear case must fire no update, so leaving it
			// unregistered makes gomock fail loudly on a stray call. The
			// exact-value matcher guards against pushing an empty string or the
			// stale legacy value.
			if tc.wantUpdatePassword != "" {
				env.s.EXPECT().UpdateSQLUserPassword(
					gomock.Any(), env.clusterID, sqlUserName,
					&client.UpdateSQLUserPasswordBody{Password: tc.wantUpdatePassword},
				).Return(nil, nil, nil)
			}
			env.s.EXPECT().ListSQLUsers(gomock.Any(), env.clusterID, gomock.Any()).Times(3).Return(
				&client.ListSQLUsersResponse{Users: []client.SQLUser{user}}, nil, nil)
			env.s.EXPECT().DeleteSQLUser(gomock.Any(), env.clusterID, user.Name)

			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				TerraformVersionChecks:   sqlUserWriteOnlyVersionChecks(),
				Steps: []resource.TestStep{
					{
						Config: sqlUserClusterConfig(clusterName) + fmt.Sprintf(`
resource "cockroach_sql_user" "u" {
  name       = "%s"
  password   = "%s"
  cluster_id = cockroach_cluster.serverless.id
}
`, sqlUserName, legacyPassword),
						Check: resource.TestCheckResourceAttr("cockroach_sql_user.u", "password", legacyPassword),
					},
					{
						Config: sqlUserClusterConfig(clusterName) + fmt.Sprintf(tc.step2Config, sqlUserName),
						Check:  tc.step2Check,
					},
				},
			})
		})
	}
}

// TestResolveUpdatedPassword exercises the decision logic directly. It
// covers the three rotation outcomes (no-op, legacy edit, write-only
// version bump), the warning emitted when legacy `password` is cleared
// from a populated state, and — symmetric with the Create path — the
// error returned when a rotation is requested but `password_wo` is
// unknown at apply time.
func TestResolveUpdatedPassword(t *testing.T) {
	known := func(s string) types.String { return types.StringValue(s) }
	null := types.StringNull
	unknownStr := types.StringUnknown
	intVal := func(i int64) types.Int64 { return types.Int64Value(i) }
	nullInt := types.Int64Null

	for _, tc := range []struct {
		name           string
		plan, state    SQLUser
		passwordWO     types.String
		wantPassword   string
		wantUpdate     bool
		wantErr        bool
		wantWarn       bool
		wantErrSummary string
	}{
		{
			name:       "no change at all",
			plan:       SQLUser{Password: null(), PasswordWOVersion: nullInt()},
			state:      SQLUser{Password: null(), PasswordWOVersion: nullInt()},
			passwordWO: null(),
		},
		{
			name:         "legacy password edit pushes new value",
			plan:         SQLUser{Password: known("new"), PasswordWOVersion: nullInt()},
			state:        SQLUser{Password: known("old"), PasswordWOVersion: nullInt()},
			passwordWO:   null(),
			wantPassword: "new",
			wantUpdate:   true,
		},
		{
			name:       "legacy password unchanged is a no-op",
			plan:       SQLUser{Password: known("same"), PasswordWOVersion: nullInt()},
			state:      SQLUser{Password: known("same"), PasswordWOVersion: nullInt()},
			passwordWO: null(),
		},
		{
			// Symmetric with the unknown password_wo case: an unknown legacy
			// `password` at apply time must error rather than pushing an
			// empty string.
			name:           "unknown legacy password errors",
			plan:           SQLUser{Password: unknownStr(), PasswordWOVersion: nullInt()},
			state:          SQLUser{Password: known("old"), PasswordWOVersion: nullInt()},
			passwordWO:     null(),
			wantErr:        true,
			wantErrSummary: "Unknown password value",
		},
		{
			name:       "clearing legacy password emits warning, no push",
			plan:       SQLUser{Password: null(), PasswordWOVersion: nullInt()},
			state:      SQLUser{Password: known("old"), PasswordWOVersion: nullInt()},
			passwordWO: null(),
			wantWarn:   true,
		},
		{
			name:         "password_wo version bump rotates",
			plan:         SQLUser{Password: null(), PasswordWOVersion: intVal(2)},
			state:        SQLUser{Password: null(), PasswordWOVersion: intVal(1)},
			passwordWO:   known("rotated"),
			wantPassword: "rotated",
			wantUpdate:   true,
		},
		{
			name:         "first-time password_wo (version null -> set) rotates",
			plan:         SQLUser{Password: null(), PasswordWOVersion: intVal(1)},
			state:        SQLUser{Password: null(), PasswordWOVersion: nullInt()},
			passwordWO:   known("initial"),
			wantPassword: "initial",
			wantUpdate:   true,
		},
		{
			name:       "password_wo set but version unchanged is a no-op",
			plan:       SQLUser{Password: null(), PasswordWOVersion: intVal(1)},
			state:      SQLUser{Password: null(), PasswordWOVersion: intVal(1)},
			passwordWO: known("same"),
		},
		{
			name:           "version bumped with unknown password_wo errors",
			plan:           SQLUser{Password: null(), PasswordWOVersion: intVal(2)},
			state:          SQLUser{Password: null(), PasswordWOVersion: intVal(1)},
			passwordWO:     unknownStr(),
			wantErr:        true,
			wantErrSummary: "Unknown password_wo value",
		},
		{
			// Single-apply migration: user drops legacy `password` and adds
			// `password_wo` + version in the same edit. The write-only value
			// must be pushed; without this the legacy-clear warning branch
			// would swallow the rotation and silently keep the old password
			// on the server while the plan appeared clean.
			name:         "migrate_legacy_password_to_password_wo_in_one_apply_pushes_new_value",
			plan:         SQLUser{Password: null(), PasswordWOVersion: intVal(1)},
			state:        SQLUser{Password: known("old"), PasswordWOVersion: nullInt()},
			passwordWO:   known("new"),
			wantPassword: "new",
			wantUpdate:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotPassword, gotUpdate, gotDiags := resolveUpdatedPassword(tc.plan, tc.state, tc.passwordWO)
			if gotPassword != tc.wantPassword {
				t.Errorf("password: got %q, want %q", gotPassword, tc.wantPassword)
			}
			if gotUpdate != tc.wantUpdate {
				t.Errorf("shouldUpdate: got %v, want %v", gotUpdate, tc.wantUpdate)
			}
			if got, want := gotDiags.HasError(), tc.wantErr; got != want {
				t.Errorf("HasError: got %v, want %v (diags=%v)", got, want, gotDiags)
			}
			if tc.wantErr && len(gotDiags) > 0 && gotDiags[0].Summary() != tc.wantErrSummary {
				t.Errorf("error summary: got %q, want %q", gotDiags[0].Summary(), tc.wantErrSummary)
			}
			if got, want := gotDiags.WarningsCount() > 0, tc.wantWarn; got != want {
				t.Errorf("has warnings: got %v, want %v (diags=%v)", got, want, gotDiags)
			}
		})
	}
}

// skipIfTerraformBelowWriteOnly skips the test when the local Terraform CLI
// (the same binary the plugin testing framework will invoke) predates
// write-only attribute support. We perform this check before any test
// scaffolding (e.g. gomock expectations) so the skip is clean.
func skipIfTerraformBelowWriteOnly(t *testing.T) {
	t.Helper()
	bin := os.Getenv("TF_ACC_TERRAFORM_PATH")
	if bin == "" {
		bin = "terraform"
	}
	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		t.Skipf("could not determine Terraform CLI version (%s): %v", bin, err)
	}
	match := terraformVersionRegex.FindStringSubmatch(string(out))
	if len(match) != 2 {
		t.Skipf("could not parse Terraform CLI version from output: %q", out)
	}
	v, err := version.NewVersion(match[1])
	if err != nil {
		t.Skipf("invalid Terraform CLI version %q: %v", match[1], err)
	}
	if v.LessThan(minTerraformVersionForWriteOnly) {
		t.Skipf("Terraform CLI %s is below %s, which is required for write-only attributes",
			v, minTerraformVersionForWriteOnly)
	}
}

func testSqlUserResource(
	t *testing.T, clusterName, sqlUserNamePass, sqlUserNameWO, sqlUserNameNoPass string, useMock bool,
) {
	var (
		clusterResourceName = "cockroach_cluster.serverless"
		resourceNamePass    = "cockroach_sql_user.with_pass"
		resourceNameWO      = "cockroach_sql_user.with_pass_wo"
		resourceNameNoPass  = "cockroach_sql_user.no_pass"
	)
	defer HookGlobal(&generateRandomPassword, func() (string, error) {
		return testPassword, nil
	})()
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// password_wo is a write-only attribute, which requires Terraform 1.11+.
		TerraformVersionChecks: sqlUserWriteOnlyVersionChecks(),
		Steps: []resource.TestStep{
			{
				Config: getTestSqlUserResourceConfig(
					clusterName, sqlUserNamePass, sqlUserNameWO, sqlUserNameNoPass,
					"cockroach@legacy-1", "cockroach@wo-1", 1,
				),
				Check: resource.ComposeTestCheckFunc(
					testSqlUserExists(resourceNamePass, clusterResourceName),
					testSqlUserExists(resourceNameWO, clusterResourceName),
					testSqlUserExists(resourceNameNoPass, clusterResourceName),
					resource.TestCheckResourceAttr(resourceNamePass, "name", sqlUserNamePass),
					resource.TestCheckResourceAttr(resourceNameWO, "name", sqlUserNameWO),
					resource.TestCheckResourceAttr(resourceNameNoPass, "name", sqlUserNameNoPass),
					resource.TestCheckResourceAttr(resourceNamePass, "password", "cockroach@legacy-1"),
					resource.TestCheckResourceAttr(resourceNameWO, "password_wo_version", "1"),
					resource.TestCheckNoResourceAttr(resourceNameWO, "password_wo"),
				),
			},
			{
				Config: getTestSqlUserResourceConfig(
					clusterName, sqlUserNamePass, sqlUserNameWO, sqlUserNameNoPass,
					"cockroach@legacy-2", "cockroach@wo-2", 2,
				),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceNamePass, "password", "cockroach@legacy-2"),
					resource.TestCheckResourceAttr(resourceNameWO, "password_wo_version", "2"),
					resource.TestCheckNoResourceAttr(resourceNameWO, "password_wo"),
				),
			},
			{
				Config: getTestSqlUserResourceConfig(
					clusterName, sqlUserNamePass, sqlUserNameWO, sqlUserNameNoPass,
					"cockroach@legacy-2", "cockroach@wo-unbumped", 2,
				),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceNamePass, "password", "cockroach@legacy-2"),
					resource.TestCheckResourceAttr(resourceNameWO, "password_wo_version", "2"),
					resource.TestCheckNoResourceAttr(resourceNameWO, "password_wo"),
				),
			},
			{
				ResourceName:            resourceNamePass,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			{
				ResourceName:            resourceNameWO,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password_wo_version"},
			},
		},
	})
}

// runSqlUserPlanTimeRejectTest is the shared harness for tests that expect
// a sql_user config to be rejected before any SQL-user API call fires
// (config validators, attribute validators, etc.).
func runSqlUserPlanTimeRejectTest(
	t *testing.T, tag, configTemplate string, expectErr *regexp.Regexp,
) {
	t.Helper()
	skipIfTerraformBelowWriteOnly(t)
	clusterName := fmt.Sprintf("%s-sql-user-%s-%s", tfTestPrefix, tag, GenerateRandomString(4))
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   sqlUserWriteOnlyVersionChecks(),
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(configTemplate, clusterName),
				ExpectError: expectErr,
			},
		},
	})
}

func testSqlUserExists(resourceName, clusterResourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		var listUserOptions client.ListSQLUsersOptions
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

		if rs.Primary.ID == "" && clusterRs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		clusterID := clusterRs.Primary.Attributes["id"]
		log.Printf("[DEBUG] clusterID: %s, name %s", clusterRs.Primary.Attributes["id"], clusterRs.Primary.Attributes["name"])

		traceAPICall("ListSQLUsers")
		clusterResp, _, err := p.service.ListSQLUsers(context.TODO(), clusterID, &listUserOptions)
		if err == nil {
			for _, user := range clusterResp.Users {
				if user.GetName() == rs.Primary.Attributes["name"] {
					return nil
				}
			}
		}

		return fmt.Errorf("user %s does not exist", rs.Primary.ID)
	}
}

func getTestSqlUserResourceConfig(
	clusterName, userNamePass, userNameWO, userNameNoPass, password, passwordWO string, passwordWOVersion int,
) string {
	return sqlUserClusterConfig(clusterName) + fmt.Sprintf(`
resource "cockroach_sql_user" "with_pass" {
  name       = "%s"
  password   = "%s"
  cluster_id = cockroach_cluster.serverless.id
}

resource "cockroach_sql_user" "with_pass_wo" {
  name                = "%s"
  password_wo         = "%s"
  password_wo_version = %d
  cluster_id          = cockroach_cluster.serverless.id
}

resource "cockroach_sql_user" "no_pass" {
  name       = "%s"
  cluster_id = cockroach_cluster.serverless.id
}
`, userNamePass, password, userNameWO, passwordWO, passwordWOVersion, userNameNoPass)
}

func setupSqlUserMockEnv(
	t *testing.T, clusterName string, reads clusterReadCounts,
) (env sqlUserMockEnv, cleanup func()) {
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
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).Times(reads.getCluster)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).Times(reads.getBackupConfig)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Times(1)

	env = sqlUserMockEnv{ctrl: ctrl, s: s, clusterID: clusterID, cluster: cluster}
	return env, unhook
}

// sqlUserClusterConfig returns the HCL for a serverless cluster resource
// used by every sql_user test.
func sqlUserClusterConfig(clusterName string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "serverless" {
  name           = "%s"
  cloud_provider = "GCP"
  serverless     = {}
  regions        = [{ name = "us-central1" }]
}
`, clusterName)
}

// sqlUserWriteOnlyVersionChecks returns the common TerraformVersionChecks
// slice requiring Terraform 1.11+.
func sqlUserWriteOnlyVersionChecks() []tfversion.TerraformVersionCheck {
	return []tfversion.TerraformVersionCheck{
		tfversion.SkipBelow(minTerraformVersionForWriteOnly),
	}
}
