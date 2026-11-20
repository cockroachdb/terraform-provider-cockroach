# Releasing `terraform-provider-cockroach`

This is the canonical runbook for releasing a new version of the provider.

There's currently no set release cadence. Whenever a couple features and/or bug
fixes are ready, we kick off a release.

Releases are performed by CRL employees with write access to this repository,
and take two people: one to dispatch the workflow and one to approve the run.

## Release steps

1. **Check `CHANGELOG.md`.** Everything you want to ship should already be
   under the `## [Unreleased]` header. Don't add a version header — the release
   workflow writes that for you. Versions follow
   [semver](https://semver.org/) semantics.

2. **Dispatch the [Release](https://github.com/cockroachdb/terraform-provider-cockroach/actions/workflows/release.yml)
   workflow from `main`.** Enter the version without the `v` prefix, for
   example `1.16.0`. That's the whole release — the workflow runs the
   acceptance tests, writes the changelog header, tags, and publishes.

3. **Get the run approved.** The run pauses immediately and waits for a
   reviewer. With the `release` environment set up as described under
   [Repository setup](#the-release-environment), it can't be you. Once someone
   approves, the rest runs unattended.

4. **Confirm the release is published.** The new version should show up on the
   [Terraform provider registry](https://registry.terraform.io/providers/cockroachdb/cockroach/latest)
   shortly after — usually within minutes, though it can occasionally take up
   to a day.

5. **Announce the release.** If you're a CRL employee, post in the internal
   #cc-terraform Slack channel letting stakeholders know what was released.

## What the workflow does

| Job               | What it does                                                           |
| ----------------- | ---------------------------------------------------------------------- |
| `verify`          | Waits for an approver, then pins the SHA at the tip of main and checks the version is unused and the changelog is releasable |
| `acceptance-test` | Runs the full suite against the live Cloud API on the pinned SHA. Takes about an hour and creates real clusters |
| `stamp-changelog` | Commits `## [version] - date` to main, on top of the pinned SHA        |
| `tag`             | Tags the stamped commit                                                 |
| `goreleaser`      | Builds, signs, and publishes                                            |

The commit that gets tagged is the commit that passed the tests, plus a
changelog-only diff. The workflow checks that diff is changelog-only before it
tags.

### Nothing untested gets tagged

The release commit is pinned at the start, and the changelog commit is pushed
as a fast-forward of it. If anything lands on main while the tests are running,
that push is rejected and no tag is created. You'll get an error naming both
SHAs; re-dispatch to test the new tip.

## When something goes wrong

**Acceptance tests fail.** Nothing has been written — no changelog commit, no
tag. Fix the problem, merge the fix, and dispatch again.

**A test failed on a known flake.** Dispatch again with `skip_tests` checked.
This skips the suite entirely, so only use it when you've already established
the failure is not real.

**Anything fails after the changelog was stamped.** Don't re-dispatch — the
version header is already on main, so `verify` will reject the run. Use
**Re-run failed jobs** on the workflow run instead. The jobs that already
succeeded keep their outputs, so the retry picks up from where it stopped. This
is also the path when GoReleaser fails after the tag was pushed.

**Nobody approved and the run timed out.** Waiting runs expire after 30 days.
Nothing was written, so just dispatch again.

## Tagging by hand

The release workflow is the supported path. If you do push a `v*` tag yourself,
the `pre-release-ci` workflow runs the acceptance suite on it — after the fact,
so it tells you whether the tag was good rather than preventing a bad one. You
then need to publish separately.

## Repository setup

### The `release` environment

The `verify` job sets `environment: release`, which is what makes a release
take two people. The approval requirement lives in the environment's settings,
not in the workflow. **If the environment has no reviewers configured, the
workflow still runs — it just doesn't stop for anyone.** A missing environment
is created automatically with no protection rules, so there's no error to tell
you the gate is off.

Under Settings → Environments → `release`:

- **Required reviewers** — a team, or several people. One person isn't enough,
  since a release stalls whenever that person is the one dispatching.
- **Prevent self-review** — this is the part that requires two people. Without
  it the dispatcher can approve their own run.

### Secrets

`GPG_PRIVATE_KEY` and `PASSPHRASE` sign the release. `COCKROACH_API_KEY` and
`COCKROACH_CONTINUUM_API_KEY` run the acceptance tests. All four are
repository-level. Moving them into the `release` environment would make
`goreleaser` and `acceptance-test` reference that environment too, which asks
for a separate approval per job.

`stamp-changelog` pushes to main with the default `GITHUB_TOKEN`. If main is
ever protected against direct pushes, add a `RELEASE_PUSH_TOKEN` secret holding
a token that's allowed to bypass it; the workflow picks it up automatically.
