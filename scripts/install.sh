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
#
# Requirements on PATH:
#   - curl or wget (download)
#   - one SHA-256 checksum tool: sha256sum (GNU/most Linux distros),
#     shasum (macOS/BSD), or openssl (fallback) — checksum verification
#     is mandatory and never skipped
#   - tar and gzip (archive extraction)
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

# require_checksum_verifier picks a SHA-256 verifier and sets
# CHECKSUM_VERIFIER to "gnu", "bsd", or "openssl" — resolved once, then
# dispatched on by verify_checksum() at the call site, the same
# resolver/wrapper split as require_downloader/fetch_url_to_file above.
#
# Preference order: sha256sum (GNU coreutils; most Linux distros) >
# shasum -a 256 (BSD/macOS — stock macOS has no sha256sum at all, only
# shasum and openssl) > openssl dgst -sha256 (near-universal fallback).
# Verification is never weakened or skipped: if none of the three is on
# PATH, abort non-zero with an actionable message instead of continuing
# without verifying the download.
#
# Each candidate must pass a functional probe (hash a zero-byte input),
# not just `command -v` existence: `command -v` only proves a file with
# that name is on PATH, not that it can actually run. macOS's `shasum` is
# a `#!/usr/bin/perl` script — a broken/missing perl makes it fail with
# "shasum: not found"/exit 127 despite `command -v shasum` succeeding,
# reproducing the exact "dies at verifying checksum" breakage this
# resolver exists to eliminate. A candidate that exists but fails its
# probe is skipped in favor of the next one, not treated as fatal.
probe_checksum_tool() {
  case "$1" in
    gnu) printf '' | sha256sum >/dev/null 2>&1 ;;
    bsd) printf '' | shasum -a 256 >/dev/null 2>&1 ;;
    openssl) printf '' | openssl dgst -sha256 >/dev/null 2>&1 ;;
  esac
}

require_checksum_verifier() {
  if command -v sha256sum >/dev/null 2>&1 && probe_checksum_tool gnu; then
    CHECKSUM_VERIFIER=gnu
  elif command -v shasum >/dev/null 2>&1 && probe_checksum_tool bsd; then
    CHECKSUM_VERIFIER=bsd
  elif command -v openssl >/dev/null 2>&1 && probe_checksum_tool openssl; then
    CHECKSUM_VERIFIER=openssl
  else
    echo "install.sh: no working SHA-256 checksum tool found on PATH (checked sha256sum, shasum, openssl — present-but-broken installs are skipped, not treated as found); install one of them (e.g. 'sha256sum' is in GNU coreutils, 'shasum' ships with macOS/Perl, or install openssl) and re-run this script." >&2
    exit 1
  fi
}

# verify_checksum checks archive file $1 against checksums.txt line $2
# ("<hash>  <filename>", the format sha256sum/shasum -c both expect),
# using whichever verifier require_checksum_verifier resolved into
# CHECKSUM_VERIFIER. Must be called with $1 in the current directory.
#
# A hash MISMATCH always aborts non-zero with a clear message — this is
# never downgraded to a warning, on any of the three verifier paths.
verify_checksum() {
  file="$1"
  checksum_line="$2"

  case "$CHECKSUM_VERIFIER" in
    gnu)
      printf '%s\n' "$checksum_line" >checksum.line
      sha256sum -c checksum.line
      ;;
    bsd)
      printf '%s\n' "$checksum_line" >checksum.line
      shasum -a 256 -c checksum.line
      ;;
    openssl)
      # `openssl dgst` has no built-in -c/--check mode and a different
      # output format than sha256sum/shasum. The label before "(file)="
      # varies by OpenSSL version/build too — OpenSSL 1.x prints
      # "SHA256(file)= <hash>", OpenSSL 3.x prints "SHA2-256(file)= <hash>"
      # — but the hash itself is always the last whitespace-separated
      # field either way, so `awk '{print $NF}'` is version-agnostic and
      # doesn't need to match the label. Compared case-insensitively
      # since checksums.txt generators and openssl builds differ in case.
      expected_hash="$(printf '%s\n' "$checksum_line" | awk '{print $1}' | tr '[:upper:]' '[:lower:]')"
      actual_hash="$(openssl dgst -sha256 "$file" | awk '{print $NF}' | tr '[:upper:]' '[:lower:]')"
      if [ -z "$expected_hash" ] || [ -z "$actual_hash" ] || [ "$expected_hash" != "$actual_hash" ]; then
        echo "install.sh: checksum mismatch for ${file} (expected ${expected_hash}, got ${actual_hash})" >&2
        exit 1
      fi
      echo "${file}: OK"
      ;;
    *)
      # Unreachable today: require_checksum_verifier is the only writer of
      # CHECKSUM_VERIFIER, and its only branches are gnu/bsd/openssl or a
      # non-zero exit. Kept anyway — this function sits on the
      # download-integrity trust boundary, so an unrecognized value must
      # fail closed by construction rather than by an unenforced invariant
      # elsewhere in the script (e.g. a future refactor that widens how
      # CHECKSUM_VERIFIER gets set).
      echo "install.sh: internal error — unknown checksum verifier '${CHECKSUM_VERIFIER}'; refusing to install unverified download." >&2
      exit 1
      ;;
  esac
}

