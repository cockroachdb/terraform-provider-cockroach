variable "cluster_id" {
  type = string
}

resource "cockroach_private_endpoint_trusted_owner" "example" {
  cluster_id        = var.cluster_id
  type              = "AWS_ACCOUNT_ID"
  external_owner_id = "012345678901"
}
