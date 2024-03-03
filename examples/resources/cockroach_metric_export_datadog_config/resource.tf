variable "cluster_id" {
  type = string
}

variable "datadog_site" {
  type     = string
  nullable = false
}

variable "datadog_api_key" {
  type      = string
  nullable  = false
  sensitive = true
}

resource "cockroach_metric_export_datadog_config" "example" {
  id      = var.cluster_id
  site    = var.datadog_site
  api_key = var.datadog_api_key
}