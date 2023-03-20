variable "cluster_name" {
  type     = string
  nullable = false
}

variable "gcp_region" {
  type     = string
  nullable = false
  default  = "us-west2"
}

variable "gcp_zone" {
  type     = string
  nullable = false
  default  = "us-west2-a"
}

variable "gcp_project" {
  type     = string
  nullable = false
}

variable "additional_regions" {
  type     = list(string)
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
  default  = "n2-standard-2"
}

terraform {
  required_providers {
    cockroach = {
      source = "cockroachdb/cockroach"
    }
    google = {
      source  = "hashicorp/google"
      version = "~> 4.0"
    }
  }
}
provider "cockroach" {
  # export COCKROACH_API_KEY with the cockroach cloud API Key
}

provider "google" {
  # See https://registry.terraform.io/providers/hashicorp/google/latest/docs
  # for configuration steps.

  project = var.gcp_project
  region  = var.gcp_region
  zone    = var.gcp_zone
}

resource "cockroach_cluster" "example" {
  name           = var.cluster_name
  cloud_provider = "GCP"
  dedicated = {
    storage_gib                = var.storage_gib
    machine_type               = var.machine_type
    private_network_visibility = true
  }
  regions = [{
    name       = var.gcp_region,
    node_count = var.cluster_node_count
    }
  ]
}

resource "google_service_account" "example" {
  account_id = "${var.cluster_name}-cmek"
}

resource "google_service_account_iam_binding" "example" {
  members            = ["serviceAccount:crl-kms-user-${substr(cockroach_cluster.example.id, -12, -1)}@${cockroach_cluster.example.account_id}.iam.gserviceaccount.com"]
  role               = "roles/iam.serviceAccountTokenCreator"
  service_account_id = google_service_account.example.name
}

resource "google_kms_key_ring" "example" {
  name     = "keyring-example"
  location = "global"
}

resource "google_kms_crypto_key" "example" {
  key_ring = google_kms_key_ring.example.id
  name     = "crdb-cmek"
  purpose  = "ENCRYPT_DECRYPT"
  version_template {
    algorithm        = "GOOGLE_SYMMETRIC_ENCRYPTION"
    protection_level = "SOFTWARE"
  }
}

resource "google_kms_crypto_key_iam_binding" "example" {
  crypto_key_id = google_kms_crypto_key.example.id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"
  members       = ["serviceAccount:${google_service_account.example.email}"]
}

resource "cockroach_cmek" "example" {
  id = cockroach_cluster.example.id
  regions = /*concat(*/ [
    {
      region : var.gcp_region
      key : {
        auth_principal : google_service_account.example.email
        type : "GCP_CLOUD_KMS"
        uri : google_kms_crypto_key.example.id
      }
    }
  ] #,
  #
  # Additional regions can be added after CMEK is enabled by updating
  # the `region` attribute and adding their name and node count to
  # `additional_regions`. These regions will be managed separately from
  # the parent cluster, but will otherwise behave the same. Cluster data
  # sources will always show the entire list of regions, regardless of
  # whether they're managed by the cluster or CMEK resource.
  #
  # These should be concatenated with the current region(s).
  #[for r in var.additional_regions : {
  #  region: r,
  #  key: {
  #    auth_principal: google_service_account.example.email
  #    type: "GCP_CLOUD_KMS"
  #    uri: google_kms_crypto_key.example.id
  #  }
  #}])

  #additional_regions = [for r in var.additional_regions :
  #  {
  #    name = r
  #    node_count = var.cluster_node_count
  #  }
  #]
}
