# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- Renamed example files to the correct name so they are automatically included
  in the docs.

### Added

- New `STANDARD` clusters with provisioned capacity on shared hardware.
- New `shared` attribute which should be used in place of `serverless`.

### Changed

- Renamed `SERVERLESS` plan type to `BASIC`.
- Renamed `DEDICATED` plan type to `ADVANCED`.

### Deprecated

- Deprecated support for `serverless` attribute. Users should rename to
  `shared` and convert any `spend_limit` attribute.
- Deprecated support for `spend_limit` attribute in `serverless` config. Users
  can instead enforce resource limits with `usage_limits`.

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
