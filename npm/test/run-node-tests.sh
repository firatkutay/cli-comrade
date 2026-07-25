#!/usr/bin/env bash
# Runs every *.test.js file in this directory with plain `node` (no test
# framework dependency) and fails loudly if any of them exits non-zero.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

status=0
for test_file in "${SCRIPT_DIR}"/*.test.js; do
  echo "--- running $(basename "${test_file}") ---"
  if ! node "${test_file}"; then
    echo "FAILED: ${test_file}" >&2
    status=1
  fi
done

exit "${status}"
