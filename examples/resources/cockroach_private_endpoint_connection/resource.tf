## Example with AWS PrivateLink

# Enable the PrivateLink service on the CockroachDB Cloud cluster.
resource "cockroach_private_endpoint_services" "aws_cluster_services" {
  cluster_id = cockroach_cluster.my_cluster.id
}

# Create a PrivateLink endpoint and associate it with the PrivateLink Service. 
resource "aws_vpc_endpoint" "my_endpoint" {
  vpc_id             = "vpc-7fc0a543"
  service_name       = cockroach_private_endpoint_services.aws_cluster_services.services_map["us-east-1"].name
  vpc_endpoint_type  = "Interface"
  subnet_ids         = ["subnet-de0406d2"]
  security_group_ids = ["sg-3f238186"]
}

# Establish the connection between the endpoint and the service.
resource "cockroach_private_endpoint_connection" "connection" {
  cluster_id  = cockroach_cluster.my_cluster.id
  endpoint_id = aws_vpc_endpoint.my_endpoint.id
}

## Example with Azure Private Link

# Enable the Private Link service on the CockroachDB Cloud cluster.
resource "cockroach_private_endpoint_services" "azure_cluster_services" {
  cluster_id = cockroach_cluster.my_cluster.id
}

# Create a private link endpoint and associate it with the Private Link Service. 
resource "azurerm_private_endpoint" "my_endpoint" {
  name                = "my_endpoint"
  location            = var.location
  resource_group_name = var.resource_group_name
  subnet_id           = azurerm_subnet.my_subnet.id
  private_service_connection {
    name                           = cockroach_private_endpoint_services.azure_cluster_services.services_map["eastus2"].name
    private_connection_resource_id = cockroach_private_endpoint_services.azure_cluster_services.services_map["eastus2"].endpoint_service_id
    is_manual_connection           = true
    request_message                = "Azure Private Link test"
  }
}

# Establish a connection between the endpoint and the service.
resource "cockroach_private_endpoint_connection" "connection" {
  cluster_id  = cockroach_cluster.my_cluster.id
  endpoint_id = azurerm_private_endpoint.my_endpoint.id
}

## Example with GCP Private Service Connect

# Enable the Private Service Connect services on the CockroachDB Cloud cluster.
resource "cockroach_private_endpoint_services" "gcp_cluster_services" {
  cluster_id = cockroach_cluster.my_cluster.id
}

# Create the GCP Private Service Connect endpoint using the GCP API or the GCP
# Console. You will need the service id to create the endpoint. You can get the
# service information by running `terraform show` and noting
# `cockroach_private_endpoint_connection.services[*].name`,
# `cockroach_private_endpoint_connection.services[*].endpoint_service_id`

# Establish a connection between the endpoint and the service.
resource "cockroach_private_endpoint_connection" "connection" {
  cluster_id  = cockroach_cluster.my_cluster.id
  endpoint_id = "6133183410995353"
}
