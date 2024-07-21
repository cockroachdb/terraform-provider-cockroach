/*
 Copyright 2024 The Cockroach Authors

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
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"
)

// TestIntegrationRoleGrantResource attempts to create, check, and destroy a
// real role grant for a user. It will be skipped if TF_ACC isn't set.
func TestAccRoleGrantResource(t *testing.T) {
	t.Parallel()
	clusterName := fmt.Sprintf("%s-rg-resource-%s", tfTestPrefix, GenerateRandomString(4))

	// Skip the org test for the Acceptance Test because the test relies on the
	// user having access to set an org level role. In order to avoid adding
	// configuration, we use the service account user as the user to grant
	// resources to.  This user doesn't seem to have the necessary permissions
	// to perform the org grant so we skip it.
	testRoleGrantResource(t, clusterName, true /* skipOrgTest */, false /* , false /* useMock */)
}

// TestIntegrationRoleGrantResource attempts to create, check, and destroy a
// role grant for a user, but uses a mocked API service.
func TestIntegrationRoleGrantResource(t *testing.T) {
	userID := uuid.Must(uuid.NewUUID()).String()
	clusterID := uuid.Must(uuid.NewUUID()).String()
	clusterName := "test-cluster-name"
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
		Id:            clusterID,
		CloudProvider: "GCP",
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{},
		},
		State: "CREATED",
		Regions: []client.Region{
			{
				Name: "us-central1",
			},
		},
		CreatorId: userID,
	}

	orgMemberRole := client.BuiltInRole{
		Name: client.ORGANIZATIONUSERROLETYPE_ORG_MEMBER,
		Resource: client.Resource{
			Id:   nil,
			Type: client.RESOURCETYPETYPE_ORGANIZATION,
		},
	}
	orgAdminRole := client.BuiltInRole{
		Name: client.ORGANIZATIONUSERROLETYPE_ORG_ADMIN,
		Resource: client.Resource{
			Id:   nil,
			Type: client.RESOURCETYPETYPE_ORGANIZATION,
		},
	}
	orgBillingCoordinatorRole := client.BuiltInRole{
		Name: client.ORGANIZATIONUSERROLETYPE_BILLING_COORDINATOR,
		Resource: client.Resource{
			Id:   nil,
			Type: client.RESOURCETYPETYPE_ORGANIZATION,
		},
	}
	clusterOperatorWriteRole := client.BuiltInRole{
		Name: client.ORGANIZATIONUSERROLETYPE_CLUSTER_OPERATOR_WRITER,
		Resource: client.Resource{
			Id:   &clusterID,
			Type: client.RESOURCETYPETYPE_CLUSTER,
		},
	}

	PreCreateGetResponse := client.GetAllRolesForUserResponse{
		Roles: &[]client.BuiltInRole{
			orgMemberRole,
			orgAdminRole,
		},
	}

	PostCreateGetResponse := client.GetAllRolesForUserResponse{
		Roles: &[]client.BuiltInRole{
			orgMemberRole,
			orgAdminRole,
			clusterOperatorWriteRole,
			orgBillingCoordinatorRole,
		},
	}

	// Called by Cluster Create
	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).
		Return(&cluster, nil, nil)

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	// Called by Create - Cluster role addition
	s.EXPECT().GetAllRolesForUser(gomock.Any(), userID).
		Return(&PreCreateGetResponse, nil, nil)
	s.EXPECT().AddUserToRole(gomock.Any(), userID, string(client.RESOURCETYPETYPE_CLUSTER), clusterID, string(client.ORGANIZATIONUSERROLETYPE_CLUSTER_OPERATOR_WRITER)).
		Return(nil, nil, nil)

	// Called by Create - Org role addition
	s.EXPECT().GetAllRolesForUser(gomock.Any(), userID).
		Return(&client.GetAllRolesForUserResponse{
			Roles: &[]client.BuiltInRole{
				orgMemberRole,
				orgAdminRole,
				clusterOperatorWriteRole,
			},
		}, nil, nil)
	s.EXPECT().AddUserToRole(gomock.Any(), userID, string(client.RESOURCETYPETYPE_ORGANIZATION), "", string(client.ORGANIZATIONUSERROLETYPE_BILLING_COORDINATOR)).
		Return(nil, nil, nil)

	// Called by testRoleGrantExists below
	s.EXPECT().GetAllRolesForUser(gomock.Any(), userID).
		Return(&PostCreateGetResponse, nil, nil).Times(4)

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	// Called by pre update Read for second step
	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
	s.EXPECT().GetAllRolesForUser(gomock.Any(), userID).
		Return(&PostCreateGetResponse, nil, nil).Times(2)

	// Called by Create in the 2nd resource.TestCase step below to test failure
	// when role already exists.
	s.EXPECT().GetAllRolesForUser(gomock.Any(), userID).
		Return(&PostCreateGetResponse, nil, nil)

	// Called by the Import step
	s.EXPECT().GetAllRolesForUser(gomock.Any(), userID).
		Return(&PostCreateGetResponse, nil, nil).Times(1)

	// Called by Delete
	s.EXPECT().RemoveUserFromRole(gomock.Any(), userID, string(client.RESOURCETYPETYPE_CLUSTER), clusterID, string(client.ORGANIZATIONUSERROLETYPE_CLUSTER_OPERATOR_WRITER)).
		Return(nil, nil, nil)
	s.EXPECT().RemoveUserFromRole(gomock.Any(), userID, string(client.RESOURCETYPETYPE_ORGANIZATION), "", string(client.ORGANIZATIONUSERROLETYPE_BILLING_COORDINATOR)).
		Return(nil, nil, nil)
	s.EXPECT().DeleteCluster(gomock.Any(), gomock.Any()).
		Return(nil, nil, nil)

	// Post cleanup verification
	s.EXPECT().GetAllRolesForUser(gomock.Any(), userID).
		Return(&PreCreateGetResponse, nil, nil)

	testRoleGrantResource(t, clusterName, false /* skipOrgTest */, true /* useMock */)
}

