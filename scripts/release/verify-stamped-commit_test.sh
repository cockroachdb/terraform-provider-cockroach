#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
SCRIPT="$PWD/verify-stamped-commit.sh"
STAMP="$PWD/stamp-changelog-version.sh"
source ../test_helpers.sh

stamp_and_commit() {
  VERSION=1.27.0 bash "$STAMP" > /dev/null
  git commit --quiet --message 'Release 1.27.0' -- CHANGELOG.md
}

new_fixture
stamp_and_commit
expect_success_output "accepts a changelog-only commit on the tested commit" \
  "plus a changelog-only change" \
  env TESTED_SHA="$FIXTURE_SHA" bash "$SCRIPT"

new_fixture
stamp_and_commit
expect_failure_output "rejects a missing TESTED_SHA" \
  "TESTED_SHA is not set" \
  env -u TESTED_SHA bash "$SCRIPT"

new_fixture
git commit --quiet --allow-empty --message 'Landed during the test run'
stamp_and_commit
expect_failure_output "rejects a stamp that is not on the tested commit" \
  "expected the tested commit ${FIXTURE_SHA}" \
  env TESTED_SHA="$FIXTURE_SHA" bash "$SCRIPT"

new_fixture
VERSION=1.27.0 bash "$STAMP" > /dev/null
echo 'sneaky' > other.txt
git add CHANGELOG.md other.txt
git commit --quiet --message 'Release 1.27.0 and something else'
expect_failure_output "rejects a stamp that touches another file" \
  "changes more than CHANGELOG.md" \
  env TESTED_SHA="$FIXTURE_SHA" bash "$SCRIPT"

print_results
