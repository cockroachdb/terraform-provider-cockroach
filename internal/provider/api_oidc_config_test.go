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
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccApiOidcConfigResource attempts to create, check, and destroy
// a real API OIDC Config. It will be skipped if TF_ACC isn't set.
// In order to work the ApiOidcEnabled Feature Flag must be enabled and
// the test org must have Org SSO enabled (no need for SAML/OIDC).
func TestAccApiOidcConfigResource(t *testing.T) {
	t.Parallel()
	issuer := fmt.Sprintf("issuer-%s", GenerateRandomString(4))
	audience := "audience"
	jwks := "{}"
	claim := "subject"
	identityMap := []IdentityMapEntry{
		{
			CcIdentity:    types.StringValue("cc_id1"),
			TokenIdentity: types.StringValue("token_id1"),
			IsRegex:       types.BoolValue(false),
		},
		{
			CcIdentity:    types.StringValue("cc_id2"),
			TokenIdentity: types.StringValue("token_id2"),
			IsRegex:       types.BoolValue(true),
		},
	}

	testApiOidcConfigResource(t, issuer, audience, jwks, claim, identityMap, false)
}

// TestIntegrationApiOidcConfigResource attempts to create, check, and destroy
// an API OIDC Config, but uses a mocked API service.
func TestIntegrationApiOidcConfigResource(t *testing.T) {
	id := uuid.Must(uuid.NewUUID())
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()
	issuer := "issuer"
	audience := "audience"
	claim := "claim"
	jwks := "{}"
	identityMap := []IdentityMapEntry{
		{
			CcIdentity:    types.StringValue("cc_id1"),
			TokenIdentity: types.StringValue("token_id1"),
			IsRegex:       types.BoolValue(false),
		},
		{
			CcIdentity:    types.StringValue("cc_id2"),
			TokenIdentity: types.StringValue("token_id2"),
			IsRegex:       types.BoolValue(true),
		},
	}
	response := client.ApiOidcConfig{
		Id:          id.String(),
		Issuer:      issuer,
		Audience:    audience,
		Jwks:        jwks,
		Claim:       &claim,
		IdentityMap: identityMapFromTerraformState(&identityMap),
	}

	s.EXPECT().GetApiOidcConfig(gomock.Any(), id.String()).
		Return(&response, nil, nil).AnyTimes()
	s.EXPECT().CreateApiOidcConfig(gomock.Any(), gomock.Any()).
		Return(&response, nil, nil)
	s.EXPECT().DeleteApiOidcConfig(gomock.Any(), id.String()).
		Return(&response, nil, nil)

	testApiOidcConfigResource(t, issuer, audience, jwks, claim, identityMap, true)
}

func testApiOidcConfigResource(t *testing.T, issuer, audience, jwks, claim string, identityMap []IdentityMapEntry, useMock bool) {
	var (
		resourceNameTest = "cockroach_api_oidc_config.test"
	)
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestApiOidcConfig(issuer, audience, jwks, claim, identityMap),
				Check: resource.ComposeTestCheckFunc(
					testApiOidcConfig(resourceNameTest, issuer, audience, jwks, claim, identityMap),
					resource.TestCheckResourceAttr(resourceNameTest, "issuer", issuer),
					resource.TestCheckResourceAttr(resourceNameTest, "audience", audience),
					resource.TestCheckResourceAttr(resourceNameTest, "jwks", jwks),
					resource.TestCheckResourceAttr(resourceNameTest, "claim", claim),
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

func testApiOidcConfig(
	resourceName, issuer, audience, jwks, claim string, identityMap []IdentityMapEntry,
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

		roleResp, _, err := p.service.GetApiOidcConfig(context.TODO(), rs.Primary.ID)
		if err == nil {
			respIdentityMap := *roleResp.IdentityMap
			if roleResp.Issuer == issuer && roleResp.Audience == audience && roleResp.Jwks == jwks && *roleResp.Claim == claim && *respIdentityMap[0].CcIdentity == identityMap[0].CcIdentity.ValueString() {
				return nil
			}
		}

		return fmt.Errorf("API OIDC Config does not have correct values")
	}
}

func getTestApiOidcConfig(issuer, audience, jwks, claim string, identityMap []IdentityMapEntry) string {
	identityMapString := "[\n"
	for _, identityMapEntry := range identityMap {
		identityMapString += "{\n"
		identityMapString += fmt.Sprintf("token_identity = %s\n", identityMapEntry.TokenIdentity)
		identityMapString += fmt.Sprintf("cc_identity = %s\n", identityMapEntry.CcIdentity)
		identityMapString += fmt.Sprintf("is_regex = %s\n", identityMapEntry.IsRegex)
		identityMapString += "},\n"
	}
	identityMapString += "]"
	return fmt.Sprintf(`
resource "cockroach_api_oidc_config" "test" {
  issuer = "%s"
  audience = "%s"
  jwks = "%s"
  claim = "%s"
  identity_map = %s
}
`, issuer, audience, jwks, claim, identityMapString)
}
