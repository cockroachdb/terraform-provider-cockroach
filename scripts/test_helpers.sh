#!/usr/bin/env bash
# Shared helpers for the shell script tests under scripts/.
#
# Usage:
#   cd "$(dirname "${BASH_SOURCE[0]}")"
#   source ../test_helpers.sh
#   expect_success "name" env VAR=value bash ./some-script.sh
#   print_results
#
# Modeled on the harness in cockroachdb/actions, trimmed to what we use here.

PASS=0
FAIL=0
FIXTURE_DIRS=()

_run_test() {
  local name="$1" expect_zero="$2" expected_output="$3"
  shift 3

  local output exit_code
  output=$("$@" 2>&1) && exit_code=0 || exit_code=$?

  if [[ "$expect_zero" == "yes" && "$exit_code" -ne 0 ]]; then
    _fail "$name" "expected exit 0, got ${exit_code}" "$output"
    return
  fi
  if [[ "$expect_zero" == "no" && "$exit_code" -eq 0 ]]; then
    _fail "$name" "expected a non-zero exit, got 0" "$output"
    return
  fi
  if [[ -n "$expected_output" ]] && ! printf '%s\n' "$output" | grep --quiet --fixed-strings -- "$expected_output"; then
    _fail "$name" "expected output containing: ${expected_output}" "$output"
    return
  fi

  echo "PASS: $name"
  PASS=$((PASS + 1))
}

_fail() {
  echo "FAIL: $1 — $2"
  echo "  output: $3"
  FAIL=$((FAIL + 1))
}

# expect_success <name> <command...>
expect_success() {
  local name="$1"; shift
  _run_test "$name" yes "" "$@"
}

# expect_success_output <name> <expected substring> <command...>
expect_success_output() {
  local name="$1" expected="$2"; shift 2
  _run_test "$name" yes "$expected" "$@"
}

# expect_failure_output <name> <expected substring> <command...>
expect_failure_output() {
  local name="$1" expected="$2"; shift 2
  _run_test "$name" no "$expected" "$@"
}

# expect_equal <name> <expected> <actual>
expect_equal() {
  local name="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "PASS: $name"
    PASS=$((PASS + 1))
  else
    _fail "$name" "expected '${expected}'" "$actual"
  fi
}

# expect_step_output <name> <key> <expected value>
# Reads the last value written for the key in $GITHUB_OUTPUT.
expect_step_output() {
  local name="$1" key="$2" expected="$3"
  local actual="" matches=""
  # grep is the if condition so a missing key reports a failure rather than
  # killing the test file under set -e.
  if [[ -f "${GITHUB_OUTPUT:-}" ]] && matches=$(grep --fixed-strings -- "${key}=" "$GITHUB_OUTPUT"); then
    actual=$(printf '%s\n' "$matches" | tail -1 | cut -d= -f2-)
  fi
  expect_equal "$name" "$expected" "$actual"
}

# Creates a throwaway repo with a bare origin, a releasable CHANGELOG.md, and
# one commit pushed to main. Leaves the shell inside the working clone and sets
# $FIXTURE_SHA to the commit on main.
#
# Each test case gets its own fixture so the scripts, which commit and push,
# can't affect each other.
new_fixture() {
  local dir
  dir=$(mktemp -d)
  FIXTURE_DIRS+=("$dir")

  # --initial-branch so the fixture doesn't inherit whatever init.defaultBranch
  # the machine has. A bare repo pointing HEAD at master leaves clones of it on
  # an unborn branch.
  git init --quiet --bare --initial-branch=main "$dir/origin.git"
  git clone --quiet "$dir/origin.git" "$dir/work" 2>/dev/null
  cd "$dir/work" || exit 1
  git config user.email test@example.com
  git config user.name 'Test User'

  cat > CHANGELOG.md <<'CHANGELOG'
# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added

- A new thing

## [1.26.0] - 2026-11-13

### Fixed

- An old thing
CHANGELOG

  git add CHANGELOG.md
  git commit --quiet --message 'Initial commit'
  git push --quiet origin main

  export FIXTURE_SHA
  FIXTURE_SHA=$(git rev-parse HEAD)
  export GITHUB_OUTPUT="$dir/github_output"
  : > "$GITHUB_OUTPUT"
}

cleanup_fixtures() {
  for dir in ${FIXTURE_DIRS[@]+"${FIXTURE_DIRS[@]}"}; do
    rm -rf "$dir"
  done
}

print_results() {
  cleanup_fixtures
  echo ""
  echo "Results: ${PASS} passed, ${FAIL} failed"
  [[ "$FAIL" -eq 0 ]] || exit 1
}
