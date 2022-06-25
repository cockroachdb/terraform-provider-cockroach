terraform {
  required_providers {
    cockroach = {
      source = "registry.terraform.io/hashicorp/cockroach"
    }
  }
}
provider "cockroach" {
# export COCKROACH_API_KEY with the cockroach cloud API Key
}

resource "cockroach_cluster" "cockroach" {
    name           = "cockroach-serverless"
    cloud_provider = "GCP"
    wait_for_cluster_ready = true
    create_spec = {
    serverless = {
         regions = ["us-east1"]
         spend_limit = 1
    }
   }
#   update_spec = {
#     serverless = {
#       spend_limit = 1
#     }
#   }
}

resource "cockroach_sql_user" "cockroach" {
  name = "prafull"
  password = "prafull@123456"
  id = cockroach_cluster.cockroach.id
}

