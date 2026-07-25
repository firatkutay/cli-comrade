#!/usr/bin/env bash
# Assembles the 6 ready-to-publish npm packages (1 main dispatcher package +
# 5 platform-specific binary packages) from goreleaser's `dist/` output.
#
# This file is committed executable (mode 755) and is meant to be run
# either directly (./scripts/build-npm-packages.sh ...) or via
# `bash scripts/build-npm-packages.sh ...` -- both work. If you ever see
# "Permission denied" running it directly from a fresh clone, the exec bit
# was lost somewhere in the commit history; see
# npm/test/test-assemble.sh's git-index-mode guard test, which fails the
# whole suite the moment that happens again (this has shipped broken once
# already -- see the commit that added this comment).
#
# Usage:
#   scripts/build-npm-packages.sh <version> [dist-dir] [out-dir]
#
#   <version>   the npm package version to stamp everywhere, e.g. "0.4.2"
#               (the git tag WITHOUT its leading "v" -- see the npm
#               section of docs/PACKAGING.md). Must exactly match the
#               "tag" goreleaser recorded in <dist-dir>/metadata.json --
#               see the version-guard section below.
#   [dist-dir]  goreleaser's dist/ directory (default: "dist").
#   [out-dir]   where to write the assembled package directories
#               (default: "npm/packages" -- gitignored, never committed).
#               Replaced ATOMICALLY: assembly happens in a private staging
#               directory first, and out-dir is only ever touched once
#               every package has assembled successfully (see "atomic
#               swap" below) -- a failure partway through never leaves a
#               partial 3-of-5-platforms package set on disk.
#
# Requirements on PATH: jq (package.json templating).
#
# Fails loudly (non-zero exit, message to stderr) when:
#   - <version> is not a well-formed semver-ish string;
#   - <dist-dir>/metadata.json is missing, or its "tag" (minus the leading
#     "v") does not equal <version> -- npm versions are immutable, so
#     assembling under the wrong version is an unrecoverable mistake and
#     this is refused rather than silently allowed;
#   - a platform's built binary is missing from dist-dir, or more than one
#     candidate directory matches (ambiguous -- refuses to guess);
#   - [out-dir] already exists and contains anything OTHER than a
#     previously-generated package directory (protects against a
#     caller-supplied path like "." or the dist dir itself).
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

# --- version guard: <version> must match what this dist/ was actually
# built from. goreleaser always writes metadata.json with the real tag it
# built -- comparing against it (rather than trusting the caller's
# <version> blindly) is the only thing standing between a typo and
# publishing an npm package labelled e.g. "9.9.9" that actually contains a
# 0.4.2 binary, which -- because npm versions are immutable -- would be
# unrecoverable. -------------------------------------------------------

METADATA_FILE="${DIST_DIR}/metadata.json"
if [ ! -f "${METADATA_FILE}" ]; then
  echo "build-npm-packages.sh: \"${METADATA_FILE}\" not found -- goreleaser always writes this; run goreleaser (e.g. \`make release-snapshot\`) first" >&2
  exit 1
fi
dist_tag="$(jq -r '.tag // empty' "${METADATA_FILE}")"
if [ -z "${dist_tag}" ]; then
  echo "build-npm-packages.sh: \"${METADATA_FILE}\" has no usable \"tag\" field" >&2
  exit 1
fi
dist_version="${dist_tag#v}"
if [ "${dist_version}" != "${VERSION}" ]; then
  echo "build-npm-packages.sh: version mismatch -- asked to stamp \"${VERSION}\", but \"${METADATA_FILE}\" says this dist/ was built from tag \"${dist_tag}\" (version \"${dist_version}\")." >&2
  echo "build-npm-packages.sh: refusing -- npm versions are immutable, so publishing the wrong version is unrecoverable. Pass \"${dist_version}\", or rebuild dist/ for \"${VERSION}\"." >&2
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

# Every directory name this script ever generates under out-dir -- used
# below to decide whether an existing out-dir is safe to replace.
KNOWN_PKG_DIR_NAMES=(cli-comrade comrade-linux-x64 comrade-linux-arm64 comrade-darwin-x64 comrade-darwin-arm64 comrade-win32-x64)

