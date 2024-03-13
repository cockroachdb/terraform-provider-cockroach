resource "cockroach_cluster" "cockroach" {
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
