// Package doctor implements `comrade doctor`'s read-only self-diagnostic:
// an ordered registry of independent checks, each returning a structured
// Result, that internal/cli/doctor.go renders as a checklist. Every check
// here is pure business logic — no color, no i18n rendering, no
// bubbletea — so this package has no dependency on internal/tui or
// internal/i18n's Translator; a Result's Summary is a MessageID plus
// args, rendered by the caller in whatever language it resolved.
//
// Nothing in this package ever sends a credential anywhere by default:
// Deps.Live gates the one check (reach) that can, and only when true.
package doctor

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/secrets"
	"github.com/firatkutay/cli-comrade/internal/update"
)

// Severity classifies one Check's outcome.
type Severity int

const (
	// SeverityOK means the check found nothing wrong.
	SeverityOK Severity = iota
	// SeverityWarn means the check found something worth the user's
	// attention, but not broken — comrade still works.
	SeverityWarn
	// SeverityFail means the check found something broken — the exit
	// code the whole `comrade doctor` run reports is driven by whether
	// ANY result is SeverityFail (see internal/cli/doctor.go).
	SeverityFail
	// SeveritySkip means the check could not run at all in this
	// environment (e.g. the active provider needs no credential, or the
	// current shell could not be detected) — neither a pass nor a
	// problem.
	SeveritySkip
)

// String renders sev as a short, stable, lowercase identifier — used by
// --json output and in COMRADE_DEBUG-style diagnostics. It is
// deliberately NOT routed through internal/i18n: like safety.RiskClass's
// own String() (internal/tui's RiskBadge), this is internal vocabulary,
// not prose — internal/cli/doctor.go's checklist rendering resolves its
// own separate, translated/colored label from the Severity value instead
// of this string.
func (s Severity) String() string {
	switch s {
	case SeverityOK:
		return "ok"
	case SeverityWarn:
		return "warn"
	case SeverityFail:
		return "fail"
	case SeveritySkip:
		return "skip"
	default:
		return "unknown"
	}
}

// Result is one Check's outcome.
type Result struct {
	// ID is the owning Check's own ID (e.g. "version", "path") — stable,
	// internal vocabulary, never localized.
	ID string
	// Severity classifies this outcome.
	Severity Severity
	// Summary names the MessageID internal/cli/doctor.go renders (via an
	// i18n.Translator) as this result's one-line description, with
	// SummaryArgs interpolated into it. A Result's Summary is never a
	// pre-rendered string: this package has no Translator of its own (see
	// the package doc comment), so every check returns the MessageID/args
	// pair and lets the caller resolve the active language.
	Summary     i18n.MessageID
	SummaryArgs []any
	// Fix is a copy-pasteable remediation — almost always a literal
	// comrade (or vendor, e.g. `ollama pull llama3.1`) command — for a
	// non-OK/non-Skip result. Deliberately a plain string, not a
	// MessageID: a shell command is not prose to translate, exactly like
	// a config key name or provider name is left untranslated elsewhere
	// in this codebase. Empty for SeverityOK/SeveritySkip.
	Fix string
	// Detail is optional, raw supplementary diagnostic text (a resolved
	// path, an underlying error's own message) — like Fix, deliberately a
	// plain, unlocalized string. internal/cli/doctor.go only surfaces
	// this when COMRADE_DEBUG is set (matching this codebase's existing
	// debug-gated-raw-detail convention — see e.g.
	// internal/cli/upgrade.go's translateUpgradeFetchError) or
	// unconditionally in --json output.
	Detail string
}

// Check is one named, independent diagnostic. Run must never panic and
// must never mutate anything on disk or over the network beyond what its
// own doc comment states (`comrade doctor` is read-only) — it receives a
// context already scoped to checkTimeout by Runner.Run.
type Check struct {
	ID  string
	Run func(ctx context.Context, deps Deps) Result
}

