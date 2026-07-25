#!/usr/bin/env bash
# Assembles the 6 ready-to-publish npm packages (1 main dispatcher package +
# 5 platform-specific binary packages) from goreleaser's `dist/` output.
#
# Usage:
#   scripts/build-npm-packages.sh <version> [dist-dir] [out-dir]
#
#   <version>   the npm package version to stamp everywhere, e.g. "0.4.2"
#               (the git tag WITHOUT its leading "v" -- see docs/PACKAGING.md).
#   [dist-dir]  goreleaser's dist/ directory (default: "dist").
#   [out-dir]   where to write the assembled package directories
#               (default: "npm/packages" -- gitignored, never committed).
#
# Requirements on PATH: jq (package.json templating).
#
# Fails loudly (non-zero exit, message to stderr) when:
#   - <version> is not a well-formed semver-ish string;
#   - a platform's built binary is missing from dist-dir, or more than one
#     candidate directory matches (ambiguous -- refuses to guess).
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." >/dev/null 2>&1 && pwd)"
NPM_DIR="${REPO_ROOT}/npm"

VERSION="${1:-}"
DIST_DIR="${2:-${REPO_ROOT}/dist}"
OUT_DIR="${3:-${NPM_DIR}/packages}"

# --- validation --------------------------------------------------------

if [ -z "${VERSION}" ]; then
  echo "build-npm-packages.sh: missing required <version> argument" >&2
  echo "usage: scripts/build-npm-packages.sh <version> [dist-dir] [out-dir]" >&2
  exit 1
fi

# Semver core + optional prerelease/build-metadata, e.g. "1.2.3",
# "1.2.3-beta.1", "1.2.3+abc123". Deliberately rejects a leading "v" -- the
# caller must strip the git tag's "v" prefix before calling this script.
SEMVER_RE='^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'
if ! [[ "${VERSION}" =~ ${SEMVER_RE} ]]; then
  echo "build-npm-packages.sh: malformed version \"${VERSION}\" (expected e.g. \"1.2.3\", no leading \"v\")" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "build-npm-packages.sh: jq is required on PATH but was not found" >&2
  exit 1
fi

if [ ! -d "${DIST_DIR}" ]; then
  echo "build-npm-packages.sh: dist directory \"${DIST_DIR}\" does not exist -- run goreleaser first" >&2
  exit 1
fi

# --- platform matrix -----------------------------------------------------
# goos:goarch:npm_os:npm_cpu:binary_name -- mirrors .goreleaser.yaml's build
# matrix (linux+darwin amd64/arm64, windows amd64 only) and
# npm/main/bin/platform-map.js's PLATFORM_PACKAGES exactly. Keep all three
# in sync if this ever changes.
PLATFORMS=(
  "linux:amd64:linux:x64:comrade"
  "linux:arm64:linux:arm64:comrade"
  "darwin:amd64:darwin:x64:comrade"
  "darwin:arm64:darwin:arm64:comrade"
  "windows:amd64:win32:x64:comrade.exe"
)

rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}"

created_platform_dirs=()

for entry in "${PLATFORMS[@]}"; do
  IFS=':' read -r goos goarch npm_os npm_cpu bin_name <<<"${entry}"

  # goreleaser names each target's build directory
  # "<binary-id>_<goos>_<goarch>_<goarm/goamd64 suffix>" -- the microarch
  # suffix (e.g. "v1", "v8.0") is a goreleaser/Go-toolchain default we do
  # not want to hardcode, so glob for it instead of assuming a fixed value.
  mapfile -t candidates < <(find "${DIST_DIR}" -maxdepth 1 -type d -name "comrade_${goos}_${goarch}*")

  if [ "${#candidates[@]}" -eq 0 ]; then
    echo "build-npm-packages.sh: no built binary found for ${goos}/${goarch} under \"${DIST_DIR}\" (looked for comrade_${goos}_${goarch}*/${bin_name})" >&2
    exit 1
  fi
  if [ "${#candidates[@]}" -gt 1 ]; then
    echo "build-npm-packages.sh: ambiguous build output for ${goos}/${goarch} -- multiple candidate directories matched:" >&2
    printf '  %s\n' "${candidates[@]}" >&2
    exit 1
  fi

  src_binary="${candidates[0]}/${bin_name}"
  if [ ! -f "${src_binary}" ]; then
    echo "build-npm-packages.sh: expected binary \"${src_binary}\" does not exist" >&2
    exit 1
  fi

  pkg_dir_name="comrade-${npm_os}-${npm_cpu}"
  pkg_out_dir="${OUT_DIR}/${pkg_dir_name}"
  mkdir -p "${pkg_out_dir}/bin"

  jq \
    --arg name "@firatkutay/comrade-${npm_os}-${npm_cpu}" \
    --arg version "${VERSION}" \
    --arg os "${npm_os}" \
    --arg cpu "${npm_cpu}" \
    --arg bin "${bin_name}" \
    --arg platform "${npm_os}-${npm_cpu}" \
    '.name = $name
     | .version = $version
     | .description = ("Prebuilt `comrade` binary for " + $platform + ". Installed automatically as an optionalDependency of `cli-comrade` -- do not install this package directly.")
     | .os = [$os]
     | .cpu = [$cpu]
     | .bin = { comrade: ("bin/" + $bin) }' \
    "${NPM_DIR}/platform-template/package.json" >"${pkg_out_dir}/package.json"

  cp "${src_binary}" "${pkg_out_dir}/bin/${bin_name}"
  chmod +x "${pkg_out_dir}/bin/${bin_name}"

  created_platform_dirs+=("${pkg_dir_name}")
  echo "build-npm-packages.sh: assembled ${pkg_dir_name} (from ${candidates[0]})"
done

# --- main dispatcher package ----------------------------------------------

main_out_dir="${OUT_DIR}/cli-comrade"
mkdir -p "${main_out_dir}/bin"

cp "${NPM_DIR}/main/bin/comrade.js" "${main_out_dir}/bin/comrade.js"
cp "${NPM_DIR}/main/bin/platform-map.js" "${main_out_dir}/bin/platform-map.js"
chmod +x "${main_out_dir}/bin/comrade.js"

jq \
  --arg version "${VERSION}" \
  '.version = $version
   | .optionalDependencies = (.optionalDependencies | with_entries(.value = $version))' \
  "${NPM_DIR}/main/package.json" >"${main_out_dir}/package.json"

echo "build-npm-packages.sh: assembled cli-comrade (main dispatcher package)"
echo "build-npm-packages.sh: done -- ${#created_platform_dirs[@]} platform package(s) + 1 main package in \"${OUT_DIR}\", version ${VERSION}"
