package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/secrets"
)

// passwordReader reads a secret line from the terminal file descriptor
// fd without echoing it, returning the raw bytes read. Its production
// value is golang.org/x/term.ReadPassword; newAuthLoginCmd takes it as a
// parameter (rather than calling term.ReadPassword directly) so tests
// can inject a fake reader instead of needing a real TTY on fd — see
// term.ReadPassword's own doc comment for why it otherwise requires one.
type passwordReader func(fd int) ([]byte, error)

// emitOpenAICompatBaseURLWarning is config.EmitBaseURLWarning, held
// behind a package-level var (the same seam pattern config.go's own
// baseURLWarningWriter already establishes, applied here as a func value
// instead of an io.Writer) so promptOpenAICompatBaseURL's tests can
// capture what would otherwise write straight to config's own
// process-wide os.Stderr target — reassigning the os.Stderr *variable*
// from a test does NOT redirect it, since config's package-level writer
// already captured the original *os.File at its own var-init time; only
// this seam, or an OS-level fd dup2 (not portable to Windows, which this
// project targets — CLAUDE.md), can intercept it.
var emitOpenAICompatBaseURLWarning = config.EmitBaseURLWarning

// newAuthCmd builds the "comrade auth" command tree: login, logout,
// status (docs/history/UYGULAMA_PLANI.md FAZ 8 item 2).
//
// RunE/Args mirror newHookCmd's own established pattern (hook.go): RunE
// (print help) is what makes this command Runnable at all — without it,
// cobra's execute() returns flag.ErrHelp for ANY invocation before Args
// ever runs (see translatedUnknownSubcommand's own doc comment,
// argvalidation.go) — so a bare "comrade auth" still just prints help
// and exits 0 (RunE's own path, len(args)==0 passes Args trivially),
// while "comrade auth <unmatched>" now gets a translated, actionable
// error naming every real subcommand instead of cobra's raw "unknown
// command %q for %q" (which this specific command never actually
// surfaced before this change either — see translatedUnknownSubcommand's
// doc comment for why "silently show help, exit 0" was the true prior
// behavior, not a raw English error).
func newAuthCmd(newLoader loaderFactory) *cobra.Command {
	root := &cobra.Command{
		Use:   "auth",
		Short: "Manage stored LLM provider API keys (keychain, with a file fallback)",
		Args:  translatedUnknownSubcommand(newLoader),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.AddCommand(
		newAuthLoginCmd(newLoader, term.ReadPassword, term.IsTerminal),
		newAuthLogoutCmd(newLoader),
		newAuthStatusCmd(newLoader),
	)
	return root
}

// isKnownKeyProvider reports whether provider is one secrets.KnownProviders
// covers — i.e. a provider comrade auth login/logout will accept.
func isKnownKeyProvider(provider string) bool {
	for _, p := range secrets.KnownProviders {
		if p == provider {
			return true
		}
	}
	return false
}

func newAuthLoginCmd(newLoader loaderFactory, readPassword passwordReader, isTerminal isTerminalFunc) *cobra.Command {
	return &cobra.Command{
		Use:               "login <provider>",
		Short:             "Store an API key for a provider, then send a small test request",
		Args:              translatedExactArgs(newLoader, 1, i18n.MsgAuthLoginUsageError, strings.Join(secrets.KnownProviders, ", ")),
		ValidArgsFunction: completeFirstArgFromList(secrets.KnownProviders),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := args[0]
			// The ollama/unknown-provider checks intentionally run BEFORE
			// any config is ever loaded — this must stay a zero-config-
			// touch fast rejection (auth_test.go's TestAuthLoginRejectsOllama/
			// RejectsUnknownProvider never isolate a config dir at all,
			// relying on exactly that) — so their error text is translated
			// via envOnlyTranslator (runtime.go: COMRADE_LANG/LANG/LC_ALL
			// only, no config general.language), not the config-aware `tr`
			// built below for every other prompt in this command.
			if provider == "ollama" {
				return fmt.Errorf("%s", envOnlyTranslator().T(i18n.MsgAuthOllamaNoKeyError))
			}
			if !isKnownKeyProvider(provider) {
				return fmt.Errorf("%s", envOnlyTranslator().T(i18n.MsgAuthUnknownProviderError, provider, strings.Join(secrets.KnownProviders, ", ")))
			}

			cfg, tr, err := loadConfigWithNotice(cmd, newLoader)
			if err != nil {
				return err
			}

			// QA MINOR-5: without this check, a non-interactive stdin
			// (piped/redirected/scripted invocation) reached
			// readPassword below and failed with x/term.ReadPassword's
			// own raw platform errno ("inappropriate ioctl for device"
			// on Unix) — a message that names no actionable cause.
			if err := requireInteractiveTTY(tr, isTerminal, i18n.MsgAuthLoginRequiresTTY); err != nil {
				return err
			}

			if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgAuthEnterKeyPrompt, provider)); err != nil {
				return err
			}
			raw, err := readPassword(int(os.Stdin.Fd()))
			if err != nil {
				return fmt.Errorf("auth login: read key: %w", err)
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
				return err
			}
			key := strings.TrimSpace(string(raw))
			if key == "" {
				return fmt.Errorf("%s", tr.T(i18n.MsgAuthNoKeyEnteredError))
			}

			loader, err := newLoader()
			if err != nil {
				return err
			}

			// Captured BEFORE anything below mutates config, so the
			// ErrAuthRejected branch (the ping-classification switch
			// further down) can restore all three to exactly what they
			// were if the key turns out to be definitively bad. Rejection
			// is the ONE ping outcome that stores no key at all (see the
			// ping-classification comment below) — a hard-rejected login
			// must leave config exactly as it found it, not half-switched
			// to a provider (and, if the openai_compat prompts ran,
			// base_url/model) with no credential behind them. Every OTHER
			// outcome (success, 404-model, base_url-unsafe, generic
			// ping-failed) DOES store the key, so keeping the
			// provider/model/base_url activation there is correct —
			// restoration below is deliberately scoped to ErrAuthRejected
			// only.
			priorProvider := cfg.LLM.Provider
			priorModel := cfg.LLM.Model
			priorOpenAICompatBaseURL := cfg.LLM.OpenAICompat.BaseURL

			// Activate the provider being logged into. Without this,
			// pingProviderWithKey's own `cfg.LLM.Provider != provider`
			// guard (llmping.go) — there to null out a STALE model name
			// left over from whatever provider WAS active when switching
			// — fires on every fresh-install-or-never-switched login:
			// llm.provider defaults to "anthropic" (config/schema.go) and
			// auth login never touched it before this fix, so logging into
			// openai_compat while llm.provider was still "anthropic"
			// wiped cfg.LLM.Model straight back to "" at ping time —
			// silently discarding a model just entered at
			// promptOpenAICompatModelIfEmpty below, pinging the OpenAI
			// default model against a non-OpenAI provider, and (worse)
			// leaving the WRONG provider active afterward with the new
			// model as its GLOBAL llm.model, poisoning it for the next
			// unrelated `comrade "..."` run. Persisted and reloaded BEFORE
			// the openai_compat-specific prompts below, so their own
			// base_url/model logic — and the eventual ping, and the
			// 404-branch's effectiveModel — all see the real, now-active
			// provider and the model that was actually entered.
			//
			// providerChanged only tracks whether to WRITE and, later,
			// whether to PRINT MsgAuthProviderActivated — the print itself
			// is deliberately deferred until after the ping (see the
			// deferred-notice block below pingProvider): printing it here,
			// immediately after the write, would leave a stale "active
			// provider set to X" line standing even on a hard-rejected
			// (401/403) login, whose config this same RunE rolls back to
			// its prior value a few lines further down — the write must
			// happen now (the ping needs it in place), but the user-facing
			// claim must wait until we know the write is actually going to
			// stick.
			providerChanged := cfg.LLM.Provider != provider
			if providerChanged {
				if err := loader.SetAndSave("llm.provider", provider); err != nil {
					return err
				}
				reloaded, _, err := loader.Load()
				if err != nil {
					return err
				}
				cfg = *reloaded
			}

			// The bug this fixes: openai_compat is a single connector
			// shared by every OpenAI-compatible provider (Mistral, Groq,
			// GLM/Zhipu, Qwen, Kimi/Moonshot, OpenRouter, LM Studio —
			// CLAUDE.md's LLM Provider Mimarisi), but
			// llm.openai_compat.base_url DEFAULTS to OpenAI's own API
			// (config/schema.go). A user who never customizes base_url and
			// logs in here with, say, a Qwen/DashScope key was silently
			// pinging api.openai.com below with the wrong key and getting a
			// 401 from OpenAI itself — not from their actual provider, and
			// with no hint why. Only prompt when the loaded VALUE still
			// equals the shipped default (see
			// promptOpenAICompatBaseURLIfDefault's own doc comment for why
			// that is a value comparison, not loader.Source(key)): a user
			// who already pointed base_url somewhere else gets no new
			// prompt at all. Runs BEFORE pingProvider so a newly-entered
			// endpoint is both the one pinged and the one persisted, never
			// a stale one.
			// promptOpenAICompatModelIfEmpty (below) needs the effective
			// base_url AFTER promptOpenAICompatBaseURLIfDefault has had a
			// chance to change it — cfg itself is never mutated by that
			// call (it only writes through loader.SetAndSave), so cfg is
			// reloaded from disk in between; the second reload afterward
			// picks up a model the user may have just entered, so the
			// 404-model-not-found branch below (which names the model
			// that was actually pinged) sees it too.
			//
			// Both prompts share ONE bufio.Reader over cmd.InOrStdin(),
			// built once here rather than inside each prompt function:
			// bufio.Reader.Read greedily drains its underlying reader into
			// its own internal buffer on first use, so a second,
			// independently-constructed bufio.Reader wrapping the SAME
			// cmd.InOrStdin() would see nothing left to read — silently
			// losing a model line a piped/scripted caller supplied right
			// after the base_url line in one shot.
			// savedBaseURL is "" when promptOpenAICompatBaseURLIfDefault
			// never persisted anything (prompt skipped entirely, or the
			// user pressed Enter to keep the default) and the ENTERED
			// value otherwise — used below, after the ping, to decide
			// whether to print MsgAuthOpenAICompatBaseURLSaved. Deferred
			// for the exact same reason providerChanged's print is
			// deferred above: promptOpenAICompatBaseURL itself no longer
			// prints it, so a hard-rejected login (which rolls this value
			// back) never has a stale "saved" line to begin with.
			var savedBaseURL string
			if provider == "openai_compat" {
				reader := bufio.NewReader(cmd.InOrStdin())
				savedBaseURL, err = promptOpenAICompatBaseURLIfDefault(cmd, loader, cfg, tr, reader)
				if err != nil {
					return err
				}
				reloaded, _, err := loader.Load()
				if err != nil {
					return err
				}
				cfg = *reloaded
				if err := promptOpenAICompatModelIfEmpty(cmd, loader, cfg, tr, reader); err != nil {
					return err
				}
				reloaded, _, err = loader.Load()
				if err != nil {
					return err
				}
				cfg = *reloaded
			}

			store, err := newSecretsStore(cmd.ErrOrStderr(), tr)
			if err != nil {
				return err
			}

			// Ping BEFORE ever storing anything (QA MAJOR-2, reordered
			// per review): pingProvider verifies the IN-MEMORY key
			// directly (llm.WithKeyResolver's closure below, never the
			// store), so there is no need to write first and undo on
			// rejection. What happens next depends on WHY the ping
			// failed:
			//   - llm.ErrAuthRejected (401/403 — the provider itself
			//     definitively rejected this key): the key is wrong, not
			//     merely unverifiable. Return a genuine command error
			//     (nonzero exit) WITHOUT ever writing to the keychain/
			//     file store — no write, no delete, no window where a
			//     known-bad key sits stored, no delete-failure mode to
			//     handle at all. Config is rolled back to its priorXxx
			//     snapshot (captured above, before any of this RunE's
			//     writes) for the SAME reason: this is the one outcome
			//     that stores no key, so a provider/model/base_url switch
			//     that already landed would otherwise survive a rejected
			//     login and silently break whatever WAS working.
			//   - anything else (network/timeout/5xx/parse — the key
			//     might be fine, the PING failed): store it anyway,
			//     reported but not a command error — an offline user (or
			//     one hitting a transient provider-side error) must not
			//     be blocked from saving a key they believe is correct,
			//     see docs/history/phases/FAZ-08.md's "login stores even if ping
			//     fails" rationale. Every one of these paths DOES store
			//     the key, so no rollback — keeping the provider/model/
			//     base_url activation active is correct here.
			resp, latency, pingErr := pingProvider(cmd, newLoader, provider, key)
			if pingErr != nil && errors.Is(pingErr, llm.ErrAuthRejected) {
				if err := loader.SetAndSave("llm.provider", priorProvider); err != nil {
					return err
				}
				if err := loader.SetAndSave("llm.model", priorModel); err != nil {
					return err
				}
				if err := loader.SetAndSave("llm.openai_compat.base_url", priorOpenAICompatBaseURL); err != nil {
					return err
				}
				return fmt.Errorf("%s", tr.T(i18n.MsgAuthKeyRejected, provider, pingErr, provider))
			}

			// Every path from here on DOES store the key (or, for the
			// base_url-unsafe case just below, skips ONLY the live test
			// while still keeping the provider/base_url it just switched
			// to, for a definitive non-key reason) — none of them is the
			// ErrAuthRejected rollback path just above, so it is now safe
			// — and correct — to surface the two notices deferred from
			// the activation block and promptOpenAICompatBaseURL earlier:
			// printing them any sooner would have left a stale "active
			// provider set to X" / "saved base_url = Y" line standing on
			// a hard-rejected login whose config gets rolled back a few
			// lines above.
			if providerChanged {
				if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgAuthProviderActivated, provider)); err != nil {
					return err
				}
			}
			if savedBaseURL != "" {
				if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgAuthOpenAICompatBaseURLSaved, savedBaseURL)); err != nil {
					return err
				}
			}

			if pingErr != nil {
				// pingProvider's own llm.New call refuses to build a
				// client at all when the active provider's base_url is
				// reject-class (isBaseURLRejection — runtime.go, the SAME
				// detection do/fix/explain/chat's
				// translateBaseURLRejectedError uses). That is a
				// definitive, known cause — the endpoint itself, not the
				// key — so it gets its own translated, base_url-focused
				// message (MsgAuthStoredKeyBaseURLUnsafe) instead of the
				// generic MsgAuthStoredKeyPingFailed framing below, which
				// would misleadingly read as "a network hiccup, not
				// necessarily a bad key" for what is actually a security
				// refusal. The key is still stored: buildProvider refuses
				// before any network call, so it was never transmitted,
				// and storing it locally (0600) is harmless — only the
				// ping was skipped, and the message says so.
				if invalid, ok := isBaseURLRejection(pingErr); ok {
					if err := store.Set(cmd.Context(), provider, key); err != nil {
						return fmt.Errorf("auth login: store key: %w", err)
					}
					_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgAuthStoredKeyBaseURLUnsafe, provider, invalid.Key, invalid.Raw, invalid.Key))
					return err
				}
				// A 404 whose body mentions "model" is the provider saying
				// the MODEL name is wrong, not the key — most commonly the
				// openai_compat default (defaultOpenAICompatModel,
				// client.go) surviving against a non-OpenAI provider that
				// has never heard of it. Distinct, known cause, same as the
				// isBaseURLRejection branch above: reported as a notice
				// naming the actual model that was pinged, not the generic
				// "could not verify it right now" framing, which would
				// misleadingly suggest a network hiccup. The key is stored
				// regardless — it was never the problem.
				//
				// Caveat: this is a substring match on the provider's own
				// free-text error message, not a structured error code —
				// a 404 for an UNRELATED reason (a route that happens to
				// 404, a resource lookup) whose body happens to contain
				// the word "model" would still be misclassified as this
				// notice instead of the generic ping-failed one. The key
				// is stored either way, so the only cost of a false
				// positive is a misleading (not harmful) message; no
				// provider in this codebase's connectors is known to
				// trigger that today (see
				// TestAuthLoginOpenAICompatReportsPingFailedOn404WithoutModelWording
				// for the negative case this DOES correctly classify).
				var statusErr *llm.StatusError
				if errors.As(pingErr, &statusErr) && statusErr.StatusCode == http.StatusNotFound &&
					strings.Contains(strings.ToLower(statusErr.Message), "model") {
					effectiveModel := cfg.LLM.Model
					if effectiveModel == "" {
						effectiveModel = llm.DefaultOpenAICompatModel()
					}
					if err := store.Set(cmd.Context(), provider, key); err != nil {
						return fmt.Errorf("auth login: store key: %w", err)
					}
					_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgAuthModelNotFound, effectiveModel))
					return err
				}
				if err := store.Set(cmd.Context(), provider, key); err != nil {
					return fmt.Errorf("auth login: store key: %w", err)
				}
				_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgAuthStoredKeyPingFailed, pingErr))
				return err
			}
			if err := store.Set(cmd.Context(), provider, key); err != nil {
				return fmt.Errorf("auth login: store key: %w", err)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgAuthStoredKeyPingSucceeded,
				provider, resp.Model, latency.Round(time.Millisecond)))
			return err
		},
	}
}

