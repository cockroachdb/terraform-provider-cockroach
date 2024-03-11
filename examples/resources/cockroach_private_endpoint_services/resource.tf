variable "cluster_id" {
  type = string
}

resource "cockroach_private_endpoint_services" "cockroach" {
  cluster_id = var.cluster_id
}
