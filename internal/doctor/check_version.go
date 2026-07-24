package doctor

import (
	"context"

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
		return Result{
			Severity:    SeverityWarn,
			Summary:     i18n.MsgDoctorVersionBehind,
			SummaryArgs: []any{result.LatestVersion, result.CurrentVersion},
			Fix:         "comrade upgrade",
		}
	}
	return Result{
		Severity:    SeverityOK,
		Summary:     i18n.MsgDoctorVersionUpToDate,
		SummaryArgs: []any{result.CurrentVersion},
	}
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
