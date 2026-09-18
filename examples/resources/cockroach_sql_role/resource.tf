variable "cluster_id" {
  type = string
}

# A grouping role. It holds no privileges of its own and cannot log in;
# it exists so that privileges can be granted once and shared.
resource "cockroach_sql_role" "app_writer" {
  name       = "app_writer"
  cluster_id = var.cluster_id
}

# A principal that can log in. Unlike cockroach_sql_user, this credential
# starts with no privileges at all, which is what makes it usable for a
# service account scoped to a single application.
#
# password_wo is a write-only attribute (Terraform CLI 1.11+): its value is
# sent on create and on every rotation but is never stored in Terraform
# state. Sourcing it from an ephemeral input (for example, an ephemeral
# resource or a sensitive variable) keeps it out of state entirely.
#
# password_wo_version is the rotation trigger. Because the write-only value
# is not in state, Terraform cannot see a change to password_wo on its own;
# bump password_wo_version to signal that the new password should be
# applied. Pair every change to password_wo with a bump of this integer.
variable "ory_password" {
  type      = string
  sensitive = true
}

resource "cockroach_sql_role" "ory" {
  name                = "ory_service"
  login               = true
  password_wo         = var.ory_password
  password_wo_version = 1
  cluster_id          = var.cluster_id
}

# login can be toggled in place: flipping it to false revokes the ability to
# authenticate without destroying the role or the privileges granted to it.
# A password on a role with login = false is accepted and simply has no
# effect until login is granted again.
#
# A role with login = true and no password can still authenticate with a
# client certificate.
