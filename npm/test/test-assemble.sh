#!/usr/bin/env bash
# Tests scripts/build-npm-packages.sh's failure modes: a malformed version
# string, a dist/ directory missing a required platform's binary, a
# version that doesn't match what dist/metadata.json says goreleaser
# actually built, and a caller-supplied out-dir that would be dangerous to
# `rm -rf`. All of these must fail loudly (non-zero exit, actionable
# stderr message) and must never leave a partial/broken package set or
# touch anything outside what the script itself generates.
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

# Invoked via `bash "$BUILD_SCRIPT"` everywhere below (never
# `"$BUILD_SCRIPT"` directly): defense-in-depth on top of the git-index
# exec-bit guard in test-script-permissions.sh, so this suite's own
# assertions can never be confused by a permissions problem again (see
# that file's header for the incident this is about).
run_build_script() {
  bash "${BUILD_SCRIPT}" "$@"
}

TMP_DIR="$(mktemp -d)"
# Only invoked indirectly via the trap below; ShellCheck's own wiki
# (SC2329) notes it "is currently bad at figuring out functions that are
# invoked via trap" and recommends this directive.
# shellcheck disable=SC2329
cleanup() { rm -rf "${TMP_DIR}"; }
trap cleanup EXIT

# --- fixture: a "complete" fake dist/ with all 5 platform binaries + the
# metadata.json goreleaser always writes alongside them -----------------

make_complete_dist() {
  local dist_dir="$1" tag="$2"
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
  printf '{"project_name":"comrade","tag":"%s","version":"%s"}' "${tag}" "${tag#v}" \
    >"${dist_dir}/metadata.json"
}

stdout_log="${TMP_DIR}/assemble-stdout.log"
stderr_log="${TMP_DIR}/assemble-stderr.log"

# --- Test 1: malformed version strings must fail, valid dist untouched ---

complete_dist="${TMP_DIR}/dist-complete"
make_complete_dist "${complete_dist}" "v1.2.3"

for bad_version in "v1.2.3" "1.2" "abc" "" "1.2.3.4"; do
  out_dir="${TMP_DIR}/out-badversion-${RANDOM}"
  if run_build_script "${bad_version}" "${complete_dist}" "${out_dir}" >"${stdout_log}" 2>"${stderr_log}"; then
    fail "malformed version \"${bad_version}\" was accepted (expected non-zero exit)"
  else
    if grep -qi "version" "${stderr_log}"; then
      pass "malformed version \"${bad_version}\" rejected with an actionable message"
    else
      fail "malformed version \"${bad_version}\" was rejected but stderr didn't mention \"version\": $(cat "${stderr_log}")"
    fi
  fi
  [ -e "${out_dir}" ] && fail "malformed version \"${bad_version}\" must not leave a partial output directory"
done

# --- Test 2: a missing platform binary must fail loudly, naming the gap,
# and leave NO output directory at all (atomic: all-or-nothing) ---------

incomplete_dist="${TMP_DIR}/dist-incomplete"
make_complete_dist "${incomplete_dist}" "v1.2.3"
rm -rf "${incomplete_dist}/comrade_darwin_arm64_v8.0"

out_dir="${TMP_DIR}/out-incomplete"
if run_build_script "1.2.3" "${incomplete_dist}" "${out_dir}" >"${stdout_log}" 2>"${stderr_log}"; then
  fail "a dist/ missing the darwin/arm64 binary was accepted (expected non-zero exit)"
else
  if grep -q "darwin/arm64" "${stderr_log}"; then
    pass "missing darwin/arm64 binary rejected, naming the exact missing target"
  else
    fail "missing binary was rejected but stderr didn't name darwin/arm64: $(cat "${stderr_log}")"
  fi
fi
if [ -e "${out_dir}" ]; then
  fail "a missing-binary run must leave NO output directory at all (got one at ${out_dir}) -- assembly is supposed to be atomic"
else
  pass "a missing-binary run leaves no output directory (atomic all-or-nothing assembly)"
fi

# --- Test 3: version must match dist/metadata.json's tag, or fail loudly,
# naming BOTH the requested and the actual version -----------------------

out_dir="${TMP_DIR}/out-version-mismatch"
if run_build_script "9.9.9" "${complete_dist}" "${out_dir}" >"${stdout_log}" 2>"${stderr_log}"; then
  fail "version \"9.9.9\" against a dist/ tagged v1.2.3 was accepted (expected non-zero exit)"