func testRoleGrantResource(t *testing.T, clusterName string, skipOrgTest, useMock bool) {
	var (
		orgGrantResourceName     = "cockroach_user_role_grant.org_grant"
		clusterGrantResourceName = "cockroach_user_role_grant.cluster_grant"
		clusterResourceName      = "cockroach_cluster.test"
	)
	defer HookGlobal(&generateRandomPassword, func() (string, error) {
		return testPassword, nil
	})()

	// compose checkFuncs so we can optionally exclude the org test.
	checkFuncs := []resource.TestCheckFunc{
		testRoleGrantExists(clusterGrantResourceName, clusterResourceName, string(client.ORGANIZATIONUSERROLETYPE_CLUSTER_OPERATOR_WRITER), string(client.FOLDERRESOURCETYPETYPE_CLUSTER)),
	}
	if !skipOrgTest {
		checkFuncs = append(checkFuncs, testRoleGrantExists(orgGrantResourceName, clusterResourceName, string(client.ORGANIZATIONUSERROLETYPE_BILLING_COORDINATOR), string(client.RESOURCETYPETYPE_ORGANIZATION)))
	}

	resource.Test(t, resource.TestCase{
		// After removal of the test resources, verify that the user running the
		// tests still has at least one non ORG_MEMBER role to show that this
		// deletion did not remove the other roles like the user_role_grants
		// resource would.
		CheckDestroy:             verifyRemainingRolesIntact(clusterResourceName),
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestRoleGrantResourceConfig(clusterName, skipOrgTest),
				Check:  resource.ComposeTestCheckFunc(checkFuncs...),
			},
			{
				Config:      getTestRoleGrantResourceConfigUpdated(clusterName, skipOrgTest),
				ExpectError: regexp.MustCompile(`Role already exists`),
			},
			{
				ResourceName: clusterGrantResourceName,
				ImportState:  true,
				// It looks like ImportStateVerify is not supported unless there
				// is a singular ID which identifies your resource.
				// https://github.com/hashicorp/terraform-plugin-testing/blob/99ed7b5eeab11197f3bee6c57105bb38fe966d45/helper/resource/testing_new_import_state.go#L200-L211
				ImportStateVerify: false,
				// A custom ImportStateIdFunc is needed here for 2 reasons:
				// 1. we're using a non-standard synethisized ID
				// 2. we need to look up the userID from the fetched resource
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					resources := s.RootModule().Resources
					clusterResource, ok := resources[clusterResourceName]
					if !ok {
						return "", fmt.Errorf("not found: %s", clusterResourceName)
					}
					userID := clusterResource.Primary.Attributes["creator_id"]
					roleGrantResource, ok := resources[clusterGrantResourceName]
					if !ok {
						return "", fmt.Errorf("not found: %s", clusterGrantResourceName)
					}
					resourceID := roleGrantResource.Primary.Attributes["role.resource_id"]
					return fmt.Sprintf("%s,%s,%s,%s", userID, string(client.ORGANIZATIONUSERROLETYPE_CLUSTER_OPERATOR_WRITER), string(client.FOLDERRESOURCETYPETYPE_CLUSTER), resourceID), nil
				},
			},
		},
	})
}

