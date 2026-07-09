package secrets

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/zalando/go-keyring"
)

// serviceName is the go-keyring "service" every credential is stored
// under, with the provider name as the "user"/account. It also names the
// probe entry detectKeychainAvailable checks (see keychainProbeAccount).
const serviceName = "cli-comrade"

// keychainProbeAccount is a reserved account name, never a real provider,
// that detectKeychainAvailable reads (never writes) to decide whether a
// working OS keychain backend is reachable — see its doc comment.
const keychainProbeAccount = "__cli_comrade_availability_probe__"

// backend is the single-provider get/set/delete surface store dispatches
// every Store method to, implemented once for the OS keychain
// (keychainBackend) and once for the file fallback (fileBackend). It is
// unexported: NewStore is the only way to obtain a Store, and it always
// wires in exactly one backend, chosen by detectKeychainAvailable.
type backend interface {
	kind() Source
	get(provider string) (key string, found bool, err error)
	set(provider, key string) error
	delete(provider string) error
}

// detectKeychainAvailable probes whether a real, reachable OS keychain
// backend is behind the go-keyring package by reading a reserved account
// name that is never written. On every OS go-keyring supports, a
// keyring.Get for an absent entry returns keyring.ErrNotFound — proof the
// backend itself is reachable, just empty for this account. Any other
// error (a Linux Secret Service/D-Bus connection failure on a headless
// machine being the expected case here, but this deliberately does not
// special-case that one error — go-keyring's own
// ErrUnsupportedPlatform, a locked keyring, or any other backend failure
// all mean the same thing for this decision) means no keychain is usable,
// so NewStore falls back to the file backend instead.
func detectKeychainAvailable() bool {
	_, err := keyring.Get(serviceName, keychainProbeAccount)
	return err == nil || errors.Is(err, keyring.ErrNotFound)
}

// NewStore constructs a Store, choosing its backend once: the OS
// keychain when detectKeychainAvailable reports one is reachable,
// otherwise the 0600 file fallback at credentialsPath. stderr receives
// the one-time "this file is obfuscated, not encrypted" warning the
// first time the file backend is actually used (Get/Set/Delete/Status
// all count) — never printed at all when the keychain backend is active.
func NewStore(credentialsPath string, stderr io.Writer) Store {
	var b backend
	if detectKeychainAvailable() {
		b = keychainBackend{service: serviceName}
	} else {
		b = &fileBackend{path: credentialsPath}
	}
	return &store{backend: b, stderr: stderr}
}

// store is Store's sole implementation: every method dispatches to
// exactly one backend, decided once by NewStore.
type store struct {
	backend  backend
	stderr   io.Writer
	warnOnce sync.Once
}

// warnIfFileFallback prints fileFallbackWarning to s.stderr, exactly
// once per store, the first time any method actually runs against the
// file backend. It is a no-op (and never fires the warning) when the
// keychain backend is active.
func (s *store) warnIfFileFallback() {
	if s.backend.kind() != SourceFile {
		return
	}
	s.warnOnce.Do(func() {
		_, _ = io.WriteString(s.stderr, fileFallbackWarning)
	})
}

func (s *store) Get(_ context.Context, provider string) (string, Source, error) {
	s.warnIfFileFallback()
	key, found, err := s.backend.get(provider)
	if err != nil {
		return "", SourceNone, err
	}
	if !found {
		return "", SourceNone, nil
	}
	return key, s.backend.kind(), nil
}

func (s *store) Set(_ context.Context, provider, key string) error {
	s.warnIfFileFallback()
	return s.backend.set(provider, key)
}

func (s *store) Delete(_ context.Context, provider string) error {
	s.warnIfFileFallback()
	return s.backend.delete(provider)
}

func (s *store) Status(_ context.Context) ([]ProviderStatus, error) {
	s.warnIfFileFallback()
	statuses := make([]ProviderStatus, 0, len(KnownProviders))
	for _, provider := range KnownProviders {
		_, found, err := s.backend.get(provider)
		if err != nil {
			return nil, err
		}
		source := SourceNone
		if found {
			source = s.backend.kind()
		}
		statuses = append(statuses, ProviderStatus{Provider: provider, Source: source})
	}
	return statuses, nil
}
