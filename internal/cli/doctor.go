package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	comradecontext "github.com/firatkutay/cli-comrade/internal/context"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/doctor"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/secrets"
	"github.com/firatkutay/cli-comrade/internal/tui"
	"github.com/firatkutay/cli-comrade/internal/update"
)

// doctorHTTPTimeout bounds the *http.Client the reach check's keyless GET
// (and, under --live, the one authenticated ping) runs through — longer
// than updateNoticeNetworkTimeout's 3s (a --live ping is a real
// completion request, not a metadata fetch), but still a firm upper
// bound so `comrade doctor` can never hang indefinitely on an
// unresponsive endpoint.
const doctorHTTPTimeout = 10 * time.Second

// doctorDeps bundles every OS/network dependency `comrade doctor` needs,
// exactly like upgradeDeps/initDeps bundle their own commands'
// dependencies — defaultDoctorDeps wires the real ones in NewRootCmd;
// tests construct their own doctorDeps directly.
type doctorDeps struct {
	version    string
	goos       string
	getenv     func(string) string
	lookPath   func(string) (string, error)
	executable func() (string, error)
	run        func(ctx context.Context, name string, args ...string) ([]byte, error)
	fetcher    update.ReleaseFetcher
	httpClient *http.Client
	now        func() time.Time
	livePing   func(ctx context.Context, cfg config.Config, provider, key string) (llm.CompletionResponse, time.Duration, error)
}

// defaultDoctorDeps wires doctorDeps to the real operating system and
// network this process is actually running under, for build-time version
// string.
func defaultDoctorDeps(version string) doctorDeps {
	return doctorDeps{
		version:    version,
		goos:       runtime.GOOS,
		getenv:     os.Getenv,
		lookPath:   exec.LookPath,
		executable: os.Executable,
		run:        comradecontext.RunCommand,
		fetcher:    &update.GitHubClient{},
		httpClient: &http.Client{Timeout: doctorHTTPTimeout},
		now:        time.Now,
		livePing:   pingProviderWithKey,
	}
}

// doctorFailedError is `comrade doctor`'s own error when at least one
// check result is doctor.SeverityFail — P-1's exit-code rule: warnings
// are non-fatal (the command still exits 0), a Fail makes the whole run
// exit 1. It exposes ExitCode() int so cmd/comrade/main.go can honor a
// non-default exit code via errors.As against the
// `interface{ ExitCode() int }` shape, instead of every command error
// collapsing to the SAME blanket os.Exit(1) main.go otherwise applies —
// see main.go's own doc comment on this mechanism.
type doctorFailedError struct {
	message string
}

func (e *doctorFailedError) Error() string { return e.message }

// ExitCode implements the `interface{ ExitCode() int }` shape
// cmd/comrade/main.go looks for via errors.As.
func (e *doctorFailedError) ExitCode() int { return 1 }

