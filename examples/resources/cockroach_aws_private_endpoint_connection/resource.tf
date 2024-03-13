variable "cluster_id" {
  type        = string
  description = "the id for the CockroachDB Cloud cluster"
}

resource "cockroach_private_endpoint_connection" "cockroach" {
  cluster_id  = var.cluster_id
  endpoint_id = "the endpoint id assigned by cloud provider to the client-side of the connection"
}
