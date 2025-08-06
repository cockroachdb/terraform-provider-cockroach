variable "destination_cluster_id" {
  type     = string
  nullable = false
}

variable "source_cluster_id" {
  type     = string
  nullable = false
}

# Fetch the latest backup
data "cockroach_backups" "latest_backup" {
  cluster_id = var.source_cluster_id
  sort_order = "DESC"
  limit      = 1
}

# Restore from most recent backup
resource "cockroach_restore" "restore_from_latest_backup" {
  destination_cluster_id = var.destination_cluster_id
  type                   = "CLUSTER"
  backup_id              = data.cockroach_backups.latest_backup.backups[0].id
}