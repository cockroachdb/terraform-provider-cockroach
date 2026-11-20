#!/usr/bin/env bash
set -euo pipefail

# Checks the autotag action actually created the tag we asked for. It exits 0
# with tag_created=false when the changelog isn't in a releasable state, so a
# successful step is not enough on its own.
#
# Inputs (environment):
#   TAG_CREATED   the action's tag_created output
#   TAG           the action's tag output
#   EXPECTED_TAG  the tag the release was dispatched for, e.g. v1.27.0

log_error() { echo "::error title=${2:-Error}::$1"; }

tag_created="${TAG_CREATED:-}"
tag="${TAG:-}"
expected_tag="${EXPECTED_TAG:-}"

if [[ "$tag_created" != "true" ]]; then
  log_error "No tag was created. The changelog on the stamped commit is not in a releasable state."
  exit 1
fi

if [[ "$tag" != "$expected_tag" ]]; then
  log_error "Tagged ${tag}, expected ${expected_tag}."
  exit 1
fi

echo "Tagged ${tag}." >> "${GITHUB_STEP_SUMMARY:-/dev/null}"
echo "Tagged ${tag}."
