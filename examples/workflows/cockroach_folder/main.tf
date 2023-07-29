variable "folder_parent_name" {
  type     = string
  nullable = false
}

variable "folder_child_name" {
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

resource "cockroach_folder" "example-folder-parent" {
  name      = var.folder_parent_name
  parent_id = "root"
}

resource "cockroach_folder" "example-folder-child" {
  name      = var.folder_child_name
  parent_id = cockroach_folder.example-folder-parent.id
}
