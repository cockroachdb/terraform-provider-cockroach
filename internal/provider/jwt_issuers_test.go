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
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v6/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testIssuerURL = "https://issuer-%s.com"

	testAudience        = "audience"
	testAudienceUpdated = "audience_updated"

	testJWKS        = "{}"
	testJWKSUpdated = "{updated: 1}"

	testClaim        = "sub"
	testClaimUpdated = "email"

	testTokenID1 = "token_id1"
	testCCID1    = "cc_id1"

	testTokenID2 = "token_id2"
	testCCID2    = "cc_id2"

	testIdentityMap = []IdentityMapEntry{
		{
			TokenIdentity: types.StringValue(testTokenID1),
			CcIdentity:    types.StringValue(testCCID1),
		},
		{
			TokenIdentity: types.StringValue(testTokenID2),
			CcIdentity:    types.StringValue(testCCID2),
		},
	}
	testIdentityMapUpdated = []IdentityMapEntry{
		{
			TokenIdentity: types.StringValue(testTokenID1),
			CcIdentity:    types.StringValue(testCCID1),
		},
	}

	issuerUUIDRegex = "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
)

// TestAccJWTIssuerResource attempts to create, check, and destroy
// a real JWT Issuer. It will be skipped if TF_ACC isn't set.
// In order to work, the `JwtIssuerEnabled` Feature Flag must be enabled and
// the test org must have Org SSO enabled (no need for SAML/OIDC).
func TestAccJWTIssuerResource(t *testing.T) {
	t.Parallel()

	issuerURL := fmt.Sprintf(testIssuerURL, GenerateRandomString(4))
	testJWTIssuerResource(t, issuerURL, false)
}

// TestIntegrationJWTIssuerResource attempts to create, check, and destroy
// a JWT issuer resource, but uses a mocked API service.
func TestIntegrationJWTIssuerResource(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		require.NoError(t, os.Setenv(CockroachAPIKey, "fake"))
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	id := uuid.Must(uuid.NewUUID())
	issuerURL := fmt.Sprintf(testIssuerURL, GenerateRandomString(4))
	jwtIssuer := &client.JWTIssuer{
		Id:          id.String(),
		IssuerUrl:   issuerURL,
		Audience:    testAudience,
		Jwks:        &testJWKS,
		Claim:       &testClaim,
		IdentityMap: identityMapFromTerraformState(&testIdentityMap),
	}
	jwtIssuerUpdated := &client.JWTIssuer{
		Id:          id.String(),
		IssuerUrl:   issuerURL,
		Audience:    testAudienceUpdated,
		Jwks:        &testJWKSUpdated,
		Claim:       &testClaimUpdated,
		IdentityMap: identityMapFromTerraformState(&testIdentityMapUpdated),
	}

	// Create
	s.EXPECT().AddJWTIssuer(gomock.Any(), gomock.Any()).
		Return(jwtIssuer, nil, nil)
	s.EXPECT().GetJWTIssuer(gomock.Any(), id.String()).
		Return(jwtIssuer, nil, nil).Times(3)

	// Update
	s.EXPECT().UpdateJWTIssuer(gomock.Any(), id.String(), gomock.Any()).
		Return(jwtIssuerUpdated, nil, nil)
	s.EXPECT().GetJWTIssuer(gomock.Any(), id.String()).
		Return(jwtIssuerUpdated, nil, nil).Times(3)

	// Delete
	s.EXPECT().DeleteJWTIssuer(gomock.Any(), id.String()).
		Return(jwtIssuerUpdated, nil, nil)
	s.EXPECT().GetJWTIssuer(gomock.Any(), id.String()).
		Return(nil, &http.Response{StatusCode: http.StatusNotFound}, errors.New("not found"))

	testJWTIssuerResource(t, issuerURL, true)
}