# require_gzip aborts with an actionable message if gzip is missing.
# `tar -xzf` shells out to a child gzip process; on minimal images that
# ship tar but not gzip this otherwise fails deep inside tar with a
# cryptic "tar (child): gzip: Cannot exec: No such file or directory"
# instead of a clear, actionable message.
require_gzip() {
  if ! command -v gzip >/dev/null 2>&1; then
    echo "install.sh: gzip was not found on PATH; tar needs it to extract the release archive. Install gzip and re-run this script." >&2
    exit 1
  fi
}

# require_tool aborts with an actionable message if tool $1 is missing,
# citing what it's needed for ($2). Covers the remaining core utilities
# this script has no verifier-style fallback for (tar, install) — on a
# minimal image missing one of these, the previous behavior was a late,
# cryptic failure (e.g. "install: applet not found" on busybox, AFTER a
# successful download+verify) instead of a clear, up-front message. Same
# fail-fast rationale as require_gzip, generalized so tar/install don't
# need their own bespoke functions.
require_tool() {
  tool_name="$1"
  purpose="$2"
  if ! command -v "$tool_name" >/dev/null 2>&1; then
    echo "install.sh: ${tool_name} was not found on PATH; ${purpose}. Install it and re-run this script." >&2
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
  require_checksum_verifier
  require_tool tar "it's needed to extract the release archive"
  require_gzip
  require_tool install "it's needed to place the binary with the correct permissions"
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
  # Note: a CRLF-terminated checksums.txt (e.g. served with Windows line
  # endings) makes this suffix match fail too — the trailing \r becomes
  # part of $2's last field, so it never equals archive_suffix. That
  # surfaces as the same "no release asset found" message below rather
  # than a distinct diagnosis. Fail-closed either way; cosmetic only.
  checksum_line="$(awk -v suf="$archive_suffix" \
    '{ if (substr($2, length($2) - length(suf) + 1) == suf) print $0 }' \
    "${workdir}/checksums.txt")"
  if [ -z "$checksum_line" ]; then
    echo "install.sh: no release asset found for os=${os} arch=${arch} (checked ${base_url}/checksums.txt)" >&2
    exit 1
  fi
  archive="$(printf '%s\n' "$checksum_line" | awk '{print $2}')"

  # Reject a filename column that isn't a bare, same-directory name before
  # it's used to build any path. checksums.txt is untrusted input (it's a
  # downloaded file); without this, a "/" or leading "." lets a crafted
  # checksums.txt write the download outside the mktemp workdir — before
  # verification even runs. Requires control of checksums.txt to exploit;
  # closed here defensively since it costs nothing.
  case "$archive" in
    */* | .*)
      echo "install.sh: refusing unsafe archive filename from checksums.txt: ${archive}" >&2
      exit 1
      ;;
  esac

  version_number="${archive%"$archive_suffix"}"
  version_number="${version_number#"${BIN_NAME}"_}"

  echo "install.sh: downloading ${archive} (v${version_number})..."
  fetch_url_to_file "${base_url}/${archive}" "${workdir}/${archive}"

  echo "install.sh: verifying checksum..."
  (
    cd "$workdir"
    verify_checksum "$archive" "$checksum_line"
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
