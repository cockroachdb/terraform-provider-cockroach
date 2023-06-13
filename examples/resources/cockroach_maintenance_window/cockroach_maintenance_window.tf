variable "cluster_id" {
  type = string
}

resource "cockroach_maintenance_window" "example" {
  id              = var.cluster_id
  offset_duration = "172800s"
  window_duration = "21600s"
}
