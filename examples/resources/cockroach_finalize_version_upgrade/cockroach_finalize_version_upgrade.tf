variable "cluster_id" {
  type = string
}

variable "cockroach_version" {
  type = string
}

resource "cockroach_finalize_version_upgrade" "cockroach" {
  id                = var.cluster_id
  cockroach_version = var.cockroach_version
}
