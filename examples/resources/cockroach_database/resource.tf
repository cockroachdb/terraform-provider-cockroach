variable "cluster_id" {
  type = string
}

resource "cockroach_database" "cockroach" {
  name       = "example-database"
  cluster_id = var.cluster_id
}
