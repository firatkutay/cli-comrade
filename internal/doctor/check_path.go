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
		return Result{Severity: SeveritySkip}
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

// pathFixInstruction is a short, platform-appropriate, copy-pasteable
// remediation for "comrade is not on PATH" / "PATH resolves to a stale
// copy" — deliberately plain, unlocalized text (see doctor.Result.Fix's
// own doc comment): re-running the installer is what actually fixes
// both cases (a fresh install re-adds/repoints the PATH entry).
func pathFixInstruction(goos string) string {
	if goos == "windows" {
		return "re-run scripts/install.ps1, or add comrade's install directory to your PATH manually"
	}
	return "re-run scripts/install.sh, or add comrade's install directory to your PATH manually"
}
