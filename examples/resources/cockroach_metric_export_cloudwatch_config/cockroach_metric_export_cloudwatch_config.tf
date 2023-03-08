variable "cluster_id" {
  type     = string
  nullable = false
}

variable "role_arn" {
  type     = string
  nullable = false
}

variable "log_group_name" {
  type     = string
  nullable = false
}

variable "aws_region" {
  type = string
}

resource "cockroach_metric_export_cloudwatch_config" "example" {
  id             = var.cluster_id
  role_arn       = var.role_arn
  log_group_name = var.log_group_name
  target_region  = var.aws_region
}