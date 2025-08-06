resource "cockroach_restore" "cluster_restore" {
  destination_cluster_id = "12345678-1234-4123-8123-123456789abc"
  type                   = "CLUSTER"
  source_cluster_id      = "22222222-2222-4222-8222-222222222222"
}

resource "cockroach_restore" "database_restore" {
  destination_cluster_id = "12345678-1234-4123-8123-123456789abc"
  type                   = "DATABASE"
  source_cluster_id      = "22222222-2222-4222-8222-222222222222"

  restore_opts = {
    new_db_name = "new_database"
    schema_only = true
  }

  objects {
    database = "database1"
  }
}

resource "cockroach_restore" "table_restore" {
  destination_cluster_id = "12345678-1234-4123-8123-123456789abc"
  type                   = "TABLE"
  backup_id              = "11111111-1111-4111-8111-111111111111"

  restore_opts = {
    into_db                   = "other_database"
    skip_localities_check     = true
    skip_missing_foreign_keys = true
    skip_missing_sequences    = true
  }

  objects {
    database = "database1"
    schema   = "schema1"
    tables   = ["table_a", "table_b"]
  }

  objects {
    database = "database2"
    schema   = "schema2"
    tables   = ["table_c"]
  }
}