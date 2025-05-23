variable "cluster_id" {
  type     = string
  nullable = false
}

variable "role_arn" {
  type     = string
  nullable = false
}

variable "log_group_name" {
  type = string
}

variable "aws_region" {
  type = string
}

variable "external_id" {
  type = string
}

resource "cockroach_metric_export_cloudwatch_config" "example" {
  external_id    = var.external_id
  id             = var.cluster_id
  role_arn       = var.role_arn
  log_group_name = var.log_group_name
  target_region  = var.aws_region
}
