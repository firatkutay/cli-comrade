package cli

import (
	"context"
	"io"
	"path/filepath"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/secrets"
)

// credentialsFileName is the file secrets.NewStore falls back to, inside
// the same platform config directory config.Loader uses for config.toml
// (config.DefaultDir) — never inside config.toml itself, per CLAUDE.md
// security rule #2.
const credentialsFileName = "credentials"

// newSecretsStore resolves the platform credentials path and constructs
// a secrets.Store, wiring stderr as the destination for the file
// fallback's one-time "not encrypted" warning — rendered in tr's
// language (QA MINOR-4) via secrets.NewStoreWithWarning, rather than
// internal/secrets' own hardcoded English default (see that function's
// doc comment for why internal/secrets itself stays i18n-agnostic).
// Every FAZ 8 command that touches a stored credential — `comrade auth
// login/logout/status` and `comrade config models`'s openai_compat key
// lookup — goes through this one constructor, so they all resolve the
// exact same store.
func newSecretsStore(stderr io.Writer, tr i18n.Translator) (secrets.Store, error) {
	dir, err := config.DefaultDir()
	if err != nil {
		return nil, err
	}
	warning := tr.T(i18n.MsgSecretsFileFallbackWarning)
	return secrets.NewStoreWithWarning(filepath.Join(dir, credentialsFileName), stderr, warning), nil
}

// secretsKeyResolver builds an llm.KeyResolver that checks store first —
// the keychain or file-fallback credential FAZ 8's `comrade auth login`
// wrote — and, only when store has nothing for provider (or a store
// error prevents saying either way), falls through to
// llm.ResolveEnvKey's env-only lookup. This is the seam
// docs/history/UYGULAMA_PLANI.md FAZ 8 item 3 calls for: keychain/file > COMRADE_*
// env > known vendor env vars > KeyMissingError, all without llm ever
// importing internal/secrets (see llm.KeyResolver's doc comment).
func secretsKeyResolver(store secrets.Store) llm.KeyResolver {
	return func(provider string) (string, error) {
		if key, source, err := store.Get(context.Background(), provider); err == nil && source != secrets.SourceNone {
			return key, nil
		}
		return llm.ResolveEnvKey(provider)
	}
}