// promptOpenAICompatBaseURLIfDefault decides whether to ask for the real
// endpoint by comparing the loaded llm.openai_compat.base_url against
// config.Default()'s own value for that same key — NOT
// loader.Source(key), despite that looking like the more direct "did the
// user ever set this" signal. It isn't one here: Loader.ensureFileExists
// writes defaultConfigTOML VERBATIM to disk the first time any command
// ever runs (before this function is ever reached — loadConfigWithNotice
// itself already triggered it earlier in this same RunE), and that
// template spells out base_url's value explicitly. From that point on,
// for the entire lifetime of the install, Loader.Source reports
// SourceFile for this key — the value is genuinely "in the file" — even
// though a real user never once touched it. A Source()-based check here
// would silently never fire for anyone; comparing the effective VALUE
// against the known default is the check that actually distinguishes
// "still pointed at OpenAI" from "customized," matching what this
// function needs regardless of how that value ended up in the resolved
// config. The trade-off is a user who explicitly re-set base_url back to
// literally OpenAI's own URL gets one harmless extra prompt (Enter keeps
// it) — vastly preferable to the check never triggering for anyone.
//
// Returns the value it persisted ("" if the prompt never fired, or fired
// but the user pressed Enter to keep the default) — the caller prints
// MsgAuthOpenAICompatBaseURLSaved itself, deferred until after the ping,
// rather than this function printing it immediately (see newAuthLoginCmd's
// own doc comment on why that print is deferred: a hard-rejected login
// rolls this value back, and must never have printed a "saved" line for a
// value that no longer sticks).
func promptOpenAICompatBaseURLIfDefault(cmd *cobra.Command, loader *config.Loader, cfg config.Config, tr i18n.Translator, reader *bufio.Reader) (string, error) {
	if cfg.LLM.OpenAICompat.BaseURL != config.Default().LLM.OpenAICompat.BaseURL {
		return "", nil
	}
	return promptOpenAICompatBaseURL(cmd, loader, tr, cfg.LLM.OpenAICompat.BaseURL, reader)
}

// promptOpenAICompatBaseURL reads a single line from reader (a
// bufio.Reader the caller built over cmd.InOrStdin() — see
// newAuthLoginCmd's own doc comment on why this and
// promptOpenAICompatModelIfEmpty must share ONE reader rather than each
// building its own), naming currentDefault (the still-in-effect shipped
// default) in the prompt itself (MsgAuthOpenAICompatBaseURLPrompt). An
// empty line (a bare Enter) leaves llm.openai_compat.base_url untouched —
// genuine OpenAI users must not be forced to retype their endpoint — and
// returns ("", nil) without writing anything. A non-empty line is
// validated with config.CheckBaseURL, the SAME reject-class check
// internal/llm/client.go's buildProvider applies at client-construction
// time: a rejected value is reported via the existing
// MsgLLMBaseURLRejected message (reused rather than adding a
// near-duplicate) and re-prompted, never silently kept or saved. An
// accepted value is persisted via loader.SetAndSave before this returns —
// so the caller's subsequent pingProvider call (which re-Loads config
// from disk) sees it too — and returned as-is; this function does NOT
// print MsgAuthOpenAICompatBaseURLSaved itself (see the caller's own doc
// comment on why that print is the CALLER's job, deferred until after the
// ping).
func promptOpenAICompatBaseURL(cmd *cobra.Command, loader *config.Loader, tr i18n.Translator, currentDefault string, reader *bufio.Reader) (string, error) {
	const key = "llm.openai_compat.base_url"
	for {
		if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgAuthOpenAICompatBaseURLPrompt, currentDefault)); err != nil {
			return "", err
		}
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("auth login: read base_url: %w", err)
		}
		value := strings.TrimSpace(line)
		if value == "" {
			return "", nil
		}
		warning, checkErr := config.CheckBaseURL(key, value)
		if checkErr != nil {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), tr.T(i18n.MsgLLMBaseURLRejected, key, value, key)); err != nil {
				return "", err
			}
			if errors.Is(err, io.EOF) {
				return "", nil
			}
			continue
		}
		// Same warned-but-allowed case config.CheckBaseURL's own doc
		// comment documents (http:// to a non-loopback host: the API key
		// will be sent unencrypted) — `comrade config set` already
		// surfaces this via config.EmitBaseURLWarning (Validate,
		// validate.go); reused here (via the emitOpenAICompatBaseURLWarning
		// seam above) rather than adding a second, near-duplicate warning
		// string, so a value typed at THIS credential-entry prompt gets
		// the identical notice a value set via `config set` would. Guarded
		// on non-empty here (rather than relying on
		// config.EmitBaseURLWarning's own internal empty-check) so the
		// seam only ever observes a REAL warning, never an empty no-op
		// call. Kept UN-deferred (unlike MsgAuthOpenAICompatBaseURLSaved
		// below): this is a security warning about the value the user just
		// typed, not a confirmation of what got persisted, so it belongs
		// at entry time regardless of how the ping later turns out.
		if warning != "" {
			emitOpenAICompatBaseURLWarning(warning)
		}
		if err := loader.SetAndSave(key, value); err != nil {
			return "", err
		}
		return value, nil
	}
}

