#!/bin/sh
# scripts/install_test.sh — POSIX-sh unit tests for scripts/install.sh:
# the PATH-setup logic (configure_path_in_rc and its helpers
# rc_file_for_shell / path_export_line_for_shell), the checksums.txt
# cosign-signature verification added for GitHub issue #28
# (require_signature_verifier / verify_checksums_signature /
# allow_unsigned_or_fail), and the curl/wget downloader dispatch
# (require_downloader / fetch_url_to_file).
#
# Runs entirely offline: no network download, no real "comrade" install,
# and no modification of the real invoking user's actual shell rc files —
# every test executes in a subshell against a throwaway $HOME created
# with mktemp -d and destroyed afterward. The signature tests use an
# EPHEMERAL, in-test-generated ECDSA key pair (never the real embedded
# COSIGN_PUB/production key) — the same approach
# internal/update/signature_test.go uses in Go — and the downloader
# dispatch tests use a stub wget/curl script, so no real network access
# is needed to prove either mechanism.
#
# Usage:
#   sh scripts/install_test.sh
#   dash scripts/install_test.sh
#
# Wired into `go test ./...` (the project's `make test` gate) via
# internal/cli/scripts_test.go, which shells out to this file — see that
# test for why: it keeps this POSIX-only test running on every gate
# without adding a second, shell-only command surface.
set -eu

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

# Source install.sh with COMRADE_INSTALL_SH_TEST=1 so its trailing
# `main "$@"` call is skipped — this only defines functions and the
# PATH_MARKER variable, it does not download or install anything.
COMRADE_INSTALL_SH_TEST=1
export COMRADE_INSTALL_SH_TEST
# shellcheck source=./install.sh
. "${script_dir}/install.sh"

failures=0
tests_run=0

fail() {
  failures=$((failures + 1))
  echo "FAIL: $1" >&2
}

pass() {
  echo "ok: $1"
}

# run_test executes the test function named $1 inside a subshell with a
# fresh throwaway HOME, so no test can observe another test's rc-file
# state, and none of them ever touch the real invoking user's dotfiles.
run_test() {
  test_name="$1"
  tests_run=$((tests_run + 1))
  tmp_home="$(mktemp -d)"
  if (
    HOME="$tmp_home"
    export HOME
    "$test_name" "$tmp_home"
  ); then
    pass "$test_name"
  else
    fail "$test_name"
  fi
  rm -rf "$tmp_home"
}

# count_matches prints how many lines of file $2 contain literal
# substring $1, treating "file does not exist" or "no match" as 0
# rather than letting grep's non-zero exit trip `set -e`.
count_matches() {
  if [ ! -f "$2" ]; then
    echo 0
    return 0
  fi
  grep -Fc -- "$1" "$2" 2>/dev/null || true
}

# --- bash: rc lacking the dir -> marked line appended exactly once ---
test_bash_appends_marked_line_once() {
  home="$1"
  install_dir="$home/.local/bin"
  mkdir -p "$install_dir"

  configure_path_in_rc "$install_dir" "bash" >/dev/null

  rc="$home/.bashrc"
  if [ ! -f "$rc" ]; then
    echo "  expected $rc to exist" >&2
    return 1
  fi

  marker_count="$(count_matches "$PATH_MARKER" "$rc")"
  if [ "$marker_count" -ne 1 ]; then
    echo "  expected PATH_MARKER once in $rc, got $marker_count" >&2
    return 1
  fi

  export_count="$(count_matches 'export PATH="$HOME/.local/bin:$PATH"' "$rc")"
  if [ "$export_count" -ne 1 ]; then
    echo "  expected the export line once in $rc, got $export_count" >&2
    return 1
  fi
  return 0
}

# --- running configure_path_in_rc again must not duplicate the edit ---
test_rerun_is_idempotent() {
  home="$1"
  install_dir="$home/.local/bin"
  mkdir -p "$install_dir"

  configure_path_in_rc "$install_dir" "bash" >/dev/null
  configure_path_in_rc "$install_dir" "bash" >/dev/null
  configure_path_in_rc "$install_dir" "bash" >/dev/null

  rc="$home/.bashrc"
  marker_count="$(count_matches "$PATH_MARKER" "$rc")"
  if [ "$marker_count" -ne 1 ]; then
    echo "  expected PATH_MARKER once after 3 runs, got $marker_count" >&2
    return 1
  fi
  return 0
}

