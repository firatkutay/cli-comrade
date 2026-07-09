package secrets

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// withMockKeychain switches go-keyring's package-level provider to its
// in-memory mock for the duration of t, so NewStore's
// detectKeychainAvailable probe reports "available" and every Store
// method in the test actually exercises keychainBackend — never the real
// OS keychain. t.Cleanup restores an unavailable-keychain state
// afterward so a later test in this same process that forgets to call
// either helper still gets deterministic (file-fallback) behavior,
// rather than silently inheriting this test's mock state.
func withMockKeychain(t *testing.T) {
	t.Helper()
	keyring.MockInit()
	t.Cleanup(func() { keyring.MockInitWithError(keyring.ErrUnsupportedPlatform) })
}

// withUnavailableKeychain forces go-keyring's package-level provider to
// report an error for every operation — simulating exactly the headless
// Linux (no D-Bus Secret Service) case this package's file fallback
// exists for — so NewStore's detectKeychainAvailable probe reports
// "unavailable" and the returned Store dispatches to fileBackend.
func withUnavailableKeychain(t *testing.T) {
	t.Helper()
	keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)
	t.Cleanup(func() { keyring.MockInitWithError(keyring.ErrUnsupportedPlatform) })
}

func TestKnownProvidersIsEveryConfigProviderExceptOllama(t *testing.T) {
	assert.Equal(t, []string{"anthropic", "openai_compat", "google"}, KnownProviders)

	all := config.ProviderNames()
	for _, p := range all {
		if p == "ollama" {
			assert.NotContains(t, KnownProviders, p)
			continue
		}
		assert.Contains(t, KnownProviders, p)
	}
}

func TestStoreGetReturnsSourceNoneWhenNothingStored(t *testing.T) {
	withMockKeychain(t)
	store := NewStore(unusedCredentialsPath(t), &strings.Builder{})

	key, source, err := store.Get(context.Background(), "anthropic")

	require.NoError(t, err)
	assert.Equal(t, "", key)
	assert.Equal(t, SourceNone, source)
}

func TestStoreKeychainSetGetRoundTrip(t *testing.T) {
	withMockKeychain(t)
	store := NewStore(unusedCredentialsPath(t), &strings.Builder{})
	ctx := context.Background()

	require.NoError(t, store.Set(ctx, "anthropic", "sk-ant-test-value"))

	key, source, err := store.Get(ctx, "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "sk-ant-test-value", key)
	assert.Equal(t, SourceKeychain, source)
}

func TestStoreKeychainDeleteRemovesEntry(t *testing.T) {
	withMockKeychain(t)
	store := NewStore(unusedCredentialsPath(t), &strings.Builder{})
	ctx := context.Background()
	require.NoError(t, store.Set(ctx, "google", "gemini-key"))

	require.NoError(t, store.Delete(ctx, "google"))

	key, source, err := store.Get(ctx, "google")
	require.NoError(t, err)
	assert.Equal(t, "", key)
	assert.Equal(t, SourceNone, source)
}

func TestStoreDeleteReturnsErrNoCredentialWhenNothingStored(t *testing.T) {
	withMockKeychain(t)
	store := NewStore(unusedCredentialsPath(t), &strings.Builder{})

	err := store.Delete(context.Background(), "anthropic")

	assert.ErrorIs(t, err, ErrNoCredential)
}

func TestStoreStatusReportsEveryKnownProviderInOrder(t *testing.T) {
	withMockKeychain(t)
	store := NewStore(unusedCredentialsPath(t), &strings.Builder{})
	ctx := context.Background()
	require.NoError(t, store.Set(ctx, "openai_compat", "sk-oai-test"))

	statuses, err := store.Status(ctx)

	require.NoError(t, err)
	require.Equal(t, []ProviderStatus{
		{Provider: "anthropic", Source: SourceNone},
		{Provider: "openai_compat", Source: SourceKeychain},
		{Provider: "google", Source: SourceNone},
	}, statuses)
}

func TestStoreFallsBackToFileWhenKeychainUnavailable(t *testing.T) {
	withUnavailableKeychain(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	store := NewStore(path, &strings.Builder{})
	ctx := context.Background()

	require.NoError(t, store.Set(ctx, "anthropic", "sk-ant-file-fallback"))

	key, source, err := store.Get(ctx, "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "sk-ant-file-fallback", key)
	assert.Equal(t, SourceFile, source)
}

func TestFileFallbackCreatesFileWith0600Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on windows; see repairPerms")
	}
	withUnavailableKeychain(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	store := NewStore(path, &strings.Builder{})

	require.NoError(t, store.Set(context.Background(), "google", "secret"))

	info, err := os.Stat(path)
	require.NoError(t, err, "credentials file must exist after Set")
	// t.TempDir() resolves under os.TempDir() (native /tmp on this
	// WSL2 sandbox, not the /mnt/c DrvFs mount cli-comrade's own
	// checkout lives under), so this file's permission bits are real
	// POSIX bits, not DrvFs's synthetic always-777 — asserting the
	// exact mode here is a load-bearing check, not a false pass.
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"expected the credentials file to be created 0600, got %v (path=%s, tempdir base=%s)",
		info.Mode().Perm(), path, os.TempDir())
}

func TestFileFallbackRepairsLoosenedPermissionsOnRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on windows; see repairPerms")
	}
	withUnavailableKeychain(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	store := NewStore(path, &strings.Builder{})
	ctx := context.Background()
	require.NoError(t, store.Set(ctx, "anthropic", "sk-ant-loose"))

	require.NoError(t, os.Chmod(path, 0o644))

	_, _, err := store.Get(ctx, "anthropic")
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "expected Get to repair permissions back to 0600")
}

