variable "cluster_id" {
  type = string
}

variable "auth_principal" {
  type = string
}

variable "aws_external_id" {
  type = string
}

resource "cockroach_log_export_config" "example" {
  id              = var.cluster_id
  auth_principal  = var.auth_principal
  log_name        = "example"
  type            = "GCP_CLOUD_LOGGING"
  redact          = true
  aws_external_id = var.aws_external_id
  groups = [
    {
      log_name : "sql",
      channels : ["SQL_SCHEMA", "SQL_EXEC"],
      redact : false
    },
    {
      log_name : "devops",
      channels : ["OPS", "HEALTH", "STORAGE"]
      min_level : "WARNING"
    }
  ]
  omitted_channels = ["SQL_PERF"]
}
