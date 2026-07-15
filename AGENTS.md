# Agent guide

This repository contains `terraform-provider-cockroach`, the
[Terraform provider](https://registry.terraform.io/providers/cockroachdb/cockroach/latest)
for the CockroachDB Cloud API. The provider code lives in `internal/provider`,
with a mocked API client in `mock` used by integration tests.

## Where to look

- **Development setup, debugging, and testing**: [DEVELOPMENT.md](DEVELOPMENT.md)
- **Release process**: [RELEASE.md](RELEASE.md)
- **User-facing overview and examples**: [README.md](README.md) and `examples/`

## Key commands

- `make install` — build the provider binary and copy it to your path
- `make generate` — regenerate the docs under `docs/`
- `make testacc` — run acceptance tests against the live CockroachDB Cloud API
  (requires `COCKROACH_API_KEY`; creates real resources)

## Conventions

- The files under `docs/` are generated. Don't edit them by hand — edit the
  `Description`/`MarkdownDescription` fields in the resource schemas and run
  `make generate`.
- Changes should be accompanied by an entry under `[Unreleased]` in
  `CHANGELOG.md`.
- Each resource and data source needs both integration tests (mocked API) and
  acceptance tests (live API). See the Testing section of
  [DEVELOPMENT.md](DEVELOPMENT.md) for details.
