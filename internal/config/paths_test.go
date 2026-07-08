package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEnv returns a getenv func backed by an in-memory map, so ResolvePath
// can be tested for every OS branch without depending on (or mutating)
// the real process environment or the OS the test binary runs on.
func fakeEnv(vars map[string]string) func(string) string {
	return func(key string) string {
		return vars[key]
	}
}

func TestResolvePathWindowsUsesAppData(t *testing.T) {
	got, err := ResolvePath("windows", fakeEnv(map[string]string{
		"APPDATA": `C:\Users\alice\AppData\Roaming`,
	}))

	require.NoError(t, err)
	assert.Equal(t, `C:\Users\alice\AppData\Roaming\cli-comrade\config.toml`, got)
}

func TestResolvePathWindowsErrorsWhenAppDataUnset(t *testing.T) {
	_, err := ResolvePath("windows", fakeEnv(map[string]string{}))

	assert.ErrorContains(t, err, "APPDATA")
}

func TestResolvePathUnixUsesXDGConfigHomeWhenSet(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			got, err := ResolvePath(goos, fakeEnv(map[string]string{
				"XDG_CONFIG_HOME": "/home/alice/.config-custom",
				"HOME":            "/home/alice",
			}))

			require.NoError(t, err)
			assert.Equal(t, "/home/alice/.config-custom/cli-comrade/config.toml", got)
		})
	}
}

func TestResolvePathUnixFallsBackToDotConfigWhenXDGUnset(t *testing.T) {
	got, err := ResolvePath("linux", fakeEnv(map[string]string{
		"HOME": "/home/alice",
	}))

	require.NoError(t, err)
	assert.Equal(t, "/home/alice/.config/cli-comrade/config.toml", got)
}

func TestResolvePathUnixErrorsWhenNeitherXDGNorHomeSet(t *testing.T) {
	_, err := ResolvePath("linux", fakeEnv(map[string]string{}))

	assert.ErrorContains(t, err, "HOME")
}

func TestResolvePathUnixIgnoresAppDataOnNonWindows(t *testing.T) {
	// APPDATA is a Windows-only variable; it must never leak into the
	// Unix branch even if it happens to be set (e.g. under Wine).
	got, err := ResolvePath("linux", fakeEnv(map[string]string{
		"HOME":    "/home/alice",
		"APPDATA": `C:\should\not\be\used`,
	}))

	require.NoError(t, err)
	assert.Equal(t, "/home/alice/.config/cli-comrade/config.toml", got)
}
