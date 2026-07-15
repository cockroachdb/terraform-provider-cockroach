# Releasing `terraform-provider-cockroach`

This is the canonical runbook for releasing a new version of the provider.

There's currently no set release cadence. Whenever a couple features and/or bug
fixes are ready, we kick off a release.

Releases are performed by CRL employees with write access to this repository.

## Release steps

1. **Update `CHANGELOG.md`.** Add a version header below the `[Unreleased]`
   header, moving the unreleased entries under it. The changelog version does
   **not** include the `v` prefix (git tags do). Versions follow
   [semver](https://semver.org/) semantics. The `[Unreleased]` header stays at
   the top. The diff should look like this:

   ```diff
   --- a/CHANGELOG.md
   +++ b/CHANGELOG.md
   @@ -7,6 +7,8 @@ and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0

    ## [Unreleased]

   +## [1.16.0] - 2026-10-29
   +
    ### Added
   ```

2. **Open a PR with the changelog change and merge it.** Merges are not gated
   on acceptance tests, so it's possible a previous commit has broken them.
   Consider running the
   [Manually Run Acceptance Tests](https://github.com/cockroachdb/terraform-provider-cockroach/actions/workflows/acceptance-tests.yml)
   workflow on your branch before merging: click "Run workflow" and select the
   branch.

3. **Tag the merged commit and push the tag.** The tag includes the `v` prefix:

   ```shell
   git checkout main
   git pull
   git tag v1.16.0
   git push origin v1.16.0
   ```

4. **Wait for `pre-release-ci` to pass.** Pushing the tag triggers the
   [`pre-release-ci`](https://github.com/cockroachdb/terraform-provider-cockroach/actions/workflows/pre-release-ci.yaml)
   workflow, which runs the full acceptance test suite against the live
   CockroachDB Cloud API. **There is no automated gate** — verify that
   `pre-release-ci` has passed on your tag before proceeding to the next step.

5. **Dispatch the release workflow.** Go to the
   [release action](https://github.com/cockroachdb/terraform-provider-cockroach/actions/workflows/release.yml),
   click "Run workflow", and select the tag you just pushed from the dropdown.
   This builds, signs, and publishes the release with GoReleaser.

6. **Confirm the release is published.** The new version should show up on the
   [Terraform provider registry](https://registry.terraform.io/providers/cockroachdb/cockroach/latest)
   shortly after — usually within minutes, though it can occasionally take up
   to a day.

7. **Announce the release.** If you're a CRL employee, post in the internal
   #cc-terraform Slack channel letting stakeholders know what was released.
