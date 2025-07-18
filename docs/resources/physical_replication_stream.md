---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "cockroach_physical_replication_stream Resource - terraform-provider-cockroach"
subcategory: ""
description: |-
  Physical replication stream.
---

# cockroach_physical_replication_stream (Resource)

Physical replication stream.

## Example Usage

```terraform
resource "cockroach_physical_replication_stream" "pcr_stream" {
  primary_cluster_id = cockroach_cluster.primary.id
  standby_cluster_id = cockroach_cluster.standby.id
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `primary_cluster_id` (String) ID of the primary cluster, which is the replication source.
- `standby_cluster_id` (String) ID of the standby cluster, which is the replication target.

### Optional

- `failover_at` (String) The timestamp at which to perform failover. If not specified, failover will occur at the latest consistent replicated time. Cannot be used with failover_immediately.
- `failover_immediately` (Boolean) If true, failover will occur immediately at the latest consistent replicated time. Cannot be used with failover_at.

### Read-Only

- `activated_at` (String) The timestamp indicating the actual timestamp at which failover was finalized.
- `created_at` (String) The timestamp at which the replication stream was created.
- `id` (String) A unique identifier for the replication stream.
- `replicated_time` (String) The timestamp indicating the point up to which data has been replicated.
- `replication_lag_seconds` (Number) The replication lag in seconds.
- `retained_time` (String) The timestamp indicating the earliest time that the replication stream can failover to.
- `status` (String) The status of the replication stream.

## Import

Import is supported using the following syntax:

```shell
# format: <physical replication stream id>
terraform import cockroach_physical_replication_stream.my_stream 370c7f34-72e8-4238-b474-13767a579fb3
```
