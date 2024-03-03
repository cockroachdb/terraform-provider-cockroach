variable "cluster_id" {
  type = string
}

data "cockroach_cluster_cert" "cockroach" {
  id = var.cluster_id
}
