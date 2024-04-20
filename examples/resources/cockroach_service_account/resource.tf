resource "cockroach_service_account" "prod_sa" {
  name        = "Prod cluster SA"
  description = "A service account used for managing access to the prod cluster"
}

resource "cockroach_user_role_grants" "prod_sa" {
  user_id = cockroach_service_account.prod_sa.id
  roles = [
    {
      role_name     = "CLUSTER_ADMIN",
      resource_type = "CLUSTER",
      resource_id   = cockroach_cluster.prod.id
    }
  ]
}
