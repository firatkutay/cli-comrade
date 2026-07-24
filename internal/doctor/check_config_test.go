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
