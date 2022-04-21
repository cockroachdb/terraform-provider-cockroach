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
    name           = "cockroach-cluster"
    cloud_provider = "CLOUD_PROVIDER_GCP"
    spec = {
    serverless = {
        regions = ["us-east1"]
        spend_limit = 0
        }
    }
}

