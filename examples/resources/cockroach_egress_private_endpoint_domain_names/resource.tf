# Example: Basic usage with custom domain names
resource "cockroach_egress_private_endpoint_domain_names" "basic" {
  cluster_id  = cockroach_cluster.my_cluster.id
  endpoint_id = cockroach_egress_private_endpoint.basic.id
  domain_names = [
    "*.custom.domain",
    "another.domain",
  ]
}

resource "cockroach_egress_private_endpoint" "basic" {
  cluster_id                = cockroach_cluster.my_cluster.id
  region                    = "us-east-1"
  target_service_type       = "PRIVATE_SERVICE"
  target_service_identifier = "com.amazonaws.vpce.us-east-1.vpce-svc-01234567890abcdef"
}

# Example: Usage when connecting to Confluent Cloud
resource "cockroach_egress_private_endpoint_domain_names" "confluent_cloud" {
  cluster_id  = cockroach_cluster.my_cluster.id
  endpoint_id = cockroach_egress_private_endpoint.confluent_cloud.id
  domain_names = [
    "*.us-east-1.aws.private.confluent.cloud"
  ]

  depends_on = [
    confluent_private_link_attachment_connection.cockroach_cloud
  ]
}

resource "cockroach_cluster" "my_cluster" {
  name           = "my-cluster"
  cloud_provider = "AWS"
  plan           = "ADVANCED"
  dedicated = {
    storage_gib      = 15
    num_virtual_cpus = 4
  }
  regions = [
    {
      name       = "us-east-1"
      node_count = 1
    }
  ]
}

resource "cockroach_egress_private_endpoint" "confluent_cloud" {
  cluster_id                = cockroach_cluster.my_cluster.id
  region                    = "us-east-1"
  target_service_type       = "PRIVATE_SERVICE"
  target_service_identifier = confluent_private_link_attachment.main.aws.vpc_endpoint_service_name
}

resource "confluent_environment" "main" {
  display_name = "main"
}

resource "confluent_private_link_attachment" "main" {
  cloud        = "AWS"
  region       = "us-east-1"
  display_name = "main-private-link-attachment"
  environment {
    id = confluent_environment.main.id
  }
}

resource "confluent_private_link_attachment_connection" "cockroach_cloud" {
  display_name = "cockroach-cloud-access-point"
  environment {
    id = confluent_environment.main.id
  }
  aws {
    vpc_endpoint_id = cockroach_egress_private_endpoint.confluent_cloud.endpoint_connection_id
  }
  private_link_attachment {
    id = confluent_private_link_attachment.main.id
  }
}

output "confluent_cloud_egress_private_endpoint_domains" {
  description = "Domain names configured for the Confluent Cloud endpoint"
  value = {
    endpoint_id  = cockroach_egress_private_endpoint_domain_names.confluent_cloud.endpoint_id
    cluster_id   = cockroach_egress_private_endpoint_domain_names.confluent_cloud.cluster_id
    domain_names = cockroach_egress_private_endpoint_domain_names.confluent_cloud.domain_names
  }
}
