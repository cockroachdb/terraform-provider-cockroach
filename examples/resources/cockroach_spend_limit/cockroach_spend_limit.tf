variable cluster_id {
    type = string
}

resource "cockroach_cluster" "cockroach" {
    name = var.cluster_id
    cloud_provider = "AWS"
    update_spec = {
        serverless = {
            spend_limit = 2
        }
    }
}
