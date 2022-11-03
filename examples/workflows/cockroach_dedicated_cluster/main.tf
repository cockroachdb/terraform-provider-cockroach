variable "cluster_name" {
  type     = string
  nullable = false
}

variable "sql_user_name" {
  type     = string
  nullable = false
  default  = "maxroach"
}

variable "sql_user_password" {
  type      = string
  nullable  = false
  sensitive = true
}

variable "cloud_provider" {
  type     = string
  nullable = false
  default  = "GCP"
}

variable "cloud_provider_region" {
  type     = list(string)
  nullable = false
  default  = ["us-east1"]
}

variable "cluster_nodes" {
  type     = number
  nullable = false
  default  = 1
}

variable "storage_gib" {
  type     = number
  nullable = false
  default  = 15
}

variable "machine_type" {
  type     = string
  nullable = false
  default  = "n1-standard-2"
}

variable "allow_list_name" {
  type     = string
  nullable = false
  default  = "default-allow-list"
}

variable "cidr_ip" {
  type     = string
  nullable = false
}

variable "cidr_mask" {
  type     = number
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

resource "cockroach_cluster" "cockroach" {
  name           = var.cluster_name
  cloud_provider = var.cloud_provider
  dedicated = {
    storage_gib  = var.storage_gib
    machine_type = var.machine_type
  }
  regions = [
    for r in var.cloud_provider_region : {
      name       = r,
      node_count = var.cluster_nodes
    }
  ]
}

resource "cockroach_allow_list" "cockroach" {
  name       = var.allow_list_name
  cidr_ip    = var.cidr_ip
  cidr_mask  = var.cidr_mask
  ui         = true
  sql        = true
  cluster_id = cockroach_cluster.cockroach.id
}

resource "cockroach_sql_user" "cockroach" {
  name       = var.sql_user_name
  password   = var.sql_user_password
  cluster_id = cockroach_cluster.cockroach.id
}

data "cockroach_cluster" "cockroach" {
  id = cockroach_cluster.cockroach.id
}

output "cluster" {
  value = data.cockroach_cluster.cockroach
}
