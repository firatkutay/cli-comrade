package doctor

import (
	"context"

	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/secrets"
)

// ConfigCheck reports whether internal/cli/doctor.go's own config load
// (already performed before Deps was built — this check never loads
// config itself) succeeded, and — when it did — whether a real, reachable
// OS keychain backend is available (secrets.KeychainAvailable, a
// read-only probe independent of any particular Store's own
// already-decided backend): Fail on a load error, Warn when credentials
// fall back to the 0600 file, OK otherwise.
func ConfigCheck(_ context.Context, deps Deps) Result {
	if deps.ConfigErr != nil {
		return Result{Severity: SeverityFail, Summary: i18n.MsgDoctorConfigLoadError, Detail: deps.ConfigErr.Error()}
	}
	if !secrets.KeychainAvailable() {
		return Result{Severity: SeverityWarn, Summary: i18n.MsgDoctorConfigFileFallback}
	}
	return Result{Severity: SeverityOK, Summary: i18n.MsgDoctorConfigOK}
}
