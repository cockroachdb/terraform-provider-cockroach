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
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v7/pkg/client"
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

// minTerraformVersionForWriteOnly is the first Terraform CLI release that
// supports write-only attributes (used by the password_wo field on
// cockroach_sql_user).
var minTerraformVersionForWriteOnly = version.Must(version.NewVersion("1.11.0"))

var terraformVersionRegex = regexp.MustCompile(`Terraform v(\d+\.\d+\.\d+(?:-[\w.]+)?)`)

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
	skipIfTerraformBelowWriteOnly(t)
	clusterName := fmt.Sprintf("%s-sql-user-%s", tfTestPrefix, GenerateRandomString(4))
	sqlUserNamePass := "cockroach-user"
	sqlUserNameWO := "cockroach-user-wo"
	sqlUserNameNoPass := "cockroach-user-nopass"
	sqlPasswordLegacy := "cockroach@legacy-1"
	sqlPasswordLegacyRotated := "cockroach@legacy-2"
	sqlPasswordWO := "cockroach@wo-1"
	sqlPasswordWORotated := "cockroach@wo-2"
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()
	cluster := client.Cluster{
		Name:          clusterName,
		Id:            uuid.Nil.String(),
		Plan:          "BASIC",
		CloudProvider: "GCP",
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{
				RoutingId:   "routing-id",
				UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
			},
		},
		State: "CREATED",
		Regions: []client.Region{
			{
				Name: "us-central1",
			},
		},
	}
	userPass := client.SQLUser{Name: sqlUserNamePass}
	userWO := client.SQLUser{Name: sqlUserNameWO}
	userNoPass := client.SQLUser{Name: sqlUserNameNoPass}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()

	// Step 1 creates: legacy password, write-only password, and random.
	s.EXPECT().CreateSQLUser(
		gomock.Any(), clusterID,
		&client.CreateSQLUserRequest{Name: sqlUserNamePass, Password: sqlPasswordLegacy},
	).Return(&userPass, nil, nil)
	s.EXPECT().CreateSQLUser(
		gomock.Any(), clusterID,
		&client.CreateSQLUserRequest{Name: sqlUserNameWO, Password: sqlPasswordWO},
	).Return(&userWO, nil, nil)
	s.EXPECT().CreateSQLUser(
		gomock.Any(), clusterID,
		&client.CreateSQLUserRequest{Name: sqlUserNameNoPass, Password: testPassword},
	).Return(&userNoPass, nil, nil)

	// Step 2 rotations: legacy password edited; password_wo_version bumped
	// with a new password_wo value in config.
	s.EXPECT().UpdateSQLUserPassword(
		gomock.Any(), clusterID, sqlUserNamePass,
		&client.UpdateSQLUserPasswordRequest{Password: sqlPasswordLegacyRotated},
	).Return(nil, nil, nil)
	s.EXPECT().UpdateSQLUserPassword(
		gomock.Any(), clusterID, sqlUserNameWO,
		&client.UpdateSQLUserPasswordRequest{Password: sqlPasswordWORotated},
	).Return(nil, nil, nil)

	s.EXPECT().ListSQLUsers(gomock.Any(), clusterID, gomock.Any()).AnyTimes().Return(
		&client.ListSQLUsersResponse{Users: []client.SQLUser{userPass, userWO, userNoPass}}, nil, nil)
	s.EXPECT().DeleteSQLUser(gomock.Any(), clusterID, userPass.Name)
	s.EXPECT().DeleteSQLUser(gomock.Any(), clusterID, userWO.Name)
	s.EXPECT().DeleteSQLUser(gomock.Any(), clusterID, userNoPass.Name)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	testSqlUserResource(t, clusterName, sqlUserNamePass, sqlUserNameWO, sqlUserNameNoPass, true)
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
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(version.Must(version.NewVersion("1.11.0"))),
		},
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
					// Legacy `password` is sensitive but still persisted to
					// state, so we can pin its value to confirm Create wrote
					// what the config said. `password_wo` is write-only and
					// absent from state; its trigger is the only observable.
					resource.TestCheckResourceAttr(resourceNamePass, "password", "cockroach@legacy-1"),
					resource.TestCheckResourceAttr(resourceNameWO, "password_wo_version", "1"),
				),
			},
			{
				// Step 2 exercises both rotation paths simultaneously. The
				// `UpdateSQLUserPassword` mock expectations registered above
				// have exact `Password` matchers, so if Update fired with
				// any value other than "cockroach@legacy-2" / "cockroach@wo-2"
				// gomock would fail the test. The state-level assertions
				// below additionally confirm the rotated value landed in
				// state for the legacy path (the write-only path can only
				// be observed via the version trigger).
				Config: getTestSqlUserResourceConfig(
					clusterName, sqlUserNamePass, sqlUserNameWO, sqlUserNameNoPass,
					"cockroach@legacy-2", "cockroach@wo-2", 2,
				),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceNamePass, "password", "cockroach@legacy-2"),
					resource.TestCheckResourceAttr(resourceNameWO, "password_wo_version", "2"),
				),
			},
			{
				// Import coverage for the legacy `password` path. The
				// attribute is sensitive but persisted to state; the cloud
				// API does not return cleartext on Read, so the imported
				// state will lack `password` and we must ignore it during
				// verification. This mirrors the pre-deprecation test shape
				// and stays in place until `password` is removed.
				ResourceName:            resourceNamePass,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			{
				ResourceName:      resourceNameWO,
				ImportState:       true,
				ImportStateVerify: true,
				// password_wo is write-only and never present in state, so it
				// is naturally absent on import. password_wo_version is in
				// state but cannot be recovered from an import that only
				// knows the SQL identity, so we ignore it.
				ImportStateVerifyIgnore: []string{"password_wo_version"},
			},
		},
	})
}

