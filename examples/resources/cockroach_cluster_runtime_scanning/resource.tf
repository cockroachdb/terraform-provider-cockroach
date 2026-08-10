variable "cluster_id" {
  type = string
}

resource "cockroach_cluster_runtime_scanning" "example" {
  cluster_id = var.cluster_id
  type       = "WIZ"
}
