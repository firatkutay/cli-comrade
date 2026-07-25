package doctor

import (
	"context"
	"path/filepath"

	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/update"
)

// VersionCheck reports whether this build is a dev build (Skip), behind
// the latest published GitHub release (Warn, fix `comrade upgrade`), the
// fetch itself failed (Warn — this says nothing about whether the
// installed version is actually fine), or already up to date (OK).
//
// On a successful fetch (up to date OR behind — any outcome that reached
// a real answer, not the fetch-error case), it also writes
// update.WriteState — best-effort, error ignored — so `comrade doctor`
// feeds the SAME passive version-update notice (internal/cli/
// updatenotice.go) every other command's background check does, instead
// of the two mechanisms silently disagreeing about when a check last ran.
func VersionCheck(ctx context.Context, deps Deps) Result {
	if update.IsDevBuild(deps.Version) {
		return Result{Severity: SeveritySkip, Summary: i18n.MsgDoctorVersionDevSkip}
	}

	u := &update.Updater{Fetcher: deps.Fetcher}
	result, err := u.Check(ctx, deps.Version)
	if err != nil {
		return Result{Severity: SeverityWarn, Summary: i18n.MsgDoctorVersionFetchError, Detail: err.Error()}
	}

	writeVersionCheckState(deps, result.LatestVersion)

	if result.UpdateAvailable {
		fix := "comrade upgrade"
		if npmManaged(deps) {
			fix = npmManagedFixInstruction()
		}
		return Result{
			Severity:    SeverityWarn,
			Summary:     i18n.MsgDoctorVersionBehind,
			SummaryArgs: []any{result.LatestVersion, result.CurrentVersion},
			Fix:         fix,
		}
	}
	return Result{
		Severity:    SeverityOK,
		Summary:     i18n.MsgDoctorVersionUpToDate,
		SummaryArgs: []any{result.CurrentVersion},
	}
}

// npmManaged reports whether this process is running under a Node
// package manager-managed install (see update.IsNPMManaged) — VersionCheck
// uses this so its Fix instruction agrees with what `comrade upgrade`
// itself will actually do (internal/cli/upgrade.go's own refusal),
// instead of the two mechanisms silently disagreeing (PR #37 review,
// HIGH-1: doctor's own package doc comment already states this package
// exists precisely so its checks and the commands they describe never
// disagree).
//
// deps.Getenv or deps.Executable being nil (a hand-built Deps in a test
// that doesn't wire them) is treated as "not npm-managed" — the same
// fail-open default IsNPMManaged itself applies to a resolution error,
// and consistent with PathCheck's own direct (uninjected)
// filepath.EvalSymlinks call elsewhere in this package.
func npmManaged(deps Deps) bool {
	if deps.Getenv == nil || deps.Executable == nil {
		return false
	}
	return update.IsNPMManaged(deps.Getenv, deps.Executable, filepath.EvalSymlinks)
}

// npmManagedFixInstruction is VersionCheck's Fix remediation when
// npmManaged is true (PR #37 review, HIGH-1 + MEDIUM-4): deliberately
// generic ("your Node package manager") rather than npm-specific, since
// a pnpm/yarn/bun-managed install resolves under the exact same
// node_modules tree and sees the exact same COMRADE_MANAGED_BY=npm
// signal npm/main/bin/comrade.js's dispatcher sets today — there is no
// reliable way, at a globally-installed binary's own runtime, to tell
// WHICH Node package manager actually installed it (npm_config_user_agent
// is only set for scripts a package manager itself runs, not for a bare
// global-binary invocation). See doctor.Result.Fix's own doc comment for
// why this is plain, untranslated text rather than a MessageID — a
// contained i18n conversion of Fix as a whole is a bigger, separate
// change (see this function's own PR discussion).
func npmManagedFixInstruction() string {
	return "update it with your Node package manager instead (e.g. `npm update -g cli-comrade` for npm) — comrade upgrade refuses to self-update under a Node-managed install"
}

// writeVersionCheckState persists a successful fetch's outcome to
// update_check.json (update.WriteState), throttling the NEXT background
// check the same way any other successful check would — best-effort, any
// failure (resolving the path, or the write itself) is silently ignored,
// exactly like internal/cli/updatenotice.go's own maybeNotifyUpdate does
// for the identical file.
func writeVersionCheckState(deps Deps, latestVersion string) {
	if deps.Getenv == nil || deps.Now == nil {
		return
	}
	path, err := update.StatePathFor(deps.GOOS, deps.Getenv)
	if err != nil {
		return
	}
	_ = update.WriteState(path, update.CheckState{
		LastCheckedAt:      deps.Now(),
		LatestKnownVersion: latestVersion,
	})
}
