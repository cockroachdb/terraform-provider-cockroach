#!/usr/bin/env bash
set -euo pipefail

# Stamps a release version header into CHANGELOG.md by inserting
# "## [VERSION] - DATE" between "## [Unreleased]" and the unreleased entries.
#
# Inputs (environment):
#   VERSION         release version without the "v" prefix, e.g. 1.27.0
#   RELEASE_DATE    YYYY-MM-DD, defaults to today in UTC
#   CHANGELOG_PATH  defaults to CHANGELOG.md

# GitHub Actions log annotations.
log_error()  { echo "::error title=${2:-Error}::$1"; }
log_notice() { echo "::notice title=${2:-Notice}::$1"; }

version="${VERSION:-}"
release_date="${RELEASE_DATE:-$(date -u +%F)}"
changelog="${CHANGELOG_PATH:-CHANGELOG.md}"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  log_error "VERSION must be x.y.z without a 'v' prefix, got '${version}'."
  exit 1
fi

if [[ ! "$release_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
  log_error "RELEASE_DATE must be YYYY-MM-DD, got '${release_date}'."
  exit 1
fi

if [[ ! -r "$changelog" ]]; then
  log_error "Changelog file '${changelog}' does not exist or is not readable."
  exit 1
fi

if ! grep --quiet --extended-regexp '^## \[Unreleased\]' "$changelog"; then
  log_error "No '## [Unreleased]' header in ${changelog}."
  exit 1
fi

if grep --quiet --extended-regexp "^## \[${version}\]" "$changelog"; then
  log_error "${changelog} already has a '## [${version}]' header. Pick a new version, or the release already happened."
  exit 1
fi

# The entries to release are the non-blank lines between [Unreleased] and the
# next version header. Releasing with nothing there would tag an empty version.
unreleased_entries=$(awk '
  /^## \[Unreleased\]/ { in_unreleased = 1; next }
  in_unreleased && /^## \[/ { exit }
  in_unreleased && /[^[:space:]]/ { print }
' "$changelog")

if [[ -z "$unreleased_entries" ]]; then
  log_error "Nothing under '## [Unreleased]' in ${changelog}, so there is nothing to release."
  exit 1
fi

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

awk -v header="## [${version}] - ${release_date}" '
  !seen && /^## \[Unreleased\]/ { seen = 1; print; next }
  seen && !inserted && /[^[:space:]]/ { print header; print ""; inserted = 1 }
  { print }
' "$changelog" > "$tmp"

if ! grep --quiet --extended-regexp "^## \[${version}\] - ${release_date}$" "$tmp"; then
  log_error "Failed to insert the '## [${version}]' header into ${changelog}."
  exit 1
fi

cat "$tmp" > "$changelog"
log_notice "Stamped ${changelog} with '## [${version}] - ${release_date}'."
