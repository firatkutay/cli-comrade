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
#   COMRADE_NO_MODIFY_PATH  set to any non-empty value to stop the installer
#                         from appending a PATH export to your shell rc file
#                         when the install dir isn't already on PATH (default:
#                         unset — the rc file is edited automatically)
set -eu

REPO="firatkutay/cli-comrade"
BIN_NAME="comrade"

# PATH_MARKER is the idempotency marker prepended to the PATH export line
# configure_path_in_rc appends to a shell rc file. Its presence in a rc
# file is the sole signal that this installer already edited that file —
# re-running install.sh must never append a second copy.
PATH_MARKER="# Added by the cli-comrade installer — https://github.com/firatkutay/cli-comrade"

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

# rc_file_for_shell prints the shell rc file that should receive the PATH
# export for the given shell name ($1: bash/zsh/fish/anything else).
rc_file_for_shell() {
  case "$1" in
    bash) printf '%s\n' "$HOME/.bashrc" ;;
    zsh) printf '%s\n' "$HOME/.zshrc" ;;
    fish) printf '%s\n' "$HOME/.config/fish/config.fish" ;;
    *) printf '%s\n' "$HOME/.profile" ;;
  esac
}

# path_export_line_for_shell prints the shell-appropriate PATH export line
# to append for install dir $2 under shell $1. When $2 resolved to exactly
# $HOME/.local/bin, the literal (unexpanded) "$HOME/.local/bin" form is
# written instead of the expanded path, so the line stays correct even if
# the rc file is later sourced with a different HOME (e.g. restored on
# another account).
path_export_line_for_shell() {
  shell_arg="$1"
  dir_arg="$2"
  if [ "$dir_arg" = "$HOME/.local/bin" ]; then
    dir_expr='$HOME/.local/bin'
  else
    dir_expr="$dir_arg"
  fi
  case "$shell_arg" in
    fish) printf 'set -gx PATH %s $PATH\n' "$dir_expr" ;;
    *) printf 'export PATH="%s:$PATH"\n' "$dir_expr" ;;
  esac
}

# configure_path_in_rc appends an idempotent PATH export line for
# install_dir ($1) to the shell rc file appropriate for shell_name ($2),
# unless COMRADE_NO_MODIFY_PATH is set or the rc file's directory isn't
# writable — either case degrades to the old print-only warning rather
# than failing the install. Safe to call repeatedly: PATH_MARKER makes
# the edit idempotent, so re-running install.sh never duplicates it.
configure_path_in_rc() {
  install_dir_arg="$1"
  shell_name_arg="$2"

  if [ -n "${COMRADE_NO_MODIFY_PATH:-}" ]; then
    echo "install.sh: note — ${install_dir_arg} is not on your PATH; add it to your shell rc file (COMRADE_NO_MODIFY_PATH is set, so this was not done automatically)."
    return 0
  fi

  rc_file="$(rc_file_for_shell "$shell_name_arg")"
  rc_dir="$(dirname "$rc_file")"
  mkdir -p "$rc_dir" 2>/dev/null || true

  if [ ! -d "$rc_dir" ] || [ ! -w "$rc_dir" ]; then
    echo "install.sh: note — ${install_dir_arg} is not on your PATH; add it to your shell rc file."
    return 0
  fi

  if [ -f "$rc_file" ] && grep -Fq -- "$PATH_MARKER" "$rc_file" 2>/dev/null; then
    return 0
  fi

  export_line="$(path_export_line_for_shell "$shell_name_arg" "$install_dir_arg")"

  if { printf '\n%s\n%s\n' "$PATH_MARKER" "$export_line" >>"$rc_file"; } 2>/dev/null; then
    echo "install.sh: added ${install_dir_arg} to your PATH in ${rc_file}."
    case "$shell_name_arg" in
      fish) echo "install.sh: restart your shell or run:  set -gx PATH ${install_dir_arg} \$PATH" ;;
      *) echo "install.sh: restart your shell or run:  export PATH=\"${install_dir_arg}:\$PATH\"" ;;
    esac
  else
    echo "install.sh: note — ${install_dir_arg} is not on your PATH; add it to your shell rc file."
  fi
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

  shell_name="$(basename "${SHELL:-sh}")"

  case ":${PATH}:" in
    *":${install_dir}:"*) ;;
    *) configure_path_in_rc "$install_dir" "$shell_name" ;;
  esac

  shell_hint="$shell_name"
  case "$shell_hint" in
    bash | zsh | fish) ;;
    *) shell_hint="bash|zsh|fish" ;;
  esac
  echo "install.sh: run 'comrade init ${shell_hint}' to set up shell integration (error capture + completions)."
}

# main is skipped when this script is sourced with
# COMRADE_INSTALL_SH_TEST=1 set — scripts/install_test.sh uses that to
# source install.sh and unit-test configure_path_in_rc (and its helpers)
# directly, in isolation, with no network access and no real install.
if [ "${COMRADE_INSTALL_SH_TEST:-}" != "1" ]; then
  main "$@"
fi
