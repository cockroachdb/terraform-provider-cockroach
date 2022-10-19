variable cluster_id {
    type = string
}

resource "cockroach_aws_private_endpoint_connection" "cockroach" {
    id              = var.cluster_id
    cloud_provider  = "AWS"
    region          = "AWS region in which the endpoint was created"
    endpoint_id     = "endpoint id assigned by consumer AWS"
    status          = "ENDPOINT_AVAILABLE"
}
