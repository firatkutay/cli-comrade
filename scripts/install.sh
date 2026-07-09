#!/bin/sh
# Installs the comrade CLI (https://github.com/firatkutay/cli-comrade) by
# downloading the latest (or COMRADE_VERSION-pinned) GitHub release
# artifact, verifying its checksum, and placing the binary on PATH.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh
#
# Env overrides:
#   COMRADE_VERSION      release tag to install, e.g. "v0.1.0" (default: latest)
#   COMRADE_INSTALL_DIR   install directory (default: $HOME/.local/bin, falling
#                         back to /usr/local/bin if that can't be created)
set -eu

REPO="firatkutay/cli-comrade"
BIN_NAME="comrade"

# fetch_url downloads $1 to stdout using whichever of curl/wget
# require_downloader already confirmed is available, so every download
# in this script (the version lookup, the archive, checksums.txt) goes
# through one code path instead of two parallel curl-only/wget-only
# copies that could silently drift.
fetch_url() {
  if [ "$DOWNLOADER" = curl ]; then
    curl -fsSL "$1"
  else
    wget -qO- "$1"
  fi
}

# fetch_url_to_file downloads $1 to the file path $2, using the same
# resolved downloader as fetch_url.
fetch_url_to_file() {
  if [ "$DOWNLOADER" = curl ]; then
    curl -fsSL -o "$2" "$1"
  else
    wget -qO "$2" "$1"
  fi
}

# require_downloader picks curl or wget (in that order) and fails with a
# friendly, actionable message if neither is on PATH — this is the FAZ 4
# reviewer finding this script only ever had a hard curl dependency;
# every download in this script now goes through fetch_url/
# fetch_url_to_file, which dispatch on whichever was actually found.
require_downloader() {
  if command -v curl >/dev/null 2>&1; then
    DOWNLOADER=curl
  elif command -v wget >/dev/null 2>&1; then
    DOWNLOADER=wget
  else
    echo "install.sh: neither curl nor wget was found on PATH; install one of them and re-run this script." >&2
    exit 1
  fi
}

resolve_version() {
  if [ -n "${COMRADE_VERSION:-}" ]; then
    printf '%s\n' "$COMRADE_VERSION"
    return 0
  fi
  fetch_url "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep -m1 '"tag_name"' \
    | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/'
}

detect_os() {
  case "$(uname -s)" in
    Linux) echo linux ;;
    Darwin) echo darwin ;;
    *)
      echo "install.sh: unsupported OS: $(uname -s)" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) echo amd64 ;;
    arm64 | aarch64) echo arm64 ;;
    *)
      echo "install.sh: unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

main() {
  require_downloader
  version="$(resolve_version)"
  if [ -z "$version" ]; then
    echo "install.sh: could not resolve a version to install (set COMRADE_VERSION to override)" >&2
    exit 1
  fi
  version_number="${version#v}"
  os="$(detect_os)"
  arch="$(detect_arch)"

  archive="${BIN_NAME}_${version_number}_${os}_${arch}.tar.gz"
  base_url="https://github.com/${REPO}/releases/download/${version}"

  workdir="$(mktemp -d)"
  trap 'rm -rf "$workdir"' EXIT INT TERM

  echo "install.sh: downloading ${archive} (${version})..."
  fetch_url_to_file "${base_url}/${archive}" "${workdir}/${archive}"
  fetch_url_to_file "${base_url}/checksums.txt" "${workdir}/checksums.txt"

  echo "install.sh: verifying checksum..."
  (
    cd "$workdir"
    grep " ${archive}\$" checksums.txt > checksum.line
    sha256sum -c checksum.line
  )

  tar -xzf "${workdir}/${archive}" -C "$workdir" "${BIN_NAME}"

  install_dir="${COMRADE_INSTALL_DIR:-}"
  sudo_prefix=""
  if [ -z "$install_dir" ]; then
    install_dir="$HOME/.local/bin"
    if ! mkdir -p "$install_dir" 2>/dev/null || [ ! -w "$install_dir" ]; then
      install_dir="/usr/local/bin"
      if ! mkdir -p "$install_dir" 2>/dev/null || [ ! -w "$install_dir" ]; then
        # Neither the user-writable ~/.local/bin nor /usr/local/bin is
        # usable without elevation — fall back to sudo, prompting the
        # user for their password exactly once, rather than failing
        # outright (the common case on a fresh machine with no
        # ~/.local/bin yet and a root-owned /usr/local/bin).
        if command -v sudo >/dev/null 2>&1; then
          echo "install.sh: ${install_dir} is not writable; using sudo (you may be prompted for your password)."
          sudo_prefix="sudo"
        else
          echo "install.sh: ${install_dir} is not writable and sudo is not available; set COMRADE_INSTALL_DIR to a writable directory and re-run." >&2
          exit 1
        fi
      fi
    fi
  else
    mkdir -p "$install_dir"
  fi

  $sudo_prefix mkdir -p "$install_dir"
  $sudo_prefix install -m 0755 "${workdir}/${BIN_NAME}" "${install_dir}/${BIN_NAME}"
  echo "install.sh: installed ${BIN_NAME} to ${install_dir}/${BIN_NAME}"

  case ":${PATH}:" in
    *":${install_dir}:"*) ;;
    *)
      echo "install.sh: note — ${install_dir} is not on your PATH; add it to your shell rc file."
      ;;
  esac

  echo "install.sh: run 'comrade init' to set up shell integration."
}

main "$@"
