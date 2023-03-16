variable "cluster_name" {
  type     = string
  nullable = false
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

variable "machine_type" {
  type     = string
  nullable = false
  default  = "n1-standard-2"
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
  name              = var.cluster_name
  cloud_provider    = var.cloud_provider
  dedicated = {
    storage_gib  = var.storage_gib
    machine_type = var.machine_type
  }
  regions = [
    for r in var.cloud_provider_regions : {
      name       = r,
      node_count = var.cluster_node_count
    }
  ]
}

resource "cockroach_client_ca_cert" "example" {
  id = cockroach_cluster.example.id
  x509_pem_cert = file("client-ca.crt")
}