// Deps bundles every OS/network/credential touchpoint the check registry
// needs, exactly like upgradeDeps/initDeps bundle their own commands'
// dependencies (internal/cli) — every field here is independently
// injectable so each check's tests can fake exactly the seam it exercises
// without touching a real filesystem, network, or keychain.
type Deps struct {
	// Cfg is the effective config internal/cli/doctor.go already loaded
	// (loadConfigWithNotice-equivalent) before building Deps — checks
	// never load config themselves.
	Cfg config.Config
	// ConfigErr is non-nil when loading Cfg itself failed; Cfg is then the
	// zero value and every check that depends on Cfg.LLM.Provider treats
	// it as unknown (Skip) rather than guessing.
	ConfigErr error
	// Version is the build-time comrade version (main.version), exactly
	// as update.IsDevBuild/update.Updater.Check expect it.
	Version string
	// Fetcher resolves the latest published GitHub release — the version
	// check's sole network dependency.
	Fetcher update.ReleaseFetcher
	// HTTP is the client the reach check's keyless GET runs through.
	HTTP *http.Client
	// Store is the credential store the key/reach/baseurl checks read
	// from — nil is treated as "no stored credential", never a panic.
	Store secrets.Store
	// Getenv, LookPath, Executable, GOOS mirror
	// context.Collector/shellinit.RCPath's own injectable OS-environment
	// seams, so every check is tests without depending on the real
	// process environment or the OS the test binary runs on.
	Getenv     func(string) string
	LookPath   func(string) (string, error)
	Executable func() (string, error)
	GOOS       string
	// Now is injectable so the version check's update.WriteState call
	// writes a deterministic timestamp in tests.
	Now func() time.Time
	// Run executes a command and returns its combined output — the same
	// signature as shellinit.CommandRunner/context.CommandRunner — used
	// only by the shellhook check's PowerShell $PROFILE resolution
	// (shellinit.RCPath). nil is safe: RCPath's non-PowerShell branches
	// never call it, and its PowerShell branch degrades to an
	// "unresolved" note instead of panicking.
	Run func(ctx context.Context, name string, args ...string) ([]byte, error)
	// Live gates the reach check's opt-in authenticated ping — false
	// (comrade doctor's default) never sends a credential anywhere; only
	// --live sets this.
	Live bool
	// LivePing performs Live's one authenticated request, reusing the
	// SAME hardened path `comrade auth login`'s own ping does
	// (internal/cli's pingProviderWithKey) — internal/doctor never builds
	// an llm.Client itself, keeping this package free of any dependency
	// on internal/cli (which itself depends on internal/doctor — see
	// newDoctorCmd). nil is safe: the reach check simply skips the live
	// step when Live is true but no LivePing was wired in (should never
	// happen in production; only a hand-built Deps in a test could hit
	// this).
	LivePing func(ctx context.Context, cfg config.Config, provider, key string) (llm.CompletionResponse, time.Duration, error)
}

// checkTimeout bounds how long any single check's own ctx is allowed to
// run — the same short, bounded-worst-case precedent
// internal/cli/updatenotice.go's updateNoticeNetworkTimeout already
// established for a single background network call.
const checkTimeout = 3 * time.Second

// Runner holds the fixed, ordered registry of checks `comrade doctor`
// runs.
type Runner struct {
	Checks []Check
}

// NewRunner builds a Runner with every check registered in this package's
// canonical render order: version, path, shellhook, key, reach, baseurl,
// config.
func NewRunner() Runner {
	return Runner{Checks: []Check{
		{ID: "version", Run: VersionCheck},
		{ID: "path", Run: PathCheck},
		{ID: "shellhook", Run: ShellHookCheck},
		{ID: "key", Run: KeyCheck},
		{ID: "reach", Run: ReachCheck},
		{ID: "baseurl", Run: BaseURLCheck},
		{ID: "config", Run: ConfigCheck},
	}}
}

// Run executes every registered check concurrently (each under its own
// checkTimeout-bounded context derived from ctx), but always returns
// results in r.Checks' own fixed registry order — never completion
// order — so rendering is deterministic regardless of which check (e.g.
// a slow network call) happens to finish last.
func (r Runner) Run(ctx context.Context, deps Deps) []Result {
	results := make([]Result, len(r.Checks))
	var wg sync.WaitGroup
	for i, c := range r.Checks {
		wg.Add(1)
		go func(i int, c Check) {
			defer wg.Done()
			checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
			defer cancel()
			results[i] = c.Run(checkCtx, deps)
			results[i].ID = c.ID
		}(i, c)
	}
	wg.Wait()
	return results
}
