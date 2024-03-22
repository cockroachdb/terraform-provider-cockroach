resource "cockroach_allow_list" "vpn" {
  name       = "vpn"
  cidr_ip    = "123.123.1.1"
  cidr_mask  = 32
  ui         = true
  sql        = true
  cluster_id = cockroach_cluster.staging.id
}
