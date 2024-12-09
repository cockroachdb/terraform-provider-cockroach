variable "cluster_name" {
  type     = string
  nullable = false
}

variable "cluster_node_count" {
  type     = number
  nullable = false
  default  = 3
}

variable "storage_gib" {
  type     = number
  nullable = false
  default  = 15
}

variable "vcpu_count" {
  type     = number
  nullable = false
  default  = 4
}

variable "gcp_region" {
  type     = string
  default  = "us-west2"
  nullable = false
}

variable "datadog_site" {
  type     = string
  nullable = false
  default  = "US1"
}

variable "datadog_api_key" {
  type      = string
  nullable  = false
  sensitive = true
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
  cloud_provider = "GCP"
  dedicated = {
    storage_gib      = var.storage_gib
    num_virtual_cpus = var.vcpu_count
  }
  regions = [{
    name       = var.gcp_region,
    node_count = var.cluster_node_count
    }
  ]
}

resource "cockroach_metric_export_datadog_config" "example" {
  id      = cockroach_cluster.example.id
  site    = var.datadog_site
  api_key = var.datadog_api_key
}

resource "cockroach_metric_export_prometheus_config" "example" {
  id = cockroach_cluster.example.id
}
