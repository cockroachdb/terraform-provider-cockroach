#!/usr/bin/env bash
set -euo pipefail

# Commits the stamped CHANGELOG.md onto main. Expects the working tree to hold
# the result of stamp-changelog-version.sh and nothing else.
#
# The push is a fast-forward of TESTED_SHA, so git rejects it if main moved
# while the acceptance tests were running. That rejection is what keeps an
# untested commit from being tagged. The explicit check below only turns it
# into a legible error.
#
# Inputs (environment):
#   VERSION     release version without the "v" prefix, e.g. 1.27.0
#   TESTED_SHA  commit the acceptance tests ran against
#   DRY_RUN     set to 1 to run the checks without committing or pushing
#
# Outputs:
#   sha  the stamped commit, written to $GITHUB_OUTPUT when set

log_error()  { echo "::error title=${2:-Error}::$1"; }
log_notice() { echo "::notice title=${2:-Notice}::$1"; }

version="${VERSION:-}"
tested_sha="${TESTED_SHA:-}"
dry_run="${DRY_RUN:-0}"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  log_error "VERSION must be x.y.z without a 'v' prefix, got '${version}'."
  exit 1
fi

if [[ -z "$tested_sha" ]]; then
  log_error "TESTED_SHA is not set."
  exit 1
fi

# The tagged commit won't be the commit that was tested. This is what makes
# that acceptable: the only difference is the changelog. Checked with status
# rather than diff so staged and untracked files count too.
changed=$(git status --porcelain=v1 --untracked-files=all | cut -c4-)
if [[ "$changed" != "CHANGELOG.md" ]]; then
  log_error "Expected only CHANGELOG.md to change, got: ${changed:-no changes}"
  exit 1
fi

main_sha=$(git ls-remote origin refs/heads/main | cut -f1)
if [[ "$main_sha" != "$tested_sha" ]]; then
  log_error "main moved from ${tested_sha} to ${main_sha} during the test run. Re-dispatch to test the new tip."
  exit 1
fi

if [[ "$dry_run" == "1" ]]; then
  log_notice "DRY_RUN is set, so the stamp was not committed or pushed."
  exit 0
fi

git config user.name 'github-actions[bot]'
git config user.email 'github-actions[bot]@users.noreply.github.com'
git commit --message "Release ${version}" -- CHANGELOG.md
git push origin "HEAD:main"

sha=$(git rev-parse HEAD)
echo "sha=${sha}" >> "${GITHUB_OUTPUT:-/dev/null}"
log_notice "Pushed the v${version} changelog stamp to main as ${sha}."
