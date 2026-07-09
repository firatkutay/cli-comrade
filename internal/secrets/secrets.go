package secrets

import (
	"context"
	"errors"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// Source identifies where a provider's API key was found. Get and Status
// only ever return SourceKeychain, SourceFile, or SourceNone — this
// Store abstraction has no knowledge of environment variables. SourceEnv
// exists so callers combining a Store's answer with their own env-var
// check (as `comrade auth status` does — see internal/cli/auth.go) can
// label every source with values from a single shared type instead of
// each defining their own.
type Source string

const (
	// SourceKeychain means the key came from the OS keychain (macOS
	// Keychain, Windows Credential Manager, or Linux Secret Service).
	SourceKeychain Source = "keychain"
	// SourceFile means the key came from the 0600 file fallback used
	// when no OS keychain is available.
	SourceFile Source = "file"
	// SourceEnv means the key came from an environment variable. No
	// Store implementation in this package ever returns this itself —
	// it is provided for callers that merge a Store's answer with their
	// own environment-variable check into one combined report.
	SourceEnv Source = "env"
	// SourceNone means no key was found by whatever check produced this
	// value.
	SourceNone Source = "none"
)

// KnownProviders lists every provider this store can hold a credential
// for: every valid llm.provider value (config.ProviderNames) except
// "ollama", which needs no credential — a local Ollama server has no
// notion of an API key to reject. Derived from config.ProviderNames
// rather than hand-copied, per this project's derive-or-guard rule:
// TestKnownProvidersMatchesConfigProviderNamesMinusOllama in
// secrets_test.go pins the exact result, so the two cannot silently
// drift apart.
var KnownProviders = computeKnownProviders()

func computeKnownProviders() []string {
	all := config.ProviderNames()
	out := make([]string, 0, len(all))
	for _, p := range all {
		if p != "ollama" {
			out = append(out, p)
		}
	}
	return out
}

// ErrNoCredential is returned by Store.Delete when no credential was
// stored for the given provider — distinguishing "there was nothing to
// remove" from a genuine backend failure, so `comrade auth logout` can
// report the former without treating it as an error.
var ErrNoCredential = errors.New("secrets: no credential stored for provider")

// ProviderStatus is one row of Store.Status's report: whether provider
// has a stored credential in this Store, and which backend it came from.
type ProviderStatus struct {
	Provider string
	Source   Source
}

// Store is the credential store abstraction internal/cli wires into an
// llm.KeyResolver (see llm.WithKeyResolver). A single Store picks exactly
// one active backend at construction time (see NewStore) — an OS
// keychain when available, otherwise the file fallback — and every
// method call goes through that one backend; Store never silently mixes
// the two.
type Store interface {
	// Get returns provider's stored key and which backend it came from.
	// A provider with no stored credential returns ("", SourceNone, nil)
	// — that is not an error.
	Get(ctx context.Context, provider string) (key string, source Source, err error)
	// Set stores key for provider, overwriting any existing value.
	Set(ctx context.Context, provider, key string) error
	// Delete removes provider's stored key. It returns ErrNoCredential
	// (check with errors.Is) when no key was stored for provider.
	Delete(ctx context.Context, provider string) error
	// Status reports, for every entry in KnownProviders, whether this
	// Store holds a credential for it and which backend it came from.
	Status(ctx context.Context) ([]ProviderStatus, error)
}
