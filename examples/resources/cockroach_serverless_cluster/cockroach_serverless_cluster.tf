resource "cockroach_cluster" "cockroach" {
  name           = "cockroach-serverless"
  cloud_provider = "GCP"
  serverless = {
    usage_limits = {
      provisioned_capacity = 1000
    }
  }
  regions = [
    {
      name = "us-east1"
    }
  ]
}
