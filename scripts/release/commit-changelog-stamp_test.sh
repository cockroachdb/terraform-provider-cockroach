#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
SCRIPT="$PWD/commit-changelog-stamp.sh"
STAMP="$PWD/stamp-changelog-version.sh"
source ../test_helpers.sh

new_fixture
VERSION=1.27.0 bash "$STAMP" > /dev/null
expect_success "pushes the stamp to main" \
  env VERSION=1.27.0 TESTED_SHA="$FIXTURE_SHA" bash "$SCRIPT"
expect_step_output "reports the stamped commit" sha "$(git rev-parse HEAD)"
expect_equal "fast-forwards main onto the stamp" \
  "$(git rev-parse HEAD)" \
  "$(git ls-remote origin refs/heads/main | cut -f1)"
expect_equal "builds the stamp on the tested commit" \
  "$FIXTURE_SHA" \
  "$(git rev-parse HEAD^)"

new_fixture
VERSION=1.27.0 bash "$STAMP" > /dev/null
expect_success_output "leaves main alone under DRY_RUN" \
  "the stamp was not committed or pushed" \
  env VERSION=1.27.0 TESTED_SHA="$FIXTURE_SHA" DRY_RUN=1 bash "$SCRIPT"
expect_equal "main is still the tested commit" \
  "$FIXTURE_SHA" \
  "$(git ls-remote origin refs/heads/main | cut -f1)"

new_fixture
VERSION=1.27.0 bash "$STAMP" > /dev/null
expect_failure_output "rejects a v prefix" \
  "VERSION must be x.y.z" \
  env VERSION=v1.27.0 TESTED_SHA="$FIXTURE_SHA" bash "$SCRIPT"

new_fixture
VERSION=1.27.0 bash "$STAMP" > /dev/null
expect_failure_output "rejects a missing TESTED_SHA" \
  "TESTED_SHA is not set" \
  env VERSION=1.27.0 bash "$SCRIPT"

new_fixture
expect_failure_output "rejects an unstamped working tree" \
  "Expected only CHANGELOG.md to change, got: no changes" \
  env VERSION=1.27.0 TESTED_SHA="$FIXTURE_SHA" bash "$SCRIPT"

new_fixture
VERSION=1.27.0 bash "$STAMP" > /dev/null
echo 'sneaky' > other.txt
git add other.txt
expect_failure_output "rejects a staged file alongside the stamp" \
  "Expected only CHANGELOG.md to change" \
  env VERSION=1.27.0 TESTED_SHA="$FIXTURE_SHA" bash "$SCRIPT"

new_fixture
VERSION=1.27.0 bash "$STAMP" > /dev/null
echo 'sneaky' > other.txt
expect_failure_output "rejects an untracked file alongside the stamp" \
  "Expected only CHANGELOG.md to change" \
  env VERSION=1.27.0 TESTED_SHA="$FIXTURE_SHA" bash "$SCRIPT"

# The guarantee the release pipeline rests on: work landing on main during the
# test run has to stop the stamp.
new_fixture
VERSION=1.27.0 bash "$STAMP" > /dev/null
git clone --quiet --branch main "$(git remote get-url origin)" ../other
git -C ../other -c user.email=t@e.com -c user.name=T \
  commit --quiet --allow-empty --message 'Landed during the test run'
git -C ../other push --quiet origin HEAD:main
expect_failure_output "rejects a stamp when main moved" \
  "main moved from ${FIXTURE_SHA}" \
  env VERSION=1.27.0 TESTED_SHA="$FIXTURE_SHA" bash "$SCRIPT"
expect_equal "leaves the landed commit on main" \
  "$(git -C ../other rev-parse HEAD)" \
  "$(git ls-remote origin refs/heads/main | cut -f1)"

print_results
