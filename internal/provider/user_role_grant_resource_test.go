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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v3/pkg/client"
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

	// The Acceptance Test relies on the user having access to set an org level role.
	// Please ensure that the service account used satisfies this condition.
	testRoleGrantResource(t, clusterName, false /* useMock */)
}

// TestIntegrationRoleGrantResource attempts to create, check, and destroy a
// role grant for a user, but uses a mocked API service.
func TestIntegrationRoleGrantResource(t *testing.T) {
	userID := uuid.Must(uuid.NewUUID()).String()
	creatorID := uuid.Must(uuid.NewUUID()).String()
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

	serviceAccount := client.ServiceAccount{
		Id:          userID,
		Name:        "Test cluster SA",
		Description: "A service account used for managing access to the test cluster",
	}

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
		CreatorId: creatorID,
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

	PostDeleteGetResponse := client.GetAllRolesForUserResponse{
		Roles: &[]client.BuiltInRole{
			orgMemberRole,
			orgAdminRole,
			clusterOperatorWriteRole,
		},
	}

	// Called by Service Account Create
	s.EXPECT().CreateServiceAccount(gomock.Any(), gomock.Any()).
		Return(&serviceAccount, nil, nil)

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
		Return(&PostCreateGetResponse, nil, nil).Times(2)

	s.EXPECT().GetServiceAccount(gomock.Any(), serviceAccount.Id).
		Return(&serviceAccount, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	s.EXPECT().GetAllRolesForUser(gomock.Any(), userID).
		Return(&PostCreateGetResponse, nil, nil).Times(2)

	// Called by pre update Read for second step
	s.EXPECT().GetServiceAccount(gomock.Any(), serviceAccount.Id).
		Return(&serviceAccount, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	s.EXPECT().GetAllRolesForUser(gomock.Any(), userID).
		Return(&PostCreateGetResponse, nil, nil).Times(2)

	// Called by Create in the 2nd resource.TestCase step below to test failure
	// when role already exists.
	s.EXPECT().GetAllRolesForUser(gomock.Any(), userID).
		Return(&PostCreateGetResponse, nil, nil)

	// Called by pre delete Read for third step
	s.EXPECT().GetServiceAccount(gomock.Any(), serviceAccount.Id).
		Return(&serviceAccount, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	s.EXPECT().GetAllRolesForUser(gomock.Any(), userID).
		Return(&PostCreateGetResponse, nil, nil).Times(2)

	// Called by org role Delete in third step
	s.EXPECT().RemoveUserFromRole(gomock.Any(), userID, string(client.RESOURCETYPETYPE_ORGANIZATION), "", string(client.ORGANIZATIONUSERROLETYPE_BILLING_COORDINATOR)).
		Return(nil, nil, nil)

	// Called by the Import step
	s.EXPECT().GetAllRolesForUser(gomock.Any(), userID).
		Return(&PostDeleteGetResponse, nil, nil)

	// Called by Delete
	s.EXPECT().GetServiceAccount(gomock.Any(), serviceAccount.Id).
		Return(&serviceAccount, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	s.EXPECT().GetCluster(gomock.Any(), clusterID).
		Return(&cluster, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)

	s.EXPECT().GetAllRolesForUser(gomock.Any(), userID).
		Return(&PostDeleteGetResponse, nil, nil).Times(2)

	s.EXPECT().RemoveUserFromRole(gomock.Any(), userID, string(client.RESOURCETYPETYPE_CLUSTER), clusterID, string(client.ORGANIZATIONUSERROLETYPE_CLUSTER_OPERATOR_WRITER)).
		Return(nil, nil, nil)

	s.EXPECT().DeleteServiceAccount(gomock.Any(), gomock.Any()).
		Return(nil, nil, nil)

	s.EXPECT().DeleteCluster(gomock.Any(), gomock.Any()).
		Return(nil, nil, nil)

	testRoleGrantResource(t, clusterName, true /* useMock */)
}

func testRoleGrantResource(t *testing.T, clusterName string, useMock bool) {
	var (
		orgGrantResourceName     = "cockroach_user_role_grant.org_grant"
		clusterGrantResourceName = "cockroach_user_role_grant.cluster_grant"
	)
	defer HookGlobal(&generateRandomPassword, func() (string, error) {
		return testPassword, nil
	})()

	checkClusterRole := testRoleGrantExists(clusterGrantResourceName, string(client.ORGANIZATIONUSERROLETYPE_CLUSTER_OPERATOR_WRITER), string(client.FOLDERRESOURCETYPETYPE_CLUSTER))
	checkOrgRole := testRoleGrantExists(orgGrantResourceName, string(client.ORGANIZATIONUSERROLETYPE_BILLING_COORDINATOR), string(client.RESOURCETYPETYPE_ORGANIZATION))

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestRoleGrantResourceConfig(clusterName, true /* includeOrgResource */),
				Check:  resource.ComposeTestCheckFunc(checkClusterRole, checkOrgRole),
			},
			{
				Config:      getTestRoleGrantResourceWithDuplicateRole(clusterName),
				ExpectError: regexp.MustCompile(`Role already exists`),
			},
			// After removal of the org role, verify that the associated user still
			// has the cluster role to show that this deletion did not remove the
			// other roles like the user_role_grants resource would.
			{
				Config: getTestRoleGrantResourceConfig(clusterName, false /* includeOrgResource */),
				Check:  checkClusterRole,
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
					roleGrantResource, ok := resources[clusterGrantResourceName]
					if !ok {
						return "", fmt.Errorf("not found: %s", clusterGrantResourceName)
					}
					resourceID := roleGrantResource.Primary.Attributes["role.resource_id"]
					userID := roleGrantResource.Primary.Attributes["user_id"]
					return fmt.Sprintf("%s,%s,%s,%s", userID, string(client.ORGANIZATIONUSERROLETYPE_CLUSTER_OPERATOR_WRITER), string(client.FOLDERRESOURCETYPETYPE_CLUSTER), resourceID), nil
				},
			},
		},
	})
}

func testRoleGrantExists(
	roleGrantResourceName, roleName, resourceType string,
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
		userID := resource.Primary.Attributes["user_id"]

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

func getTestRoleGrantResourceConfig(clusterName string, includeOrgResource bool) string {
	orgResource := `
resource "cockroach_user_role_grant" "org_grant" {
	user_id = cockroach_service_account.test_sa.id
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
resource "cockroach_service_account" "test_sa" {
  name        = "Test cluster SA"
  description = "A service account used for managing access to the test cluster"
}

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
	user_id = cockroach_service_account.test_sa.id
	role = {
		role_name = "CLUSTER_OPERATOR_WRITER",
		resource_type = "CLUSTER",
		resource_id = cockroach_cluster.test.id
	}
}
`, clusterName)

	if includeOrgResource {
		return baseResources + orgResource
	} else {
		return baseResources
	}
}

func getTestRoleGrantResourceWithDuplicateRole(clusterName string) string {
	return getTestRoleGrantResourceConfig(clusterName, true /* includeOrgResource */) + `

resource "cockroach_user_role_grant" "a_duplicate_grant" {
	user_id = cockroach_service_account.test_sa.id
	role = {
		role_name = "CLUSTER_OPERATOR_WRITER",
		resource_type = "CLUSTER",
		resource_id = cockroach_cluster.test.id
	}
}
	`
}
