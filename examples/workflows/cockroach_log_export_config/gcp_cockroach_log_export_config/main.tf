variable "gcp_project_id" {
  type     = string
  nullable = false
}

variable "gcp_region" {
  type     = string
  default  = "us-west2"
  nullable = false
}

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

variable "machine_type" {
  type     = string
  nullable = false
  default  = "n1-standard-2"
}

variable "iam_role_id" {
  type     = string
  nullable = false
  default  = "ExampleLogExportRole"
}

variable "iam_role_title" {
  type     = string
  nullable = false
  default  = "Example LogExport Role"
}

# For GCP, auth_principal should be the gcp_project_id.
variable "auth_principal" {
  type     = string
  nullable = false
}

terraform {
  required_providers {
    cockroach = {
      source = "cockroachdb/cockroach"
    }
    google = {
      source  = "hashicorp/google"
      version = "~> 4.0.0"
    }
  }
}

provider "cockroach" {
  # export COCKROACH_API_KEY with the cockroach cloud API Key
}

provider "google" {
  # For configuration help, see
  # https://registry.terraform.io/providers/hashicorp/google/latest/docs/guides/getting_started
  project = var.gcp_project_id
  region  = var.gcp_region
}

resource "cockroach_cluster" "example" {
  name           = var.cluster_name
  cloud_provider = "GCP"
  dedicated = {
    storage_gib  = var.storage_gib
    machine_type = var.machine_type
  }
  regions = [{
    name       = var.gcp_region,
    node_count = var.cluster_node_count
    }
  ]
}

# New role in your GCP project.
resource "google_project_iam_custom_role" "example-logexport-role" {
  project     = var.gcp_project_id
  role_id     = var.iam_role_id
  title       = var.iam_role_title
  permissions = ["logging.logEntries.create"]
}

# Grants example-logexport-role to the CockroachDB Cloud service account.
resource "google_project_iam_member" "role-sa-binding" {
  project = var.gcp_project_id
  role    = "projects/${var.gcp_project_id}/roles/${google_project_iam_custom_role.example-logexport-role.role_id}"
  # member is the CockroachDB Cloud log export service account for the cluster.
  # Example: crl-logging-user-a1c42be2e53b@crl-prod-abc.iam.gserviceaccount.com
  member = "serviceAccount:crl-logging-user-${element(split("-", cockroach_cluster.example.id), 4)}@${cockroach_cluster.example.account_id}.iam.gserviceaccount.com"
}

resource "cockroach_log_export_config" "example" {
  id             = cockroach_cluster.example.id
  auth_principal = var.auth_principal
  log_name       = "example"
  type           = "GCP_CLOUD_LOGGING"
  redact         = true
  groups = [
    {
      log_name = "sql",
      channels = ["SQL_SCHEMA", "SQL_EXEC"],
      redact   = false,
      # TODO(jenn): Test unspecified min_level.
      min_level : "WARNING"
    },
    {
      log_name  = "devops",
      channels  = ["OPS", "HEALTH", "STORAGE"],
      min_level = "WARNING"
    }
  ]
}
