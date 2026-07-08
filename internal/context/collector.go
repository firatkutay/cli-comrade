package context

import (
	stdctx "context"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

// Options controls which opt-in data Collect gathers. Callers translate
// their own config.Context fields (config.ContextConfig's SendHistory /
// HistoryDepth / SendEnvNames) into an Options value; this package
// intentionally does not import internal/config, so it stays usable
// independently of the config schema.
type Options struct {
	SendHistory  bool
	HistoryDepth int
	SendEnvNames bool
}

// Context is the environment snapshot handed to the LLM as grounding for
// its plans (CLAUDE.md "Bağlam Toplama"). History and EnvNames are nil
// unless their corresponding Options flag was set; EnvNames never
// carries a value, only variable names (CLAUDE.md's redaction rule).
type Context struct {
	OS              string
	Shell           string
	ShellVersion    string
	WorkingDir      string
	HomeDir         string
	IsAdmin         bool
	AdminKnown      bool
	PackageManagers []string
	LastCommand     *LastCommand
	History         []string
	EnvNames        []string
}

// Collector gathers a Context. Every OS/exec/env dependency is a field
// so tests can substitute fakes instead of depending on the real
// process environment or the OS the test binary runs on; NewCollector
// wires the real ones.
type Collector struct {
	// GOOS is the target OS name (runtime.GOOS in NewCollector).
	GOOS string
	// Getenv reads an environment variable by name.
	Getenv func(string) string
	// LookPath resolves a binary name against PATH.
	LookPath func(string) (string, error)
	// RunCommand executes a command and returns its output, used for
	// best-effort shell-version detection.
	RunCommand CommandRunner
	// Geteuid reports the process's effective user ID (unix only; see
	// IsAdmin).
	Geteuid func() int
	// Getwd returns the current working directory.
	Getwd func() (string, error)
	// UserHomeDir returns the current user's home directory.
	UserHomeDir func() (string, error)
	// Environ returns the process environment as "NAME=value" strings,
	// used only for the opt-in EnvNames extraction.
	Environ func() []string
}

// NewCollector builds a Collector wired to the real OS and process:
// runtime.GOOS, os.Getenv, exec.LookPath, RunCommand, os.Geteuid,
// os.Getwd, os.UserHomeDir, and os.Environ.
func NewCollector() *Collector {
	return &Collector{
		GOOS:        runtime.GOOS,
		Getenv:      os.Getenv,
		LookPath:    exec.LookPath,
		RunCommand:  RunCommand,
		Geteuid:     os.Geteuid,
		Getwd:       os.Getwd,
		UserHomeDir: os.UserHomeDir,
		Environ:     os.Environ,
	}
}

// Collect gathers the full Context. It never returns an error: every
// sub-collection (shell version, admin check, last_command.json,
// history) is individually best-effort and degrades to its zero value
// rather than failing the whole snapshot — per CLAUDE.md, a context
// collector must never block the primary flow (fix/do/explain) on a
// grounding detail it couldn't gather.
func (c *Collector) Collect(ctx stdctx.Context, opts Options) Context {
	shell := DetectShell(c.GOOS, c.Getenv)
	isAdmin, adminKnown := IsAdmin(c.GOOS, c.Geteuid)

	wd := ""
	if c.Getwd != nil {
		wd, _ = c.Getwd()
	}
	home := ""
	if c.UserHomeDir != nil {
		home, _ = c.UserHomeDir()
	}

	result := Context{
		OS:              c.GOOS,
		Shell:           shell,
		ShellVersion:    ShellVersion(ctx, shell, c.RunCommand),
		WorkingDir:      wd,
		HomeDir:         home,
		IsAdmin:         isAdmin,
		AdminKnown:      adminKnown,
		PackageManagers: DetectPackageManagers(c.LookPath),
	}

	if path, err := LastCommandPath(c.GOOS, c.Getenv); err == nil {
		if lc, ok := ReadLastCommand(path); ok {
			result.LastCommand = &lc
		}
	}

	if opts.SendHistory {
		result.History = ReadHistory(shell, c.Getenv, opts.HistoryDepth)
	}

	if opts.SendEnvNames && c.Environ != nil {
		result.EnvNames = EnvNames(c.Environ())
	}

	return result
}

// EnvNames extracts sorted variable NAMES (never values) out of environ
// entries ("NAME=value" strings, as returned by os.Environ()). It exists
// so context.send_env_names=true can be honored without ever handing a
// value to the LLM, per CLAUDE.md: "Env var içerikleri ASLA gönderilmez,
// sadece isimleri (opt-in)."
func EnvNames(environ []string) []string {
	names := make([]string, 0, len(environ))
	for _, kv := range environ {
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			names = append(names, kv[:idx])
		}
	}
	sort.Strings(names)
	return names
}