func TestFileFallbackBase64EncodesStoredValueOnDisk(t *testing.T) {
	withUnavailableKeychain(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	store := NewStore(path, &strings.Builder{})
	require.NoError(t, store.Set(context.Background(), "anthropic", "sk-ant-not-plaintext"))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.NotContains(t, string(raw), "sk-ant-not-plaintext", "the raw key must never appear in the file")
	assert.Contains(t, string(raw), "anthropic = ")
	assert.Contains(t, string(raw), "NOT ENCRYPTED", "file must carry the not-encrypted warning in its own header")
}

func TestFileFallbackWarnsOnceOnFirstUse(t *testing.T) {
	withUnavailableKeychain(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	var stderr strings.Builder
	store := NewStore(path, &stderr)
	ctx := context.Background()

	require.NoError(t, store.Set(ctx, "anthropic", "sk-ant-1"))
	_, _, _ = store.Get(ctx, "anthropic")
	_, _ = store.Status(ctx)

	warnings := strings.Count(stderr.String(), "NOT encrypted")
	assert.Equal(t, 1, warnings, "expected exactly one not-encrypted warning across three file-backend calls, got stderr: %q", stderr.String())
}

func TestKeychainNeverWarns(t *testing.T) {
	withMockKeychain(t)
	var stderr strings.Builder
	store := NewStore(unusedCredentialsPath(t), &stderr)

	require.NoError(t, store.Set(context.Background(), "anthropic", "sk-ant-1"))

	assert.Empty(t, stderr.String(), "the file-fallback warning must never print when the keychain backend is active")
}

func TestFileFallbackDeleteReturnsErrNoCredentialWhenNothingStored(t *testing.T) {
	withUnavailableKeychain(t)
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "credentials"), &strings.Builder{})

	err := store.Delete(context.Background(), "anthropic")

	assert.ErrorIs(t, err, ErrNoCredential)
}

func TestFileFallbackGetErrorsOnCorruptStoredValue(t *testing.T) {
	withUnavailableKeychain(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	require.NoError(t, os.WriteFile(path, []byte("anthropic = not-valid-base64!!!\n"), 0o600))
	store := NewStore(path, &strings.Builder{})

	_, _, err := store.Get(context.Background(), "anthropic")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "anthropic")
}

// unusedCredentialsPath returns a path the keychain-backed tests never
// touch (the keychain backend never reads or writes it), just to give
// NewStore a syntactically valid argument.
func unusedCredentialsPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "unused-credentials")
}

func TestDetectKeychainAvailableTrueWhenProbeFindsNothing(t *testing.T) {
	withMockKeychain(t)
	assert.True(t, detectKeychainAvailable())
}

func TestDetectKeychainAvailableFalseWhenBackendErrors(t *testing.T) {
	withUnavailableKeychain(t)
	assert.False(t, detectKeychainAvailable())
}

func TestDetectKeychainAvailableTrueWhenProbeAccountHappensToExist(t *testing.T) {
	withMockKeychain(t)
	// Not expected in practice (keychainProbeAccount is a reserved name
	// this package itself never writes to), but detectKeychainAvailable
	// must treat a present probe entry the same as ErrNotFound: both
	// mean the backend itself is reachable.
	require.NoError(t, keyring.Set(serviceName, keychainProbeAccount, "unexpected"))
	assert.True(t, detectKeychainAvailable())
}
