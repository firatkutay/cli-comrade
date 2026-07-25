#!/usr/bin/env bash
# Smoke test: assembles the real npm packages from goreleaser's dist/
# output, `npm pack`s the linux-x64 platform package and the main
# dispatcher package, installs BOTH tarballs together into a throwaway
# global prefix (no network/registry contact needed -- npm resolves the
# optionalDependency from the sibling tarball given in the same install
# call), then runs the installed `comrade` binary and checks it actually
# executed the real Go binary end-to-end. Linux-only, per task scope.
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

WORK_DIR="$(mktemp -d)"
cleanup() { rm -rf "${WORK_DIR}"; }
trap cleanup EXIT

OUT_DIR="${WORK_DIR}/assembled"
"${REPO_ROOT}/scripts/build-npm-packages.sh" "${VERSION}" "${DIST_DIR}" "${OUT_DIR}"

PLATFORM_PKG_DIR="${OUT_DIR}/comrade-linux-x64"
MAIN_PKG_DIR="${OUT_DIR}/cli-comrade"

TARBALL_DIR="${WORK_DIR}/tarballs"
mkdir -p "${TARBALL_DIR}"
npm pack --silent --pack-destination "${TARBALL_DIR}" "${PLATFORM_PKG_DIR}" >/dev/null
npm pack --silent --pack-destination "${TARBALL_DIR}" "${MAIN_PKG_DIR}" >/dev/null

platform_tarball="$(find "${TARBALL_DIR}" -maxdepth 1 -name 'firatkutay-comrade-linux-x64-*.tgz')"
main_tarball="$(find "${TARBALL_DIR}" -maxdepth 1 -name 'cli-comrade-*.tgz')"

if [ -z "${platform_tarball}" ] || [ -z "${main_tarball}" ]; then
  echo "test-smoke.sh: npm pack did not produce the expected tarball(s) in ${TARBALL_DIR}" >&2
  ls -la "${TARBALL_DIR}" >&2
  exit 1
fi

echo "test-smoke.sh: packed ${platform_tarball}"
echo "test-smoke.sh: packed ${main_tarball}"

PREFIX_DIR="${WORK_DIR}/prefix"
mkdir -p "${PREFIX_DIR}"

# Installing both tarballs in ONE `npm install` call lets npm satisfy
# cli-comrade's optionalDependency on @firatkutay/comrade-linux-x64 from
# the sibling tarball given in the same command, with no registry lookup
# needed -- this is a real `npm install`, not a hand-built node_modules
# tree, so it exercises the actual resolution/linking npm users get.
npm install -g --prefix "${PREFIX_DIR}" --no-audit --no-fund "${platform_tarball}" "${main_tarball}"

BIN_PATH="${PREFIX_DIR}/bin/comrade"
if [ ! -x "${BIN_PATH}" ]; then
  echo "test-smoke.sh: expected installed binary shim at \"${BIN_PATH}\" was not found" >&2
  find "${PREFIX_DIR}" -maxdepth 4 >&2
  exit 1
fi
echo "test-smoke.sh: installed bin shim at ${BIN_PATH}"

version_output="$("${BIN_PATH}" --version)"
echo "test-smoke.sh: \`comrade --version\` -> ${version_output}"

case "${version_output}" in
  *"comrade version"*)
    echo "test-smoke.sh: dispatcher successfully launched the real platform binary"
    ;;
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

echo "test-smoke.sh: PASSED -- packed tarballs installed via real npm install and the real binary ran"
