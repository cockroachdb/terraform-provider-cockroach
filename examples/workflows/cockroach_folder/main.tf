variable "folder_parent_name" {
  type     = string
  nullable = false
}

variable "folder_child_name" {
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
  default  = 1000
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

resource "cockroach_folder" "example_folder_parent" {
  name      = var.folder_parent_name
  parent_id = "root"
}

resource "cockroach_folder" "example_folder_child" {
  name      = var.folder_child_name
  parent_id = cockroach_folder.example_folder_parent.id
}

resource "cockroach_user_role_grant" "folder_admin_grant" {
  user_id = cockroach_cluster.example.creator_id
  role = {
    role_name     = "FOLDER_ADMIN",
    resource_type = "FOLDER",
    resource_id   = cockroach_folder.example_folder_parent.id
  }
}

resource "cockroach_user_role_grant" "cluster_creator_grant" {
  user_id = cockroach_cluster.example.creator_id
  role = {
    role_name     = "CLUSTER_CREATOR",
    resource_type = "FOLDER",
    resource_id   = cockroach_folder.example_folder_parent.id
  }
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
  parent_id = cockroach_folder.example_folder_parent.id
}

data "cockroach_folder" "child_folder_by_path" {
  path = format("/%s/%s", cockroach_folder.example_folder_parent.name, cockroach_folder.example_folder_child.name)
}

output "child_folder_computed_path" {
  value = data.cockroach_folder.child_folder_by_path.path
}
