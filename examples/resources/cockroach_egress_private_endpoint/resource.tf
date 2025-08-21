# Example: AWS endpoint for an application service
resource "cockroach_egress_private_endpoint" "aws_app_service" {
  cluster_id                = cockroach_cluster.my_aws_cluster.id
  region                    = "us-east-1"
  target_service_type       = "PRIVATE_SERVICE"
  target_service_identifier = "com.amazonaws.vpce.us-east-1.vpce-svc-01234567890abcdef"
}

# Example: GCP endpoint for an application service
resource "cockroach_egress_private_endpoint" "gcp_app_service" {
  cluster_id                = cockroach_cluster.my_gcp_cluster.id
  region                    = "us-east1"
  target_service_type       = "PRIVATE_SERVICE"
  target_service_identifier = "projects/project/regions/us-east1/serviceAttachments/0123456789abcdef"
}


# Example: AWS MSK with SASL/SCRAM authentication
resource "cockroach_egress_private_endpoint" "msk_sasl_scram" {
  cluster_id                = cockroach_cluster.my_aws_cluster.id
  region                    = "us-east-1"
  target_service_type       = "MSK_SASL_SCRAM"
  target_service_identifier = "arn:aws:kafka:us-west-2:123456789012:cluster/my-cluster/12345678-1234-1234-1234-123456789012-2"
}

# Example: AWS MSK with IAM authentication
resource "cockroach_egress_private_endpoint" "msk_iam" {
  cluster_id                = cockroach_cluster.my_aws_cluster.id
  region                    = "us-east-1"
  target_service_type       = "MSK_SASL_IAM"
  target_service_identifier = "arn:aws:kafka:us-west-2:123456789012:cluster/my-iam-cluster/87654321-4321-4321-4321-210987654321-3"
}

# Example: AWS MSK with TLS authentication
resource "cockroach_egress_private_endpoint" "msk_tls" {
  cluster_id                = cockroach_cluster.my_aws_cluster.id
  region                    = "us-east-1"
  target_service_type       = "MSK_TLS"
  target_service_identifier = "arn:aws:kafka:eu-west-1:123456789012:cluster/my-tls-cluster/abcdef12-abcd-abcd-abcd-abcdef123456-4"
}

# Example clusters (required for the examples above)
resource "cockroach_cluster" "my_aws_cluster" {
  name           = "my-aws-cluster"
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

resource "cockroach_cluster" "my_gcp_cluster" {
  name           = "my-gcp-cluster"
  cloud_provider = "GCP"
  plan           = "ADVANCED"
  dedicated = {
    storage_gib      = 15
    num_virtual_cpus = 4
  }
  regions = [
    {
      name       = "us-east1"
      node_count = 1
    }
  ]
}

# Outputs to show the created endpoint information
output "app_service_endpoint_id" {
  description = "The ID of the app service egress private endpoint"
  value       = cockroach_egress_private_endpoint.aws_app_service.id
}

output "app_service_endpoint_address" {
  description = "The address of the app service egress private endpoint"
  value       = cockroach_egress_private_endpoint.aws_app_service.endpoint_address
}

output "app_service_connection_id" {
  description = "The connection ID of the app service egress private endpoint"
  value       = cockroach_egress_private_endpoint.aws_app_service.endpoint_connection_id
}

output "msk_endpoints" {
  description = "Information about MSK egress private endpoints"
  value = {
    sasl_scram = {
      id      = cockroach_egress_private_endpoint.msk_sasl_scram.id
      address = cockroach_egress_private_endpoint.msk_sasl_scram.endpoint_address
      state   = cockroach_egress_private_endpoint.msk_sasl_scram.state
    }
    iam = {
      id      = cockroach_egress_private_endpoint.msk_iam.id
      address = cockroach_egress_private_endpoint.msk_iam.endpoint_address
      state   = cockroach_egress_private_endpoint.msk_iam.state
    }
    tls = {
      id      = cockroach_egress_private_endpoint.msk_tls.id
      address = cockroach_egress_private_endpoint.msk_tls.endpoint_address
      state   = cockroach_egress_private_endpoint.msk_tls.state
    }
  }
}
