variable "cluster_id" {
  type = string
}

resource "cockroach_allow_list" "cockroach" {
  name       = "example-allow-list"
  cidr_ip    = "192.168.1.1"
  cidr_mask  = 32
  ui         = true
  sql        = true
  cluster_id = var.cluster_id
}
