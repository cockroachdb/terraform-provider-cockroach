variable "cluster_id" {
  description = "ID of the cluster whose metrics will be exported."
  type        = string
  nullable    = false
}

variable "role_arn" {
  description = "IAM role that trusts Cockroach Cloud and grants cloudwatch:PutMetricData."
  type        = string
  nullable    = false
}

variable "target_region" {
  description = "AWS region supporting CloudWatch OTLP ingestion and PromQL. Omit to use each cluster deployment region."
  type        = string
  default     = null
}

variable "external_id" {
  description = "External ID required by the IAM role's trust policy, if any."
  type        = string
  default     = null
}

# Requires access to the limited-access OTLP metric export API.
resource "cockroach_metric_export_otlp_config" "example" {
  id = var.cluster_id
  cloudwatch = {
    role_arn      = var.role_arn
    target_region = var.target_region
    external_id   = var.external_id
  }
}
