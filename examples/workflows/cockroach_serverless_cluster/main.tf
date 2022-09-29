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

variable "serverless_spend_limit" {
  type = number
  nullable = false
  default = 0
}

variable "cloud_provider" {
  type = string
  nullable = false
  default = "GCP"
}

variable "cloud_provider_region" {
  type = list
  nullable = false
  default = ["us-east1"]
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
    wait_for_cluster_ready = true
    create_spec = {
      serverless = {
         regions = var.cloud_provider_region
         spend_limit = var.serverless_spend_limit
         }
    }
}

resource "cockroach_sql_user" "cockroach" {
  name = var.sql_user_name
  password = var.sql_user_password
  id = cockroach_cluster.cockroach.id
}

