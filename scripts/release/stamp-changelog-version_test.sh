#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
SCRIPT="$PWD/stamp-changelog-version.sh"
source ../test_helpers.sh

new_fixture
expect_success "stamps the version header" \
  env VERSION=1.27.0 RELEASE_DATE=2026-11-20 bash "$SCRIPT"
expect_equal "inserts the header below [Unreleased]" \
  "## [Unreleased]
## [1.27.0] - 2026-11-20
## [1.26.0] - 2026-11-13" \
  "$(grep --extended-regexp '^## \[' CHANGELOG.md)"
expect_equal "leaves the unreleased entries under the new header" \
  "- A new thing" \
  "$(awk '/^## \[1\.27\.0\]/,/^## \[1\.26\.0\]/' CHANGELOG.md | awk '/^- /')"

new_fixture
expect_success "defaults the date to today" \
  env VERSION=1.27.0 bash "$SCRIPT"
expect_success_output "writes today's date" "## [1.27.0] - $(date -u +%F)" cat CHANGELOG.md

new_fixture
expect_failure_output "rejects a v prefix" \
  "VERSION must be x.y.z" \
  env VERSION=v1.27.0 bash "$SCRIPT"

new_fixture
expect_failure_output "rejects a malformed date" \
  "RELEASE_DATE must be YYYY-MM-DD" \
  env VERSION=1.27.0 RELEASE_DATE=11/20/2026 bash "$SCRIPT"

new_fixture
expect_failure_output "rejects a missing changelog" \
  "does not exist or is not readable" \
  env VERSION=1.27.0 CHANGELOG_PATH=nope.md bash "$SCRIPT"

new_fixture
grep --invert-match --fixed-strings '## [Unreleased]' CHANGELOG.md > tmp && mv tmp CHANGELOG.md
expect_failure_output "rejects a changelog with no [Unreleased] header" \
  "No '## [Unreleased]' header" \
  env VERSION=1.27.0 bash "$SCRIPT"

new_fixture
expect_failure_output "rejects a version that is already released" \
  "already has a '## [1.26.0]' header" \
  env VERSION=1.26.0 bash "$SCRIPT"

new_fixture
cat > CHANGELOG.md <<'CHANGELOG'
# Changelog

## [Unreleased]

## [1.26.0] - 2026-11-13

### Fixed

- An old thing
CHANGELOG
expect_failure_output "rejects an empty [Unreleased] section" \
  "there is nothing to release" \
  env VERSION=1.27.0 bash "$SCRIPT"

print_results
