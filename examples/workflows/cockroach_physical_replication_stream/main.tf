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

variable "cluster_name_1" {
  type     = string
  nullable = false
}

variable "cluster_name_2" {
  type     = string
  nullable = false
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

variable "cidr_range_1" {
  type     = string
  nullable = false
}

variable "cidr_range_2" {
  type     = string
  nullable = false
}

resource "cockroach_cluster" "pcr_example_1" {
  name           = var.cluster_name_1
  cloud_provider = var.cloud_provider
  plan           = "ADVANCED"
  dedicated = {
    storage_gib                     = var.storage_gib
    num_virtual_cpus                = var.num_virtual_cpus
    cidr_range                      = var.cidr_range_1
    supports_cluster_virtualization = true
  }
  regions = [
    for r in var.cloud_provider_regions_1 : {
      name       = r,
      node_count = var.cluster_node_count
    }
  ]
}

resource "cockroach_cluster" "pcr_example_2" {
  name           = var.cluster_name_2
  cloud_provider = var.cloud_provider
  plan           = "ADVANCED"
  dedicated = {
    storage_gib                     = var.storage_gib
    num_virtual_cpus                = var.num_virtual_cpus
    cidr_range                      = var.cidr_range_2
    supports_cluster_virtualization = true
  }
  regions = [
    for r in var.cloud_provider_regions_2 : {
      name       = r,
      node_count = var.cluster_node_count
    }
  ]
}


resource "cockroach_physical_replication_stream" "pcr_stream" {
  primary_cluster_id = cockroach_cluster.pcr_example_1.id
  standby_cluster_id = cockroach_cluster.pcr_example_2.id
  # After a PCR stream is replicating, uncomment this field to perform
  # failover. Alternatively, set failover_at.
  # failover_immediately = true
}