func testJWTIssuerResource(
	t *testing.T, issuerURL string, useMock bool,
) {
	resourceNameTest := "cockroach_jwt_issuer.test"
	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestJWTIssuerCreateConfig(issuerURL),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						resourceNameTest,
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						resourceNameTest,
						tfjsonpath.New("id"),
						knownvalue.StringRegexp(regexp.MustCompile(issuerUUIDRegex)),
					),
				},
				Check: resource.ComposeTestCheckFunc(
					testJWTIssuer(resourceNameTest, issuerURL),
					resource.TestCheckResourceAttr(resourceNameTest, "issuer_url", issuerURL),
					resource.TestCheckResourceAttr(resourceNameTest, "audience", testAudience),
					resource.TestCheckResourceAttr(resourceNameTest, "jwks", testJWKS),
					resource.TestCheckResourceAttr(resourceNameTest, "claim", testClaim),
					resource.TestCheckResourceAttr(resourceNameTest, "identity_map.#", "2"),
					resource.TestCheckResourceAttr(resourceNameTest, "identity_map.0.token_identity", testIdentityMap[0].TokenIdentity.ValueString()),
					resource.TestCheckResourceAttr(resourceNameTest, "identity_map.0.cc_identity", testIdentityMap[0].CcIdentity.ValueString()),
					resource.TestCheckResourceAttr(resourceNameTest, "identity_map.1.token_identity", testIdentityMap[1].TokenIdentity.ValueString()),
					resource.TestCheckResourceAttr(resourceNameTest, "identity_map.1.cc_identity", testIdentityMap[1].CcIdentity.ValueString()),
				),
			},
			{
				Config: getTestJWTIssuerUpdateConfig(issuerURL),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						resourceNameTest,
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						resourceNameTest,
						tfjsonpath.New("id"),
						knownvalue.StringRegexp(regexp.MustCompile(issuerUUIDRegex)),
					),
				},
				Check: resource.ComposeTestCheckFunc(
					testJWTIssuerUpdated(resourceNameTest, issuerURL),
					resource.TestCheckResourceAttr(resourceNameTest, "issuer_url", issuerURL),
					resource.TestCheckResourceAttr(resourceNameTest, "audience", testAudienceUpdated),
					resource.TestCheckResourceAttr(resourceNameTest, "jwks", testJWKSUpdated),
					resource.TestCheckResourceAttr(resourceNameTest, "claim", testClaimUpdated),
					resource.TestCheckResourceAttr(resourceNameTest, "identity_map.#", "1"),
					resource.TestCheckResourceAttr(resourceNameTest, "identity_map.0.token_identity", testIdentityMap[0].TokenIdentity.ValueString()),
					resource.TestCheckResourceAttr(resourceNameTest, "identity_map.0.cc_identity", testIdentityMap[0].CcIdentity.ValueString()),
				),
			},
			{
				ResourceName:      resourceNameTest,
				ImportState:       true,
				ImportStateVerify: true,
				Destroy:           true,
			},
		},
		CheckDestroy: testJWTIssuerDestroy(resourceNameTest),
	})
}

func getTestJWTIssuerCreateConfig(issuerURL string) string {
	identityMapString := "[\n"
	for _, identityMapEntry := range testIdentityMap {
		identityMapString += "{\n"
		identityMapString += fmt.Sprintf("token_identity = %s\n", identityMapEntry.TokenIdentity)
		identityMapString += fmt.Sprintf("cc_identity = %s\n", identityMapEntry.CcIdentity)
		identityMapString += "},\n"
	}
	identityMapString += "]"

	return fmt.Sprintf(`
resource "cockroach_jwt_issuer" "test" {
  issuer_url = "%s"
  audience = "%s"
  jwks = "%s"
  claim = "%s"
  identity_map = %s
}
`, issuerURL, testAudience, testJWKS, testClaim, identityMapString)
}

