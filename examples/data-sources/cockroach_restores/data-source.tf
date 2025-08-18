# Fetch the latest restore job
data "cockroach_restores" "latest_restore" {
  cluster_id = cockroach_cluster.prod_cluster.id
  sort_order = "DESC"
  limit      = 1
}

# Get the id of the most recent restore job
output "latest_restore_id" {
  value = data.cockroach_restores.latest_restore.restores[0].id
}

# Fetch restore jobs within a specific date range
data "cockroach_restores" "restores_in_range" {
  cluster_id = cockroach_cluster.prod_cluster.id
  start_time = "2025-08-15"
  # List restore jobs that occurred before 8/19/2025
  end_time   = "2025-08-19"
  sort_order = "DESC"
  limit      = 5
}