# --- COMRADE_NO_MODIFY_PATH=1 -> no rc change, warning printed ---
test_no_modify_path_opt_out() {
  home="$1"
  install_dir="$home/.local/bin"
  mkdir -p "$install_dir"

  COMRADE_NO_MODIFY_PATH=1
  export COMRADE_NO_MODIFY_PATH
  out="$(configure_path_in_rc "$install_dir" "bash")"
  unset COMRADE_NO_MODIFY_PATH

  rc="$home/.bashrc"
  if [ -e "$rc" ]; then
    echo "  expected $rc not to be created when opted out" >&2
    return 1
  fi

  case "$out" in
    *"COMRADE_NO_MODIFY_PATH is set"*) ;;
    *)
      echo "  expected the opt-out warning in output, got: $out" >&2
      return 1
      ;;
  esac
  return 0
}

# --- zsh -> .zshrc chosen, fish -> config.fish chosen ---
test_zsh_selects_zshrc() {
  home="$1"
  install_dir="$home/.local/bin"
  mkdir -p "$install_dir"

  configure_path_in_rc "$install_dir" "zsh" >/dev/null

  if [ ! -f "$home/.zshrc" ]; then
    echo "  expected $home/.zshrc to exist" >&2
    return 1
  fi
  if [ -f "$home/.bashrc" ]; then
    echo "  did not expect $home/.bashrc to exist for shell=zsh" >&2
    return 1
  fi
  return 0
}

test_fish_selects_config_fish() {
  home="$1"
  install_dir="$home/.local/bin"
  mkdir -p "$install_dir"

  configure_path_in_rc "$install_dir" "fish" >/dev/null

  rc="$home/.config/fish/config.fish"
  if [ ! -f "$rc" ]; then
    echo "  expected $rc to exist" >&2
    return 1
  fi

  fish_line_count="$(count_matches 'set -gx PATH $HOME/.local/bin $PATH' "$rc")"
  if [ "$fish_line_count" -ne 1 ]; then
    echo "  expected the fish PATH line once in $rc, got $fish_line_count" >&2
    return 1
  fi
  return 0
}

# --- an unrecognized $SHELL falls back to .profile ---
test_unknown_shell_selects_profile() {
  home="$1"
  install_dir="$home/.local/bin"
  mkdir -p "$install_dir"

  configure_path_in_rc "$install_dir" "tcsh" >/dev/null

  if [ ! -f "$home/.profile" ]; then
    echo "  expected $home/.profile to exist for an unrecognized shell" >&2
    return 1
  fi
  return 0
}

# === checksums.txt signature verification (GitHub issue #28) ===

# make_test_signing_key generates an ephemeral ECDSA P-256 key pair into
# $1/test_priv.pem and $1/test_pub.pem — never the real embedded
# COSIGN_PUB — so signature tests can exercise real ECDSA verification
# without depending on (or risking) the project's actual production key.
make_test_signing_key() {
  dir="$1"
  openssl ecparam -name prime256v1 -genkey -noout -out "$dir/test_priv.pem" 2>/dev/null
  openssl ec -in "$dir/test_priv.pem" -pubout -out "$dir/test_pub.pem" 2>/dev/null
}

# sign_test_checksums signs $1/checksums.txt with $1/test_priv.pem into
# $1/checksums.txt.sig, in cosign's own format: base64 (single-line, via
# openssl's -A) of the raw ASN.1 DER ECDSA signature over the file's
# SHA-256 digest — the exact format verify_checksums_signature (and
# internal/update/signature.go's Go-side verifier) expects.
sign_test_checksums() {
  dir="$1"
  openssl dgst -sha256 -sign "$dir/test_priv.pem" -out "$dir/sig.der" "$dir/checksums.txt" 2>/dev/null
  openssl base64 -A -in "$dir/sig.der" -out "$dir/checksums.txt.sig" 2>/dev/null
}

