resource "cockroach_cluster" "cockroach" {
    name           = "cockroach-serverless"
    cloud_provider = "GCP"
    create_spec = {
        serverless = {
            regions = ["us-east1"]
            spend_limit = 1
        }
    }
}

