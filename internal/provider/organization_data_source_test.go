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
	"os"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v4/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccOrganizationDataSource attempts to read organzation info for
// the current API key. It will be skipped if TF_ACC isn't set.
func TestAccOrganizationDataSource(t *testing.T) {
	t.Parallel()

	testOrganizationDataSource(t, false)
}

func TestIntegrationOrganizationDataSource(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().GetOrganizationInfo(gomock.Any()).
		Return(&client.Organization{
			CreatedAt: time.Now(),
			Id:        "org-id",
			Label:     "org-label",
			Name:      "org-name",
		}, nil, nil).
		Times(3)

	testOrganizationDataSource(t, true)
}

func testOrganizationDataSource(t *testing.T, useMock bool) {
	orgDataSourceName := "data.cockroach_organization.test"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestOrganizationDataSourceConfig(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(orgDataSourceName, "id"),
					resource.TestCheckResourceAttrSet(orgDataSourceName, "label"),
					resource.TestCheckResourceAttrSet(orgDataSourceName, "name"),
					resource.TestCheckResourceAttrSet(orgDataSourceName, "created_at"),
				),
			},
		},
	})
}

func getTestOrganizationDataSourceConfig() string {
	return `data "cockroach_organization" "test" {}`
}
