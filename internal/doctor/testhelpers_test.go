package doctor

import (
	"context"
	"time"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/secrets"
	"github.com/firatkutay/cli-comrade/internal/update"
)

// fakeStore is a tiny in-memory secrets.Store double — every check test in
// this package uses this instead of a real OS keychain/file store.
type fakeStore struct {
	byProvider map[string]string
}

func newFakeStore(entries map[string]string) *fakeStore {
	return &fakeStore{byProvider: entries}
}

func (s *fakeStore) Get(_ context.Context, provider string) (string, secrets.Source, error) {
	key, ok := s.byProvider[provider]
	if !ok {
		return "", secrets.SourceNone, nil
	}
	return key, secrets.SourceKeychain, nil
}

func (s *fakeStore) Set(_ context.Context, provider, key string) error {
	if s.byProvider == nil {
		s.byProvider = map[string]string{}
	}
	s.byProvider[provider] = key
	return nil
}

func (s *fakeStore) Delete(_ context.Context, provider string) error {
	delete(s.byProvider, provider)
	return nil
}

func (s *fakeStore) Status(context.Context) ([]secrets.ProviderStatus, error) {
	out := make([]secrets.ProviderStatus, 0, len(secrets.KnownProviders))
	for _, p := range secrets.KnownProviders {
		source := secrets.SourceNone
		if _, ok := s.byProvider[p]; ok {
			source = secrets.SourceKeychain
		}
		out = append(out, secrets.ProviderStatus{Provider: p, Source: source})
	}
	return out, nil
}

// fakeFetcher is a tiny update.ReleaseFetcher double.
type fakeFetcher struct {
	release update.Release
	err     error
}

func (f fakeFetcher) LatestRelease(context.Context) (update.Release, error) {
	return f.release, f.err
}

// baseDeps returns a minimal, fully-wired Deps a single check's test can
// override just the field(s) it cares about — every seam defaults to a
// harmless, deterministic value so a check under test never reaches a
// real filesystem/network/keychain by accident.
func baseDeps() Deps {
	return Deps{
		Cfg:        config.Default(),
		Version:    "v1.0.0",
		Fetcher:    fakeFetcher{release: update.Release{TagName: "v1.0.0"}},
		Getenv:     func(string) string { return "" },
		LookPath:   func(string) (string, error) { return "", assertNotFoundErr },
		Executable: func() (string, error) { return "", assertNotFoundErr },
		GOOS:       "linux",
		Now:        func() time.Time { return time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC) },
		LivePing: func(context.Context, config.Config, string, string) (llm.CompletionResponse, time.Duration, error) {
			return llm.CompletionResponse{}, 0, nil
		},
	}
}

var assertNotFoundErr = errNotFound{}

type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }
