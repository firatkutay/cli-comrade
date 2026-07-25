package doctor

import (
	"context"

	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/secrets"
)

// ConfigCheck reports whether internal/cli/doctor.go's own config load
// (already performed before Deps was built — this check never loads
// config itself) succeeded, and — when it did — whether a real, reachable
// OS keychain backend is available (deps.KeychainAvailable, normally
// secrets.KeychainAvailable — a read-only probe independent of any
// particular Store's own already-decided backend, reached through Deps
// rather than called directly, exactly like every other OS/network
// touchpoint this package's checks use — see Deps.KeychainAvailable's
// own doc comment): Fail on a load error, Warn when credentials fall
// back to the 0600 file, OK otherwise.
func ConfigCheck(_ context.Context, deps Deps) Result {
	if deps.ConfigErr != nil {
		return Result{Severity: SeverityFail, Summary: i18n.MsgDoctorConfigLoadError, Detail: deps.ConfigErr.Error()}
	}
	keychainAvailable := deps.KeychainAvailable
	if keychainAvailable == nil {
		keychainAvailable = secrets.KeychainAvailable
	}
	if !keychainAvailable() {
		return Result{Severity: SeverityWarn, Summary: i18n.MsgDoctorConfigFileFallback}
	}
	return Result{Severity: SeverityOK, Summary: i18n.MsgDoctorConfigOK}
}
