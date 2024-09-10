variable "cluster_name" {
  type     = string
  nullable = false
}

variable "cloud_provider" {
  type     = string
  nullable = false
  default  = "GCP"
}

variable "cloud_provider_regions" {
  type     = list(string)
  nullable = false
  default  = ["us-central1"]
}

terraform {
  required_providers {
    cockroach = {
      source = "cockroachdb/cockroach"
    }
  }
}
provider "cockroach" {
  # export COCKROACH_API_KEY with the cockroach cloud API Key
}

#
# Step 1 - Remove this cluster resource to migrate from Basic to Standard
#
resource "cockroach_cluster" "example" {
  name           = var.cluster_name
  cloud_provider = var.cloud_provider
  plan           = "BASIC"
  serverless     = {}
  regions        = [for r in var.cloud_provider_regions : { name = r }]
}

#
# Step 2 - Uncomment this cluster resource to migrate from Basic to Standard
#
#resource "cockroach_cluster" "example" {
#  name           = var.cluster_name
#  cloud_provider = var.cloud_provider
#  plan           = "STANDARD"
#  serverless      = {
#    usage_limits = {
#      provisioned_vcpus = 2
#    }
#  }
#  regions        = [for r in var.cloud_provider_regions : { name = r }]
#}
