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
	"fmt"
	"os"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v2/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestIntegrationPersonUserDataSource(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	email := "test@cockroachlabs.com"

	s.EXPECT().GetPersonUsersByEmail(gomock.Any(), &email).
		Return(&client.GetPersonUsersByEmailResponse{
			User: &client.PersonUserInfo{
				Email: &email,
				Id:    "test-id",
			},
		}, nil, nil).
		Times(3)

	testPersonUserDataSource(t, email, true)
}

func testPersonUserDataSource(t *testing.T, email string, useMock bool) {
	dataSourceName := "data.cockroach_person_user.test"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestPersonUserDataSourceConfig(email),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(dataSourceName, "id"),
					resource.TestCheckResourceAttr(dataSourceName, "email", email),
				),
			},
		},
	})
}

func getTestPersonUserDataSourceConfig(email string) string {
	return fmt.Sprintf(
		`data "cockroach_person_user" "test" {
		email = "%s"
	}`, email)
}
