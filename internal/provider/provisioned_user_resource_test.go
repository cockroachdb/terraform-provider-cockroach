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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v7/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccProvisionedUserResource creates, mutates, and tears down a real
// provisioned user against the CockroachDB Cloud API. Skipped unless TF_ACC
// is set.
func TestAccProvisionedUserResource(t *testing.T) {
	t.Parallel()
	userName := fmt.Sprintf("%s-pu-%s@example.com", tfTestPrefix, GenerateRandomString(6))
	testProvisionedUserResource(t, userName, false /* useMock */)
}

// TestIntegrationProvisionedUserResource exercises the same flow against a
// mocked client, so it runs in any environment.
func TestIntegrationProvisionedUserResource(t *testing.T) {
	userName := "test-provisioned-user@example.com"
	displayName := "Test Provisioned User"
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	id := uuid.Must(uuid.NewUUID()).String()
	now := time.Now().UTC().Format(time.RFC3339)
	meta := &client.ScimMetadata{
		Created:      ptr(now),
		LastModified: ptr(now),
		Location:     ptr(fmt.Sprintf("/api/scim/v2/Users/%s", id)),
		ResourceType: ptr("User"),
	}
	emailsCore := []client.ScimEmail{{Value: userName}}
	scimSchemas := []string{"urn:ietf:params:scim:schemas:core:2.0:User"}

	// Step 1 state: minimal user — only user_name + emails, active defaults to true.
	userStep1 := &client.ScimUser{
		Id:       id,
		Schemas:  scimSchemas,
		UserName: ptr(userName),
		Active:   ptr(true),
		Emails:   &emailsCore,
		Meta:     meta,
	}

	// Step 2 state: add display_name and a structured name block.
	userStep2 := *userStep1
	userStep2.DisplayName = ptr(displayName)
	userStep2.Name = &client.ScimName{
		GivenName:  ptr("Test"),
		FamilyName: ptr("User"),
	}

	// Step 3 state: flip active to false.
	userStep3 := userStep2
	userStep3.Active = ptr(false)

	//
	// Step 1: Create.
	//
	s.EXPECT().CreateUser(gomock.Any(), &client.CreateUserRequest{
		Schemas:  scimSchemas,
		Emails:   emailsCore,
		UserName: ptr(userName),
		Active:   ptr(true),
	}).Return(&client.CreateUserResponse{
		Id:       id,
		Schemas:  scimSchemas,
		UserName: ptr(userName),
		Active:   ptr(true),
		Emails:   &emailsCore,
		Meta:     meta,
	}, nil, nil)
	// testProvisionedUserExists check.
	s.EXPECT().GetUser(gomock.Any(), id, gomock.Any()).Return(userStep1, nil, nil)

	//
	// Step 2: add display_name and name.
	//
	// Refresh + planned-state read.
	s.EXPECT().GetUser(gomock.Any(), id, gomock.Any()).Return(userStep1, nil, nil).Times(2)
	emailsUpdate := emailsCore
	step2Schemas := scimSchemas
	s.EXPECT().UpdateUser(gomock.Any(), id, &client.UpdateUserRequest{
		Schemas:     &step2Schemas,
		UserName:    ptr(userName),
		DisplayName: ptr(displayName),
		Active:      ptr(true),
		Emails:      &emailsUpdate,
		Name: &client.ScimName{
			GivenName:  ptr("Test"),
			FamilyName: ptr("User"),
		},
	}).Return(&userStep2, nil, nil)
	s.EXPECT().GetUser(gomock.Any(), id, gomock.Any()).Return(&userStep2, nil, nil)

	//
	// Step 3: flip active to false.
	//
	s.EXPECT().GetUser(gomock.Any(), id, gomock.Any()).Return(&userStep2, nil, nil).Times(2)
	step3Schemas := scimSchemas
	s.EXPECT().UpdateUser(gomock.Any(), id, &client.UpdateUserRequest{
		Schemas:     &step3Schemas,
		UserName:    ptr(userName),
		DisplayName: ptr(displayName),
		Active:      ptr(false),
		Emails:      &emailsUpdate,
		Name: &client.ScimName{
			GivenName:  ptr("Test"),
			FamilyName: ptr("User"),
		},
	}).Return(&userStep3, nil, nil)
	s.EXPECT().GetUser(gomock.Any(), id, gomock.Any()).Return(&userStep3, nil, nil)

	//
	// Step 4: Import.
	//
	s.EXPECT().GetUser(gomock.Any(), id, gomock.Any()).Return(&userStep3, nil, nil)

	//
	// Implicit Delete.
	//
	s.EXPECT().GetUser(gomock.Any(), id, gomock.Any()).Return(&userStep3, nil, nil)
	s.EXPECT().DeleteUser(gomock.Any(), id).Return(nil, nil)

	testProvisionedUserResource(t, userName, true /* useMock */)
}

func testProvisionedUserResource(t *testing.T, userName string, useMock bool) {
	resourceName := "cockroach_provisioned_user.test_user"
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: minimal user (active defaults to true).
			{
				Config: getTestProvisionedUserResourceConfig(userName, "", false /* includeName */, true /* active */),
				Check: resource.ComposeTestCheckFunc(
					testProvisionedUserExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "user_name", userName),
					resource.TestCheckResourceAttr(resourceName, "active", "true"),
					resource.TestCheckResourceAttr(resourceName, "emails.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "emails.0.value", userName),
				),
			},
			// Step 2: add display_name + name block.
			{
				Config: getTestProvisionedUserResourceConfig(userName, "Test Provisioned User", true /* includeName */, true /* active */),
				Check: resource.ComposeTestCheckFunc(
					testProvisionedUserExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "display_name", "Test Provisioned User"),
					resource.TestCheckResourceAttr(resourceName, "name.given_name", "Test"),
					resource.TestCheckResourceAttr(resourceName, "name.family_name", "User"),
				),
			},
			// Step 3: deactivate.
			{
				Config: getTestProvisionedUserResourceConfig(userName, "Test Provisioned User", true /* includeName */, false /* active */),
				Check: resource.ComposeTestCheckFunc(
					testProvisionedUserExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "active", "false"),
				),
			},
			// Step 4: Import.
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testProvisionedUserExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		p := testAccProvider.(*provider)
		p.service = NewService(cl)
		resources := s.RootModule().Resources

		userResource, ok := resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}
		userID := userResource.Primary.ID

		traceAPICall("GetUser")
		resp, _, err := p.service.GetUser(ctx, userID, nil /* options */)
		if err != nil {
			return fmt.Errorf("error fetching provisioned user for id %s: %s", userID, err.Error())
		}

		if resp.Id != userID {
			return fmt.Errorf("returned user id %s does not match state %s", resp.Id, userID)
		}
		if want := userResource.Primary.Attributes["user_name"]; resp.UserName != nil && *resp.UserName != want {
			return fmt.Errorf("user_name mismatch: server=%s state=%s", *resp.UserName, want)
		}
		return nil
	}
}

func getTestProvisionedUserResourceConfig(
	userName, displayName string, includeName, active bool,
) string {
	displayLine := ""
	if displayName != "" {
		displayLine = fmt.Sprintf("display_name = %q", displayName)
	}
	nameBlock := ""
	if includeName {
		nameBlock = `
		name = {
			given_name  = "Test"
			family_name = "User"
		}`
	}
	return fmt.Sprintf(`
resource "cockroach_provisioned_user" "test_user" {
	user_name = %q
	active    = %t
	%s
	emails = [
		{ value = %q },
	]
	%s
}
`, userName, active, displayLine, userName, nameBlock)
}
