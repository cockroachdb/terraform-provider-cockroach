terraform {
  required_version = ">= 1.11.0"
}

variable "cluster_id" {
  type = string
}

variable "otlp_endpoint" {
  type        = string
  description = "The HTTPS OTLP/HTTP endpoint supplied by your logging provider."
}

variable "otlp_headers" {
  type        = map(string)
  sensitive   = true
  ephemeral   = true
  description = "Authentication headers supplied by your logging provider. Requires Terraform CLI 1.11+."
}

resource "cockroach_log_export_config" "example" {
  id              = var.cluster_id
  type            = "OTLP_HTTP"
  log_name        = "crdb-logs"
  otlp_endpoint   = var.otlp_endpoint
  otlp_headers_wo = var.otlp_headers
  # Increment this number when changing the headers to rotate credentials.
  otlp_headers_wo_version = 1
  groups = [
    {
      log_name = "crdb-audit"
      channels = ["USER_ADMIN", "PRIVILEGES", "SENSITIVE_ACCESS"]
    }
  ]
}
