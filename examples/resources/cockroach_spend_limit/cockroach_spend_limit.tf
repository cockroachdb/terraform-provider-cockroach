variable cluster_id {
    type = string
}

resource "cockroach_cluster" "cockroach" {
    id = var.cluster_id
    update_spec = {
        serverless = {
            spend_limit = 2
        }
    }
}
