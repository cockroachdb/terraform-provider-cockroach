variable "cluster_id" {
  type = string
}

# password_wo is a write-only attribute (Terraform CLI 1.11+): its value is
# sent on create and on every rotation but is never stored in Terraform
# state. Sourcing it from an ephemeral input (for example, an ephemeral
# resource or a sensitive variable) keeps it out of state entirely.
#
# password_wo_version is the rotation trigger. Because the write-only value
# is not in state, Terraform cannot see a change to password_wo on its own;
# bump password_wo_version to signal that the new password should be
# applied. Pair every change to password_wo with a bump of this integer.
variable "sql_user_password" {
  type      = string
  sensitive = true
}

resource "cockroach_sql_user" "cockroach" {
  name                = "example-sql-user"
  password_wo         = var.sql_user_password
  password_wo_version = 1
  cluster_id          = var.cluster_id
}

# The legacy `password` attribute remains available for backwards
# compatibility but is deprecated: it persists the cleartext password in
# `terraform.tfstate`. New consumers should use `password_wo` above; existing
# consumers should plan a migration.
#
# resource "cockroach_sql_user" "legacy" {
#   name       = "legacy-sql-user"
#   password   = var.sql_user_password
#   cluster_id = var.cluster_id
# }
