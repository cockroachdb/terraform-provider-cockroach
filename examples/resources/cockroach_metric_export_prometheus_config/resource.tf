variable "cluster_id" {
  type = string
}

resource "cockroach_metric_export_prometheus_config" "example" {
  id = var.cluster_id
}