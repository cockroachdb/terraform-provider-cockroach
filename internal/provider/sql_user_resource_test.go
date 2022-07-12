package provider

import (
	"context"
	"fmt"
	"log"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccSqlUserResource(t *testing.T) {
	t.Parallel()
	var (
		clusterResourceName = "cockroach_cluster.serverless"
		resourceName        = "cockroach_sql_user.sqluser"
		sqlUserName         = "cockroach-user"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSqlUserResource(sqlUserName, "cockroach@123456"),
				Check: resource.ComposeTestCheckFunc(
					testAccSqlUserExists(resourceName, clusterResourceName),
					resource.TestCheckResourceAttr(resourceName, "name", sqlUserName),
				),
			},
		},
	})
}

func testAccSqlUserExists(resourceName, clusterResourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		var listUserOptions client.ListSQLUsersOptions
		p, _ := convertProviderType(testAccProvider)
		p.service = client.NewService(cl)

		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}
		clusterRs, ok := s.RootModule().Resources[clusterResourceName]
		if !ok {
			return fmt.Errorf("not found: %s", clusterResourceName)
		}

		if rs.Primary.ID == "" && clusterRs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		id := clusterRs.Primary.Attributes["id"]
		log.Printf("[DEBUG] clusterID: %s, name %s", clusterRs.Primary.Attributes["id"], clusterRs.Primary.Attributes["name"])

		if clusterResp, _, err := p.service.ListSQLUsers(context.TODO(), id, &listUserOptions); err == nil {
			for _, user := range clusterResp.Users {
				if user.GetName() == rs.Primary.Attributes["name"] {
					return nil
				}
			}
		}

		return fmt.Errorf("cluster(%s:%s) does not exist", rs.Primary.Attributes["id"], rs.Primary.ID)
	}
}

func testAccSqlUserResource(name, password string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "serverless" {
    name           = "cockroach-sql-user"
    cloud_provider = "GCP"
    wait_for_cluster_ready = true
    create_spec = {
    serverless = {
         regions = ["us-east1"]
         spend_limit = 1
    }
   }
}

resource "cockroach_sql_user" "sqluser" {
  name = "%s"
  password = "%s"
  id = cockroach_cluster.serverless.id
}
`, name, password)
}
