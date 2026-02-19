# Egress rules only take effect when the cluster's egress traffic policy is set
# to deny-by-default. When you create egress rules, the policy is automatically
# set to deny-by-default. Use cockroach_egress_traffic_policy to explicitly
# manage the policy state.

# Example: Allow egress to a specific FQDN on HTTPS port
resource "cockroach_egress_rule" "my_egress_rule" {
  cluster_id  = cockroach_cluster.my_cluster.id
  name        = "external-api"
  description = "Allow outbound traffic to external API service"
  type        = "FQDN"
  destination = "api.example.com"
  ports       = [443]
}

# Example: Allow egress to a domain on multiple ports
resource "cockroach_egress_rule" "kafka_cluster" {
  cluster_id  = cockroach_cluster.my_cluster.id
  name        = "kafka-cluster"
  description = "Allow outbound traffic to Kafka brokers"
  type        = "FQDN"
  destination = "kafka.internal.example.com"
  ports       = [9092, 9093, 9094]
}

# Example: Allow egress to a CIDR range (all ports)
resource "cockroach_egress_rule" "internal_network" {
  cluster_id  = cockroach_cluster.my_cluster.id
  name        = "internal-network"
  description = "Allow outbound traffic to internal network"
  type        = "CIDR"
  destination = "10.0.0.0/8"
  # ports omitted = all ports allowed
}

# Example: Allow egress to a specific IP on specific ports
resource "cockroach_egress_rule" "database_server" {
  cluster_id  = cockroach_cluster.my_cluster.id
  name        = "database-server"
  description = "Allow outbound traffic to external database"
  type        = "CIDR"
  destination = "192.168.1.100/32"
  ports       = [5432, 3306]
}

# Example cluster (required for the examples above)
resource "cockroach_cluster" "my_cluster" {
  name           = "my-advanced-cluster"
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

# Outputs
output "my_egress_rule_id" {
  description = "The ID of the egress rule"
  value       = cockroach_egress_rule.my_egress_rule.id
}

output "my_egress_rule_state" {
  description = "The state of the egress rule"
  value       = cockroach_egress_rule.my_egress_rule.state
}

