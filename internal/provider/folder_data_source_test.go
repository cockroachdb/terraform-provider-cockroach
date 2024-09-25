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
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v4/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccFolderDataResource attempts to create 2 folders and then access them
// as data sources.
// It will be skipped if TF_ACC isn't set.
func TestAccFolderDataSource(t *testing.T) {
	t.Parallel()
	parentName := fmt.Sprintf("%s-parent-%s", tfTestPrefix, GenerateRandomString(4))
	childName := fmt.Sprintf("%s-child-%s", tfTestPrefix, GenerateRandomString(4))

	testFolderDataSource(t, parentName, childName, false)
}

func TestIntegrationFolderDataSource(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	parentName := fmt.Sprintf("%s-parent-%s", tfTestPrefix, GenerateRandomString(4))
	childName := fmt.Sprintf("%s-child-%s", tfTestPrefix, GenerateRandomString(4))

	rootID := "root"
	parentID := uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")).String()
	childID := uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000002")).String()
	childPath := fmt.Sprintf("/%s/%s", parentName, childName)

	parentFolder := client.FolderResource{
		ResourceId: parentID,
		Name:       parentName,
		ParentId:   rootID,
	}
	childFolder := client.FolderResource{
		ResourceId: childID,
		Name:       childName,
		ParentId:   parentID,
	}

	httpOkResponse := &http.Response{Status: http.StatusText(http.StatusOK)}

	// Create
	s.EXPECT().CreateFolder(gomock.Any(),
		&client.CreateFolderRequest{Name: parentName, ParentId: &rootID}).
		Return(&parentFolder, httpOkResponse, nil)
	s.EXPECT().CreateFolder(gomock.Any(),
		&client.CreateFolderRequest{Name: childName, ParentId: &parentID}).
		Return(&childFolder, httpOkResponse, nil)

	// Data Source Read
	s.EXPECT().ListFolders(gomock.Any(), &client.ListFoldersOptions{Path: &childPath}).
		Return(&client.ListFoldersResponse{Folders: []client.FolderResource{childFolder}}, httpOkResponse, nil).
		Times(3)
	s.EXPECT().GetFolder(gomock.Any(), parentID).
		Return(&parentFolder, httpOkResponse, nil).
		Times(3)

	// Pre Delete Resource Read
	s.EXPECT().GetFolder(gomock.Any(), parentID).
		Return(&parentFolder, httpOkResponse, nil).
		Times(1)
	s.EXPECT().GetFolder(gomock.Any(), childID).
		Return(&childFolder, httpOkResponse, nil).
		Times(1)

	// Delete
	s.EXPECT().DeleteFolder(gomock.Any(), childID).
		Return(httpOkResponse, nil)
	s.EXPECT().DeleteFolder(gomock.Any(), parentID).
		Return(httpOkResponse, nil)

	testFolderDataSource(t, parentName, childName, true)
}

func testFolderDataSource(t *testing.T, parentName, childName string, useMock bool) {
	parentDataSourceName := "data.cockroach_folder.parent_by_id"
	childDataSourceName := "data.cockroach_folder.child_by_path"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestFolderDataSourceConfig(parentName, childName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(parentDataSourceName, "name", parentName),
					resource.TestCheckResourceAttr(parentDataSourceName, "path", fmt.Sprintf("/%s", parentName)),
					resource.TestCheckResourceAttrSet(parentDataSourceName, "id"),
					resource.TestCheckResourceAttrSet(parentDataSourceName, "parent_id"),

					resource.TestCheckResourceAttr(childDataSourceName, "name", childName),
					resource.TestCheckResourceAttr(childDataSourceName, "path", fmt.Sprintf("/%s/%s", parentName, childName)),
					resource.TestCheckResourceAttrSet(childDataSourceName, "id"),
					resource.TestCheckResourceAttrSet(childDataSourceName, "parent_id"),
				),
			},
		},
	})
}

func getTestFolderDataSourceConfig(parentName, childName string) string {
	return fmt.Sprintf(`
resource "cockroach_folder" "test_folder_parent" {
	name      = "%[1]s"
	parent_id = "root"
}

resource "cockroach_folder" "test_folder_child" {
	name      = "%[2]s"
	parent_id = cockroach_folder.test_folder_parent.id
}

data "cockroach_folder" "child_by_path" {
	path = "/%[1]s/%[2]s"

	# ensure the test folder is created before we try to pull it back by path.
	depends_on = [cockroach_folder.test_folder_child]
}

data "cockroach_folder" "parent_by_id" {
	id = cockroach_folder.test_folder_parent.id
}
`, parentName, childName)
}
