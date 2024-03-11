variable "cluster_id" {
  type = string
}

variable "sql_user_name" {
  type     = string
  nullable = true
}

variable "sql_user_password" {
  type      = string
  sensitive = true
  nullable  = true
}

variable "database" {
  type     = string
  nullable = true
}

variable "os" {
  type     = string
  nullable = true
}

data "cockroach_connection_string" "cockroach" {
  id       = var.cluster_id
  sql_user = var.sql_user_name
  password = var.sql_user_password
  database = var.database
  os       = var.os
}
