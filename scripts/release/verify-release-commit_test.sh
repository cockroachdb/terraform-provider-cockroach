#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
SCRIPT="$PWD/verify-release-commit.sh"
source ../test_helpers.sh

new_fixture
expect_success_output "accepts the tip of main" \
  "Releasing v1.27.0 from ${FIXTURE_SHA}" \
  env VERSION=1.27.0 bash "$SCRIPT"
expect_step_output "reports the release commit" sha "$FIXTURE_SHA"

new_fixture
expect_failure_output "rejects a v prefix" \
  "VERSION must be x.y.z" \
  env VERSION=v1.27.0 bash "$SCRIPT"

new_fixture
expect_failure_output "rejects a missing version" \
  "VERSION must be x.y.z" \
  env -u VERSION bash "$SCRIPT"

new_fixture
git commit --quiet --allow-empty --message 'Unpushed work'
expect_failure_output "rejects a commit that is not the tip of main" \
  "Releases are cut from the tip of main" \
  env VERSION=1.27.0 bash "$SCRIPT"

new_fixture
git tag v1.27.0
git push --quiet origin v1.27.0
expect_failure_output "rejects an already released version" \
  "Tag v1.27.0 already exists" \
  env VERSION=1.27.0 bash "$SCRIPT"

print_results
