# This example assumes you already have an ADVANCED cluster and 
# an active maintenance window on that cluster.

resource "cockroach_blackout_window" "example" {
  cluster_id = "123e4567-e89b-12d3-a456-426614174000"
  start_time = "2025-03-15T09:00:00Z"
  end_time   = "2025-03-18T09:00:00Z"
}
