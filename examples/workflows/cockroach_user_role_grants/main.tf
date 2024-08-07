variable "cluster_name" {
  type     = string
  nullable = false
}

variable "provisioned_virtual_cpus" {
  type     = number
  nullable = false
  default  = 2
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

variable "person_email" {
  type     = string
  nullable = false
}

// Get a service account's user id by visiting its detail page. 
// The user id is in the url like "/service-accounts/{service account user id}".
variable "service_account_user_id" {
  type     = string
  nullable = false
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

resource "cockroach_cluster" "example" {
  name           = var.cluster_name
  cloud_provider = var.cloud_provider
  plan           = "STANDARD"
  serverless = {
    usage_limits = {
      provisioned_virtual_cpus = var.provisioned_virtual_cpus
    }
  }
  regions = [for r in var.cloud_provider_regions : { name = r }]
}

data "cockroach_person_user" "example" {
  email = var.person_email
}

resource "cockroach_user_role_grants" "example-person" {
  user_id = data.cockroach_person_user.example.id
  roles = [
    {
      role_name     = "ORG_MEMBER",
      resource_type = "ORGANIZATION",
      resource_id   = ""
    },
    {
      role_name     = "ORG_ADMIN",
      resource_type = "ORGANIZATION",
      resource_id   = ""
    },
    {
      role_name     = "CLUSTER_ADMIN",
      resource_type = "CLUSTER",
      resource_id   = cockroach_cluster.example.id
    },
  ]
}

resource "cockroach_user_role_grants" "example-service-account" {
  user_id = var.service_account_user_id
  roles = [
    {
      role_name     = "ORG_MEMBER",
      resource_type = "ORGANIZATION",
      resource_id   = ""
    },
    {
      role_name     = "CLUSTER_DEVELOPER",
      resource_type = "CLUSTER",
      resource_id   = cockroach_cluster.example.id
    },
  ]
}
