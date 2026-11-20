#!/usr/bin/env bash
set -euo pipefail

# Checks that a release can be cut from the checked-out commit, and reports
# which commit that is. Run from the repository root with a full clone.
#
# Inputs (environment):
#   VERSION  release version without the "v" prefix, e.g. 1.27.0
#
# Outputs:
#   sha  the resolved release commit, written to $GITHUB_OUTPUT when set

log_error() { echo "::error title=${2:-Error}::$1"; }

version="${VERSION:-}"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  log_error "VERSION must be x.y.z without a 'v' prefix, got '${version}'."
  exit 1
fi

sha=$(git rev-parse HEAD)
main_sha=$(git ls-remote origin refs/heads/main | cut -f1)
if [[ "$sha" != "$main_sha" ]]; then
  log_error "Releases are cut from the tip of main. Checked-out commit is ${sha}, main is ${main_sha}."
  exit 1
fi

if [[ -n "$(git ls-remote --tags origin "refs/tags/v${version}")" ]]; then
  log_error "Tag v${version} already exists."
  exit 1
fi

# The ":-/dev/null" fallbacks make this a no-op outside CI.
echo "sha=${sha}" >> "${GITHUB_OUTPUT:-/dev/null}"
echo "Releasing v${version} from ${sha}." >> "${GITHUB_STEP_SUMMARY:-/dev/null}"
echo "Releasing v${version} from ${sha}."
