package provider

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// TestAccFolderResource attempts to create, check, update, and destroy real
// folders. It will be skipped if TF_ACC isn't set.
func TestAccFolderResource(t *testing.T) {
	t.Parallel()
	var (
		folderName       = fmt.Sprintf("tftest-folder-%s", GenerateRandomString(4))
		newFolderName    = fmt.Sprintf("tftest-folder-%s", GenerateRandomString(4))
		parentFolderName = fmt.Sprintf("tftest-folder-%s", GenerateRandomString(4))
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
		folderName       = fmt.Sprintf("tftest-folder-%s", GenerateRandomString(4))
		newFolderName    = fmt.Sprintf("tftest-folder-%s", GenerateRandomString(4))
		parentFolderName = fmt.Sprintf("tftest-folder-%s", GenerateRandomString(4))

		parentFolderID = uuid.Nil.String()
		childFolderID  = uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")).String()
	)

	parentFolder := client.Folder{
		Id:   parentFolderID,
		Name: parentFolderName,
	}
	childFolder := client.Folder{
		Id:   childFolderID,
		Name: folderName,
	}
	renamedChildFolder := client.Folder{
		Id:   childFolderID,
		Name: newFolderName,
	}
	updatedChildFolder := client.Folder{
		Id:       childFolderID,
		Name:     newFolderName,
		ParentId: uuid.Nil.String(),
	}

	// Create
	s.EXPECT().CreateFolder(gomock.Any(), gomock.Any()).
		Return(&parentFolder, nil, nil)
	s.EXPECT().CreateFolder(gomock.Any(), gomock.Any()).
		Return(&childFolder, nil, nil)

	// Read
	s.EXPECT().GetFolder(gomock.Any(), childFolderID).
		Return(&childFolder, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(4)

	// Update
	s.EXPECT().
		RenameFolder(gomock.Any(), childFolderID, &client.RenameFolderRequest{NewName: newFolderName}).
		Return(&renamedChildFolder, nil, nil)
	s.EXPECT().GetFolder(gomock.Any(), childFolderID).
		Return(&renamedChildFolder, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(4)
	s.EXPECT().
		MoveFolder(
			gomock.Any(),
			childFolderID,
			&client.MoveFolderRequest{DestinationParentId: uuid.Nil.String()},
		).
		Return(&updatedChildFolder, nil, nil)
	s.EXPECT().GetFolder(gomock.Any(), childFolderID).
		Return(&updatedChildFolder, &http.Response{Status: http.StatusText(http.StatusOK)}, nil).
		Times(4)

	// Delete
	s.EXPECT().DeleteFolder(gomock.Any(), childFolderID)
	s.EXPECT().DeleteFolder(gomock.Any(), parentFolderID)

	testFolderResource(t, parentFolderName, folderName, newFolderName, true)
}

func testFolderResource(t *testing.T, parentFolderName, folderName, newFolderName string, useMock bool) {
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
				Config: getTestFolderResourceConfig(parentFolderName, newFolderName),
				Check: resource.ComposeTestCheckFunc(
					testCheckFolderExists(resourceName),
					testCheckFolderExists(resourceParentName),
					resource.TestCheckResourceAttr(resourceName, "name", newFolderName),
					resource.TestCheckResourceAttrSet(resourceName, "parent_id"),
				),
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
			"[DEBUG] folderID: %s, name %s",
			rs.Primary.Attributes["id"],
			rs.Primary.Attributes["name"],
		)

		if _, _, err := p.service.GetFolder(context.Background(), id); err == nil {
			return nil
		}

		return fmt.Errorf("folder %s does not exist", rs.Primary.ID)
	}
}

func getTestFolderResourceConfig(parentFolderName, folderName string) string {
	return fmt.Sprintf(`
resource "cockroach_folder" "test_folder_parent" {
    name = "%s"
}

resource "cockroach_folder" "test_folder_child" {
    name = "%s"
}
`, parentFolderName, folderName)
}
