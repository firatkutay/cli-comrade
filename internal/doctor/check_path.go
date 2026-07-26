package doctor

import (
	"context"
	"path/filepath"

	"github.com/firatkutay/cli-comrade/internal/i18n"
)

// PathCheck reports whether the "comrade" binary is on PATH at all
// (Fail, platform-appropriate fix if not), and — when it is — whether
// PATH resolves to the SAME binary that is actually running this
// diagnostic (OK) or to a different, stale copy (Warn, naming the stale
// path).
func PathCheck(_ context.Context, deps Deps) Result {
	binaryName := "comrade"
	if deps.GOOS == "windows" {
		binaryName = "comrade.exe"
	}

	if deps.LookPath == nil {
		return Result{Severity: SeveritySkip, Summary: i18n.MsgDoctorSkipDependencyUnavailable}
	}
	foundPath, err := deps.LookPath(binaryName)
	if err != nil {
		return Result{
			Severity:    SeverityFail,
			Summary:     i18n.MsgDoctorPathNotFound,
			SummaryArgs: []any{binaryName},
			Fix:         pathFixInstruction(deps.GOOS),
		}
	}

	if deps.Executable == nil {
		return Result{Severity: SeverityOK, Summary: i18n.MsgDoctorPathOK, SummaryArgs: []any{foundPath}}
	}
	runningPath, err := deps.Executable()
	if err != nil {
		return Result{Severity: SeverityOK, Summary: i18n.MsgDoctorPathOK, SummaryArgs: []any{foundPath}}
	}

	resolvedFound, foundErr := filepath.EvalSymlinks(foundPath)
	resolvedRunning, runningErr := filepath.EvalSymlinks(runningPath)
	if foundErr == nil && runningErr == nil && resolvedFound != resolvedRunning {
		return Result{
			Severity:    SeverityWarn,
			Summary:     i18n.MsgDoctorPathStale,
			SummaryArgs: []any{foundPath},
			Fix:         pathFixInstruction(deps.GOOS),
			Detail:      foundPath,
		}
	}

	return Result{Severity: SeverityOK, Summary: i18n.MsgDoctorPathOK, SummaryArgs: []any{foundPath}}
}

// pathFixInstallUnix/pathFixInstallWindows are pathFixInstruction's two
// bare, copy-pasteable remediation commands for "comrade is not on PATH"
// / "PATH resolves to a stale copy": re-running the installer is what
// actually fixes both cases (a fresh install re-adds/repoints the PATH
// entry). These are the SAME one-liners README.md/docs/GUIDE.md/
// docs/INSTALL.md already document as the primary install method — using
// them here (rather than a bare "scripts/install.sh"-style relative
// path, which presumes a cloned repo checkout most `comrade doctor`
// callers won't have) means Fix is a command that is ACTUALLY runnable
// wherever a user happens to be, matching doctor.Result.Fix's own "a
// shell command is not prose to translate" contract (issue #39: this
// package's Fix values are bare commands everywhere else — see
// check_baseurl.go/check_key.go/check_reach.go/check_shellhook.go/
// check_version.go — check_path.go was the one remaining exception,
// smuggling a full explanatory sentence ("re-run ..., or add ... to your
// PATH manually") into Fix instead. That explanation now lives in the
// TRANSLATED MsgDoctorPathNotFound/MsgDoctorPathStale Summary text
// instead — mirrors PR #37's identical fix for check_version.go's own
// Node-managed caveat, see VersionCheck's doc comment).
const (
	pathFixInstallUnix    = "curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh"
	pathFixInstallWindows = "irm https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.ps1 | iex"
)

// pathFixInstruction returns the platform-appropriate bare command above.
func pathFixInstruction(goos string) string {
	if goos == "windows" {
		return pathFixInstallWindows
	}
	return pathFixInstallUnix
}
