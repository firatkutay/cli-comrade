package doctor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/go-keyring"

	"github.com/firatkutay/cli-comrade/internal/i18n"
)

func TestConfigCheckFailsOnLoadError(t *testing.T) {
	deps := baseDeps()
	deps.ConfigErr = errors.New("parse config file: unexpected EOF")

	result := ConfigCheck(context.Background(), deps)

	assert.Equal(t, SeverityFail, result.Severity)
	assert.Equal(t, i18n.MsgDoctorConfigLoadError, result.Summary)
	assert.Contains(t, result.Detail, "unexpected EOF")
}

func TestConfigCheckWarnsOnFileFallback(t *testing.T) {
	keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)
	t.Cleanup(func() { keyring.MockInitWithError(keyring.ErrUnsupportedPlatform) })

	deps := baseDeps()

	result := ConfigCheck(context.Background(), deps)

	assert.Equal(t, SeverityWarn, result.Severity)
	assert.Equal(t, i18n.MsgDoctorConfigFileFallback, result.Summary)
}

func TestConfigCheckOKWhenKeychainAvailable(t *testing.T) {
	keyring.MockInit()
	t.Cleanup(func() { keyring.MockInitWithError(keyring.ErrUnsupportedPlatform) })

	deps := baseDeps()

	result := ConfigCheck(context.Background(), deps)

	assert.Equal(t, SeverityOK, result.Severity)
	assert.Equal(t, i18n.MsgDoctorConfigOK, result.Summary)
}

// TestConfigCheckUsesDepsKeychainAvailableSeam proves ConfigCheck reads
// deps.KeychainAvailable — never secrets.KeychainAvailable directly — by
// faking BOTH outcomes through the seam alone, with no keyring.MockInit
// global state involved at all: routing through Deps (rather than a
// direct package-level call) is exactly what makes this possible.
func TestConfigCheckUsesDepsKeychainAvailableSeam(t *testing.T) {
	deps := baseDeps()
	deps.KeychainAvailable = func() bool { return false }
	result := ConfigCheck(context.Background(), deps)
	assert.Equal(t, SeverityWarn, result.Severity)
	assert.Equal(t, i18n.MsgDoctorConfigFileFallback, result.Summary)

	deps.KeychainAvailable = func() bool { return true }
	result = ConfigCheck(context.Background(), deps)
	assert.Equal(t, SeverityOK, result.Severity)
	assert.Equal(t, i18n.MsgDoctorConfigOK, result.Summary)
}

// TestConfigCheckFallsBackToRealKeychainAvailableWhenNilSeam proves
// ConfigCheck's defensive nil-seam fallback (Deps.KeychainAvailable's own
// doc comment) — a Deps built without setting KeychainAvailable at all
// still resolves to the real secrets.KeychainAvailable rather than
// panicking on a nil func call.
func TestConfigCheckFallsBackToRealKeychainAvailableWhenNilSeam(t *testing.T) {
	keyring.MockInit()
	t.Cleanup(func() { keyring.MockInitWithError(keyring.ErrUnsupportedPlatform) })

	deps := baseDeps()
	deps.KeychainAvailable = nil

	result := ConfigCheck(context.Background(), deps)

	assert.Equal(t, SeverityOK, result.Severity)
	assert.Equal(t, i18n.MsgDoctorConfigOK, result.Summary)
}