// promptOpenAICompatModelIfEmpty asks for this provider's model name when
// cfg's base_url is no longer OpenAI's own default (i.e. the user is
// pointed at some other OpenAI-compatible provider — Qwen, Groq, GLM,
// etc.) AND llm.model is still empty. Without this, buildProvider
// (internal/llm/client.go) silently falls back to
// llm.DefaultOpenAICompatModel() — an OpenAI-specific model name — against
// a provider that has never heard of it, failing with a confusing 404 far
// downstream of this command. Called by newAuthLoginCmd right after
// promptOpenAICompatBaseURLIfDefault, against cfg RELOADED from disk after
// that call (never the cfg loaded before it), so a base_url the user just
// typed at that prompt is what decides whether this one fires too — the
// prompt's own local cfg parameter is never mutated by
// promptOpenAICompatBaseURLIfDefault (it only writes through
// loader.SetAndSave), so checking the original cfg here would miss a
// provider switch that just happened in the very same invocation.
//
// reader is the SAME bufio.Reader promptOpenAICompatBaseURLIfDefault (via
// promptOpenAICompatBaseURL) was given — never a second one independently
// wrapping cmd.InOrStdin(), which would silently lose a model line a
// piped/scripted caller supplied right after the base_url line (see
// newAuthLoginCmd's own doc comment on the shared-reader rationale). An
// empty line (a bare Enter) leaves llm.model untouched — the user can
// always set it later with `comrade config set llm.model` — matching
// promptOpenAICompatBaseURL's own empty-line behavior. A non-empty line is
// persisted via loader.SetAndSave before this returns, so the caller's
// subsequent pingProvider call (which re-Loads config from disk) sees it
// too. Unlike promptOpenAICompatBaseURL, there is no format to validate —
// any non-empty model name is accepted as-is; the provider's own API is
// what will accept or reject it, surfaced via the ping's own
// MsgAuthModelNotFound classification.
func promptOpenAICompatModelIfEmpty(cmd *cobra.Command, loader *config.Loader, cfg config.Config, tr i18n.Translator, reader *bufio.Reader) error {
	if cfg.LLM.OpenAICompat.BaseURL == config.Default().LLM.OpenAICompat.BaseURL {
		return nil
	}
	if cfg.LLM.Model != "" {
		return nil
	}
	if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgAuthOpenAICompatModelPrompt)); err != nil {
		return err
	}
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("auth login: read model: %w", err)
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return nil
	}
	return loader.SetAndSave("llm.model", value)
}

