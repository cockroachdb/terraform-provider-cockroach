variable "cluster_id" {
  type = string
}

data "cockroach_cluster" "cockroach" {
  id = var.cluster_id
}
