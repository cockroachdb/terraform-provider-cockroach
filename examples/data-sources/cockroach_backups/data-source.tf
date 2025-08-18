# Fetch the latest backup
data "cockroach_backups" "latest_backup" {
  cluster_id = cockroach_cluster.prod_cluster.id
  sort_order = "DESC"
  limit      = 1
}

# Get the id of the most recent backup
output "latest_backup_id" {
  value = data.cockroach_backups.latest_backup.backups[0].id
}

# Fetch backups within a specific date range
data "cockroach_backups" "backups_in_range" {
  cluster_id = cockroach_cluster.prod_cluster.id
  start_time = "2025-08-15"
  # List backups that occurred before 8/19/2025
  end_time   = "2025-08-19"
  sort_order = "DESC"
  limit      = 5
}