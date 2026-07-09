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

resolve_version() {
  if [ -n "${COMRADE_VERSION:-}" ]; then
    printf '%s\n' "$COMRADE_VERSION"
    return 0
  fi
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
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
  curl -fsSL -o "${workdir}/${archive}" "${base_url}/${archive}"
  curl -fsSL -o "${workdir}/checksums.txt" "${base_url}/checksums.txt"

  echo "install.sh: verifying checksum..."
  (
    cd "$workdir"
    grep " ${archive}\$" checksums.txt > checksum.line
    sha256sum -c checksum.line
  )

  tar -xzf "${workdir}/${archive}" -C "$workdir" "${BIN_NAME}"

  install_dir="${COMRADE_INSTALL_DIR:-}"
  if [ -z "$install_dir" ]; then
    install_dir="$HOME/.local/bin"
    if ! mkdir -p "$install_dir" 2>/dev/null || [ ! -w "$install_dir" ]; then
      install_dir="/usr/local/bin"
    fi
  fi
  mkdir -p "$install_dir"

  install -m 0755 "${workdir}/${BIN_NAME}" "${install_dir}/${BIN_NAME}"
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
