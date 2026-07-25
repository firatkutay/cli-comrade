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
# Usage: scripts/check-coverage-floors.sh [path/to/coverage-floors.txt]
# Run from anywhere; it cds to the repo root itself.

set -uo pipefail

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

# A package with NO test files at all (e.g. a hypothetical future
# no-test package with an explicit 0 floor) produces zero lines in the
# profile for that package -- pkg_stats reports "0 0" for it, and the
# awk block below treats that as 0.0% exactly (never a division by
# zero), so it only ever compares against a floor of 0.
pkg_stats() {
	awk -v pkg="$1/" '
		{
			split($1, a, ":")
			path = a[1]
			# pkg is the full "<import-path>/" prefix (from go list, with
			# a trailing slash appended) -- match profile lines whose
			# source file path starts with EXACTLY that prefix, so
			# internal/cli never accidentally matches a sibling package
			# whose name happens to start the same way.
			if (index(path, pkg) == 1) {
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
