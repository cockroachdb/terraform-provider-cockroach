variable "user_id" {
  type = string
}

resource "cockroach_user_role_grant" "admin_grant" {
  user_id = var.user_id
  role = {
    role_name     = "CLUSTER_ADMIN",
    resource_type = "CLUSTER",
    resource_id   = cockroach_cluster.example.id
  }
}
