resource "cockroach_private_endpoint_connection" "serverless" {
  cluster_id  = cockroach_cluster.serverless.id
  endpoint_id = "endpoint-id"
}
