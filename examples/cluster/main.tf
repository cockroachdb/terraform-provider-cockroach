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

variable "region_names" {
  type    = list(string)
  default = ["us-east1"]
}

resource "cockroach_cluster" "cockroach" {
    name           = "cockroach-serverless"
    cloud_provider = "GCP"
    create_spec = {
    serverless = {
         regions = ["us-east1"]
         spend_limit = 1
    }
#    dedicated: {
#      region_nodes = {
#        "ap-south-1": 3
#      }
#      hardware = {
#        storage_gib = 15
#        machine_spec = {
#          machine_type = "m5.large"
#        }
#      }
#    }
   }
}