// pingProvider is `comrade auth login`'s own use of pingProviderWithKey
// (llmping.go): it loads cfg itself (key was just entered and may not
// even be persisted successfully yet by the time this races against it,
// so the stored-credential resolver is bypassed entirely — key is used
// directly instead), then delegates the actual ping to the shared helper
// `comrade doctor --live` also uses.
func pingProvider(cmd *cobra.Command, newLoader loaderFactory, provider, key string) (llm.CompletionResponse, time.Duration, error) {
	loader, err := newLoader()
	if err != nil {
		return llm.CompletionResponse{}, 0, err
	}
	cfg, _, err := loader.Load()
	if err != nil {
		return llm.CompletionResponse{}, 0, err
	}
	return pingProviderWithKey(cmd.Context(), *cfg, provider, key)
}

func newAuthLogoutCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:               "logout <provider>",
		Short:             "Remove a stored API key",
		Args:              translatedExactArgs(newLoader, 1, i18n.MsgAuthLogoutUsageError, strings.Join(secrets.KnownProviders, ", ")),
		ValidArgsFunction: completeFirstArgFromList(secrets.KnownProviders),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := args[0]
			_, tr, err := loadConfigWithNotice(cmd, newLoader)
			if err != nil {
				return err
			}

			store, err := newSecretsStore(cmd.ErrOrStderr(), tr)
			if err != nil {
				return err
			}

			if err := store.Delete(cmd.Context(), provider); err != nil {
				if errors.Is(err, secrets.ErrNoCredential) {
					_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgAuthNoStoredKey, provider))
					return err
				}
				return fmt.Errorf("auth logout: %w", err)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgAuthRemovedStoredKey, provider))
			return err
		},
	}
}

func newAuthStatusCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:               "status",
		Short:             "Show which providers have a stored or environment API key",
		Args:              translatedNoArgs(newLoader),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, tr, err := loadConfigWithNotice(cmd, newLoader)
			if err != nil {
				return err
			}

			store, err := newSecretsStore(cmd.ErrOrStderr(), tr)
			if err != nil {
				return err
			}
			statuses, err := store.Status(cmd.Context())
			if err != nil {
				return fmt.Errorf("auth status: %w", err)
			}

			byProvider := make(map[string]secrets.ProviderStatus, len(statuses))
			for _, s := range statuses {
				byProvider[s.Provider] = s
			}

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(tw, tr.T(i18n.MsgAuthStatusHeader)); err != nil {
				return err
			}
			for _, provider := range secrets.KnownProviders {
				if _, err := fmt.Fprintf(tw, "%s\t%s\n", provider, providerStatusLabel(byProvider[provider], tr)); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(tw, tr.T(i18n.MsgAuthStatusOllamaRow)); err != nil {
				return err
			}
			return tw.Flush()
		},
	}
}

// providerStatusLabel renders one auth status table row's value —
// "set (keychain)"/"set (file)" from the Store, or "set (env: NAME)"
// falling back to an environment-variable check, or "not set" — without
// ever printing the key's own value (CLAUDE.md security rule #3's "never
// log key values", extended here to every FAZ 8 command's output). The
// credential source name itself (st.Source: "keychain"/"file") is left
// untranslated, like a risk-class name — it is Store's own internal
// vocabulary, not prose.
func providerStatusLabel(st secrets.ProviderStatus, tr i18n.Translator) string {
	if st.Source != "" && st.Source != secrets.SourceNone {
		return tr.T(i18n.MsgAuthStatusSet, st.Source)
	}
	if envVar, ok := firstSetEnvVar(st.Provider); ok {
		return tr.T(i18n.MsgAuthStatusSetEnv, envVar)
	}
	return tr.T(i18n.MsgAuthStatusNotSet)
}

// firstSetEnvVar returns the first of provider's known environment
// variables (llm.ProviderEnvVars, in the same priority order
// llm.ResolveEnvKey checks them) that is actually set, for display in
// `comrade auth status` — never the value itself.
func firstSetEnvVar(provider string) (string, bool) {
	for _, name := range llm.ProviderEnvVars(provider) {
		if os.Getenv(name) != "" {
			return name, true
		}
	}
	return "", false
}
