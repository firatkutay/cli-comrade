#!/usr/bin/env bash
# check-coverage-floors.sh — the coverage ratchet gate (GitHub issue #21).
#
# Fails when either:
#   1. any package's CURRENT coverage falls below the (decimal, one-tenth-
#      point) floor recorded for it in coverage-floors.txt, or
#   2. coverage-floors.txt and `go list ./...` have drifted out of sync in
#      EITHER direction — a package `go list` reports with no floor entry,
#      or a floor entry for a package `go list` no longer reports. This is
#      the bidirectional drift guard a hand-maintained mirror (the floors
#      file mirrors the module's package list) requires: a one-directional
#      check would let a brand-new package ship with no floor at all.
#
# Precision note: this script computes each package's coverage itself,
# directly from a `-coverprofile` profile (covered statements / total
# statements), rather than trusting `go test -cover`'s own printed
# percentage. `go test -cover` formats with "%.1f" (standard rounding),
# which could round a true 82.65% up to a displayed "82.7%" — silently
# PASSING against an 82.7 floor that the true, unrounded value should
# FAIL. Computing the ratio ourselves and comparing it against the floor
# with plain floating-point `<` (coverage-floors.txt's floors are already
# truncated, never rounded — see its own header) avoids that asymmetry
# entirely: no rounding is ever applied to the actual measured value.
#
# See coverage-floors.txt's own header for the re-baselining procedure.
#
# Invocation: always `bash scripts/check-coverage-floors.sh` (see Makefile/
# CI), never `./scripts/check-coverage-floors.sh` — the latter depends on
# the file's own exec bit, which some filesystems/checkouts don't
# preserve reliably; `bash` explicitly does not care.
#
# Requires bash >= 4 (declare -A, mapfile) -- see the version guard right
# below. This is a SCRIPT requirement, independent of coverage-floors.txt's
# own "floors are Linux-measured" note: that note is about WHERE the
# floors were measured and re-baselined from, not about what this script
# itself needs to run.
#
# Usage: scripts/check-coverage-floors.sh [path/to/coverage-floors.txt]
# Run from anywhere; it cds to the repo root itself.

set -uo pipefail

if [ -z "${BASH_VERSINFO:-}" ] || [ "${BASH_VERSINFO[0]}" -lt 4 ]; then
	echo "check-coverage-floors: requires bash >= 4 (uses declare -A / mapfile); found ${BASH_VERSION:-an unknown, pre-4 bash}." >&2
	echo "check-coverage-floors: macOS ships bash 3.2 by default -- install a current one with: brew install bash" >&2
	echo "check-coverage-floors: then re-run this script via that bash explicitly, e.g.: /opt/homebrew/bin/bash scripts/check-coverage-floors.sh" >&2
	exit 1
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
floors_file="${1:-$repo_root/coverage-floors.txt}"

if [ ! -f "$floors_file" ]; then
	echo "check-coverage-floors: floors file not found: $floors_file" >&2
	exit 1
fi

cd "$repo_root" || exit 1

declare -A floor_of
while IFS= read -r raw_line || [ -n "$raw_line" ]; do
	line="${raw_line%%#*}"
	line="$(printf '%s' "$line" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
	[ -z "$line" ] && continue
	read -r pkg floor <<<"$line"
	if [ -z "$pkg" ] || [ -z "$floor" ]; then
		echo "check-coverage-floors: malformed line in $floors_file: $raw_line" >&2
		exit 1
	fi
	if [ -n "${floor_of[$pkg]+set}" ]; then
		echo "check-coverage-floors: duplicate floor entry for $pkg in $floors_file (first: ${floor_of[$pkg]}, again: $floor) -- a repeated key silently wins last-one-in otherwise; fix the file" >&2
		exit 1
	fi
	floor_of["$pkg"]="$floor"
done <"$floors_file"

mapfile -t all_pkgs < <(go list ./...)

drift=0
for pkg in "${all_pkgs[@]}"; do
	if [ -z "${floor_of[$pkg]+set}" ]; then
		echo "check-coverage-floors: $pkg has no floor entry in $floors_file (source has it, mirror doesn't) — add one" >&2
		drift=1
	fi
done

declare -A pkg_exists
for pkg in "${all_pkgs[@]}"; do
	pkg_exists["$pkg"]=1
done
for pkg in "${!floor_of[@]}"; do
	if [ -z "${pkg_exists[$pkg]+set}" ]; then
		echo "check-coverage-floors: $floors_file has a floor entry for $pkg, which no longer exists (mirror has it, source doesn't) — remove it" >&2
		drift=1
	fi
done

if [ "$drift" -ne 0 ]; then
	exit 1
fi

profile="$(mktemp)"
trap 'rm -f "$profile"' EXIT

echo "check-coverage-floors: running go test -coverprofile ./..."
test_output="$(go test ./... -coverprofile="$profile" -covermode=set 2>&1)"
test_status=$?
echo "$test_output"

if [ "$test_status" -ne 0 ]; then
	echo "check-coverage-floors: go test itself failed; fix the failing tests before checking coverage floors" >&2
	exit 1
fi

# Even a package with NO test files at all still gets a profile line per
# source file (all statements, count 0) on the Go toolchain this repo
# pins -- e.g. "somepkg/file.go:3.16,5.2 1 0" -- so pkg_stats below never
# actually hits its own total==0 fallback for a real Go package today;
# that fallback exists purely so a future toolchain change (or a
# genuinely empty/unbuildable package) can't turn into a division by
# zero.
pkg_stats() {
	awk -v pkg="$1/" '
		{
			split($1, a, ":")
			path = a[1]
			plen = length(pkg)
			# pkg is the full "<import-path>/" prefix (from go list, with a
			# trailing slash appended). Match profile lines whose source
			# file path starts with EXACTLY that prefix, AND has no
			# further "/" after it -- i.e. the file lives directly in pkg,
			# not in one of pkgs OWN subpackages. Without the second
			# check, a nested package (e.g. internal/llm/openai) would
			# match its parents prefix too and get double-counted into
			# the parents total/covered, silently corrupting the parents
			# percentage.
			if (index(path, pkg) == 1 && index(substr(path, plen + 1), "/") == 0) {
				total += $2
				if ($3 + 0 > 0) covered += $2
			}
		}
		END { printf "%d %d\n", covered + 0, total + 0 }
	' "$profile"
}

fail=0
for pkg in "${all_pkgs[@]}"; do
	floor="${floor_of[$pkg]}"
	read -r covered total <<<"$(pkg_stats "$pkg")"

	if [ "$total" -eq 0 ]; then
		actual="0.000"
	else
		actual="$(awk -v c="$covered" -v t="$total" 'BEGIN{printf "%.3f", c/t*100}')"
	fi

	below="$(awk -v a="$actual" -v f="$floor" 'BEGIN{print (a+0 < f+0) ? 1 : 0}')"
	if [ "$below" -eq 1 ]; then
		echo "check-coverage-floors: FAIL $pkg coverage ${actual}% is BELOW its floor of ${floor}%" >&2
		fail=1
	else
		echo "check-coverage-floors: OK   $pkg coverage ${actual}% >= floor ${floor}%"
	fi
done

if [ "$fail" -ne 0 ]; then
	echo "check-coverage-floors: one or more packages fell below their recorded coverage floor." >&2
	exit 1
fi

echo "check-coverage-floors: all packages meet their recorded coverage floor."
