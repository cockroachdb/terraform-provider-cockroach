variable "cluster_id" {
  type = string
}

# Remember that even variables marked sensitive will show up
# in the Terraform state file. Always follow best practices
# when managing sensitive info.
# https://developer.hashicorp.com/terraform/tutorials/configuration-language/sensitive-variables#sensitive-values-in-state
variable "sql_user_password" {
  type      = string
  sensitive = true
}

resource "cockroach_sql_user" "cockroach" {
  name       = "example-sql-user"
  password   = var.sql_user_password
  cluster_id = var.cluster_id
}
