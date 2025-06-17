# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.12.2] - 2025-06-17

### Fixed

- Mark `internal_dns` and `sql_dns` fields in cluster resources as stable after creation.

## [1.12.1] - 2025-05-01

### Fixed

- Fixed an inconsistency error caused by user role grant resources referencing service
  accounts (managed by Terraform) that are deleted externally.

## [1.12.0] - 2025-04-25

### Added

- Added support for label management in the `cockroach_cluster` and
  `cockroach_folder` resources.

- Added `full_version` attribute to the cluster data source and resource for
  fetching the full version string. (e.g. v25.1.0)

- Added AWS external ID support for both CloudWatch metric export and log export configurations.

### Fixed

- Fixed an issue where fields, in metric/log export, incorrectly marked as
computed led to miscalculation of diffs when fields were commented out or
removed from the terraform config

## [1.11.2] - 2025-02-07

### Fixed

- Add name back to allowlist creations.

## [1.11.1] - 2025-01-16

### Fixed

- Fix a state consistency error that occurred when an allowlist was created
  without a name.

## [1.11.0] - 2024-12-09

### Changed

- Remove `machine_type` from examples and clear up in docs that this field
  requires a feature flag and is not recommended for use.

### Fixed

- Update docs to reflect that maintenance windows are for Advanced clusters
  only.

- Disallow 0 from being passed for `disk_iops` and update the docs to indicate
  that omitting the attribute is the correct way to get the default behavior.

- Validate that `node_count` is not passed in with serverless cluster regions.
  Also, clear up in the documentation that this is not allowed.

### Added

- Added support for manual version control of Standard tier clusters.

## [1.10.0] - 2024-11-15

### Fixed

- Added missing import.sh files to resource docs to better indicate support of
  the Import command.

## Changed

- Remove unnecessary api call to Folders endpoint to manually validate
  `cockroach_cluster.parent_id`.

- Allow removal (deletion) of locked clusters.  The api now supports deletion
  of locked clusters so we remove the wait prior to following through with the
  deletion.

### Added

- Added support for authentication via JWT.

- Setting and fetching of `cidr_range` is now available for GCP Advanced tier
  clusters.

