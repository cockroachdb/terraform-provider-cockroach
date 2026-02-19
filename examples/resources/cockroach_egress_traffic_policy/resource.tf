# Note: When using cockroach_egress_rule, the egress traffic policy is
# automatically set to deny-by-default. This resource is useful when you want to:
# 1. Explicitly manage the policy state
# 2. Block all outbound traffic without creating any egress rules
# 3. Reset the policy back to allow-all after removing egress rules

# Example: Manage egress traffic policy for a cluster
resource "cockroach_egress_traffic_policy" "outbound_traffic" {
  id        = cockroach_cluster.my_cluster.id
  allow_all = false # Set to true to allow all outbound traffic
}

# Example cluster
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
output "egress_policy_cluster_id" {
  description = "The cluster ID for the egress traffic policy"
  value       = cockroach_egress_traffic_policy.outbound_traffic.id
}

output "egress_policy_allow_all" {
  description = "Whether all outbound traffic is allowed"
  value       = cockroach_egress_traffic_policy.outbound_traffic.allow_all
}
