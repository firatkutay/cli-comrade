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

# fetch_url_to_file downloads $1 to the file path $2, using whichever
# downloader require_downloader resolved.
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
# every download in this script now goes through fetch_url_to_file, which
# dispatches on whichever was actually found.
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

# resolve_base_url returns the release download base URL to use.
#
# Deliberately does NOT call api.github.com/repos/.../releases/latest: that
# endpoint is unauthenticated and rate-limited to 60 req/hr per source IP,
# which is hostile to a curl|sh one-liner shared publicly (a handful of
# installs from behind the same NAT/CI runner exhausts it). GitHub's
# no-API "latest/download/<asset>" redirect has no such limit, so the
# default (unpinned) path resolves to that; a pinned COMRADE_VERSION uses
# the equivalent tag-scoped download URL. Either way, the actual version
# number is read back out of checksums.txt's matched filename, never out
# of a separate API/version-lookup call.
resolve_base_url() {
  if [ -n "${COMRADE_VERSION:-}" ]; then
    printf 'https://github.com/%s/releases/download/%s\n' "$REPO" "$COMRADE_VERSION"
  else
    printf 'https://github.com/%s/releases/latest/download\n' "$REPO"
  fi
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
  os="$(detect_os)"
  arch="$(detect_arch)"
  base_url="$(resolve_base_url)"
  archive_suffix="_${os}_${arch}.tar.gz"

  workdir="$(mktemp -d)"
  trap 'rm -rf "$workdir"' EXIT INT TERM

  echo "install.sh: fetching checksums..."
  fetch_url_to_file "${base_url}/checksums.txt" "${workdir}/checksums.txt"

  # Find the checksums.txt line for our os/arch by exact filename suffix
  # match (avoids regex-metachar escaping on the dots in ".tar.gz") and
  # pull the archive's real filename — which embeds the resolved version
  # — straight out of it, rather than resolving the version separately.
  checksum_line="$(awk -v suf="$archive_suffix" \
    '{ if (substr($2, length($2) - length(suf) + 1) == suf) print $0 }' \
    "${workdir}/checksums.txt")"
  if [ -z "$checksum_line" ]; then
    echo "install.sh: no release asset found for os=${os} arch=${arch} (checked ${base_url}/checksums.txt)" >&2
    exit 1
  fi
  archive="$(printf '%s\n' "$checksum_line" | awk '{print $2}')"
  version_number="${archive%"$archive_suffix"}"
  version_number="${version_number#"${BIN_NAME}"_}"

  echo "install.sh: downloading ${archive} (v${version_number})..."
  fetch_url_to_file "${base_url}/${archive}" "${workdir}/${archive}"

  echo "install.sh: verifying checksum..."
  (
    cd "$workdir"
    printf '%s\n' "$checksum_line" > checksum.line
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

  shell_name="$(basename "${SHELL:-sh}")"
  case "$shell_name" in
    bash | zsh | fish) ;;
    *) shell_name="bash|zsh|fish" ;;
  esac
  echo "install.sh: run 'comrade init ${shell_name}' to set up shell integration (error capture + completions)."
}

main "$@"
