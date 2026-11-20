#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
SCRIPT="$PWD/verify-tag.sh"
source ../test_helpers.sh

expect_success_output "accepts the expected tag" \
  "Tagged v1.27.0." \
  env TAG_CREATED=true TAG=v1.27.0 EXPECTED_TAG=v1.27.0 bash "$SCRIPT"

# The autotag action exits 0 with tag_created=false when the changelog isn't
# releasable, so this is the case the script exists for.
expect_failure_output "rejects a run that created no tag" \
  "No tag was created" \
  env TAG_CREATED=false TAG= EXPECTED_TAG=v1.27.0 bash "$SCRIPT"

expect_failure_output "rejects a missing tag_created" \
  "No tag was created" \
  env -u TAG_CREATED TAG=v1.27.0 EXPECTED_TAG=v1.27.0 bash "$SCRIPT"

expect_failure_output "rejects a tag that is not the dispatched version" \
  "Tagged v1.26.0, expected v1.27.0." \
  env TAG_CREATED=true TAG=v1.26.0 EXPECTED_TAG=v1.27.0 bash "$SCRIPT"

print_results
