# Updating Backup Retention

Backup retention can be controlled by setting the attribute
`backup_config.retention_days` on the `cockroach_cluster` resource.

When working with backup retention, it's important to note that this value can
only be set once.   It is necessary to open a support ticket to modify the
setting again. For this reason, consider the following when managing the value
in Terraform:

* (Optional) Refrain from including `backup_config.retention_days` in the `cockroach_cluster` resource to
  rely on server-side management of the value instead.
* If the initial value for `backup_config.retention_days` is the default value (i.e.
  30), it will be possible to modify the retention setting one more time.
* If the initial value set for `backup_config.retention_days` is not the default, it
  will not be possible to modify the retention setting again. Further
  modifications will require a support ticket.

Changing the value of `backup_config.retention_days` after using your one change
will be a multi-step operation. Here are two workflows that will work.  Both of
these options assume you already have modified `retention_days` once and as a
result must open a support ticket:

* Change it, and then open a ticket before applying the change to Terraform. To
  do this:
	1. Update `backup_config.retention_days` to the new value in Terraform.
	1. Before applying the run, contact support to change
	   `backup_config.retention_days` to the new value.
	1. Apply the changes in Terraform. A Terraform `READ` operation will
	   complete, recognize the existing value, and update the tf state.
* Temporarily remove management of the `backup_config.retention_days` from
  Terraform, update it via ticket, and then add it back. To do this:
	1. Remove management of `backup_config.retention_days` from the
	`cockroach_cluster` resource
	1. Run the apply. Nothing will change but Terraform is no longer managing
	   that value.
	1. Open a support ticket to update `backup_config.retention_days` to the new
	   value.
	1. (Optional) Add `backup_config.retention_days` back to the Terraform
	   config with the new value and apply the no-op update.