# --- a validly-signed checksums.txt is accepted ---
test_verify_checksums_signature_accepts_valid_signature() {
  home="$1"
  work="$(mktemp -d)"
  make_test_signing_key "$work"
  printf 'deadbeef00112233445566778899aabbccddeeff00112233445566778899aa  comrade_9.9.9_linux_amd64.tar.gz\n' >"$work/checksums.txt"
  sign_test_checksums "$work"

  saved_cosign_pub="$COSIGN_PUB"
  COSIGN_PUB="$(cat "$work/test_pub.pem")"

  result=0
  verify_checksums_signature "$work/checksums.txt" "$work/checksums.txt.sig" >/dev/null 2>&1 || result=1

  COSIGN_PUB="$saved_cosign_pub"
  rm -rf "$work"

  if [ "$result" -ne 0 ]; then
    echo "  expected a validly-signed checksums.txt to verify successfully" >&2
    return 1
  fi
  return 0
}

# --- a checksums.txt tampered with AFTER signing is rejected ---
#
# Non-vacuity check for this test suite: temporarily editing the "!=" in
# verify_checksums_signature's ecdsa.VerifyASN1-equivalent condition (or
# here, deliberately not tampering the file before signing) makes this
# test fail, confirming it actually exercises rejection rather than
# vacuously passing — see this PR's own report for that proof.
test_verify_checksums_signature_rejects_tampered_checksums() {
  home="$1"
  work="$(mktemp -d)"
  make_test_signing_key "$work"
  printf 'deadbeef00112233445566778899aabbccddeeff00112233445566778899aa  comrade_9.9.9_linux_amd64.tar.gz\n' >"$work/checksums.txt"
  sign_test_checksums "$work"

  # Tamper AFTER signing: the on-disk signature no longer matches.
  printf 'tampered-extra-line\n' >>"$work/checksums.txt"

  saved_cosign_pub="$COSIGN_PUB"
  COSIGN_PUB="$(cat "$work/test_pub.pem")"

  # verify_checksums_signature calls `exit 1` on a mismatch — it must run
  # inside an explicit subshell so that exit only terminates the
  # subshell, not this test function (and the run_test subshell above
  # it), letting us assert on the failure instead of being killed by it.
  if (verify_checksums_signature "$work/checksums.txt" "$work/checksums.txt.sig") >/dev/null 2>&1; then
    COSIGN_PUB="$saved_cosign_pub"
    rm -rf "$work"
    echo "  expected a tampered checksums.txt to be REJECTED, but verify_checksums_signature reported success" >&2
    return 1
  fi

  COSIGN_PUB="$saved_cosign_pub"
  rm -rf "$work"
  return 0
}

# --- allow_unsigned_or_fail: default policy is fail-closed ---
test_allow_unsigned_or_fail_aborts_by_default() {
  home="$1"
  if (
    unset COMRADE_INSTALL_ALLOW_UNSIGNED 2>/dev/null
    allow_unsigned_or_fail "test reason."
  ) >/dev/null 2>&1; then
    echo "  expected allow_unsigned_or_fail to abort (non-zero exit) when COMRADE_INSTALL_ALLOW_UNSIGNED is unset" >&2
    return 1
  fi
  return 0
}

# --- allow_unsigned_or_fail: explicit override warns loudly and continues ---
test_allow_unsigned_or_fail_warns_and_continues_when_overridden() {
  home="$1"
  out="$(COMRADE_INSTALL_ALLOW_UNSIGNED=1 allow_unsigned_or_fail "test reason." 2>&1)"
  rc=$?
  if [ "$rc" -ne 0 ]; then
    echo "  expected allow_unsigned_or_fail to return 0 when COMRADE_INSTALL_ALLOW_UNSIGNED is set, got rc=$rc" >&2
    return 1
  fi
  case "$out" in
    *"WARNING"*"test reason."*"COMRADE_INSTALL_ALLOW_UNSIGNED is set"*) ;;
    *)
      echo "  expected a WARNING mentioning the reason and the override, got: $out" >&2
      return 1
      ;;
  esac
  return 0
}

