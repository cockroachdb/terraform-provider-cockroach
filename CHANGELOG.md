# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
