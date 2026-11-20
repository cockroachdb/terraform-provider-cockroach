#!/usr/bin/env bash
set -euo pipefail

# Checks the checked-out commit is the tested commit plus a changelog-only
# change, which is the condition for tagging it.
#
# Inputs (environment):
#   TESTED_SHA  commit the acceptance tests ran against

log_error() { echo "::error title=${2:-Error}::$1"; }

tested_sha="${TESTED_SHA:-}"

if [[ -z "$tested_sha" ]]; then
  log_error "TESTED_SHA is not set."
  exit 1
fi

parent=$(git rev-parse HEAD^)
if [[ "$parent" != "$tested_sha" ]]; then
  log_error "Stamped commit's parent is ${parent}, expected the tested commit ${tested_sha}."
  exit 1
fi

changed=$(git diff --name-only HEAD^ HEAD)
if [[ "$changed" != "CHANGELOG.md" ]]; then
  log_error "Stamped commit changes more than CHANGELOG.md: ${changed}"
  exit 1
fi

echo "$(git rev-parse HEAD) is ${tested_sha} plus a changelog-only change."
