variable "folder_parent_name" {
  type     = string
  nullable = false
}

variable "folder_child_name" {
  type     = string
  nullable = false
}

// Get a service account's user id by visiting its detail page. 
// The user id is in the url like "/service-accounts/{service account user id}".
variable "service_account_user_id" {
  type     = string
  nullable = false
}

variable "cluster_name" {
  type     = string
  nullable = false
}

variable "provisioned_capacity" {
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

resource "cockroach_folder" "example-folder-parent" {
  name      = var.folder_parent_name
  parent_id = "root"
}

resource "cockroach_folder" "example-folder-child" {
  name      = var.folder_child_name
  parent_id = cockroach_folder.example-folder-parent.id
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
      role_name     = "FOLDER_MOVER",
      resource_type = "ORGANIZATION",
      resource_id   = ""
    },
    {
      role_name     = "FOLDER_ADMIN",
      resource_type = "FOLDER",
      resource_id   = cockroach_folder.example-folder-parent.id
    },
    {
      role_name     = "CLUSTER_CREATOR",
      resource_type = "FOLDER",
      resource_id   = cockroach_folder.example-folder-parent.id
    },
  ]
}

resource "cockroach_cluster" "example" {
  name           = var.cluster_name
  cloud_provider = var.cloud_provider
  serverless = {
    usage_limits = {
      provisioned_capacity = var.provisioned_capacity
    }
  }
  regions   = [for r in var.cloud_provider_regions : { name = r }]
  parent_id = cockroach_folder.example-folder-parent.id
}
