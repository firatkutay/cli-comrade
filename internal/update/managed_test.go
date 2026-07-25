package update

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/firatkutay/cli-comrade/internal/envkeys"
)

// identityEvalSymlinks stands in for filepath.EvalSymlinks for tests
// that don't care about symlink resolution itself — it just returns
// whatever path it was given, unchanged.
func identityEvalSymlinks(path string) (string, error) { return path, nil }

// fixedExecutable builds an ExecutableFunc (matching os.Executable's
// signature) that always returns path, nil.
func fixedExecutable(path string) ExecutableFunc {
	return func() (string, error) { return path, nil }
}

// noEnv is a GetenvFunc (matching os.Getenv's signature) that reports
// every variable as unset — used by the path-signal table below so each
// case exercises ONLY the path signal, never the env signal.
func noEnv(string) string { return "" }

// TestIsNPMManagedPathSignal is the table-driven regression guard for
// the fallback path-based signal (COMRADE_MANAGED_BY unset throughout):
// a real npm install layout on each of the three supported OS path
// forms must match, a directory that merely CONTAINS "node_modules" as
// a substring of its own name must NOT match, and "node_modules"
// appearing as the first, a middle, or the last path segment must all
// match — proving the check compares whole segments, not a substring.
func TestIsNPMManagedPathSignal(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "linux npm install layout",
			path: "/home/user/project/node_modules/@firatkutay/comrade-linux-x64/bin/comrade",
			want: true,
		},
		{
			name: "darwin npm install layout",
			path: "/Users/user/project/node_modules/.bin/comrade",
			want: true,
		},
		{
			name: "windows npm install layout (backslash separators)",
			path: `C:\Users\user\project\node_modules\@firatkutay\comrade-win32-x64\bin\comrade.exe`,
			want: true,
		},
		{
			name: "backup directory must not match as a substring",
			path: "/home/user/my_node_modules_backup/comrade",
			want: false,
		},
		{
			name: "node_modules as the first path segment",
			path: "node_modules/bin/comrade",
			want: true,
		},
		{
			name: "node_modules as a middle path segment",
			path: "/opt/tools/node_modules/bin/comrade",
			want: true,
		},
		{
			name: "node_modules as the last path segment",
			path: "/opt/tools/node_modules",
			want: true,
		},
		{
			name: "no node_modules anywhere in the path",
			path: "/usr/local/bin/comrade",
			want: false,
		},
		{
			name: "empty path",
			path: "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNPMManaged(noEnv, fixedExecutable(tt.path), identityEvalSymlinks)
			assert.Equal(t, tt.want, got, "path %q", tt.path)
		})
	}
}

// TestIsNPMManagedExecutableErrorFailsOpen proves an os.Executable
// failure is treated as "not npm-managed" (fail open) rather than
// propagated or treated as a false positive — the cost of a false
// positive here (refusing a legitimate upgrade) is worse than the cost
// of a false negative.
func TestIsNPMManagedExecutableErrorFailsOpen(t *testing.T) {
	executable := func() (string, error) { return "", errors.New("boom: cannot resolve executable") }

	got := IsNPMManaged(noEnv, executable, identityEvalSymlinks)

	assert.False(t, got, "an os.Executable error must fail open (not npm-managed), never block a legitimate upgrade")
}

// TestIsNPMManagedEvalSymlinksErrorFailsOpen proves a
// filepath.EvalSymlinks failure (e.g. a dangling symlink) is likewise
// treated as "not npm-managed" (fail open), even when the unresolved
// path itself would otherwise have matched the node_modules segment
// check.
func TestIsNPMManagedEvalSymlinksErrorFailsOpen(t *testing.T) {
	executable := fixedExecutable("/home/user/project/node_modules/bin/comrade")
	evalSymlinks := func(string) (string, error) { return "", errors.New("dangling symlink") }

	got := IsNPMManaged(noEnv, executable, evalSymlinks)

	assert.False(t, got, "an EvalSymlinks error must fail open (not npm-managed), even though the raw path would have matched")
}

// TestIsNPMManagedEnvSignal is the table-driven guard for the primary,
// env-based signal: COMRADE_MANAGED_BY=npm must report managed
// regardless of the executable's own path (a non-node_modules path is
// used deliberately, so this isolates the env signal from the path
// signal); unset or any other value must fall through to the path
// signal, which also does not match here, so the overall result is
// "not managed".
func TestIsNPMManagedEnvSignal(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		isSet    bool
		want     bool
	}{
		{name: "COMRADE_MANAGED_BY=npm", envValue: "npm", isSet: true, want: true},
		{name: "COMRADE_MANAGED_BY unset", isSet: false, want: false},
		{name: "COMRADE_MANAGED_BY=some-other-value", envValue: "homebrew", isSet: true, want: false},
		{name: "COMRADE_MANAGED_BY=empty string", envValue: "", isSet: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				if key == envkeys.ManagedByEnvVar && tt.isSet {
					return tt.envValue
				}
				return ""
			}

			got := IsNPMManaged(getenv, fixedExecutable("/usr/local/bin/comrade"), identityEvalSymlinks)

			assert.Equal(t, tt.want, got)
		})
	}
}
