package cli

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/secrets"
)

// TestSecretsKeyResolverPrecedence pins docs/history/UYGULAMA_PLANI.md FAZ 8 item 3's
// resolution order end to end: secrets.Store (keychain/file) beats
// COMRADE_<PROVIDER>_API_KEY, which beats the provider's known vendor env
// var, which beats a KeyMissingError when nothing resolves at all.
func TestSecretsKeyResolverPrecedence(t *testing.T) {
	withIsolatedConfigDir(t)

	cases := []struct {
		name    string
		setup   func(t *testing.T, store secrets.Store)
		wantKey string
		wantErr bool
	}{
		{
			name: "store beats both env vars",
			setup: func(t *testing.T, store secrets.Store) {
				t.Setenv("COMRADE_ANTHROPIC_API_KEY", "comrade-env-key")
				t.Setenv("ANTHROPIC_API_KEY", "vendor-env-key")
				require.NoError(t, store.Set(context.Background(), "anthropic", "keychain-key"))
			},
			wantKey: "keychain-key",
		},
		{
			name: "comrade-prefixed env beats vendor env when store is empty",
			setup: func(t *testing.T, _ secrets.Store) {
				t.Setenv("COMRADE_ANTHROPIC_API_KEY", "comrade-env-key")
				t.Setenv("ANTHROPIC_API_KEY", "vendor-env-key")
			},
			wantKey: "comrade-env-key",
		},
		{
			name: "vendor env used when store and comrade env are both empty",
			setup: func(t *testing.T, _ secrets.Store) {
				t.Setenv("COMRADE_ANTHROPIC_API_KEY", "")
				t.Setenv("ANTHROPIC_API_KEY", "vendor-env-key")
			},
			wantKey: "vendor-env-key",
		},
		{
			name: "missing everywhere is a KeyMissingError",
			setup: func(t *testing.T, _ secrets.Store) {
				t.Setenv("COMRADE_ANTHROPIC_API_KEY", "")
				t.Setenv("ANTHROPIC_API_KEY", "")
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withMockKeychain(t)
			store, err := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
			require.NoError(t, err)
			tc.setup(t, store)

			key, err := secretsKeyResolver(store)("anthropic")

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantKey, key)
		})
	}
}
