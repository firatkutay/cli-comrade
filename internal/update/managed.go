package update

import "strings"

// managedByEnvVar is the environment variable npm/main/bin/comrade.js
// sets (in a copy of its own process env, before spawning the real Go
// binary) so a self-update check downstream can tell it is running
// under npm's dispatcher without having to inspect its own path at all.
const managedByEnvVar = "COMRADE_MANAGED_BY"

// managedByEnvValue is managedByEnvVar's expected value for an
// npm-managed install. Any other value (or the variable being unset) is
// treated as "not npm-managed" by the env signal — see IsNPMManaged.
const managedByEnvValue = "npm"

// GetenvFunc is the exact signature of os.Getenv — named here so
// IsNPMManaged's callers (and its own tests) can inject a fake without
// any global/package-level state, per this repo's "no global state; DI
// everywhere" rule.
type GetenvFunc func(string) string

// ExecutableFunc is the exact signature of os.Executable — see
// GetenvFunc's doc comment for why this is injected rather than called
// directly.
type ExecutableFunc func() (string, error)

// EvalSymlinksFunc is the exact signature of filepath.EvalSymlinks —
// see GetenvFunc's doc comment for why this is injected rather than
// called directly.
type EvalSymlinksFunc func(string) (string, error)

// IsNPMManaged reports whether the currently running comrade binary is
// managed by npm — the case `comrade upgrade` must refuse to self-update
// in (see internal/cli/upgrade.go), since overwriting the binary
// node_modules/@firatkutay/comrade-<platform>/bin/comrade in place would
// desync npm's own recorded installed version from what is actually on
// disk: the next `npm update` would silently revert the in-place
// self-update.
//
// Two independent signals are checked, either one being sufficient:
//
//  1. The env signal (primary): npm/main/bin/comrade.js sets
//     COMRADE_MANAGED_BY=npm in the child environment before spawning
//     the real binary — cheap, exact, and covers the overwhelmingly
//     common case (the binary invoked through npm's own dispatcher).
//  2. The path signal (fallback): if the binary is invoked directly,
//     bypassing the dispatcher (e.g. a user or script hardcodes the
//     resolved node_modules path), the env var is never set. Resolving
//     the running executable's own path (through any symlinks) and
//     checking for a path segment exactly equal to "node_modules"
//     catches that case too.
//
// executable and evalSymlinks are injected (matching os.Executable and
// filepath.EvalSymlinks) so tests can exercise both signals without
// touching this process's real executable path or filesystem — see
// managed_test.go.
//
// Either underlying call failing (an error from executable, or from
// evalSymlinks — e.g. a dangling symlink, or a path that no longer
// exists on disk) is treated as "not npm-managed" rather than
// propagated: this is a fail-OPEN check, deliberately, because the cost
// of a false positive here (refusing to install a real update) is a user
// who cannot upgrade at all, which is worse than the cost of a false
// negative (a rare direct-invocation edge case silently allowed to
// self-update).
func IsNPMManaged(getenv GetenvFunc, executable ExecutableFunc, evalSymlinks EvalSymlinksFunc) bool {
	if getenv(managedByEnvVar) == managedByEnvValue {
		return true
	}

	exePath, err := executable()
	if err != nil {
		return false
	}
	resolvedPath, err := evalSymlinks(exePath)
	if err != nil {
		return false
	}
	return hasNodeModulesSegment(resolvedPath)
}

// hasNodeModulesSegment reports whether path contains a path segment
// exactly equal to "node_modules" — a directory merely named
// "my_node_modules_backup" (or any other name that only CONTAINS
// "node_modules" as a substring) must not match, so this splits path
// into its individual segments and compares each one exactly, rather
// than using strings.Contains.
//
// path is normalized by replacing every backslash with a forward slash
// before splitting on "/", so this correctly recognizes both Windows
// ("C:\...\node_modules\...") and Unix ("/.../node_modules/...")
// forms regardless of which OS this process itself is actually running
// on — deliberately NOT relying solely on the native filepath.Separator
// for the split, since the resolved executable path being checked can
// legitimately be in the other OS's form in cross-platform test
// coverage, and a real Windows path may also appear with forward
// slashes (many Node/npm tools normalize to "/" even on Windows).
func hasNodeModulesSegment(path string) bool {
	normalized := strings.ReplaceAll(path, `\`, "/")
	for _, segment := range strings.Split(normalized, "/") {
		if segment == "node_modules" {
			return true
		}
	}
	return false
}
