#!/bin/sh
# scripts/install_test.sh — POSIX-sh unit tests for the PATH-setup logic
# factored out of scripts/install.sh (configure_path_in_rc and its
# helpers rc_file_for_shell / path_export_line_for_shell).
#
# Runs entirely offline: no network download, no real "comrade" install,
# and no modification of the real invoking user's actual shell rc files —
# every test executes in a subshell against a throwaway $HOME created
# with mktemp -d and destroyed afterward.
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

run_test test_bash_appends_marked_line_once
run_test test_rerun_is_idempotent
run_test test_no_modify_path_opt_out
run_test test_zsh_selects_zshrc
run_test test_fish_selects_config_fish
run_test test_unknown_shell_selects_profile

echo "----"
echo "install_test.sh: ${tests_run} test(s) run, ${failures} failure(s)"

if [ "$failures" -ne 0 ]; then
  exit 1
fi
