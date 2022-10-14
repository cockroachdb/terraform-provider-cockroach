resource "cockroach_cluster" "cockroach" {
  name           = "cockroach-dedicated"
  cloud_provider = "GCP"
  dedicated = {
    storage_gib  = 15
    machine_type = "n1-standard-4"
  }
  regions = [
    {
      name       = "us-central1"
      node_count = 1
    }
  ]
}