else
  if grep -q "9.9.9" "${stderr_log}" && grep -q "1.2.3" "${stderr_log}"; then
    pass "version/metadata mismatch (9.9.9 vs v1.2.3) rejected, naming both versions"
  else
    fail "version mismatch was rejected but stderr didn't name both versions: $(cat "${stderr_log}")"
  fi
fi
[ -e "${out_dir}" ] && fail "a version-mismatch run must not leave an output directory behind"

missing_metadata_dist="${TMP_DIR}/dist-no-metadata"
make_complete_dist "${missing_metadata_dist}" "v1.2.3"
rm -f "${missing_metadata_dist}/metadata.json"
out_dir="${TMP_DIR}/out-no-metadata"
if run_build_script "1.2.3" "${missing_metadata_dist}" "${out_dir}" >"${stdout_log}" 2>"${stderr_log}"; then
  fail "a dist/ with no metadata.json was accepted (expected non-zero exit)"
else
  pass "a dist/ with no metadata.json is rejected"
fi
[ -e "${out_dir}" ] && fail "a missing-metadata run must not leave an output directory behind"

# --- Test 4: refuse to `rm -rf` a caller-supplied out-dir that is NOT a
# previously-generated package set (e.g. "." or the dist dir itself) -----

danger_repo="${TMP_DIR}/fake-repo"
mkdir -p "${danger_repo}"
echo "do not delete me" >"${danger_repo}/important-file.txt"
if ( cd "${danger_repo}" && run_build_script "1.2.3" "${complete_dist}" "." ) >"${stdout_log}" 2>"${stderr_log}"; then
  fail "out-dir=\".\" pointed at a real directory was accepted (expected refusal)"
else
  pass "out-dir=\".\" is refused rather than rm -rf'd"
fi
if [ -f "${danger_repo}/important-file.txt" ]; then
  pass "out-dir=\".\" refusal left the caller's directory untouched"
else
  fail "out-dir=\".\" refusal DELETED the caller's directory contents -- important-file.txt is gone"
fi

danger_dist="${TMP_DIR}/dist-as-outdir"
make_complete_dist "${danger_dist}" "v1.2.3"
if run_build_script "1.2.3" "${danger_dist}" "${danger_dist}" >"${stdout_log}" 2>"${stderr_log}"; then
  fail "out-dir pointed at the dist dir itself was accepted (expected refusal)"
else
  pass "out-dir pointed at the dist dir itself is refused rather than rm -rf'd"
fi
if [ -f "${danger_dist}/comrade_linux_amd64_v1/comrade" ]; then
  pass "out-dir=dist-dir refusal left the dist binaries untouched"
else
  fail "out-dir=dist-dir refusal DELETED the dist binaries"
fi

# --- Test 5: sanity check the "happy path" still succeeds on this fixture,
# including LICENSE/README, and that re-running against the SAME (now
# legitimate) out-dir is allowed to replace it -- the out-dir safety check
# in Test 4 must reject unknown content, not existing content in general.

out_dir="${TMP_DIR}/out-happy"
if run_build_script "1.2.3" "${complete_dist}" "${out_dir}" >"${stdout_log}" 2>"${stderr_log}"; then
  if [ -f "${out_dir}/cli-comrade/package.json" ] && [ -f "${out_dir}/comrade-win32-x64/bin/comrade.exe" ] \
    && [ -f "${out_dir}/cli-comrade/LICENSE" ] && [ -f "${out_dir}/cli-comrade/README.md" ] \
    && [ -f "${out_dir}/comrade-linux-x64/LICENSE" ]; then
    pass "a complete dist/ + valid, metadata-matching version assembles all 6 package directories (with LICENSE/README)"
  else
    fail "assembly reported success but expected output files are missing"
  fi
else
  fail "assembly failed on a complete, valid fixture: $(cat "${stderr_log}")"
fi

if run_build_script "1.2.3" "${complete_dist}" "${out_dir}" >"${stdout_log}" 2>"${stderr_log}"; then
  pass "re-running against an existing, previously-generated out-dir replaces it successfully"
else
  fail "re-running against an existing, previously-generated out-dir was refused: $(cat "${stderr_log}")"
fi

echo "---"
if [ "${failures}" -eq 0 ]; then
  echo "test-assemble.sh: all checks passed"
  exit 0
fi
echo "test-assemble.sh: ${failures} check(s) failed" >&2
exit 1
