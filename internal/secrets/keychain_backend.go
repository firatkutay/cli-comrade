package secrets

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// keychainBackend stores each provider's key as one go-keyring entry
// under service, with the provider name as the account/"user". It never
// touches keychainProbeAccount itself — that name is reserved for
// detectKeychainAvailable's read-only probe.
type keychainBackend struct {
	service string
}

func (b keychainBackend) kind() Source { return SourceKeychain }

func (b keychainBackend) get(provider string) (string, bool, error) {
	v, err := keyring.Get(b.service, provider)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("secrets: keychain get: %w", err)
	}
	return v, true, nil
}

func (b keychainBackend) set(provider, key string) error {
	if err := keyring.Set(b.service, provider, key); err != nil {
		return fmt.Errorf("secrets: keychain set: %w", err)
	}
	return nil
}

func (b keychainBackend) delete(provider string) error {
	err := keyring.Delete(b.service, provider)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return ErrNoCredential
		}
		return fmt.Errorf("secrets: keychain delete: %w", err)
	}
	return nil
}
