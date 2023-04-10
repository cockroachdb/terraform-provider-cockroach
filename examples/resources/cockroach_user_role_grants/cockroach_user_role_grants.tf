variable "user_id" {
  type = string
}

resource "cockroach_user_role_grants" "cockroach" {
  user_id = var.user_id
  roles = [
    {
      role_name     = "ORG_ADMIN",
      resource_type = "ORGANIZATION",
      resource_id   = ""
    },
    {
      role_name     = "CLUSTER_ADMIN",
      resource_type = "ORGANIZATION",
      resource_id   = ""
    },
    {
      role_name     = "ORG_MEMBER",
      resource_type = "ORGANIZATION",
      resource_id   = ""
    },
  ]
}