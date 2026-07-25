#!/usr/bin/env bash
# Tests scripts/build-npm-packages.sh's failure modes: a malformed version
# string, and a dist/ directory missing a required platform's binary. Both
# must fail loudly (non-zero exit, actionable stderr message) rather than
# silently producing a partial/broken package set.
set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." >/dev/null 2>&1 && pwd)"
BUILD_SCRIPT="${REPO_ROOT}/scripts/build-npm-packages.sh"

failures=0

fail() {
  echo "FAIL: $1" >&2
  failures=$((failures + 1))
}

pass() {
  echo "PASS: $1"
}

TMP_DIR="$(mktemp -d)"
cleanup() { rm -rf "${TMP_DIR}"; }
trap cleanup EXIT

# --- fixture: a "complete" fake dist/ with all 5 platform binaries -------

make_complete_dist() {
  local dist_dir="$1"
  mkdir -p \
    "${dist_dir}/comrade_linux_amd64_v1" \
    "${dist_dir}/comrade_linux_arm64_v8.0" \
    "${dist_dir}/comrade_darwin_amd64_v1" \
    "${dist_dir}/comrade_darwin_arm64_v8.0" \
    "${dist_dir}/comrade_windows_amd64_v1"
  echo "fake-linux-amd64-binary" >"${dist_dir}/comrade_linux_amd64_v1/comrade"
  echo "fake-linux-arm64-binary" >"${dist_dir}/comrade_linux_arm64_v8.0/comrade"
  echo "fake-darwin-amd64-binary" >"${dist_dir}/comrade_darwin_amd64_v1/comrade"
  echo "fake-darwin-arm64-binary" >"${dist_dir}/comrade_darwin_arm64_v8.0/comrade"
  echo "fake-windows-amd64-binary" >"${dist_dir}/comrade_windows_amd64_v1/comrade.exe"
}

# --- Test 1: malformed version strings must fail, valid dist untouched ---

complete_dist="${TMP_DIR}/dist-complete"
make_complete_dist "${complete_dist}"

stdout_log="${TMP_DIR}/assemble-stdout.log"
stderr_log="${TMP_DIR}/assemble-stderr.log"

for bad_version in "v1.2.3" "1.2" "abc" "" "1.2.3.4"; do
  out_dir="${TMP_DIR}/out-badversion-${RANDOM}"
  if "${BUILD_SCRIPT}" "${bad_version}" "${complete_dist}" "${out_dir}" >"${stdout_log}" 2>"${stderr_log}"; then
    fail "malformed version \"${bad_version}\" was accepted (expected non-zero exit)"
  else
    if grep -qi "version" "${stderr_log}"; then
      pass "malformed version \"${bad_version}\" rejected with an actionable message"
    else
      fail "malformed version \"${bad_version}\" was rejected but stderr didn't mention \"version\": $(cat "${stderr_log}")"
    fi
  fi
  [ -d "${out_dir}" ] && fail "malformed version \"${bad_version}\" must not leave a partial output directory"
done

# --- Test 2: a missing platform binary must fail loudly, naming the gap --

incomplete_dist="${TMP_DIR}/dist-incomplete"
make_complete_dist "${incomplete_dist}"
rm -rf "${incomplete_dist}/comrade_darwin_arm64_v8.0"

out_dir="${TMP_DIR}/out-incomplete"
if "${BUILD_SCRIPT}" "1.2.3" "${incomplete_dist}" "${out_dir}" >"${stdout_log}" 2>"${stderr_log}"; then
  fail "a dist/ missing the darwin/arm64 binary was accepted (expected non-zero exit)"
else
  if grep -q "darwin/arm64" "${stderr_log}"; then
    pass "missing darwin/arm64 binary rejected, naming the exact missing target"
  else
    fail "missing binary was rejected but stderr didn't name darwin/arm64: $(cat "${stderr_log}")"
  fi
fi
[ -e "${out_dir}/cli-comrade/package.json" ] && fail "a missing-binary run must not leave a completed main package behind"

# --- Test 3: sanity check the "happy path" still succeeds on this fixture

out_dir="${TMP_DIR}/out-happy"
if "${BUILD_SCRIPT}" "1.2.3" "${complete_dist}" "${out_dir}" >"${stdout_log}" 2>"${stderr_log}"; then
  if [ -f "${out_dir}/cli-comrade/package.json" ] && [ -f "${out_dir}/comrade-win32-x64/bin/comrade.exe" ]; then
    pass "a complete dist/ + valid version assembles all 6 package directories"
  else
    fail "assembly reported success but expected output files are missing"
  fi
else
  fail "assembly failed on a complete, valid fixture: $(cat "${stderr_log}")"
fi

echo "---"
if [ "${failures}" -eq 0 ]; then
  echo "test-assemble.sh: all checks passed"
  exit 0
fi
echo "test-assemble.sh: ${failures} check(s) failed" >&2
exit 1