# --- require_signature_verifier: picks openssl when it's on PATH ---
test_require_signature_verifier_selects_openssl_when_present() {
  home="$1"
  OPENSSL_BIN=openssl
  export OPENSSL_BIN
  require_signature_verifier
  if [ "$SIGNATURE_VERIFIER" != "openssl" ]; then
    echo "  expected SIGNATURE_VERIFIER=openssl when a working openssl is on PATH, got '$SIGNATURE_VERIFIER'" >&2
    return 1
  fi
  return 0
}

# --- require_signature_verifier: fails closed by default without openssl ---
test_require_signature_verifier_fails_closed_without_openssl_by_default() {
  home="$1"
  if (
    OPENSSL_BIN="/nonexistent/openssl-binary-does-not-exist"
    export OPENSSL_BIN
    unset COMRADE_INSTALL_ALLOW_UNSIGNED 2>/dev/null
    require_signature_verifier
  ) >/dev/null 2>&1; then
    echo "  expected require_signature_verifier to fail-closed when openssl is missing and COMRADE_INSTALL_ALLOW_UNSIGNED is unset" >&2
    return 1
  fi
  return 0
}

# --- require_signature_verifier: explicit override falls back to
# checksum-only (SIGNATURE_VERIFIER=none) with a loud warning ---
test_require_signature_verifier_falls_back_when_overridden() {
  home="$1"
  out_file="$(mktemp)"
  (
    OPENSSL_BIN="/nonexistent/openssl-binary-does-not-exist"
    export OPENSSL_BIN
    COMRADE_INSTALL_ALLOW_UNSIGNED=1
    export COMRADE_INSTALL_ALLOW_UNSIGNED
    require_signature_verifier
    echo "SIGNATURE_VERIFIER=$SIGNATURE_VERIFIER"
  ) >"$out_file" 2>&1
  rc=$?

  if [ "$rc" -ne 0 ]; then
    echo "  expected require_signature_verifier to return 0 under the override, got rc=$rc; output: $(cat "$out_file")" >&2
    rm -f "$out_file"
    return 1
  fi
  if ! grep -q "SIGNATURE_VERIFIER=none" "$out_file"; then
    echo "  expected SIGNATURE_VERIFIER=none after falling back, got: $(cat "$out_file")" >&2
    rm -f "$out_file"
    return 1
  fi
  if ! grep -q "WARNING" "$out_file"; then
    echo "  expected a WARNING to be printed when falling back under the override" >&2
    rm -f "$out_file"
    return 1
  fi
  rm -f "$out_file"
  return 0
}

# === curl/wget downloader dispatch ===
#
# These prove fetch_url_to_file (the SAME helper used for both
# checksums.txt AND, as of GitHub issue #28, checksums.txt.sig) actually
# dispatches through whichever downloader require_downloader resolved,
# rather than hardcoding one — the exact regression this guards against
# is a bare `curl` call sneaking into the new checksums.txt.sig fetch,
# which would silently break every wget-only machine (mirroring the
# stock-macOS-has-no-sha256sum lesson PR #32 already learned once for the
# checksum tool itself). A stub wget/curl script (never the real network)
# records how it was invoked and hands back a fixed fixture file.