// verifyRemainingRolesIntact checks to ensure our user account still has some
// non ORG_MEMBER roles attached to it. What differentiates the user_role_grant
// resource from the user_role_grants resource (besides an s) is that it manages
// only a single role grant.  If more roles are removed than just this one, this
// is an error case.  A side affect of this check is that it relies on other
// role grants already existing for this user.
func verifyRemainingRolesIntact(clusterResourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		p := testAccProvider.(*provider)
		p.service = NewService(cl)
		resources := s.RootModule().Resources

		// Grab the userID from the cluster Resource
		clusterResource, ok := resources[clusterResourceName]
		if !ok {
			return fmt.Errorf("not found: %s", clusterResourceName)
		}
		userID := clusterResource.Primary.Attributes["creator_id"]

		traceAPICall("GetAllRolesRoleUser")
		roleResp, _, err := p.service.GetAllRolesForUser(ctx, userID)
		if err != nil {
			return fmt.Errorf("error fetching roles for user %s: %s", userID, err.Error())
		}

		for _, role := range roleResp.GetRoles() {
			if role.Name != client.ORGANIZATIONUSERROLETYPE_ORG_MEMBER {
				return nil
			}
		}

		return fmt.Errorf(
			"no non ORG_MEMBER roles found for user after resource cleanup."+
				"(user_id=%s). The failure case this is checking for is whether roles "+
				"were removed inadvertently.  More likely, is that the service "+
				"account user needs to have at least one additional role beside "+
				"ORG_MEMBER assigned to it. ",
			userID,
		)
	}
}

func testRoleGrantExists(
	roleGrantResourceName, clusterResourceName, roleName, resourceType string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		p := testAccProvider.(*provider)
		p.service = NewService(cl)
		resources := s.RootModule().Resources

		resource, ok := resources[roleGrantResourceName]
		if !ok {
			return fmt.Errorf("not found: %s", roleGrantResourceName)
		}
		resourceID := resource.Primary.Attributes["role.resource_id"]

		clusterResource, ok := resources[clusterResourceName]
		if !ok {
			return fmt.Errorf("not found: %s", clusterResourceName)
		}
		userID := clusterResource.Primary.Attributes["creator_id"]

		resourceRole := Role{
			RoleName:     types.StringValue(roleName),
			ResourceType: types.StringValue(resourceType),
			ResourceId:   types.StringValue(resourceID),
		}

		traceAPICall("GetAllRolesRoleUser")
		roleResp, _, err := p.service.GetAllRolesForUser(ctx, userID)
		if err != nil {
			return fmt.Errorf("error fetching roles for user %s: %s", userID, err.Error())
		}

		for _, role := range roleResp.GetRoles() {
			if roleEqualsBuiltInRole(&resourceRole, &role) {
				return nil
			}
		}
		return fmt.Errorf(
			"Could not find user role grant for user (id=%s) and role (%s/%s/%s).",
			userID,
			roleName,
			resourceType,
			resourceID,
		)
	}
}

