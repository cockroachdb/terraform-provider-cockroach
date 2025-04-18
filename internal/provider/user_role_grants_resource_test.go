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
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccRoleGrantsResource attempts to create, check, and destroy a
// real role grant for a user. It will be skipped if TF_ACC isn't set.
func TestAccRoleGrantsResource(t *testing.T) {
	t.Parallel()
	// The Acceptance Test relies on the user having access to set an org level role.
	// Please ensure that the service account used satisfies this condition.
	testRoleResource(t, false /* useMock */)
}

// TestIntegrationRoleGrantsResource attempts to create, check, and destroy
// a role grant to a user, but uses a mocked API service.
func TestIntegrationRoleGrantsResource(t *testing.T) {
	userId := uuid.Must(uuid.NewUUID()).String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	serviceAccount := client.ServiceAccount{
		Id:          userId,
		Name:        "Test cluster SA",
		Description: "A service account used for managing access to the test cluster",
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()
	listResponse := client.ListRoleGrantsResponse{
		Grants: &[]client.UserRoleGrants{
			{
				UserId: userId,
				Roles: []client.BuiltInRole{
					{
						Name: client.ORGANIZATIONUSERROLETYPE_CLUSTER_ADMIN,
						Resource: client.Resource{
							Id:   nil,
							Type: client.RESOURCETYPETYPE_ORGANIZATION,
						},
					},
					{
						Name: client.ORGANIZATIONUSERROLETYPE_ORG_ADMIN,
						Resource: client.Resource{
							Id:   nil,
							Type: client.RESOURCETYPETYPE_ORGANIZATION,
						},
					},
					{
						Name: client.ORGANIZATIONUSERROLETYPE_ORG_MEMBER,
						Resource: client.Resource{
							Id:   nil,
							Type: client.RESOURCETYPETYPE_ORGANIZATION,
						},
					},
				},
			},
		},
		Pagination: nil,
	}

	restrictedGetResponse := client.GetAllRolesForUserResponse{
		Roles: &[]client.BuiltInRole{
			{
				Name: client.ORGANIZATIONUSERROLETYPE_ORG_MEMBER,
				Resource: client.Resource{
					Id:   nil,
					Type: client.RESOURCETYPETYPE_ORGANIZATION,
				},
			},
		},
	}

	permissionedGetResponse := client.GetAllRolesForUserResponse{
		Roles: &[]client.BuiltInRole{
			{
				Name: client.ORGANIZATIONUSERROLETYPE_CLUSTER_ADMIN,
				Resource: client.Resource{
					Id:   nil,
					Type: client.RESOURCETYPETYPE_ORGANIZATION,
				},
			},
			{
				Name: client.ORGANIZATIONUSERROLETYPE_ORG_ADMIN,
				Resource: client.Resource{
					Id:   nil,
					Type: client.RESOURCETYPETYPE_ORGANIZATION,
				},
			},
			{
				Name: client.ORGANIZATIONUSERROLETYPE_ORG_MEMBER,
				Resource: client.Resource{
					Id:   nil,
					Type: client.RESOURCETYPETYPE_ORGANIZATION,
				},
			},
		},
	}

	s.EXPECT().CreateServiceAccount(gomock.Any(), gomock.Any()).
		Return(&serviceAccount, nil, nil)
	s.EXPECT().GetAllRolesForUser(gomock.Any(), userId).
		Return(&restrictedGetResponse, nil, nil)
	s.EXPECT().SetRolesForUser(gomock.Any(), userId, gomock.Any()).
		Return(nil, nil, nil)
	s.EXPECT().GetAllRolesForUser(gomock.Any(), userId).
		Return(&permissionedGetResponse, nil, nil).Times(3)
	s.EXPECT().GetServiceAccount(gomock.Any(), serviceAccount.Id).
		Return(&serviceAccount, &http.Response{Status: http.StatusText(http.StatusOK)}, nil)
	s.EXPECT().ListRoleGrants(gomock.Any(), gomock.Any()).
		Return(&listResponse, nil, nil).Times(2)
	s.EXPECT().SetRolesForUser(gomock.Any(), userId, &client.CockroachCloudSetRolesForUserRequest{}).
		Return(nil, nil, nil)
	s.EXPECT().DeleteServiceAccount(gomock.Any(), gomock.Any()).
		Return(nil, nil, nil)

	testRoleResource(t, true /* useMock */)
}

func testRoleResource(t *testing.T, useMock bool) {
	var (
		resourceNameTest = "cockroach_user_role_grants.test"
	)
	defer HookGlobal(&generateRandomPassword, func() (string, error) {
		return testPassword, nil
	})()
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestRoleFullyPermissionsedResourceConfig(),
				Check: resource.ComposeTestCheckFunc(
					testRoleMembership(resourceNameTest, "CLUSTER_ADMIN", true /* hasRole */),
					testRoleMembership(resourceNameTest, "ORG_MEMBER", true /* hasRole */),
					testRoleMembership(resourceNameTest, "ORG_ADMIN", true /* hasRole */),
				),
			},
			{
				ResourceName:      resourceNameTest,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testRoleMembership(
	resourceName, roleName string, hasRole bool,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		p := testAccProvider.(*provider)
		p.service = NewService(cl)

		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}
		userId := rs.Primary.Attributes["user_id"]
		if rs.Primary.ID != userId {
			return fmt.Errorf("user ID does not match resource ID")
		}

		traceAPICall("GetAllRolesForUser")
		roleResp, _, err := p.service.GetAllRolesForUser(context.TODO(), userId)
		if err == nil {
			for _, role := range roleResp.GetRoles() {
				if string(role.Name) == roleName && hasRole {
					return nil
				}
			}
			if !hasRole {
				return nil
			}
		}

		return fmt.Errorf("user %s does not have correct roles", userId)
	}
}

func getTestRoleFullyPermissionsedResourceConfig() string {
	return `
resource "cockroach_service_account" "test_sa" {
  name        = "Test cluster SA"
  description = "A service account used for managing access to the test cluster"
}

resource "cockroach_user_role_grants" "test" {
  user_id = cockroach_service_account.test_sa.id
  roles = [
    {
      role_name = "ORG_ADMIN",
      resource_type = "ORGANIZATION",
      resource_id = ""
    },
    {
      role_name = "CLUSTER_ADMIN",
      resource_type = "ORGANIZATION",
      resource_id = ""
    },
    {
      resource_id   = ""
      resource_type = "ORGANIZATION"
      role_name     = "ORG_MEMBER"
    },
  ]
}
`
}
