#!/usr/bin/env bash
# Smoke test: assembles the real npm packages from goreleaser's dist/
# output, serves all 6 from a minimal, dependency-free local registry
# (npm/test/local-registry.js -- stdlib node:http only, no
# publish/auth/proxy endpoint of any kind), then runs a real
# `npm install -g cli-comrade` with cli-comrade as the ONLY direct install
# target, so its optionalDependency on the matching platform package is
# resolved TRANSITIVELY -- exactly like a real end-user install, and
# exactly what proves the dispatcher (not the platform package's own bin)
# is what actually gets linked. Nothing ever leaves localhost; nothing is
# published to the real npm registry. Linux-only, per task scope.
#
# Why not just `npm install` both tarballs directly? Both cli-comrade and
# the platform package declare `bin.comrade` -- passing them as two
# co-equal top-level install targets makes npm link BOTH bin fields into
# the same prefix/bin/comrade slot, and the platform package's binary can
# win, silently bypassing the dispatcher entirely (a real bug an earlier
# version of this test had: it asserted `--version` succeeded but never
# checked which file the shim actually resolved to). Installing
# `cli-comrade` alone and letting npm resolve the optionalDependency on
# its own is the only setup that distinguishes "the dispatcher ran" from
# "the platform binary got linked directly and the dispatcher was never
# invoked".
#
# Usage:
#   npm/test/test-smoke.sh [dist-dir] [version]
#
#   [dist-dir]  goreleaser's dist/ directory (default: "<repo>/dist").
#   [version]   npm version to assemble/install (default: the nearest git
#               tag with its leading "v" stripped, or "0.0.0-smoketest" if
#               no tag is reachable at all, e.g. a shallow clone).
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." >/dev/null 2>&1 && pwd)"

DIST_DIR="${1:-${REPO_ROOT}/dist}"
VERSION="${2:-}"

if [ -z "${VERSION}" ]; then
  VERSION="$(git -C "${REPO_ROOT}" describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || true)"
  if [ -z "${VERSION}" ]; then
    VERSION="0.0.0-smoketest"
  fi
fi

if [ "$(uname -s)" != "Linux" ]; then
  echo "test-smoke.sh: this smoke test only runs on Linux (matches the task's stated scope)" >&2
  exit 1
fi

if [ ! -d "${DIST_DIR}" ]; then
  echo "test-smoke.sh: dist directory \"${DIST_DIR}\" not found -- run goreleaser first (e.g. \`make release-snapshot\`)" >&2
  exit 1
fi

for tool in curl node npm; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "test-smoke.sh: required tool \"${tool}\" not found on PATH" >&2
    exit 1
  fi
done

WORK_DIR="$(mktemp -d)"
REGISTRY_PID=""
cleanup() {
  if [ -n "${REGISTRY_PID}" ] && kill -0 "${REGISTRY_PID}" 2>/dev/null; then
    kill "${REGISTRY_PID}" 2>/dev/null || true
  fi
  rm -rf "${WORK_DIR}"
}
trap cleanup EXIT

# --- assemble the real packages -------------------------------------------

OUT_DIR="${WORK_DIR}/assembled"
bash "${REPO_ROOT}/scripts/build-npm-packages.sh" "${VERSION}" "${DIST_DIR}" "${OUT_DIR}"

# --- start the minimal local registry (no auth, no publish endpoint, no
# uplink/proxy of any kind -- see local-registry.js's own header) ---------

REGISTRY_LOG="${WORK_DIR}/registry.log"
node "${SCRIPT_DIR}/local-registry.js" "${OUT_DIR}" >"${REGISTRY_LOG}" 2>&1 &
REGISTRY_PID=$!

registry_port=""
for _ in $(seq 1 60); do
  if [ -s "${REGISTRY_LOG}" ]; then
    registry_port="$(sed -n 's/^PORT=//p' "${REGISTRY_LOG}" | head -n1)"
    [ -n "${registry_port}" ] && break
  fi
  if ! kill -0 "${REGISTRY_PID}" 2>/dev/null; then
    echo "test-smoke.sh: local-registry.js exited before printing its port" >&2
    cat "${REGISTRY_LOG}" >&2
    exit 1
  fi
  sleep 0.5
