variable "cluster_id" {
  type = string
}

variable "sql_user_password" {
  type = string
}

resource "cockroach_sql_user" "cockroach" {
  name       = "example-sql-user"
  password   = var.sql_user_password
  cluster_id = var.cluster_id
}
