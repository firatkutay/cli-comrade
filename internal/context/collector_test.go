package context

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorCollectWiresEverySubCollection(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "cli-comrade")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))
	lastCmdPath := filepath.Join(stateDir, "last_command.json")
	fixture := LastCommand{
		Command:   "pyton --version",
		ExitCode:  127,
		Timestamp: time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC),
		Shell:     "bash",
	}
	raw, err := json.Marshal(fixture)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(lastCmdPath, raw, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".bash_history"), []byte("one\ntwo\nthree\n"), 0o644))

	env := map[string]string{
		"HOME":           dir,
		"XDG_STATE_HOME": dir,
		"SHELL":          "/bin/bash",
	}

	c := &Collector{
		GOOS:   "linux",
		Getenv: fakeEnv(env),
		LookPath: func(name string) (string, error) {
			if name == "apt" {
				return "/usr/bin/apt", nil
			}
			return "", errors.New("not found")
		},
		RunCommand: func(_ stdctx.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("GNU bash, version 5.2.15(1)-release\n"), nil
		},
		Geteuid:     func() int { return 1000 },
		Getwd:       func() (string, error) { return "/work/dir", nil },
		UserHomeDir: func() (string, error) { return dir, nil },
		Environ:     func() []string { return []string{"HOME=" + dir, "SECRET_TOKEN=hunter2forever"} },
	}

	got := c.Collect(stdctx.Background(), Options{SendHistory: true, HistoryDepth: 2, SendEnvNames: true})

	assert.Equal(t, "linux", got.OS)
	assert.Equal(t, "bash", got.Shell)
	assert.Equal(t, "GNU bash, version 5.2.15(1)-release", got.ShellVersion)
	assert.Equal(t, "/work/dir", got.WorkingDir)
	assert.Equal(t, dir, got.HomeDir)
	assert.False(t, got.IsAdmin)
	assert.True(t, got.AdminKnown)
	assert.Equal(t, []string{"apt"}, got.PackageManagers)

	require.NotNil(t, got.LastCommand)
	assert.Equal(t, "pyton --version", got.LastCommand.Command)
	assert.Equal(t, 127, got.LastCommand.ExitCode)

	assert.Equal(t, []string{"two", "three"}, got.History, "history depth=2 must return only the last 2 entries")

	assert.Equal(t, []string{"HOME", "SECRET_TOKEN"}, got.EnvNames)
	for _, name := range got.EnvNames {
		assert.NotContains(t, name, "hunter2forever")
	}
}

func TestCollectorCollectOmitsOptInDataByDefault(t *testing.T) {
	c := &Collector{
		GOOS:        "linux",
		Getenv:      fakeEnv(map[string]string{}),
		LookPath:    func(string) (string, error) { return "", errors.New("not found") },
		RunCommand:  func(_ stdctx.Context, _ string, _ ...string) ([]byte, error) { return nil, errors.New("no shell") },
		Geteuid:     func() int { return 0 },
		Getwd:       func() (string, error) { return "", errors.New("no cwd") },
		UserHomeDir: func() (string, error) { return "", errors.New("no home") },
		Environ:     func() []string { return []string{"SECRET=hunter2forever"} },
	}

	got := c.Collect(stdctx.Background(), Options{})

	assert.Nil(t, got.History, "history must stay nil when SendHistory=false")
	assert.Nil(t, got.EnvNames, "env names must stay nil when SendEnvNames=false")
	assert.Nil(t, got.LastCommand, "no last_command.json present in this fake environment")
	assert.Equal(t, "", got.WorkingDir)
	assert.Equal(t, "", got.HomeDir)
}

func TestEnvNamesNeverIncludesValues(t *testing.T) {
	got := EnvNames([]string{"PATH=/usr/bin", "API_KEY=sk-should-never-appear", "HOME=/home/alice"})

	assert.Equal(t, []string{"API_KEY", "HOME", "PATH"}, got)
	for _, name := range got {
		assert.NotContains(t, name, "sk-should-never-appear")
		assert.NotContains(t, name, "/usr/bin")
		assert.NotContains(t, name, "/home/alice")
	}
}

func TestNewCollectorWiresRealOS(t *testing.T) {
	c := NewCollector()
	assert.NotEmpty(t, c.GOOS)
	assert.NotNil(t, c.Getenv)
	assert.NotNil(t, c.LookPath)
	assert.NotNil(t, c.RunCommand)
	assert.NotNil(t, c.Geteuid)
	assert.NotNil(t, c.Getwd)
	assert.NotNil(t, c.UserHomeDir)
	assert.NotNil(t, c.Environ)
}
