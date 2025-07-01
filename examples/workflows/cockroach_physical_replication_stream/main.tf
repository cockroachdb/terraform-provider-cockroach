variable "cluster_name_1" {
  type     = string
  nullable = false
}

variable "cluster_name_2" {
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

variable "cloud_provider_regions_1" {
  type     = list(string)
  nullable = false
  default  = ["us-east1"]
}

variable "cloud_provider_regions_2" {
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
  default  = 4
}

variable "allow_list_name" {
  type     = string
  nullable = false
  default  = "default-allow-list"
}

variable "os" {
  type     = string
  nullable = true
  default  = "MAC"
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

variable "cidr_prefix_length" {
  type     = number
  nullable = false
  default  = 0
}

variable "cidr_range_1" {
  type     = string
  nullable = false
}

variable "cidr_range_2" {
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

resource "cockroach_cluster" "pcr_example_1" {
  name           = var.cluster_name_1
  cloud_provider = var.cloud_provider
  plan           = "ADVANCED"
  dedicated = {
    storage_gib      = var.storage_gib
    num_virtual_cpus = var.num_virtual_cpus
    cidr_range       = var.cidr_range_1
  }
  regions = [
    for r in var.cloud_provider_regions_1 : {
      name       = r,
      node_count = var.cluster_node_count
    }
  ]
  backup_config = {
    enabled           = true
    frequency_minutes = 60
    retention_days    = 30
  }
}

resource "cockroach_cluster" "pcr_example_2" {
  name           = var.cluster_name_2
  cloud_provider = var.cloud_provider
  plan           = "ADVANCED"
  dedicated = {
    storage_gib      = var.storage_gib
    num_virtual_cpus = var.num_virtual_cpus
    cidr_range       = var.cidr_range_2
  }
  regions = [
    for r in var.cloud_provider_regions_2 : {
      name       = r,
      node_count = var.cluster_node_count
    }
  ]
  backup_config = {
    enabled           = true
    frequency_minutes = 60
    retention_days    = 30
  }
}

data "cockroach_cluster_cert" "pcr_example_1" {
  id = cockroach_cluster.pcr_example_1.id
}

resource "cockroach_allow_list" "pcr_example_1" {
  name       = var.allow_list_name
  cidr_ip    = var.cidr_ip
  cidr_mask  = var.cidr_prefix_length
  ui         = true
  sql        = true
  cluster_id = cockroach_cluster.pcr_example_1.id
}

resource "cockroach_sql_user" "pcr_example_1" {
  name       = var.sql_user_name
  password   = var.sql_user_password
  cluster_id = cockroach_cluster.pcr_example_1.id
}

resource "cockroach_database" "pcr_example_1" {
  name       = var.database
  cluster_id = cockroach_cluster.pcr_example_1.id
}

data "cockroach_cluster" "pcr_example_1" {
  id = cockroach_cluster.pcr_example_1.id
}

data "cockroach_connection_string" "pcr_example_1" {
  id       = cockroach_cluster.pcr_example_1.id
  sql_user = cockroach_sql_user.pcr_example_1.name
  database = var.database

  # Caution: Including the `password` field will result in
  # the password showing up in plain text in the
  # connection string output! We recommend following terraform best practices
  # for securing sensitive variables.
  # https://developer.hashicorp.com/terraform/tutorials/configuration-language/sensitive-variables
  #
  # password = cockroach_sql_user.pcr_example_1.password

  os = var.os
}

output "cluster_status_1" {
  value = data.cockroach_cluster.pcr_example_1.operation_status
}

output "cert_1" {
  value = data.cockroach_cluster_cert.pcr_example_1.cert
}

output "connection_string_1" {
  value = data.cockroach_connection_string.pcr_example_1.connection_string
}

data "cockroach_cluster_cert" "pcr_example_2" {
  id = cockroach_cluster.pcr_example_2.id
}

resource "cockroach_allow_list" "pcr_example_2" {
  name       = var.allow_list_name
  cidr_ip    = var.cidr_ip
  cidr_mask  = var.cidr_prefix_length
  ui         = true
  sql        = true
  cluster_id = cockroach_cluster.pcr_example_2.id
}

/*
resource "cockroach_sql_user" "pcr_example_2" {
  name       = var.sql_user_name
  password   = var.sql_user_password
  cluster_id = cockroach_cluster.pcr_example_2.id
}

resource "cockroach_database" "pcr_example_2" {
  name       = var.database
  cluster_id = cockroach_cluster.pcr_example_2.id
}
*/

data "cockroach_cluster" "pcr_example_2" {
  id = cockroach_cluster.pcr_example_2.id
}

/*
data "cockroach_connection_string" "pcr_example_2" {
  id       = cockroach_cluster.pcr_example_2.id
  sql_user = cockroach_sql_user.pcr_example_2.name
  database = var.database

  # Caution: Including the `password` field will result in
  # the password showing up in plain text in the
  # connection string output! We recommend following terraform best practices
  # for securing sensitive variables.
  # https://developer.hashicorp.com/terraform/tutorials/configuration-language/sensitive-variables
  #
  # password = cockroach_sql_user.pcr_example_2.password

  os = var.os
}
*/

output "cluster_status_2" {
  value = data.cockroach_cluster.pcr_example_2.operation_status
}

output "cert_2" {
  value = data.cockroach_cluster_cert.pcr_example_2.cert
}

/*
output "connection_string_2" {
  value = data.cockroach_connection_string.pcr_example_2.connection_string
}
*/

resource "cockroach_physical_replication_stream" "pcr_stream" {
  primary_cluster_id = cockroach_cluster.pcr_example_1.id
  standby_cluster_id = cockroach_cluster.pcr_example_2.id
}