// newDoctorCmd builds the "comrade doctor" command: a read-only,
// non-interactive self-diagnostic — an ordered registry of independent
// checks (internal/doctor), rendered as a checklist. Unlike every other
// command that touches config, this deliberately does NOT use
// loadConfigWithNotice: a config LOAD failure must still let every other
// check run and must surface as one Fail row in the checklist itself
// (internal/doctor's "config" check), not abort the whole command before
// anything is printed at all — see doctor.Deps.ConfigErr's own doc
// comment.
func newDoctorCmd(newLoader loaderFactory, deps doctorDeps) *cobra.Command {
	var (
		asJSON bool
		live   bool
	)

	cmd := &cobra.Command{
		Use:               "doctor",
		Short:             "Run a read-only self-diagnostic and report any problems found",
		Args:              translatedNoArgs(newLoader),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			loader, err := newLoader()
			if err != nil {
				return err
			}
			cfg, created, loadErr := loader.Load()
			cfgValue := config.Config{}
			if loadErr == nil {
				cfgValue = *cfg
			}
			tr := newTranslator(cfgValue)
			if loadErr == nil && created {
				if _, err := fmt.Fprint(cmd.ErrOrStderr(), tr.T(i18n.MsgFirstRunNotice, loader.Path())); err != nil {
					return err
				}
			}

			// A secrets.Store construction failure (e.g. config.DefaultDir
			// can't resolve APPDATA/HOME) leaves store nil — every check
			// that reads it (key/reach/baseurl) already treats a nil Store
			// as "nothing stored", falling through to its own env-var
			// fallback, exactly like a Store that simply has no entry for
			// this provider.
			var store secrets.Store
			if s, storeErr := newSecretsStore(cmd.ErrOrStderr(), tr); storeErr == nil {
				store = s
			}

			runnerDeps := doctor.Deps{
				Cfg:        cfgValue,
				ConfigErr:  loadErr,
				Version:    deps.version,
				Fetcher:    deps.fetcher,
				HTTP:       deps.httpClient,
				Store:      store,
				Getenv:     deps.getenv,
				LookPath:   deps.lookPath,
				Executable: deps.executable,
				GOOS:       deps.goos,
				Now:        deps.now,
				Run:        deps.run,
				Live:       live,
				LivePing:   deps.livePing,
			}

			results := doctor.NewRunner().Run(cmd.Context(), runnerDeps)

			if asJSON {
				if err := printDoctorJSON(cmd.OutOrStdout(), results, tr); err != nil {
					return err
				}
			} else {
				colorEnabled := resolveColorEnabled(cfgValue, os.Environ(), cmd.OutOrStdout())
				if err := printDoctorTable(cmd.OutOrStdout(), results, tr, colorEnabled); err != nil {
					return err
				}
			}

			if failCount := countDoctorSeverity(results, doctor.SeverityFail); failCount > 0 {
				return &doctorFailedError{message: tr.T(i18n.MsgDoctorFailedSummary, failCount)}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, enUsageDefault(i18n.MsgFlagJSON))
	cmd.Flags().BoolVar(&live, "live", false, enUsageDefault(i18n.MsgFlagLive))
	return cmd
}

// doctorTitles maps every internal/doctor check ID to its row title
// MessageID — the single hand-maintained mirror of internal/doctor's own
// check-ID registry (doctor.NewRunner) this package needs, guarded by
// TestDoctorTitlesCoverEveryRegisteredCheck (doctor_test.go) so the two
// cannot silently drift apart (a check ID added to the registry without
// a matching title here fails that test immediately).
var doctorTitles = map[string]i18n.MessageID{
	"version":   i18n.MsgDoctorVersionTitle,
	"path":      i18n.MsgDoctorPathTitle,
	"shellhook": i18n.MsgDoctorShellHookTitle,
	"key":       i18n.MsgDoctorKeyTitle,
	"reach":     i18n.MsgDoctorReachTitle,
	"baseurl":   i18n.MsgDoctorBaseURLTitle,
	"config":    i18n.MsgDoctorConfigTitle,
}

// printDoctorTable renders results as a plain, sequential checklist via
// internal/tui's non-interactive PrintDoctorLine (see that package's own
// doc comment on why this is not a bubbletea program) — one line per
// result ("<severity marker> <title>: <summary>"), plus an indented
// "fix:" line for any non-empty Result.Fix, plus an indented "detail:"
// line for any non-empty Result.Detail, but ONLY when COMRADE_DEBUG is
// set — matching this codebase's established debug-gated-raw-detail
// convention (e.g. internal/cli/upgrade.go's translateUpgradeFetchError)
// rather than cluttering the default checklist with internal diagnostic
// text.
func printDoctorTable(w io.Writer, results []doctor.Result, tr i18n.Translator, colorEnabled bool) error {
	debug := os.Getenv("COMRADE_DEBUG") != ""
	for _, r := range results {
		title := tr.T(doctorTitleFor(r.ID))
		text := title + ": " + tr.T(r.Summary, r.SummaryArgs...)
		if err := tui.PrintDoctorLine(w, r.Severity, text, colorEnabled); err != nil {
			return err
		}
		if r.Fix != "" {
			if _, err := fmt.Fprint(w, tr.T(i18n.MsgDoctorFixLabel, r.Fix)); err != nil {
				return err
			}
		}
		if debug && r.Detail != "" {
			if _, err := fmt.Fprint(w, tr.T(i18n.MsgDoctorDetailLabel, r.Detail)); err != nil {
				return err
			}
		}
	}
	return nil
}

// doctorTitleFor resolves id's row title MessageID via doctorTitles,
// falling back to id itself (rendered verbatim, untranslated) for an
// unrecognized ID — defensive only; TestDoctorTitlesCoverEveryRegisteredCheck
// guarantees this fallback is never actually reached for any ID
// doctor.NewRunner() registers.
func doctorTitleFor(id string) i18n.MessageID {
	if title, ok := doctorTitles[id]; ok {
		return title
	}
	return i18n.MessageID(id)
}

// doctorJSONResult is `comrade doctor --json`'s one-object-per-line
// shape (mirrors `comrade history --json`'s printHistoryJSON pattern) —
// Summary is the fully rendered, translated text (not the raw
// MessageID), so --json output is immediately useful without a second
// lookup; Detail is always included here (unlike table output, which
// gates it behind COMRADE_DEBUG) since --json is inherently a raw,
// scriptable data dump, matching `comrade history --json`'s own
// unredacted precedent.
type doctorJSONResult struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Fix      string `json:"fix,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// printDoctorJSON prints results as one compact JSON object per line
// (JSONL), exactly like internal/cli/history.go's printHistoryJSON.
func printDoctorJSON(w io.Writer, results []doctor.Result, tr i18n.Translator) error {
	enc := json.NewEncoder(w)
	for _, r := range results {
		entry := doctorJSONResult{
			ID:       r.ID,
			Severity: r.Severity.String(),
			Summary:  tr.T(r.Summary, r.SummaryArgs...),
			Fix:      r.Fix,
			Detail:   r.Detail,
		}
		if err := enc.Encode(entry); err != nil {
			return err
		}
	}
	return nil
}

// countDoctorSeverity counts how many results have exactly sev.
func countDoctorSeverity(results []doctor.Result, sev doctor.Severity) int {
	n := 0
	for _, r := range results {
		if r.Severity == sev {
			n++
		}
	}
	return n
}