- Management of cluster backup settings is now supported using the
  `backup_config` attribute on the `cockroach_cluster` resource.  For more
  information, refer to
  [Cluster Backups](https://www.cockroachlabs.com/docs/cockroachcloud/managed-backups).

## [1.9.0] - 2024-10-07

- Added support for skipping [Innovation
  Releases](https://www.cockroachlabs.com/docs/cockroachcloud/upgrade-policy#innovation-releases)
  when upgrading dedicated clusters.
- Clarify cluster.private_network_visibility attribute documentation.

## Changed

- Replaced `api_oidc_config` with `jwt_issuer` resource

## [1.8.0] - 2024-09-18

### Added

- `upgrade_type` was added as an attribute for serverless clusters.
- New `STANDARD` clusters with provisioned serverless capacity.
- Ability to upgrade from `BASIC` plan to `STANDARD` plan.

### Changed

- Renamed `SERVERLESS` plan type to `BASIC`.
- Renamed `DEDICATED` plan type to `ADVANCED`.

### Fixed

- Fixed error when the primary attribute was specified in multiple regions,
  even when false in all but one region.

### Deprecated

- Deprecated support for `spend_limit` attribute in `serverless` config. Users
  can instead enforce resource limits with `usage_limits`.

## Fixed

- Updated to version 2.0.1 of cockroach-cloud-sdk-go

## [1.7.7] - 2024-06-20

- Added warning about using private endpoints with old versions.

## Fixed

- Update the user_role_grants resource docs to indicate the requirement of ORG_MEMBER.

- Fixed private_endpoint_connection documentation issues

## Added

- Enable log export for serverless clusters.

- Add services_map to private_endpoint_services resource.

## [1.7.6] - 2024-06-10

## Fixed

- Update docs for allowlist resource to clear up with cidr_mask is

- Realign variables used in the cockroach_dedicated_cluster with the README

- Removed mention of Limited Access for Azure clusters in README

- Added some example values for clarity in README

- Fix bug when reading `cockroach_private_endpoint_services.#.aws.service_name`.

## [1.7.5] - 2024-06-06

## Fixed

- Pinned version of go-releaser to version 1

- Fixed apply churn when the optional name attribute in the allowlist resource was
  not included.

## [1.7.0] - 2024-05-17

## Added

- New `cockroach_metric_export_prometheus_config` resource allows user to configure prometheus metric
  export integration in AWS and GCP cloud providers.

## [1.6.0] - 2024-05-02

## Added

- The [cockroach_api_key](https://registry.terraform.io/providers/cockroachdb/cockroach/latest/docs/resources/api_key) 
  resource was added.

- The [cockroach_service_account](https://registry.terraform.io/providers/cockroachdb/cockroach/latest/docs/resources/service_account)
  resource was added.

- Added `delete_protection` to the Cluster resource and data source. When set
  to true, attempts to delete the cluster will fail. Set to false to disable
  delete protection.

## [1.5.0] - 2024-04-26

- No changes.

## [1.4.1] - 2024-04-04

## Added

- Added `private_endpoint_connection` examples for AWS, Azure, GCP.

### Fixed

- Added any missing examples for data sources, resources and imports.

## [1.4.0] - 2024-03-22

### Added

- The
  [cockroach_folder](https://registry.terraform.io/providers/cockroachdb/cockroach/latest/docs/data-sources/folder)
  data source was added.

- The `user_role_grant` resource was added to allow management of a single role
  grant.  This resource will not affect other role grants. See
  [user_role_grants](https://registry.terraform.io/providers/cockroachdb/cockroach/latest/docs/resources/user_role_grant)
  for more information.

## [1.3.2] - 2024-03-15

### Changed

- The `private_endpoint_connection` resource can now be used to create private
  endpoint connections on every supported cloud-provider and cluster type,
  except Serverless clusters on Azure as that configuration is not yet
  available.

- Migrated the testing framework to
  https://github.com/hashicorp/terraform-plugin-testing.

- The `private_endpoint_services` resource can now be used to create private
  endpoint services on every supported cloud provider.

- Use CockroachDB v23.1 and v23.2 in tests.

### Fixed

- The `private_endpoint_services` resource could be created without populating
  the state file with the service information.

- Renamed example files to the correct name so they are automatically included
  in the docs.

## Added

- Allow the logging of requests to the cloud SDK by setting either TF_LOG or
  TF_LOG_PROVIDER envvars to either DEBUG or TRACE.

## [1.3.1] - 2023-12-01

### Fixed

- Fixed a nil pointer exception which could occur while retrying a certain
  class of api failures.
- Fixed a bug where the `log_export_config` would sometimes fail to detect changes to
  the group level `redact` field due to it being declared as a `computed` field.

## [1.3.0] - 2023-09-14

### Added

- New api_oidc_config resource allows users to configure an external JWT signer for API tokens.
  JWT API tokens are in [limited access](https://www.cockroachlabs.com/docs/v23.1/cockroachdb-feature-availability).

## [1.2.0] - 2023-08-29

### Added

- New cockroach_private_endpoint_connection resource allows users to configure
  trusted owner entries for private endpoints.

## [1.1.0] - 2023-08-15

### Added

- New folder resource and the new `parent_id` for clusters support users managing nested resources. 
Folders is in [limited access](https://www.cockroachlabs.com/docs/v23.1/cockroachdb-feature-availability).

## [1.0.0] - 2023-08-10

### Notes

- The CockroachDB Cloud Terraform Provider is now generally available with semantic versioning compatibility promises.

### Changed

- connection_params field on the connection_string data source is now an object instead of a string map.

### Fixed

- Fixed an issue where changing `num_virtual_cpus` on a `cockroach_cluster` resource would fail to scale the cluster
  and would result in an inconsistent state error.
- Added validation to prevent multiple serverless regions from being marked as "primary", which could result in an
  inconsistent state error.
- Fixed "not a valid value" errors that occurred when reading recently added enum values, such as cluster status.

## [0.7.0] - 2023-07-13

### Added

- New cockroach_version_deferral resource allows users to defer automated minor version
  upgrades by a fix 60-day interval.
- Allowlists and AWS PrivateLink services are now supported for serverless clusters. PrivateLink for serverless clusters
  is in [limited access](https://www.cockroachlabs.com/docs/v23.1/cockroachdb-feature-availability).

### Fixed

- Fixed an issue where the provider could crash when importing a serverless cluster.
- Fixed an issue where usage limits weren't reported properly in cockroach_cluster data sources.

## [0.6.0] - 2023-06-21

### Added

- New cockroach_maintenance_window resource allows users to define a maintenance window in which dedicated clusters will
  apply patch updates.

### Changed

- Read operations are now automatically retried if the response is a 500-level error.

### Fixed

- Reading SQL User, IP Allowlist, Role Grant, and Database resources no longer results in an error if their
  pagination index is outside the default limit.
- Regions can no longer be marked as primary in dedicated clusters. Currently, primary regions are a serverless-only
  concept and attempting to set a dedicated region as primary would result in an inconsistent state error.
- Metric Export and Log Export resources now wait for pending cluster updates to finish before attempting Create and
  Update operations.

## [0.5.0] - 2023-05-12

### Added

- Updated Cockroach Cloud SDK to version 1.1.0 which incorporates Limited Access Features. Notably the ability to pass
  AZURE as a cloud provider is now possible.

### Fixed

- Updated cockroach_dedicated_cluster workflow example in the README to use the
current var names and add required values that were previously missing.
