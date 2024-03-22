resource "cockroach_cluster" "dedicated" {
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

resource "cockroach_cluster" "serverless" {
  name           = "cockroach-serverless"
  cloud_provider = "GCP"
  serverless = {
    spend_limit = 1
  }
  regions = [
    {
      name = "us-east1"
    }
  ]
}
