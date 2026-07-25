// Package envkeys is a leaf package (zero non-stdlib imports, by
// design) holding the small set of environment-variable names/values
// this tree's own processes hand to each other and needs to strip from
// spawned children — PR #37 review, P3.
//
// ManagedByEnvVar previously lived in internal/update, which
// internal/executor then imported just to reference this one string
// (MEDIUM-2's fix). That inverted this tree's own layering: executor is
// the lowest-level execution primitive in this codebase, and
// internal/update's own dependency closure (192 packages — archive/tar,
// net/http, the crypto/internal/fips140 stack, ...) is nearly as large
// as internal/executor's own PRE-existing closure (194). There was no
// import cycle only because internal/update happened to import zero
// cli-comrade packages of its own — that was luck, not a guarantee, and
// it left executor pulling in a release-download/checksum/signature
// stack to reference one constant.
//
// Every process-spawn site in this tree (internal/executor,
// internal/cli/config.go's $EDITOR launch — see StripManaged's own doc
// comment for the site inventory) and internal/update itself (which
// needs the SAME constant for IsNPMManaged's env signal) import this
// package instead, so there is exactly one implementation, referenced
// from the bottom of the dependency graph rather than the middle of it.
package envkeys

import "strings"

// ManagedByEnvVar is the environment variable npm/main/bin/comrade.js's
// dispatcher sets (in a copy of its own process env, before spawning the
// real Go binary) so a self-update check downstream can tell it is
// running under a Node package manager's install without having to
// inspect its own path at all — see internal/update.IsNPMManaged.
//
// The one cross-language copy of this literal that CANNOT be derived
// from this constant (npm/main/bin/comrade.js is JavaScript, a
// different language entirely) is guarded bidirectionally by
// managed_mirror_test.go, alongside this file — see its own doc comment.
const ManagedByEnvVar = "COMRADE_MANAGED_BY"

// ManagedByEnvValueNPM is ManagedByEnvVar's expected value for a
// Node-package-manager-managed install. Any other value (or the
// variable being unset) is treated as "not npm-managed" by
// update.IsNPMManaged's env signal.
const ManagedByEnvValueNPM = "npm"

// StripManaged returns environ (in os.Environ()'s "KEY=VALUE" shape)
// with every entry whose key matches ManagedByEnvVar removed — used by
// every process-spawn site in this tree so ManagedByEnvVar never leaks
// into a spawned command's own environment:
//
//   - internal/executor.Run (a generated plan step) — MEDIUM-2's
//     original fix.
//   - internal/cli's `comrade config edit` launching $EDITOR (PR #37
//     review, N1) — an editor's own shell-out (e.g. vim's `:!comrade
//     upgrade`) would otherwise inherit it exactly like a plan step
//     would.
//
// internal/context/shell.go's CommandRunner is deliberately NOT one of
// these sites: it only runs a small, fixed set of shell/version-probe
// binaries this package itself decides to invoke (e.g. detecting the
// shell's own --version) — never comrade, so there is nothing to strip
// there.
//
// The key match is via strings.EqualFold, not ==, per PR #37 review's
// N2: Windows environment-variable lookups are case-INSENSITIVE at the
// OS level (syscall.Getenv on Windows calls GetEnvironmentVariable,
// which resolves a variable regardless of case), so
// update.IsNPMManaged's env signal would still DETECT a lowercase
// comrade_managed_by=npm on Windows even though a case-SENSITIVE strip
// here would fail to REMOVE it — reopening exactly the
// deliberate-attacker half of MEDIUM-2 (a user-settable variable that
// should never survive into a spawned command) on that one platform.
func StripManaged(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		key, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(key, ManagedByEnvVar) {
			continue
		}
		out = append(out, kv)
	}
	return out
}
