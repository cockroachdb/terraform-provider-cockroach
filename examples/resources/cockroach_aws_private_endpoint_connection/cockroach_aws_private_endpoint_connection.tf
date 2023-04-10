variable "cluster_id" {
  type = string
}

resource "cockroach_private_endpoint_connection" "cockroach" {
  cluster_id     = var.cluster_id
  endpoint_id    = "endpoint id assigned by consumer AWS"
  cloud_provider = "AWS"
  region_name    = "AWS region in which the endpoint was created"
}
