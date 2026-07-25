package cli

// This file is issue #16's chosen drift guard for internal/doctor's three
// small helpers that duplicate an internal/cli original rather than
// importing it — the import direction only runs one way (internal/cli
// already imports internal/doctor for the check registry), so
// internal/doctor cannot import internal/cli back without recreating
// that exact cycle, and a genuinely shared "third package" home would
// mean touching auth.go/init.go/secretsstore.go's own established,
// heavily-documented call sites for no behavioral gain. Instead, each
// doctor-side helper below was exported (see its own doc comment) SOLELY
// so this file — already living in package cli, which already imports
// internal/doctor — can call cli's own private original and doctor's
// exported mirror side by side across the same table of scenarios and
// assert they agree. If either copy's behavior ever changes without the
// other following, one of these three tests fails immediately: this is
// exactly the Derive-or-Guard "bidirectional drift guard" house rule
// applied to a mirror that genuinely cannot be collapsed into a single
// shared implementation.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/doctor"
	"github.com/firatkutay/cli-comrade/internal/i18n"
)

// TestFirstSetEnvVarMatchesDoctorMirror pins cli's own unexported
// firstSetEnvVar (auth.go) against doctor.FirstSetEnvVar (check_key.go)
// across every case FAZ 8's env-var precedence test already covers, using
// REAL process environment variables (t.Setenv) so both copies observe
// the exact same input.
func TestFirstSetEnvVarMatchesDoctorMirror(t *testing.T) {
	cases := []struct {
		name       string
		comradeEnv string
		vendorEnv  string
	}{
		{name: "neither set"},
		{name: "only comrade-prefixed set", comradeEnv: "comrade-env-key"},
		{name: "only vendor set", vendorEnv: "vendor-env-key"},
		{name: "both set — comrade-prefixed wins", comradeEnv: "comrade-env-key", vendorEnv: "vendor-env-key"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("COMRADE_ANTHROPIC_API_KEY", tc.comradeEnv)
			t.Setenv("ANTHROPIC_API_KEY", tc.vendorEnv)

			cliVar, cliOK := firstSetEnvVar("anthropic")
			doctorVar, doctorOK := doctor.FirstSetEnvVar(os.Getenv, "anthropic")

			assert.Equal(t, cliOK, doctorOK, "firstSetEnvVar and doctor.FirstSetEnvVar must agree on whether anything resolved")
			assert.Equal(t, cliVar, doctorVar, "firstSetEnvVar and doctor.FirstSetEnvVar must agree on WHICH env var resolved")
		})
	}
}

// TestReadFileOrEmptyMatchesDoctorMirror pins cli's own unexported
// readFileOrEmpty (init.go) against doctor.ReadFileOrEmpty
// (check_shellhook.go): both must return the same content for an
// existing file and both must treat a missing file as empty content with
// no error (the one behavior this pair exists to guarantee — cli's copy
// additionally wraps a REAL read error with an "init: read %s: %w"
// prefix, which is display-only decoration this test deliberately does
// not compare).
func TestReadFileOrEmptyMatchesDoctorMirror(t *testing.T) {
	t.Run("existing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "profile.rc")
		require.NoError(t, os.WriteFile(path, []byte("# comrade block\nexport PATH\n"), 0o600))

		cliContent, cliErr := readFileOrEmpty(path)
		doctorContent, doctorErr := doctor.ReadFileOrEmpty(path)

		require.NoError(t, cliErr)
		require.NoError(t, doctorErr)
		assert.Equal(t, cliContent, doctorContent)
	})

	t.Run("missing file is empty, not an error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "does-not-exist.rc")

		cliContent, cliErr := readFileOrEmpty(path)
		doctorContent, doctorErr := doctor.ReadFileOrEmpty(path)

		require.NoError(t, cliErr)
		require.NoError(t, doctorErr)
		assert.Empty(t, cliContent)
		assert.Empty(t, doctorContent)
	})

	// TestReadFileOrEmptyMatchesDoctorMirror's third case: an EXISTING but
	// unreadable file (permission denied) must surface as a real, non-nil
	// error from BOTH copies — never silently swallowed into empty
	// content like the "missing file" case above. This is the exact
	// divergence a mutation making doctor's ReadFileOrEmpty swallow ALL
	// read errors slipped past before this subtest existed:
	// ShellHookCheck (check_shellhook.go) branches on that error to emit
	// a Warn, so a mirror that silently returns ("", nil) instead would
	// turn a genuine permission problem into a false "shell hook not
	// installed" — skipped when running as root (Geteuid()==0 ignores
	// file permissions) or on Windows (chmod 0000 does not remove read
	// access the way it does on POSIX).
	t.Run("unreadable file is a real error, not swallowed", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod 0000 does not make a file unreadable on Windows")
		}
		if os.Geteuid() == 0 {
			t.Skip("root ignores file permission bits, so this file would still be readable")
		}
		path := filepath.Join(t.TempDir(), "unreadable.rc")
		require.NoError(t, os.WriteFile(path, []byte("secret"), 0o600))
		require.NoError(t, os.Chmod(path, 0o000))

		_, cliErr := readFileOrEmpty(path)
		_, doctorErr := doctor.ReadFileOrEmpty(path)

		assert.Error(t, cliErr, "cli's readFileOrEmpty must surface a permission error, not swallow it")
		assert.Error(t, doctorErr, "doctor.ReadFileOrEmpty must surface a permission error, not swallow it")
	})
}

// TestResolveKeyForLiveMatchesSecretsKeyResolver pins
// doctor.ResolveKeyForLive (check_reach.go) against cli's own
// secretsKeyResolver (secretsstore.go) across the same store/env
// precedence table TestSecretsKeyResolverPrecedence already exercises
// for secretsKeyResolver alone — both must resolve to the exact same key
// (or both must fail) for every scenario.
func TestResolveKeyForLiveMatchesSecretsKeyResolver(t *testing.T) {
	cases := []struct {
		name       string
		storedKey  string // "" means nothing stored
		comradeEnv string
		vendorEnv  string
		wantErr    bool
	}{
		{name: "store beats both env vars", storedKey: "keychain-key", comradeEnv: "comrade-env-key", vendorEnv: "vendor-env-key"},
		{name: "comrade env beats vendor env when store empty", comradeEnv: "comrade-env-key", vendorEnv: "vendor-env-key"},
		{name: "vendor env used when store and comrade env empty", vendorEnv: "vendor-env-key"},
		{name: "missing everywhere is an error", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withMockKeychain(t)
			withIsolatedConfigDir(t)
			t.Setenv("COMRADE_ANTHROPIC_API_KEY", tc.comradeEnv)
			t.Setenv("ANTHROPIC_API_KEY", tc.vendorEnv)

			store, err := newSecretsStore(os.Stderr, i18n.NewTranslator(i18n.LangEN))
			require.NoError(t, err)
			if tc.storedKey != "" {
				require.NoError(t, store.Set(context.Background(), "anthropic", tc.storedKey))
			}

			cliKey, cliErr := secretsKeyResolver(store)("anthropic")
			doctorKey, doctorErr := doctor.ResolveKeyForLive(context.Background(), doctor.Deps{Store: store}, "anthropic")

			if tc.wantErr {
				assert.Error(t, cliErr)
				assert.Error(t, doctorErr)
				return
			}
			require.NoError(t, cliErr)
			require.NoError(t, doctorErr)
			assert.Equal(t, cliKey, doctorKey, "secretsKeyResolver and doctor.ResolveKeyForLive must resolve to the same key")
		})
	}
}