func getTestJWTIssuerUpdateConfig(issuerURL string) string {
	identityMapString := "[\n"
	for _, identityMapEntry := range testIdentityMapUpdated {
		identityMapString += "{\n"
		identityMapString += fmt.Sprintf("token_identity = %s\n", identityMapEntry.TokenIdentity)
		identityMapString += fmt.Sprintf("cc_identity = %s\n", identityMapEntry.CcIdentity)
		identityMapString += "},\n"
	}
	identityMapString += "]"

	return fmt.Sprintf(`
resource "cockroach_jwt_issuer" "test" {
  issuer_url = "%s"
  audience = "%s"
  jwks = "%s"
  claim = "%s"
  identity_map = %s
}
`, issuerURL, testAudienceUpdated, testJWKSUpdated, testClaimUpdated, identityMapString)
}

func testJWTIssuer(resourceName, issuerURL string) resource.TestCheckFunc {
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

		expectedResp := &client.JWTIssuer{
			IssuerUrl: issuerURL,
			Audience:  testAudience,
			Jwks:      &testJWKS,
			Claim:     &testClaim,
			IdentityMap: &[]client.JWTIssuerIdentityMapEntry{
				{
					TokenIdentity: testIdentityMap[0].TokenIdentity.ValueString(),
					CcIdentity:    testIdentityMap[0].CcIdentity.ValueString(),
				},
				{
					TokenIdentity: testIdentityMap[1].TokenIdentity.ValueString(),
					CcIdentity:    testIdentityMap[1].CcIdentity.ValueString(),
				},
			},
		}

		traceAPICall("GetJWTIssuer")
		jwtIssuerResp, _, err := p.service.GetJWTIssuer(context.TODO(), rs.Primary.ID)
		if err == nil {
			// Copy over the resource ID as it is dynamic in nature.
			expectedResp.Id = jwtIssuerResp.Id
			if assert.ObjectsAreEqual(expectedResp, jwtIssuerResp) {
				return nil
			}
		}

		return fmt.Errorf("JWT Issuer configuration mismatch, expected %+v, got %+v",
			expectedResp, jwtIssuerResp)
	}
}

func testJWTIssuerUpdated(resourceName, issuerURL string) resource.TestCheckFunc {
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

		expectedResp := &client.JWTIssuer{
			IssuerUrl: issuerURL,
			Audience:  testAudienceUpdated,
			Jwks:      &testJWKSUpdated,
			Claim:     &testClaimUpdated,
			IdentityMap: &[]client.JWTIssuerIdentityMapEntry{
				{
					TokenIdentity: testIdentityMapUpdated[0].TokenIdentity.ValueString(),
					CcIdentity:    testIdentityMapUpdated[0].CcIdentity.ValueString(),
				},
			},
		}

		traceAPICall("GetJWTIssuer")
		jwtIssuerResp, _, err := p.service.GetJWTIssuer(context.TODO(), rs.Primary.ID)
		if err == nil {
			// Copy over the resource ID as it is dynamic in nature.
			expectedResp.Id = jwtIssuerResp.Id
			if assert.ObjectsAreEqual(expectedResp, jwtIssuerResp) {
				return nil
			}
		}

		return fmt.Errorf("JWT Issuer configuration mismatch, expected %+v, got %+v",
			expectedResp, jwtIssuerResp)
	}
}

func testJWTIssuerDestroy(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) (err error) {
		p := testAccProvider.(*provider)
		p.service = NewService(cl)

		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		traceAPICall("GetJWTIssuer")
		_, httpResponse, err := p.service.GetJWTIssuer(context.TODO(), rs.Primary.ID)
		if err != nil && httpResponse != nil && httpResponse.StatusCode == http.StatusNotFound {
			return nil
		}

		var statusCode int
		if httpResponse != nil {
			statusCode = httpResponse.StatusCode
		}
		return fmt.Errorf("JWT Issuer not destroyed, HTTP response code: %d, error: %v", statusCode, err)
	}
}