// runSqlUserPlanTimeRejectTest is the shared harness for tests that expect
// a sql_user config to be rejected before any SQL-user API call fires
// (config validators, attribute validators, etc.). It sets up cluster-only
// mock expectations — nothing user-related — and asserts the step fails
// with the expected error. configTemplate must have a single %s slot for
// the generated cluster name.
func runSqlUserPlanTimeRejectTest(
	t *testing.T, tag, configTemplate string, expectErr *regexp.Regexp,
) {
	t.Helper()
	skipIfTerraformBelowWriteOnly(t)
	clusterName := fmt.Sprintf("%s-sql-user-%s-%s", tfTestPrefix, tag, GenerateRandomString(4))
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()
	cluster := client.Cluster{
		Name: clusterName, Id: clusterID, Plan: "BASIC", CloudProvider: "GCP",
		Config: client.ClusterConfig{Serverless: &client.ServerlessClusterConfig{
			RoutingId: "routing-id", UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
		}},
		State:   "CREATED",
		Regions: []client.Region{{Name: "us-central1"}},
	}
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&cluster, nil, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().ListSQLUsers(gomock.Any(), clusterID, gomock.Any()).
		Return(&client.ListSQLUsersResponse{}, nil, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).AnyTimes()

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(version.Must(version.NewVersion("1.11.0"))),
		},
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(configTemplate, clusterName),
				ExpectError: expectErr,
			},
		},
	})
}

// TestIntegrationSqlUserResource_PasswordWOVersionRequiresPasswordWO asserts
// that setting `password_wo_version` without `password_wo` is rejected at
// plan time by the RequiredTogether config validator, rather than silently
// falling through to the random-password branch in Create.
func TestIntegrationSqlUserResource_PasswordWOVersionRequiresPasswordWO(t *testing.T) {
	runSqlUserPlanTimeRejectTest(t, "rt", `
resource "cockroach_cluster" "serverless" {
  name           = "%s"
  cloud_provider = "GCP"
  serverless     = {}
  regions        = [{ name = "us-central1" }]
}

resource "cockroach_sql_user" "bad" {
  name                = "bad-user"
  password_wo_version = 1
  cluster_id          = cockroach_cluster.serverless.id
}
`, regexp.MustCompile(`(?s)must be configured together.*password_wo.*password_wo_version`))
}

