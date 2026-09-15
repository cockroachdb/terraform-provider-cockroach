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

variable "upgrade_type" {
  type     = string
  nullable = false
  default  = "AUTOMATIC"
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

# This example requires an organization on Cockroach Continuum. Organizations
# that are not on Continuum set a plan instead of an edition; see the
# cockroach_basic_cluster, cockroach_standard_cluster, and
# cockroach_advanced_cluster examples.
#
# Use the MISSION_CRITICAL edition with a dedicated block instead of a
# serverless one for a cluster on dedicated hardware.
resource "cockroach_cluster" "example" {
  name           = var.cluster_name
  cloud_provider = var.cloud_provider
  edition        = "STANDARD"
  serverless = {
    usage_limits = {
      provisioned_virtual_cpus = var.provisioned_virtual_cpus
    }
    upgrade_type = var.upgrade_type
  }
  regions = [for r in var.cloud_provider_regions : { name = r }]
  backup_config = {
    enabled           = true
    frequency_minutes = 60
    retention_days    = 30
  }
  labels = {
    environment   = "production",
    "cost-center" = "mkt-5678"
  }
}

resource "cockroach_sql_user" "example" {
  name       = var.sql_user_name
  password   = var.sql_user_password
  cluster_id = cockroach_cluster.example.id
}

resource "cockroach_database" "example" {
  name       = "example-database"
  cluster_id = cockroach_cluster.example.id
}

output "cluster_edition" {
  value = cockroach_cluster.example.edition
}

output "cluster_version" {
  value = cockroach_cluster.example.full_version
}
