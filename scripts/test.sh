#!/usr/bin/env bash
set -euo pipefail

# Runs every *_test.sh under scripts/. Each test file is a standalone bash
# script that sources test_helpers.sh and ends with print_results.

cd "$(dirname "${BASH_SOURCE[0]}")"

failed=()
while IFS= read -r test_file; do
  echo "=== ${test_file#./}"
  if bash "$test_file"; then
    echo ""
  else
    failed+=("${test_file#./}")
    echo ""
  fi
done < <(find . -mindepth 1 -name '*_test.sh' | sort)

if [[ ${#failed[@]} -gt 0 ]]; then
  echo "Failed: ${failed[*]}"
  exit 1
fi

echo "All script tests passed."
