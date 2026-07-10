package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

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

// newAuthCmd builds the "comrade auth" command tree: login, logout,
// status (UYGULAMA_PLANI.md FAZ 8 item 2).
func newAuthCmd(newLoader loaderFactory) *cobra.Command {
	root := &cobra.Command{
		Use:   "auth",
		Short: "Manage stored LLM provider API keys (keychain, with a file fallback)",
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
		Use:   "login <provider>",
		Short: "Store an API key for a provider, then send a small test request",
		Args:  cobra.ExactArgs(1),
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

			_, tr, err := loadConfigWithNotice(cmd, newLoader)
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
			//     handle at all.
			//   - anything else (network/timeout/5xx/parse — the key
			//     might be fine, the PING failed): store it anyway,
			//     reported but not a command error — an offline user (or
			//     one hitting a transient provider-side error) must not
			//     be blocked from saving a key they believe is correct,
			//     see docs/phases/FAZ-08.md's "login stores even if ping
			//     fails" rationale.
			resp, latency, pingErr := pingProvider(cmd, newLoader, provider, key)
			if pingErr != nil {
				if errors.Is(pingErr, llm.ErrAuthRejected) {
					return fmt.Errorf("%s", tr.T(i18n.MsgAuthKeyRejected, provider, pingErr, provider))
				}
				if err := store.Set(cmd.Context(), provider, key); err != nil {
					return fmt.Errorf("auth login: store key: %w", err)
				}
				_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgAuthStoredKeyPingFailed, provider, pingErr))
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

// pingProvider sends a minimal completion through a Client scoped to
// exactly one attempt — provider, using key directly (bypassing the
// stored-credential resolver entirely, since key was just entered and
// may not even be persisted successfully yet by the time this races
// against it) — reusing the user's other effective settings (base_url,
// timeout) from the loaded config so e.g. an openai_compat login against
// a non-OpenAI endpoint pings the right place.
func pingProvider(cmd *cobra.Command, newLoader loaderFactory, provider, key string) (llm.CompletionResponse, time.Duration, error) {
	loader, err := newLoader()
	if err != nil {
		return llm.CompletionResponse{}, 0, err
	}
	cfg, _, err := loader.Load()
	if err != nil {
		return llm.CompletionResponse{}, 0, err
	}
	cfg.LLM.Fallback = nil
	if cfg.LLM.Provider != provider {
		cfg.LLM.Provider = provider
		cfg.LLM.Model = ""
	}

	client, err := llm.New(*cfg, llm.WithKeyResolver(func(string) (string, error) { return key, nil }))
	if err != nil {
		return llm.CompletionResponse{}, 0, err
	}

	start := time.Now()
	resp, err := client.Complete(cmd.Context(), llm.CompletionRequest{
		Messages:  []llm.Message{{Role: "user", Content: "ping"}},
		MaxTokens: 16,
	})
	return resp, time.Since(start), err
}

func newAuthLogoutCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "logout <provider>",
		Short: "Remove a stored API key",
		Args:  cobra.ExactArgs(1),
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
		Use:   "status",
		Short: "Show which providers have a stored or environment API key",
		Args:  cobra.NoArgs,
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
