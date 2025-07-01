resource "cockroach_physical_replication_stream" "pcr_stream" {
  primary_cluster_id = cockroach_cluster.primary.id
  standby_cluster_id = cockroach_cluster.standby.id
}
