package context

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoryPathBash(t *testing.T) {
	path, ok := HistoryPath("bash", fakeEnv(map[string]string{"HOME": "/home/alice"}))
	require.True(t, ok)
	assert.Equal(t, filepath.Join("/home/alice", ".bash_history"), path)
}

func TestHistoryPathZsh(t *testing.T) {
	path, ok := HistoryPath("zsh", fakeEnv(map[string]string{"HOME": "/home/alice"}))
	require.True(t, ok)
	assert.Equal(t, filepath.Join("/home/alice", ".zsh_history"), path)
}

func TestHistoryPathFish(t *testing.T) {
	path, ok := HistoryPath("fish", fakeEnv(map[string]string{"HOME": "/home/alice"}))
	require.True(t, ok)
	assert.Equal(t, filepath.Join("/home/alice", ".local", "share", "fish", "fish_history"), path)
}

func TestHistoryPathPowerShell(t *testing.T) {
	path, ok := HistoryPath("powershell", fakeEnv(map[string]string{"APPDATA": `C:\Users\alice\AppData\Roaming`}))
	require.True(t, ok)
	assert.Equal(t, filepath.Join(`C:\Users\alice\AppData\Roaming`, "Microsoft", "Windows", "PowerShell", "PSReadLine", "ConsoleHost_history.txt"), path)
}

func TestHistoryPathUnknownShell(t *testing.T) {
	_, ok := HistoryPath("tcsh", fakeEnv(map[string]string{"HOME": "/home/alice"}))
	assert.False(t, ok)
}

func TestHistoryPathMissingEnvVar(t *testing.T) {
	_, ok := HistoryPath("bash", fakeEnv(map[string]string{}))
	assert.False(t, ok)
}

func writeHistoryFixture(t *testing.T, dir, name, content string) map[string]string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	return map[string]string{"HOME": dir}
}

func TestReadHistoryBashPlain(t *testing.T) {
	dir := t.TempDir()
	env := writeHistoryFixture(t, dir, ".bash_history", "ls -la\ncd /tmp\ngit status\n")

	got := ReadHistory("bash", fakeEnv(env), 5)
	assert.Equal(t, []string{"ls -la", "cd /tmp", "git status"}, got)
}

func TestReadHistoryZshStripsExtendedFormatPrefix(t *testing.T) {
	dir := t.TempDir()
	content := ": 1717430400:0;ls -la\n: 1717430410:2;git commit -m \"wip\"\nplain-no-prefix\n"
	env := writeHistoryFixture(t, dir, ".zsh_history", content)

	got := ReadHistory("zsh", fakeEnv(env), 10)
	assert.Equal(t, []string{"ls -la", `git commit -m "wip"`, "plain-no-prefix"}, got)
}

func TestReadHistoryFishParsesCmdLines(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, ".local", "share", "fish")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	content := "- cmd: ls -la\n  when: 1717430400\n- cmd: git status\n  when: 1717430410\n"
	require.NoError(t, os.WriteFile(filepath.Join(nested, "fish_history"), []byte(content), 0o644))

	got := ReadHistory("fish", fakeEnv(map[string]string{"HOME": dir}), 10)
	assert.Equal(t, []string{"ls -la", "git status"}, got)
}

func TestReadHistoryPowerShellPlain(t *testing.T) {
	dir := t.TempDir()
	env := writeHistoryFixture(t, dir, "ConsoleHost_history.txt", "Get-Process\nGet-ChildItem\n")
	// PowerShell path depends on APPDATA, not HOME; nest the fixture under
	// APPDATA/Microsoft/Windows/PowerShell/PSReadLine to match HistoryPath.
	nested := filepath.Join(dir, "Microsoft", "Windows", "PowerShell", "PSReadLine")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.Rename(filepath.Join(dir, "ConsoleHost_history.txt"), filepath.Join(nested, "ConsoleHost_history.txt")))
	env["APPDATA"] = dir

	got := ReadHistory("powershell", fakeEnv(env), 10)
	assert.Equal(t, []string{"Get-Process", "Get-ChildItem"}, got)
}

func TestReadHistoryDepthIsHonored(t *testing.T) {
	dir := t.TempDir()
	env := writeHistoryFixture(t, dir, ".bash_history", "one\ntwo\nthree\nfour\nfive\n")

	got := ReadHistory("bash", fakeEnv(env), 2)
	assert.Equal(t, []string{"four", "five"}, got, "must return only the most recent `depth` commands")
}

func TestReadHistoryZeroDepthReturnsNil(t *testing.T) {
	dir := t.TempDir()
	env := writeHistoryFixture(t, dir, ".bash_history", "one\ntwo\n")

	got := ReadHistory("bash", fakeEnv(env), 0)
	assert.Nil(t, got)
}

func TestReadHistoryMissingFileReturnsNil(t *testing.T) {
	got := ReadHistory("bash", fakeEnv(map[string]string{"HOME": t.TempDir()}), 5)
	assert.Nil(t, got)
}

func TestReadHistoryUnknownShellReturnsNil(t *testing.T) {
	got := ReadHistory("tcsh", fakeEnv(map[string]string{"HOME": t.TempDir()}), 5)
	assert.Nil(t, got)
}