done
if [ -z "${registry_port}" ]; then
  echo "test-smoke.sh: local-registry.js never printed a PORT= line" >&2
  cat "${REGISTRY_LOG}" >&2
  exit 1
fi

REGISTRY_URL="http://127.0.0.1:${registry_port}/"
status="$(curl -s -o /dev/null -w '%{http_code}' "${REGISTRY_URL}cli-comrade" || true)"
if [ "${status}" != "200" ]; then
  echo "test-smoke.sh: local registry at ${REGISTRY_URL} did not answer GET /cli-comrade with 200 (got ${status})" >&2
  cat "${REGISTRY_LOG}" >&2
  exit 1
fi
echo "test-smoke.sh: minimal local registry (stdlib http, no auth/publish/uplink) ready at ${REGISTRY_URL}"

# --- install ONLY cli-comrade -- the optionalDependency must resolve
# transitively, exactly like a real end user's `npm install -g cli-comrade`

PREFIX_DIR="${WORK_DIR}/prefix"
mkdir -p "${PREFIX_DIR}"
npm install -g --prefix "${PREFIX_DIR}" --registry "${REGISTRY_URL}" --no-audit --no-fund cli-comrade

BIN_PATH="${PREFIX_DIR}/bin/comrade"
if [ ! -e "${BIN_PATH}" ]; then
  echo "test-smoke.sh: expected installed binary shim at \"${BIN_PATH}\" was not found" >&2
  find "${PREFIX_DIR}" -maxdepth 5 >&2
  exit 1
fi

# The load-bearing assertion this test exists for: the shim must resolve to
# the DISPATCHER, not to a platform package's binary that happened to win
# the same bin-name slot.
resolved_target="$(readlink -f "${BIN_PATH}")"
echo "test-smoke.sh: bin shim ${BIN_PATH} -> ${resolved_target}"
case "${resolved_target}" in
  */cli-comrade/bin/comrade.js)
    echo "test-smoke.sh: shim resolves to the dispatcher (cli-comrade/bin/comrade.js), not the raw platform binary"
    ;;
  *)
    echo "test-smoke.sh: shim resolved to \"${resolved_target}\", expected it to end in cli-comrade/bin/comrade.js -- the dispatcher was bypassed" >&2
    exit 1
    ;;
esac

# The platform package's own binary must still be present, transitively
# installed, and executable -- proving `bin` on the platform package is
# load-bearing for npm's own chmod-on-install behavior even though it is
# never itself linked into the top-level bin/ directory (see the module
# comment at the top of scripts/build-npm-packages.sh, and
# npm/platform-template/package.json).
platform_binary="$(find "${PREFIX_DIR}" -type f -path '*/comrade-linux-x64/bin/comrade' | head -n1)"
if [ -z "${platform_binary}" ]; then
  echo "test-smoke.sh: could not find the transitively-installed platform binary under ${PREFIX_DIR}" >&2
  find "${PREFIX_DIR}" -maxdepth 6 >&2
  exit 1
fi
platform_binary_mode="$(stat -c '%a' "${platform_binary}")"
echo "test-smoke.sh: transitively-installed platform binary ${platform_binary} (mode ${platform_binary_mode})"
if [ "${platform_binary_mode}" != "755" ]; then
  echo "test-smoke.sh: expected the platform binary to be mode 755 after install, got ${platform_binary_mode}" >&2
  exit 1
fi

version_output="$("${BIN_PATH}" --version)"
echo "test-smoke.sh: \`comrade --version\` -> ${version_output}"
case "${version_output}" in
  *"comrade version"*) ;;
  *)
    echo "test-smoke.sh: unexpected --version output: ${version_output}" >&2
    exit 1
    ;;
esac

help_status=0
"${BIN_PATH}" --help >/dev/null 2>&1 || help_status=$?
if [ "${help_status}" -ne 0 ]; then
  echo "test-smoke.sh: \`comrade --help\` exited ${help_status}, expected 0" >&2
  exit 1
fi
echo "test-smoke.sh: \`comrade --help\` exited 0"

echo "test-smoke.sh: PASSED -- real \`npm install -g cli-comrade\` (single direct target) resolved the platform optionalDependency transitively and the dispatcher launched the real binary"