# --- fetch_url_to_file dispatches to wget, with the right args, when
# curl is absent from PATH ---
test_fetch_url_to_file_uses_wget_when_curl_absent() {
  home="$1"
  fakebin="$(mktemp -d)"
  wget_log="$(mktemp)"
  outdest="$(mktemp)"

  # The stub writes its fixture content with a plain shell builtin
  # (printf + redirection) rather than shelling out to `cp` — deliberately
  # no external-tool dependency beyond sh itself. An earlier version of
  # this stub used `ln -s "$(command -v cp)" "$fakebin/cp"` to give the
  # stub a `cp`, which is what actually broke on windows-latest CI:
  # symlink creation needs a privilege Windows doesn't grant by default,
  # so the "cp" inside the stub silently produced no output there.
  cat >"$fakebin/wget" <<'STUB'
#!/bin/sh
# Records its own invocation, then stands in for a real network fetch by
# writing fixed fixture content to the requested output path ($2, per
# fetch_url_to_file's `wget -qO "$2" "$1"` call).
echo "$@" >"$WGET_LOG"
printf 'fixture-content\n' >"$2"
STUB
  chmod +x "$fakebin/wget"

  WGET_LOG="$wget_log"
  export WGET_LOG

  real_path="$PATH"
  PATH="$fakebin"
  require_downloader
  downloader_result="$DOWNLOADER"
  fetch_url_to_file "https://example.invalid/thing" "$outdest"
  PATH="$real_path"
  rm -rf "$fakebin"

  if [ "$downloader_result" != "wget" ]; then
    echo "  expected DOWNLOADER=wget when curl is absent from PATH, got '$downloader_result'" >&2
    return 1
  fi
  if ! grep -Fq -- "-qO ${outdest} https://example.invalid/thing" "$wget_log"; then
    echo "  expected wget invoked as '-qO ${outdest} https://example.invalid/thing', got: $(cat "$wget_log")" >&2
    return 1
  fi
  if ! grep -q "fixture-content" "$outdest"; then
    echo "  expected fetch_url_to_file's output to contain the stub wget's fixture content" >&2
    return 1
  fi
  return 0
}

# --- fetch_url_to_file dispatches to curl, with the right args, when
# curl is available (curl is preferred over wget) ---
test_fetch_url_to_file_uses_curl_when_available() {
  home="$1"
  fakebin="$(mktemp -d)"
  curl_log="$(mktemp)"
  outdest="$(mktemp)"

  # See test_fetch_url_to_file_uses_wget_when_curl_absent's comment above
  # for why this writes its fixture with a plain shell builtin instead of
  # shelling out to `cp` via a symlink.
  cat >"$fakebin/curl" <<'STUB'
#!/bin/sh
# Records its own invocation, then stands in for a real network fetch by
# writing fixed fixture content to the requested output path ($3, per
# fetch_url_to_file's `curl -fsSL -o "$2" "$1"` call).
echo "$@" >"$CURL_LOG"
printf 'fixture-content\n' >"$3"
STUB
  chmod +x "$fakebin/curl"

  CURL_LOG="$curl_log"
  export CURL_LOG

  real_path="$PATH"
  PATH="$fakebin"
  require_downloader
  downloader_result="$DOWNLOADER"
  fetch_url_to_file "https://example.invalid/thing" "$outdest"
  PATH="$real_path"
  rm -rf "$fakebin"

  if [ "$downloader_result" != "curl" ]; then
    echo "  expected DOWNLOADER=curl when curl is on PATH, got '$downloader_result'" >&2
    return 1
  fi
  if ! grep -Fq -- "-fsSL -o ${outdest} https://example.invalid/thing" "$curl_log"; then
    echo "  expected curl invoked as '-fsSL -o ${outdest} https://example.invalid/thing', got: $(cat "$curl_log")" >&2
    return 1
  fi
  if ! grep -q "fixture-content" "$outdest"; then
    echo "  expected fetch_url_to_file's output to contain the stub curl's fixture content" >&2
    return 1
  fi
  return 0
}

run_test test_bash_appends_marked_line_once
run_test test_rerun_is_idempotent
run_test test_no_modify_path_opt_out
run_test test_zsh_selects_zshrc
run_test test_fish_selects_config_fish
run_test test_unknown_shell_selects_profile
run_test test_verify_checksums_signature_accepts_valid_signature
run_test test_verify_checksums_signature_rejects_tampered_checksums
run_test test_allow_unsigned_or_fail_aborts_by_default
run_test test_allow_unsigned_or_fail_warns_and_continues_when_overridden
run_test test_require_signature_verifier_selects_openssl_when_present
run_test test_require_signature_verifier_fails_closed_without_openssl_by_default
run_test test_require_signature_verifier_falls_back_when_overridden
run_test test_fetch_url_to_file_uses_wget_when_curl_absent
run_test test_fetch_url_to_file_uses_curl_when_available

echo "----"
echo "install_test.sh: ${tests_run} test(s) run, ${failures} failure(s)"

if [ "$failures" -ne 0 ]; then
  exit 1
fi
