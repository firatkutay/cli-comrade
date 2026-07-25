#!/usr/bin/env bash
# Recurrence guard for the exec-bit bug class: every shell script under
# scripts/ and npm/test/ that is invoked directly (not merely sourced or
# always run via an explicit `bash <script>`) must be committed executable
# (mode 100755) IN THE GIT INDEX. This has shipped broken twice already
# (scripts/check-coverage-floors.sh, then scripts/build-npm-packages.sh)
# -- both times invisible locally because the affected checkout happened
# to run on a filesystem/config where the working-tree permission bit
# didn't matter (e.g. drvfs with core.fileMode=false), while a real clone
# on a normal filesystem got "Permission denied", and worse, downstream
# tests mis-reported that as a WORDING regression rather than a
# permissions bug.
#
# `git ls-files -s` reads the mode bit recorded in the git index itself --
# unaffected by the local working tree's filesystem, core.fileMode, or
# whether the file happens to be +x on disk right now. This is what
# guarantees the check fails identically on a fresh clone, not just
# locally: a bit stored as 100644 stays 100644 in the index no matter
# where or how the repo is checked out.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." >/dev/null 2>&1 && pwd)"

cd "${REPO_ROOT}"

failures=0
checked=0

while read -r mode _hash _stage path; do
  checked=$((checked + 1))
  if [ "${mode}" != "100755" ]; then
    echo "FAIL: ${path} is committed as mode ${mode} in the git index, expected 100755 (executable)" >&2
    failures=$((failures + 1))
  else
    echo "PASS: ${path} is 100755 in the git index"
  fi
done < <(git ls-files -s -- 'scripts/*.sh' 'npm/test/*.sh')

if [ "${checked}" -eq 0 ]; then
  echo "test-script-permissions.sh: no scripts/*.sh or npm/test/*.sh files were found -- the glob is broken" >&2
  exit 1
fi

echo "---"
if [ "${failures}" -eq 0 ]; then
  echo "test-script-permissions.sh: all ${checked} script(s) are executable in the git index"
  exit 0
fi
echo "test-script-permissions.sh: ${failures} of ${checked} script(s) are NOT executable in the git index" >&2
exit 1
