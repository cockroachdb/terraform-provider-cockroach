variable "cluster_name" {
  type     = string
  nullable = false
}

variable "sql_user_name" {
  type     = string
  nullable = false
  default  = "maxroach"
}

# Remember that even variables marked sensitive will show up
# in the Terraform state file. Always follow best practices
# when managing sensitive info.
# https://developer.hashicorp.com/terraform/tutorials/configuration-language/sensitive-variables#sensitive-values-in-state
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

variable "cloud_provider_regions" {
  type     = list(string)
  nullable = false
  default  = ["us-east1"]
}

variable "cluster_node_count" {
  type     = number
  nullable = false
  default  = 1
}

variable "storage_gib" {
  type     = number
  nullable = false
  default  = 15
}

variable "num_virtual_cpus" {
  type     = number
  nullable = false
  default  = 2
}

variable "allow_list_name" {
  type     = string
  nullable = false
  default  = "default-allow-list"
}

variable "os" {
  type     = string
  nullable = true
}

variable "database" {
  type     = string
  nullable = false
}

# A production cluster should be locked down with a more
# targeted allowlist or VPC peering.
variable "cidr_ip" {
  type     = string
  nullable = false
  default  = "0.0.0.0"
}

variable "cidr_mask" {
  type     = number
  nullable = false
  default  = 0
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
  dedicated = {
    storage_gib      = var.storage_gib
    num_virtual_cpus = var.num_virtual_cpus
  }
  regions = [
    for r in var.cloud_provider_regions : {
      name       = r,
      node_count = var.cluster_node_count
    }
  ]
}

data "cockroach_cluster_cert" "example" {
  id = cockroach_cluster.example.id
}

resource "cockroach_allow_list" "example" {
  name       = var.allow_list_name
  cidr_ip    = var.cidr_ip
  cidr_mask  = var.cidr_mask
  ui         = true
  sql        = true
  cluster_id = cockroach_cluster.example.id
}

resource "cockroach_sql_user" "example" {
  name       = var.sql_user_name
  password   = var.sql_user_password
  cluster_id = cockroach_cluster.example.id
}

resource "cockroach_database" "example" {
  name       = var.database
  cluster_id = cockroach_cluster.example.id
}

data "cockroach_cluster" "example" {
  id = cockroach_cluster.example.id
}

data "cockroach_connection_string" "example" {
  id       = cockroach_cluster.example.id
  sql_user = cockroach_sql_user.example.name
  database = var.database

  # Caution: Including the `password` field will result in
  # the password showing up in plain text in the
  # connection string output!
  #
  # password = cockroach_sql_user.example.password

  os = var.os
}

output "cluster_status" {
  value = data.cockroach_cluster.example.operation_status
}

output "cert" {
  value = data.cockroach_cluster_cert.example.cert
}

output "connection_string" {
  value = data.cockroach_connection_string.example.connection_string
}
