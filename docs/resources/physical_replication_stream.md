---
layout: "cockroach"
page_title: "cockroach_physical_replication_stream Resource"
description: |-
  Physical replication steram.
---

# cockroach_physical_replication_stream Resource

Physical replication stream.

## Example Usage

```hcl
resource "cockroach_physical_replication_stream" "replication" {
  primary_cluster_id = "primary-cluster-id"
  standby_cluster_id = "standby-cluster-id"
}
```

## Schema

### Required

- `primary_cluster_id` (String) ID of the cluster that is being replicated.
- `standby_cluster_id` (String) ID of the cluster that data is being replicated to.

### Read-Only

- `created_at` (String) The timestamp at which the replication stream was created.
- `id` (String) A unique identifier for the replication stream.
- `replicated_time` (String) The timestamp indicating the point up to which data has been replicated.
- `replication_lag_seconds` (Number) The replication lag in seconds.
- `retained_time` (String) The timestamp indicating the lower bound that the replication stream can failover to.
- `status` (String) The status of the replication stream. 
