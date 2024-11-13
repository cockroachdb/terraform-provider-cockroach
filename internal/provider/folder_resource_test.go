package provider

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v5/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccFolderResource attempts to create, check, update, and destroy real
// folders. It will be skipped if TF_ACC isn't set.
func TestAccFolderResource(t *testing.T) {
	t.Parallel()
	var (
		folderName       = fmt.Sprintf("%s-folder-%s", tfTestPrefix, GenerateRandomString(4))
		newFolderName    = fmt.Sprintf("%s-folder-%s", tfTestPrefix, GenerateRandomString(4))
		parentFolderName = fmt.Sprintf("%s-folder-%s", tfTestPrefix, GenerateRandomString(4))
	)
	testFolderResource(t, parentFolderName, folderName, newFolderName, false)
}

// TestIntegrationFolderResource attempts to create, check, and destroy real
// folders, but uses a mocked API service.
func TestIntegrationFolderResource(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	var (
		folderName       = "child-folder"
		newFolderName    = "child-folder-renamed"
		parentFolderName = "parent-folder"

		rootParentID   = "root"
		parentFolderID = uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")).String()
		childFolderID  = uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000002")).String()
	)

	parentFolder := client.FolderResource{
		ResourceId: parentFolderID,
		Name:       parentFolderName,
		ParentId:   rootParentID,
	}
	childFolder := client.FolderResource{
		ResourceId: childFolderID,
		Name:       folderName,
		ParentId:   parentFolderID,
	}
	updatedChildFolder := client.FolderResource{
		ResourceId: childFolderID,
		Name:       newFolderName,
		ParentId:   rootParentID,
	}

	httpOkResponse := &http.Response{Status: http.StatusText(http.StatusOK)}

	// Create
	s.EXPECT().CreateFolder(gomock.Any(),
		&client.CreateFolderRequest{Name: parentFolderName, ParentId: &rootParentID}).
		Return(&parentFolder, httpOkResponse, nil)
	s.EXPECT().CreateFolder(gomock.Any(),
		&client.CreateFolderRequest{Name: folderName, ParentId: &parentFolderID}).
		Return(&childFolder, httpOkResponse, nil)

	// Read
	s.EXPECT().GetFolder(gomock.Any(), parentFolderID).
		Return(&parentFolder, httpOkResponse, nil).
		Times(2)
	s.EXPECT().GetFolder(gomock.Any(), childFolderID).
		Return(&childFolder, httpOkResponse, nil).
		Times(2)

	// Update
	s.EXPECT().GetFolder(gomock.Any(), parentFolderID).
		Return(&parentFolder, httpOkResponse, nil).
		Times(1)
	s.EXPECT().GetFolder(gomock.Any(), childFolderID).
		Return(&childFolder, httpOkResponse, nil).
		Times(1)
	s.EXPECT().UpdateFolder(gomock.Any(), childFolderID,
		&client.UpdateFolderSpecification{Name: &newFolderName, ParentId: &rootParentID}).
		Return(&updatedChildFolder, nil, nil)
	s.EXPECT().GetFolder(gomock.Any(), parentFolderID).
		Return(&parentFolder, httpOkResponse, nil).
		Times(2)
	s.EXPECT().GetFolder(gomock.Any(), childFolderID).
		Return(&updatedChildFolder, httpOkResponse, nil).
		Times(2)

	// Import
	s.EXPECT().GetFolder(gomock.Any(), childFolderID).
		Return(&updatedChildFolder, httpOkResponse, nil).
		Times(1)

	// Delete
	s.EXPECT().DeleteFolder(gomock.Any(), childFolderID).
		Return(httpOkResponse, nil)
	s.EXPECT().DeleteFolder(gomock.Any(), parentFolderID).
		Return(httpOkResponse, nil)

	testFolderResource(t, parentFolderName, folderName, newFolderName, true)
}

func testFolderResource(
	t *testing.T, parentFolderName, folderName, newFolderName string, useMock bool,
) {
	var (
		resourceName       = "cockroach_folder.test_folder_child"
		resourceParentName = "cockroach_folder.test_folder_parent"
	)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               useMock,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: getTestFolderResourceConfig(parentFolderName, folderName),
				Check: resource.ComposeTestCheckFunc(
					testCheckFolderExists(resourceName),
					testCheckFolderExists(resourceParentName),
					resource.TestCheckResourceAttr(resourceName, "name", folderName),
					resource.TestCheckResourceAttrSet(resourceName, "parent_id"),
				),
			},
			{
				Config: getTestFolderResourceUpdateConfig(parentFolderName, newFolderName),
				Check: resource.ComposeTestCheckFunc(
					testCheckFolderExists(resourceName),
					testCheckFolderExists(resourceParentName),
					resource.TestCheckResourceAttr(resourceName, "name", newFolderName),
					resource.TestCheckResourceAttr(resourceName, "parent_id", "root"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testCheckFolderExists(resourceName string) resource.TestCheckFunc {
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

		id := rs.Primary.Attributes["id"]
		log.Printf(
			"[DEBUG] folderID: %s, name %s, parentID: %s",
			rs.Primary.Attributes["id"],
			rs.Primary.Attributes["name"],
			rs.Primary.Attributes["parent_id"],
		)

		traceAPICall("GetFolder")
		if _, _, err := p.service.GetFolder(context.Background(), id); err == nil {
			return nil
		}

		return fmt.Errorf("folder %s does not exist", rs.Primary.ID)
	}
}

func getTestFolderResourceConfig(parentFolderName, childFolderName string) string {
	return fmt.Sprintf(`
resource "cockroach_folder" "test_folder_parent" {
    name = "%s"
	parent_id = "root"
}

resource "cockroach_folder" "test_folder_child" {
    name = "%s"
	parent_id = cockroach_folder.test_folder_parent.id
}
`, parentFolderName, childFolderName)
}

func getTestFolderResourceUpdateConfig(parentFolderName, childFolderName string) string {
	return fmt.Sprintf(`
resource "cockroach_folder" "test_folder_parent" {
    name = "%s"
	parent_id = "root"
}

resource "cockroach_folder" "test_folder_child" {
    name = "%s"
	parent_id = "root"
}
`, parentFolderName, childFolderName)
}
