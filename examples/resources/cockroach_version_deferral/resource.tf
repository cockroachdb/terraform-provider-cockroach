variable "cluster_id" {
  type = string
}

variable "offset_duration" {
  type = string
  # Defer minor version upgrades by 60 days.
  default = "FIXED_DEFERRAL"
}

resource "cockroach_version_deferral" "example" {
  id              = var.cluster_id
  deferral_policy = var.deferral_policy
}
