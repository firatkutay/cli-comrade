#!/usr/bin/env bash
# Smoke test: assembles the real npm packages from goreleaser's dist/
# output, publishes all 6 to a throwaway, uplink-disabled LOCAL registry
# (verdaccio -- see npm/test/package.json), then runs a real
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

for tool in jq curl node; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "test-smoke.sh: required tool \"${tool}\" not found on PATH" >&2
    exit 1
  fi
done

if [ ! -d "${SCRIPT_DIR}/node_modules/verdaccio" ]; then
  echo "test-smoke.sh: installing the pinned test-harness devDependencies (verdaccio) via npm ci..."
  ( cd "${SCRIPT_DIR}" && npm ci --no-audit --no-fund )
fi
VERDACCIO_BIN="${SCRIPT_DIR}/node_modules/.bin/verdaccio"
if [ ! -x "${VERDACCIO_BIN}" ]; then
  echo "test-smoke.sh: verdaccio binary not found at \"${VERDACCIO_BIN}\" after npm ci" >&2
  exit 1
fi

WORK_DIR="$(mktemp -d)"
VERDACCIO_PID=""
cleanup() {
  if [ -n "${VERDACCIO_PID}" ] && kill -0 "${VERDACCIO_PID}" 2>/dev/null; then
    kill "${VERDACCIO_PID}" 2>/dev/null || true
  fi
  rm -rf "${WORK_DIR}"
}
trap cleanup EXIT

# --- assemble the real packages -------------------------------------------

OUT_DIR="${WORK_DIR}/assembled"
bash "${REPO_ROOT}/scripts/build-npm-packages.sh" "${VERSION}" "${DIST_DIR}" "${OUT_DIR}"

# --- start a throwaway, offline local registry (verdaccio) ----------------
# uplinks: {} means verdaccio can never proxy to the real npm registry, no
# matter what a package below happens to reference -- this test cannot
# reach registry.npmjs.org even by accident.

REGISTRY_PORT="$(node -e "const s=require('net').createServer();s.listen(0,'127.0.0.1',()=>{console.log(s.address().port);s.close();});")"
REGISTRY_URL="http://127.0.0.1:${REGISTRY_PORT}/"

cat >"${WORK_DIR}/verdaccio-config.yaml" <<EOF
storage: ${WORK_DIR}/verdaccio-storage
auth:
  htpasswd:
    file: ${WORK_DIR}/verdaccio-htpasswd
uplinks: {}
packages:
  '@firatkutay/*':
    access: \$all
    publish: \$all
  '**':
    access: \$all
    publish: \$all
log: { type: stdout, format: pretty, level: warn }
EOF

"${VERDACCIO_BIN}" --config "${WORK_DIR}/verdaccio-config.yaml" --listen "127.0.0.1:${REGISTRY_PORT}" \
  >"${WORK_DIR}/verdaccio.log" 2>&1 &
VERDACCIO_PID=$!

# Verdaccio's own startup latency here is dominated by requiring its
# (many) module files from wherever npm/test/node_modules happens to live
# -- when that's on a slow-per-syscall mount (e.g. drvfs under WSL2), cold
# start can take tens of seconds even though the process is healthy the
# whole time. 90 attempts x 1s is a generous but still-bounded wait for
# that, not an indefinite one.
ready=0
for _ in $(seq 1 90); do
  status="$(curl -s -o /dev/null -w '%{http_code}' "${REGISTRY_URL}" || true)"
  if [ "${status}" != "000" ]; then
    ready=1
    break
  fi
  sleep 1
done
if [ "${ready}" -ne 1 ]; then
  echo "test-smoke.sh: local verdaccio registry never became ready on ${REGISTRY_URL}" >&2
  cat "${WORK_DIR}/verdaccio.log" >&2
  exit 1
fi
echo "test-smoke.sh: local offline registry (verdaccio, uplinks disabled) ready at ${REGISTRY_URL}"

# --- non-interactive login: the classic adduser PUT, verified directly
# against this running instance (avoids an interactive `npm adduser` /
# `npm login` prompt in a script) --------------------------------------

adduser_response="$(curl -s -X PUT -H 'Content-Type: application/json' \
  -d '{"name":"smoketest","password":"smoketest-not-a-real-secret","email":"smoketest@example.invalid"}' \
  "${REGISTRY_URL}-/user/org.couchdb.user:smoketest")"
token="$(echo "${adduser_response}" | jq -r '.token // empty')"
if [ -z "${token}" ]; then
  echo "test-smoke.sh: could not obtain an auth token from the local registry: ${adduser_response}" >&2
  exit 1
fi

NPMRC="${WORK_DIR}/npmrc"
{
  echo "//127.0.0.1:${REGISTRY_PORT}/:_authToken=${token}"
  echo "registry=${REGISTRY_URL}"
} >"${NPMRC}"

# --- publish all 6 assembled packages to the local registry ---------------

for pkg_dir in "${OUT_DIR}"/*/; do
  npm publish --userconfig "${NPMRC}" --silent "${pkg_dir}" >/dev/null
  echo "test-smoke.sh: published $(basename "${pkg_dir}") to the local registry"
done

# --- install ONLY cli-comrade -- the optionalDependency must resolve
# transitively, exactly like a real end user's `npm install -g cli-comrade`

PREFIX_DIR="${WORK_DIR}/prefix"
mkdir -p "${PREFIX_DIR}"
npm install -g --prefix "${PREFIX_DIR}" --userconfig "${NPMRC}" --no-audit --no-fund cli-comrade

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
# comment at the top of this file, and platform-template/package.json).
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
