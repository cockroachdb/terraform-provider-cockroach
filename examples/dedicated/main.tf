variable "cluster_name" {
  type = string
  nullable = false
}

variable "sql_user_name" {
  type = string
  nullable = false
  default = "maxroach"
}

variable "sql_user_password" {
  type = string
  nullable = false
  sensitive = true
}

variable "cloud_provider" {
  type = string
  nullable = false
  default = "GCP"
}

variable "cloud_provider_region" {
  type = list(string)
  nullable = false
  default = ["us-east1"]
}

variable "cluster_nodes" {
  type = number
  nullable = false
  default = 1
}

variable "storage_gib" {
  type = number
  nullable = false
  default = 15
}

variable "machine_type" {
  type = string
  nullable = false
  default = "n1-standard-2"
}

variable allow_list_name {
  type = string
  nullable = false
  default = "default-allow-list"
}

variable cidr_ip {
  type = string
  nullable = false
}

variable cidr_mask {
  type = number
  nullable = false
}

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
  name           = var.cluster_name
  cloud_provider = var.cloud_provider
  wait_for_cluster_ready = true
  create_spec = {
    dedicated: {
      region_nodes = merge(
        {
          for k in var.cloud_provider_region: k => "${var.cluster_nodes}"
        }
      )
      hardware = {
        storage_gib = var.storage_gib
        machine_spec = {
          machine_type = var.machine_type
        }
      }
    }
  }
}

resource "cockroach_allow_list" "cockroach" {
  name = var.allow_list_name
  cidr_ip = var.cidr_ip
  cidr_mask = var.cidr_mask
  ui = true
  sql = true
  id = cockroach_cluster.cockroach.id
}

resource "cockroach_sql_user" "cockroach" {
  name = var.sql_user_name
  password = var.sql_user_password
  id = cockroach_cluster.cockroach.id
}

