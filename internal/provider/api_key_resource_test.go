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

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v3/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestIntegrationAPIKeyResource attempts to create, check, and destroy a real
// api key. It will be skipped if TF_ACC isn't set.
func TestAccAPIKeyResource(t *testing.T) {
	t.Parallel()
	serviceAccountName := fmt.Sprintf("%s-sa-resource-%s", tfTestPrefix, GenerateRandomString(4))
	apiKeyName := fmt.Sprintf("%s-api-key-res-%s", tfTestPrefix, GenerateRandomString(4))

	testAPIKeyResource(t, serviceAccountName, apiKeyName, apiKeyName + "-updated", false /* , false /* useMock */)
}

// TestIntegrationAPIKeyResource attempts to create, check, and destroy a api
// key but uses a mocked API service.
func TestIntegrationAPIKeyResource(t *testing.T) {
	apiKeyName := "test-api-key-name"
	apiKeyNameUpdated := apiKeyName + "-updated"
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	saID := uuid.Must(uuid.NewUUID()).String()
	serviceAccountName := "a service account"
	serviceAccount := &client.ServiceAccount{
		Id: saID,
		Name: serviceAccountName,
		CreatorName: "somebody",
		CreatedAt: time.Now(),
		GroupRoles: []client.BuiltInFromGroups{},
		Roles: []client.BuiltInRole{{
			Name: client.ORGANIZATIONUSERROLETYPE_ORG_MEMBER,
			Resource: client.Resource{
				Type: client.RESOURCETYPETYPE_ORGANIZATION,
			},
		}},
	}

	apiKeyID := "CCDB1_D4zlI3pmTmk5zGrzYbMhbc"
	createTime := time.Now()
	secret := apiKeyID + "_abcdeFGd81Mtx3djD45iwPfgtnaRv01234Z9047K"
	apiKey := &client.ApiKey{
		Id: apiKeyID,
		Name: apiKeyName,
		CreatedAt: createTime,
		ServiceAccountId: saID,
	}

	// Called by Service Account Create (spin up supporting resources)
	s.EXPECT().CreateServiceAccount(gomock.Any(), &client.CreateServiceAccountRequest{
		Name:        serviceAccountName,
		Description: "",
		Roles:       []client.BuiltInRole{},
	}).Return(serviceAccount, nil, nil)

	// Called by Create
	s.EXPECT().CreateApiKey(gomock.Any(), &client.CreateApiKeyRequest{
		Name:        apiKeyName,
		ServiceAccountId: saID,
	}).Return(&client.CreateApiKeyResponse{
		ApiKey: *apiKey,
		Secret: secret,
	}, nil, nil)

	// Called by testAPIKeyExists
	s.EXPECT().GetApiKey(gomock.Any(), apiKeyID).Return(apiKey, nil, nil)

	// Called by Read prior to Update, I'm not sure why there are 2 sets of these
	s.EXPECT().GetServiceAccount(gomock.Any(), saID).Return(serviceAccount, nil, nil).Times(2)
	s.EXPECT().GetApiKey(gomock.Any(), apiKeyID).Return(apiKey, nil, nil).Times(2)

	// Make a copy
	apiKeyUpdated := *apiKey
	apiKeyUpdated.Name = apiKeyNameUpdated 

	// Called by Update
	s.EXPECT().UpdateApiKey(gomock.Any(), apiKeyID, &client.UpdateApiKeySpecification{
		Name:        &apiKeyNameUpdated,
	}).Return(&apiKeyUpdated, nil, nil)

	// Called by testAPIKeyExists
	s.EXPECT().GetApiKey(gomock.Any(), apiKeyID).Return(&apiKeyUpdated, nil, nil)

	// Called by Read as a result of the import test
	s.EXPECT().GetServiceAccount(gomock.Any(), saID).Return(serviceAccount, nil, nil)
	s.EXPECT().GetApiKey(gomock.Any(), apiKeyID).Return(&apiKeyUpdated, nil, nil)

	// Called by Read prior to Delete
	s.EXPECT().GetApiKey(gomock.Any(), apiKeyID).Return(&apiKeyUpdated, nil, nil)

	// Called by Delete
	s.EXPECT().DeleteApiKey(gomock.Any(), apiKeyID).Return(&apiKeyUpdated, nil, nil)

	// Called by Service Account Delete (clean up supporting resources)
	s.EXPECT().DeleteServiceAccount(gomock.Any(), saID).Return(serviceAccount, nil, nil)

	testAPIKeyResource(t, serviceAccountName, apiKeyName, apiKeyNameUpdated, true /* useMock */)
}

func testAPIKeyResource(t *testing.T, serviceAccountName, apiKeyName, apiKeyNameUpdated string, useMock bool) {
	var (
		apiKeyResourceName = "cockroach_api_key.test_api_key"
	)
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestAPIKeyResourceConfig(serviceAccountName, apiKeyName),
				Check:  testAPIKeyExists(apiKeyResourceName),
			},
			{
				Config: getTestAPIKeyResourceConfig(serviceAccountName, apiKeyNameUpdated),
				Check:  testAPIKeyExists(apiKeyResourceName),
			},
			{
				ResourceName: apiKeyResourceName,
				ImportState:  true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					resources := s.RootModule().Resources
					apiKeyResource, ok := resources[apiKeyResourceName]
					if !ok {
						return "", fmt.Errorf("not found: %s", apiKeyResource)
					}
					return apiKeyResource.Primary.Attributes["secret"], nil
				},
			},
		},
	})
}

func testAPIKeyExists(apiKeyResourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		p := testAccProvider.(*provider)
		p.service = NewService(cl)
		resources := s.RootModule().Resources

		resource, ok := resources[apiKeyResourceName]
		if !ok {
			return fmt.Errorf("not found: %s", apiKeyResourceName)
		}
		apiKeyID := resource.Primary.ID

		traceAPICall("GetApiKey")
		resp, _, err := p.service.GetApiKey(ctx, apiKeyID)
		if err != nil {
			return fmt.Errorf("error fetching api key for id %s: %s", apiKeyID, err.Error())
		}

		if resp.Id == apiKeyID ||
		   resp.Name != resource.Primary.Attributes["name"] {
			return nil
		}

		return fmt.Errorf(
			"Could not find a api key matching expected fields: resp: %v, resource: %v",
			resp,
			resource,
		)
	}
}

func getTestAPIKeyResourceConfig(serviceAccountName, apiKeyName string) string {
	return fmt.Sprintf(`
resource "cockroach_service_account" "test_sa" {
	name = "%s"
}
resource "cockroach_api_key" "test_api_key" {
	name = "%s"
	service_account_id = cockroach_service_account.test_sa.id
}
`, serviceAccountName, apiKeyName)
}
