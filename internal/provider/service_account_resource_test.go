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
	"os"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestIntegrationServiceAccountResource attempts to create, check, and destroy a
// real service account. It will be skipped if TF_ACC isn't set.
func TestAccServiceAccountResource(t *testing.T) {
	t.Parallel()
	serviceAccountName := fmt.Sprintf("%s-sa-resource-%s", tfTestPrefix, GenerateRandomString(4))

	testServiceAccountResource(t, serviceAccountName, false /* , false /* useMock */)
}

// TestIntegrationServiceAccountResource attempts to create, check, and destroy a
// service account, but uses a mocked API service.
func TestIntegrationServiceAccountResource(t *testing.T) {
	serviceAccountName := "test-sa-name"
	updatedDescription := "a description updated"
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	id := uuid.Must(uuid.NewUUID()).String()
	createTime := time.Now()
	serviceAccount := &client.ServiceAccount{
		Id: id,
		Name: serviceAccountName,
		Description: "a description",
		CreatorName: "somebody",
		CreatedAt: createTime,
		GroupRoles: []client.BuiltInFromGroups{},
		Roles: []client.BuiltInRole{{
			Name: client.ORGANIZATIONUSERROLETYPE_ORG_MEMBER,
			Resource: client.Resource{
				Type: client.RESOURCETYPETYPE_ORGANIZATION,
			},
		}},
	}

	// Called by Create
	s.EXPECT().CreateServiceAccount(gomock.Any(), &client.CreateServiceAccountRequest{
		Name:        serviceAccountName,
		Description: "a description",
		Roles:       []client.BuiltInRole{},
	}).Return(serviceAccount, nil, nil)

	// Called by testServiceAccountExists
	s.EXPECT().GetServiceAccount(gomock.Any(), id).Return(serviceAccount, nil, nil)

	// Called by Read prior to Update, I'm not sure why there are 2
	s.EXPECT().GetServiceAccount(gomock.Any(), id).Return(serviceAccount, nil, nil).Times(2)

	// Make a copy
	serviceAccountUpdated := *serviceAccount
	serviceAccountUpdated.Description = updatedDescription

	// Called by Update
	s.EXPECT().UpdateServiceAccount(gomock.Any(), id, &client.UpdateServiceAccountSpecification{
		Name:        &serviceAccountName,
		Description: &updatedDescription,
	}).Return(&serviceAccountUpdated, nil, nil)

	// Called by testServiceAccountExists
	s.EXPECT().GetServiceAccount(gomock.Any(), id).Return(&serviceAccountUpdated, nil, nil)

	// Called by Read as a result of the import test
	s.EXPECT().GetServiceAccount(gomock.Any(), id).Return(&serviceAccountUpdated, nil, nil)

	// Called by Read prior to Delete
	s.EXPECT().GetServiceAccount(gomock.Any(), id).Return(&serviceAccountUpdated, nil, nil)

	// Called by Delete
	s.EXPECT().DeleteServiceAccount(gomock.Any(), id).Return(&serviceAccountUpdated, nil, nil)

	testServiceAccountResource(t, serviceAccountName, true /* useMock */)
}

func testServiceAccountResource(t *testing.T, serviceAccountName string, useMock bool) {
	serviceAccountResourceName := "cockroach_service_account.test_sa"
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestServiceAccountResourceConfig(serviceAccountName, "a description"),
				Check:  testServiceAccountExists(serviceAccountResourceName),
			},
			{
				Config: getTestServiceAccountResourceConfig(serviceAccountName, "a description updated"),
				Check:  testServiceAccountExists(serviceAccountResourceName),
			},
			{
				ResourceName: serviceAccountResourceName,
				ImportState:  true,
				ImportStateVerify: true,
			},
		},
	})
}

func testServiceAccountExists(serviceAccountResourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		p := testAccProvider.(*provider)
		p.service = NewService(cl)
		resources := s.RootModule().Resources

		resource, ok := resources[serviceAccountResourceName]
		if !ok {
			return fmt.Errorf("not found: %s", serviceAccountResourceName)
		}
		serviceAccountID := resource.Primary.ID

		traceAPICall("GetServiceAccount")
		resp, _, err := p.service.GetServiceAccount(ctx, serviceAccountID)
		if err != nil {
			return fmt.Errorf("error fetching service account for id %s: %s", serviceAccountID, err.Error())
		}

		if resp.Id == serviceAccountID ||
		   resp.Name != resource.Primary.Attributes["name"] ||
		   resp.Description != resource.Primary.Attributes["description"] {
			return nil
		}

		return fmt.Errorf(
			"Could not find a service account matching expected fields: resp: %v, resource: %v",
			resp,
			resource,
		)
	}
}

func getTestServiceAccountResourceConfig(serviceAccountName, description string) string {
	return fmt.Sprintf(`
resource "cockroach_service_account" "test_sa" {
	name = "%s"
	description = "%s"
}
`, serviceAccountName, description)
}