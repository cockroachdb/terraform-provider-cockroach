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
	"os"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// TestIntegrationRoleResource attempts to create, check, and destroy
// a role grant to a user, but uses a mocked API service.
func TestIntegrationRoleResource(t *testing.T) {
	userId := uuid.Must(uuid.NewUUID()).String()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
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

	s.EXPECT().GetAllRolesForUser(gomock.Any(), userId).
		Return(&restrictedGetResponse, nil, nil)
	s.EXPECT().GetAllRolesForUser(gomock.Any(), userId).
		Return(&permissionedGetResponse, nil, nil)
	s.EXPECT().SetRolesForUser(gomock.Any(), userId, gomock.Any()).
		Return(nil, nil, nil)
	s.EXPECT().ListRoleGrants(gomock.Any(), gomock.Any()).
		Return(&listResponse, nil, nil).Times(2)
	s.EXPECT().SetRolesForUser(gomock.Any(), userId, &client.CockroachCloudSetRolesForUserRequest{}).
		Return(nil, nil, nil)

	testRoleResource(t, userId, true)
}

func testRoleResource(t *testing.T, userId string, useMock bool) {
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
				Config: getTestRoleFullyPermissionsedResourceConfig(userId),
				Check: resource.ComposeTestCheckFunc(
					testRoleMembership(resourceNameTest, userId, "CLUSTER_ADMIN", true),
					resource.TestCheckResourceAttr(resourceNameTest, "id", userId),
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
	resourceName, userId, roleName string, hasRole bool,
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

func getTestRoleFullyPermissionsedResourceConfig(userId string) string {
	return fmt.Sprintf(`
resource "cockroach_user_role_grants" "test" {
  user_id = "%s"
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
`, userId)
}
