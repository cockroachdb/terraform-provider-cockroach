variable cluster_id {
  type = string
}

variable cloud_provider {
  type = string
}

data "cockroach_cluster" "cockroach" {
  id = var.cluster_id
  cloud_provider = var.cloud_provider
}