is_known_pkg_dir_name() {
  local candidate="$1" known
  for known in "${KNOWN_PKG_DIR_NAMES[@]}"; do
    if [ "${candidate}" = "${known}" ]; then
      return 0
    fi
  done
  return 1
}

# --- assemble into a private staging directory first ----------------------
# Nothing under [out-dir] is touched until every one of the 6 packages has
# assembled successfully below -- see the atomic swap at the very end of
# this script. A failure partway through (missing binary, bad template,
# ...) leaves [out-dir] exactly as it was found; it can never observe a
# partial (e.g. 3-of-5-platforms) package set.
STAGING_DIR="$(mktemp -d)"
trap 'rm -rf "${STAGING_DIR}"' EXIT

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
  pkg_out_dir="${STAGING_DIR}/${pkg_dir_name}"
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
  # MIT requires the license notice accompany distributions, and each
  # platform package literally distributes the compiled binary -- carry
  # the repo's own LICENSE along (also declared via the template's
  # "files" list, but npm includes LICENSE by default regardless).
  cp "${REPO_ROOT}/LICENSE" "${pkg_out_dir}/LICENSE"

  created_platform_dirs+=("${pkg_dir_name}")
  echo "build-npm-packages.sh: assembled ${pkg_dir_name} (from ${candidates[0]})"
done

# --- main dispatcher package ----------------------------------------------
#
# Note on the platform packages' own "bin" field (npm/platform-template/
# package.json): it is NOT redundant with the main package's dispatcher.
# A real `npm install -g cli-comrade` verified that npm restores the exec
# bit to 755 on every file a package's own "bin" field points at, even for
# a package installed transitively as an optionalDependency that never
# gets linked into the top-level bin/ directory -- without that field,
# the platform binary can land on disk non-executable and the dispatcher's
# spawnSync would fail with EACCES. Keep it.

main_out_dir="${STAGING_DIR}/cli-comrade"
mkdir -p "${main_out_dir}/bin"

cp "${NPM_DIR}/main/bin/comrade.js" "${main_out_dir}/bin/comrade.js"
cp "${NPM_DIR}/main/bin/platform-map.js" "${main_out_dir}/bin/platform-map.js"
chmod +x "${main_out_dir}/bin/comrade.js"
cp "${REPO_ROOT}/LICENSE" "${main_out_dir}/LICENSE"
cp "${NPM_DIR}/main/README.md" "${main_out_dir}/README.md"

jq \
  --arg version "${VERSION}" \
  '.version = $version
   | .optionalDependencies = (.optionalDependencies | with_entries(.value = $version))' \
  "${NPM_DIR}/main/package.json" >"${main_out_dir}/package.json"

echo "build-npm-packages.sh: assembled cli-comrade (main dispatcher package)"

# --- atomic swap: out-dir is only ever touched now, all-or-nothing --------

if [ -e "${OUT_DIR}" ]; then
  if [ ! -d "${OUT_DIR}" ]; then
    echo "build-npm-packages.sh: \"${OUT_DIR}\" exists and is not a directory -- refusing to touch it" >&2
    exit 1
  fi
  while IFS= read -r -d '' existing_entry; do
    existing_name="$(basename "${existing_entry}")"
    if ! is_known_pkg_dir_name "${existing_name}"; then
      echo "build-npm-packages.sh: refusing to replace \"${OUT_DIR}\" -- it contains \"${existing_name}\", which is not a directory this script generates." >&2
      echo "build-npm-packages.sh: point [out-dir] at an empty or dedicated location instead (never \".\", a source directory, or the dist dir itself)." >&2
      exit 1
    fi
  done < <(find "${OUT_DIR}" -mindepth 1 -maxdepth 1 -print0)
  rm -rf "${OUT_DIR}"
fi

mkdir -p "$(dirname "${OUT_DIR}")"
mv "${STAGING_DIR}" "${OUT_DIR}"

echo "build-npm-packages.sh: done -- ${#created_platform_dirs[@]} platform package(s) + 1 main package in \"${OUT_DIR}\", version ${VERSION}"
