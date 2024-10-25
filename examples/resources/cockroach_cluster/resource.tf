resource "cockroach_cluster" "advanced" {
  name           = "cockroach-advanced"
  cloud_provider = "GCP"
  plan           = "ADVANCED"
  dedicated = {
    storage_gib  = 15
    machine_type = "n2-standard-4"
  }
  regions = [
    {
      name       = "us-central1"
      node_count = 1
    }
  ]
  delete_protection = true
  backup_config = {
    enabled           = true
    frequency_minutes = 60
    retention_days    = 30
  }
}

resource "cockroach_cluster" "standard" {
  name           = "cockroach-standard"
  cloud_provider = "GCP"
  plan           = "STANDARD"
  serverless = {
    usage_limits = {
      provisioned_virtual_cpus = 2
    }
    upgrade_type = "AUTOMATIC"
  }
  regions = [
    {
      name = "us-east1"
    }
  ]
  delete_protection = false
  backup_config = {
    enabled           = true
    frequency_minutes = 60
    retention_days    = 30
  }
}

resource "cockroach_cluster" "basic" {
  name           = "cockroach-basic"
  cloud_provider = "GCP"
  plan           = "BASIC"
  serverless     = {}
  regions = [
    {
      name = "us-east1"
    }
  ]
  delete_protection = false
}
