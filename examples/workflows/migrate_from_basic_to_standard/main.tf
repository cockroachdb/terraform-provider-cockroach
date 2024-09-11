#
# This example shows how to migrate plans from Basic to Standard
# Migrating involves first creating a Basic cluster resource (Step 1)
# and then modifying that cluster resource to make it a Standard cluster (Step 2).
# To complete this example you'll first run `terraform apply` on the Step 1 config (original state of this file)
# You'll then modify this file as indicated below, removing the Basic resource and uncommented the Standard resource
# And then you'll re-run `terraform apply` to arrive at the end state: a Standard plan cluster
#

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
