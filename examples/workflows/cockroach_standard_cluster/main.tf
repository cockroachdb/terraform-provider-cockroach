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

resource "cockroach_cluster" "example" {
  name           = var.cluster_name
  cloud_provider = var.cloud_provider
  plan           = "STANDARD"
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

resource "cockroach_service_account" "example_scoped_sa" {
  name        = "example-scoped-service-account"
  description = "A service account providing limited read access to single cluster."
}

resource "cockroach_user_role_grant" "example_limited_access_scoped_grant" {
  user_id = cockroach_service_account.example_scoped_sa.id
  role = {
    role_name     = "CLUSTER_OPERATOR_WRITER",
    resource_type = "CLUSTER",
    resource_id   = cockroach_cluster.example.id
  }
}

resource "cockroach_api_key" "example_cluster_op_key_v1" {
  name               = "example-cluster-operator-key-v1"
  service_account_id = cockroach_service_account.example_scoped_sa.id
}

output "example_cluster_op_key_v1_secret" {
  value       = cockroach_api_key.example_cluster_op_key_v1.secret
  description = "The api key for example_cluster_op_key_v1_secret"
  sensitive   = true
}

output "cluster_version" {
  value = cockroach_cluster.example.full_version
}