// TestIntegrationSqlUserResource_PasswordAndPasswordWOConflict asserts that
// setting both `password` and `password_wo` in the same resource is
// rejected at plan time by the Conflicting config validator. Without this
// guard the resolve order in Create would silently pick one and drop the
// other, and the consumer would only notice via drift on a later plan.
func TestIntegrationSqlUserResource_PasswordAndPasswordWOConflict(t *testing.T) {
	runSqlUserPlanTimeRejectTest(t, "pwc", `
resource "cockroach_cluster" "serverless" {
  name           = "%s"
  cloud_provider = "GCP"
  serverless     = {}
  regions        = [{ name = "us-central1" }]
}

resource "cockroach_sql_user" "bad" {
  name                = "bad-user"
  password             = "legacy@1"
  password_wo          = "wo@1"
  password_wo_version  = 1
  cluster_id           = cockroach_cluster.serverless.id
}
`, regexp.MustCompile(`(?s)cannot be configured together.*password.*password_wo`))
}

// TestIntegrationSqlUserResource_PasswordAndPasswordWOVersionConflict asserts
// that setting both `password` and `password_wo_version` is rejected at
// plan time. The version trigger has no meaning on the legacy path; the
// validator surfaces the mistake instead of silently applying only the
// legacy password.
func TestIntegrationSqlUserResource_PasswordAndPasswordWOVersionConflict(t *testing.T) {
	runSqlUserPlanTimeRejectTest(t, "pvc", `
resource "cockroach_cluster" "serverless" {
  name           = "%s"
  cloud_provider = "GCP"
  serverless     = {}
  regions        = [{ name = "us-central1" }]
}

resource "cockroach_sql_user" "bad" {
  name                = "bad-user"
  password             = "legacy@1"
  password_wo_version  = 1
  cluster_id           = cockroach_cluster.serverless.id
}
`, regexp.MustCompile(`(?s)cannot be configured together.*password.*password_wo_version`))
}

// TestIntegrationSqlUserResource_EmptyPasswordRejected pins the
// LengthAtLeast(1) validator on `password`. Empty string is a known
// non-null value that would otherwise slip past the IsNull() guards in
// Create and be forwarded to the API as an empty password, producing a
// raw HTTP 400. Guarding at plan time keeps the failure local and clear.
func TestIntegrationSqlUserResource_EmptyPasswordRejected(t *testing.T) {
	runSqlUserPlanTimeRejectTest(t, "epw", `
resource "cockroach_cluster" "serverless" {
  name           = "%s"
  cloud_provider = "GCP"
  serverless     = {}
  regions        = [{ name = "us-central1" }]
}

resource "cockroach_sql_user" "bad" {
  name       = "bad-user"
  password   = ""
  cluster_id = cockroach_cluster.serverless.id
}
`, regexp.MustCompile(`(?is)string length must be at least 1`))
}

// TestIntegrationSqlUserResource_EmptyPasswordWORejected mirrors the
// legacy empty-password test for `password_wo`. Same failure mode, same
// guard.
func TestIntegrationSqlUserResource_EmptyPasswordWORejected(t *testing.T) {
	runSqlUserPlanTimeRejectTest(t, "epwo", `
resource "cockroach_cluster" "serverless" {
  name           = "%s"
  cloud_provider = "GCP"
  serverless     = {}
  regions        = [{ name = "us-central1" }]
}

resource "cockroach_sql_user" "bad" {
  name                = "bad-user"
  password_wo         = ""
  password_wo_version = 1
  cluster_id          = cockroach_cluster.serverless.id
}
`, regexp.MustCompile(`(?is)string length must be at least 1`))
}