func TestParseRoleGrantResourceID(t *testing.T) {
	testCases := []struct {
		name                 string
		input                string
		expectedUserID       string
		expectedRoleName     string
		expectedResourceType string
		expectedResourceID   string
		expectedErrContains  string
	}{
		{
			"non org role parsing",
			"10000000-0000-0000-0000-000000000000,CLUSTER_ADMIN,CLUSTER,20000000-0000-0000-0000-000000000000",
			"10000000-0000-0000-0000-000000000000",
			"CLUSTER_ADMIN",
			"CLUSTER",
			"20000000-0000-0000-0000-000000000000",
			"",
		},
		{
			"org role parsing 3 field",
			"10000000-0000-0000-0000-000000000000,FOLDER_ADMIN,ORGANIZATION",
			"10000000-0000-0000-0000-000000000000",
			"FOLDER_ADMIN",
			"ORGANIZATION",
			"",
			"",
		},
		{
			"org role parsing 4 field",
			"10000000-0000-0000-0000-000000000000,FOLDER_ADMIN,ORGANIZATION,",
			"10000000-0000-0000-0000-000000000000",
			"FOLDER_ADMIN",
			"ORGANIZATION",
			"",
			"",
		},
		{
			"too few fields",
			"10000000-0000-0000-0000-000000000000,FOLDER_ADMIN",
			"",
			"",
			"",
			"",
			"expected import identifier with format",
		},
		{
			"too many fields",
			"10000000-0000-0000-0000-000000000000,FOLDER_ADMIN,CLUSTER,20000000-0000-0000-0000-000000000000,extra",
			"",
			"",
			"",
			"",
			"expected import identifier with format",
		},
		{
			"specifying invalid resourceID",
			"10000000-0000-0000-0000-000000000000,FOLDER_ADMIN,CLUSTER,badness",
			"",
			"",
			"",
			"",
			"unable to parse valid uuid from resourceID",
		},
		{
			"specifying invalid userID",
			"badness,FOLDER_ADMIN,CLUSTER,20000000-0000-0000-0000-000000000000",
			"",
			"",
			"",
			"",
			"unable to parse valid uuid from userID",
		},
		{
			"specifying resourceID when type is org",
			"10000000-0000-0000-0000-000000000000,FOLDER_ADMIN,ORGANIZATION,20000000-0000-0000-0000-000000000000",
			"",
			"",
			"",
			"",
			"when resourceType is ORGANIZATION, resourceID must be empty, got: \"20000000-0000-0000-0000-000000000000\"",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			userID, roleName, resourceType, resourceID, err := parseRoleGrantResourceID(tc.input)
			if tc.expectedErrContains == "" {
				require.NoError(t, err)
				require.Equal(t, userID, tc.expectedUserID)
				require.Equal(t, roleName, tc.expectedRoleName)
				require.Equal(t, resourceType, tc.expectedResourceType)
				require.Equal(t, resourceID, tc.expectedResourceID)
			} else {
				require.ErrorContains(t, err, tc.expectedErrContains)
			}
		})
	}
}

func getTestRoleGrantResourceConfig(clusterName string, skipOrgTest bool) string {
	orgResource := `
resource "cockroach_user_role_grant" "org_grant" {
	user_id = cockroach_cluster.test.creator_id
	role = {
		role_name = "BILLING_COORDINATOR",
		resource_type = "ORGANIZATION",
		resource_id = ""
	}
	// Add a fake dependency here to force determinism for the mocked test.
	depends_on = [cockroach_user_role_grant.cluster_grant]
}
	`

	baseResources := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
	name           = "%s"
	cloud_provider = "GCP"
	serverless = {
		spend_limit = 1
	}
	regions = [{
		name = "us-central1"
	}]
}

resource "cockroach_user_role_grant" "cluster_grant" {
	user_id = cockroach_cluster.test.creator_id
	role = {
		role_name = "CLUSTER_OPERATOR_WRITER",
		resource_type = "CLUSTER",
		resource_id = cockroach_cluster.test.id
	}
}
`, clusterName)

	if skipOrgTest {
		return baseResources
	} else {
		return baseResources + orgResource
	}
}

func getTestRoleGrantResourceConfigUpdated(clusterName string, skipOrgTest bool) string {
	return getTestRoleGrantResourceConfig(clusterName, skipOrgTest) + `

resource "cockroach_user_role_grant" "a_duplicate_grant" {
	user_id = cockroach_cluster.test.creator_id
	role = {
		role_name = "CLUSTER_OPERATOR_WRITER",
		resource_type = "CLUSTER",
		resource_id = cockroach_cluster.test.id
	}
}
	`
}
