resource "cockroach_backup_config" "backed_up_cluster" {
  id                = cockroach_cluster.backed_up_cluster.id
  enabled           = true
  frequence_minutes = 60
  retention_days    = 30
}

resource "cockroach_backup_config" "no_backups_cluster" {
  id      = cockroach_cluster.no_backups_cluster.id
  enabled = false
}