// TestIntegrationSqlUserResource_ClearLegacyPasswordDoesNotLoop pins the
// migration scenario: a consumer who created the user with the legacy
// `password` attribute later clears it (typically as the first step of
// switching to `password_wo`). resolveUpdatedPassword emits a warning in
// that case, but Update must still write the cleared value to state — if
// it short-circuits on the warning, state retains the old password value
// and every subsequent plan re-triggers Update with the same diff, looping
// indefinitely. The framework's built-in post-apply non-empty-plan check
// makes the loop visible: step 2 would fail if state were not written.
func TestIntegrationSqlUserResource_ClearLegacyPasswordDoesNotLoop(t *testing.T) {
	skipIfTerraformBelowWriteOnly(t)
	clusterName := fmt.Sprintf("%s-sql-user-clear-%s", tfTestPrefix, GenerateRandomString(4))
	sqlUserName := "migrating-user"
	sqlPassword := "legacy@cleartext-1"
	clusterID := uuid.Nil.String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()
	cluster := client.Cluster{
		Name: clusterName, Id: clusterID, Plan: "BASIC", CloudProvider: "GCP",
		Config: client.ClusterConfig{Serverless: &client.ServerlessClusterConfig{
			RoutingId: "routing-id", UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
		}},
		State:   "CREATED",
		Regions: []client.Region{{Name: "us-central1"}},
	}
	user := client.SQLUser{Name: sqlUserName}

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&cluster, nil, nil)
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	// Step 1: create with legacy password.
	s.EXPECT().CreateSQLUser(
		gomock.Any(), clusterID,
		&client.CreateSQLUserRequest{Name: sqlUserName, Password: sqlPassword},
	).Return(&user, nil, nil)
	// Step 2: clearing password must NOT call UpdateSQLUserPassword — the
	// underlying credential is unchanged. We deliberately do not register an
	// expectation for it; gomock will fail if one fires.
	s.EXPECT().ListSQLUsers(gomock.Any(), clusterID, gomock.Any()).AnyTimes().Return(
		&client.ListSQLUsersResponse{Users: []client.SQLUser{user}}, nil, nil)
	s.EXPECT().DeleteSQLUser(gomock.Any(), clusterID, user.Name)
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(version.Must(version.NewVersion("1.11.0"))),
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "cockroach_cluster" "serverless" {
  name           = "%s"
  cloud_provider = "GCP"
  serverless     = {}
  regions        = [{ name = "us-central1" }]
}

resource "cockroach_sql_user" "u" {
  name       = "%s"
  password   = "%s"
  cluster_id = cockroach_cluster.serverless.id
}
`, clusterName, sqlUserName, sqlPassword),
				Check: resource.TestCheckResourceAttr("cockroach_sql_user.u", "password", sqlPassword),
			},
			{
				// Clear `password` from config. The framework's post-apply
				// refresh-plan asserts the plan is empty after this step.
				// If Update returned early on the warning and skipped
				// State.Set, state would still hold the legacy value and
				// the post-apply plan would diff again — failing the test.
				Config: fmt.Sprintf(`
resource "cockroach_cluster" "serverless" {
  name           = "%s"
  cloud_provider = "GCP"
  serverless     = {}
  regions        = [{ name = "us-central1" }]
}

resource "cockroach_sql_user" "u" {
  name       = "%s"
  cluster_id = cockroach_cluster.serverless.id
}
`, clusterName, sqlUserName),
				Check: resource.TestCheckNoResourceAttr("cockroach_sql_user.u", "password"),
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
	return fmt.Sprintf(`
resource "cockroach_cluster" "serverless" {
    name           = "%s"
    cloud_provider = "GCP"
    serverless = {}
	regions = [{
		name = "us-central1"
	}]
}

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
`, clusterName, userNamePass, password, userNameWO, passwordWO, passwordWOVersion, userNameNoPass)
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
