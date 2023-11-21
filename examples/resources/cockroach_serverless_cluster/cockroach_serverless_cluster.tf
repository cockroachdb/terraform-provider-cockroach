resource "cockroach_cluster" "cockroach" {
  name           = "cockroach-serverless"
  cloud_provider = "GCP"
  create_spec = {
    shared = {
      usage_limits = {
        request_unit_rate_limit = 1000
      }
    }
  }
  regions = [
    {
      name = "us-east1"
    }
  ]
}
