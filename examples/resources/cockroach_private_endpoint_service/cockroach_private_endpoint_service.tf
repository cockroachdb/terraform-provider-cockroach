variable cluster_id {
    type = string
}

resource "cockroach_private_endpoint_service" "cockroach" {
    id = var.cluster_id
}
