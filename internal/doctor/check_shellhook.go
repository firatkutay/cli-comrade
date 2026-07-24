package doctor

import (
	"context"
	"os"

	comradecontext "github.com/firatkutay/cli-comrade/internal/context"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/shellinit"
)

// ShellHookCheck reports whether the current shell could even be
// detected/is one comrade supports (Skip if not — an undetectable or
// unsupported shell is an environment limitation, not a problem to fix),
// whether its rc/profile file could be resolved (Skip if not, e.g. HOME
// unset or no PowerShell binary on PATH), and — once resolved — whether
// comrade's current block is installed (OK), or absent/outdated (Warn,
// fix `comrade init <shell>`).
//
// This deliberately targets only the SINGLE rc/profile path
// shellinit.RCPath resolves for the detected shell — unlike `comrade
// init powershell` on GOOS=windows, it does not probe every installed
// PowerShell variant via shellinit.ResolvePowerShellProfiles. A stale/
// missing block found here always points the user at `comrade init
// <shell>`, which DOES run the full multi-variant flow — so the
// remediation is complete even though this read-only check itself stays
// single-profile, simpler, and consistent with every other check's flat
// "one Result" shape.
func ShellHookCheck(ctx context.Context, deps Deps) Result {
	if deps.Getenv == nil {
		return Result{Severity: SeveritySkip}
	}
	shellName := comradecontext.DetectShell(deps.GOOS, deps.Getenv)
	if shellName == "" {
		return Result{Severity: SeveritySkip, Summary: i18n.MsgDoctorShellHookUndetected}
	}
	shell, err := shellinit.ParseShell(shellName)
	if err != nil {
		return Result{Severity: SeveritySkip, Summary: i18n.MsgDoctorShellHookUnsupported, SummaryArgs: []any{shellName}}
	}

	path, ok, note := shellinit.RCPath(ctx, shell, deps.GOOS, deps.Getenv, deps.LookPath, deps.Run)
	if !ok {
		return Result{
			Severity:    SeveritySkip,
			Summary:     i18n.MsgDoctorShellHookUnresolved,
			SummaryArgs: []any{shellName},
			Detail:      note,
		}
	}

	existing, err := readFileOrEmpty(path)
	if err != nil {
		return Result{Severity: SeverityWarn, Summary: i18n.MsgDoctorShellHookUnresolved, SummaryArgs: []any{shellName}, Detail: err.Error()}
	}

	_, status, err := shellinit.ApplyBlock(existing, shell)
	if err != nil {
		return Result{Severity: SeverityWarn, Summary: i18n.MsgDoctorShellHookMissing, SummaryArgs: []any{shellName}, Fix: "comrade init " + shellName, Detail: err.Error()}
	}

	if status == shellinit.StatusAlreadyInstalled {
		return Result{Severity: SeverityOK, Summary: i18n.MsgDoctorShellHookOK, SummaryArgs: []any{shellName}}
	}
	return Result{
		Severity:    SeverityWarn,
		Summary:     i18n.MsgDoctorShellHookMissing,
		SummaryArgs: []any{shellName},
		Fix:         "comrade init " + shellName,
	}
}

// readFileOrEmpty reads path's content, treating a missing file as empty
// content rather than an error — mirrors internal/cli/init.go's own
// readFileOrEmpty exactly (kept as a small, separate copy here rather
// than an internal/cli import, which would create an import cycle:
// internal/cli already imports internal/doctor for the check registry).
func readFileOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is a well-known shell rc/profile location resolved by shellinit.RCPath, not attacker-controlled input
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}
