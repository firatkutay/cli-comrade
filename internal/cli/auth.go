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
		newAuthLoginCmd(newLoader, term.ReadPassword),
		newAuthLogoutCmd(),
		newAuthStatusCmd(),
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

func newAuthLoginCmd(newLoader loaderFactory, readPassword passwordReader) *cobra.Command {
	return &cobra.Command{
		Use:   "login <provider>",
		Short: "Store an API key for a provider, then send a small test request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := args[0]
			if provider == "ollama" {
				return fmt.Errorf("auth login: ollama needs no API key — it talks to a local server directly; set llm.ollama.base_url instead")
			}
			if !isKnownKeyProvider(provider) {
				return fmt.Errorf("auth login: unknown provider %q (expected one of: %s)", provider, strings.Join(secrets.KnownProviders, ", "))
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Enter API key for %s: ", provider); err != nil {
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
				return fmt.Errorf("auth login: no key entered")
			}

			store, err := newSecretsStore(cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			if err := store.Set(cmd.Context(), provider, key); err != nil {
				return fmt.Errorf("auth login: store key: %w", err)
			}

			// The key is stored before the ping runs, and stays stored
			// regardless of the ping's outcome: an offline user (or one
			// hitting a transient provider-side error) must not be
			// blocked from saving a key they believe is correct — see
			// docs/phases/FAZ-08.md's "login stores even if ping fails"
			// rationale. A ping failure is reported, not returned as a
			// command error, for the same reason.
			resp, latency, pingErr := pingProvider(cmd, newLoader, provider, key)
			if pingErr != nil {
				_, err := fmt.Fprintf(cmd.OutOrStdout(),
					"Stored key for %s. Test request failed (%v) — the key may still be correct; this can also mean the network or provider is unreachable right now.\n",
					provider, pingErr)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Stored key for %s. Test request succeeded (model=%s, latency=%s).\n",
				provider, resp.Model, latency.Round(time.Millisecond))
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

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout <provider>",
		Short: "Remove a stored API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := args[0]
			store, err := newSecretsStore(cmd.ErrOrStderr())
			if err != nil {
				return err
			}

			if err := store.Delete(cmd.Context(), provider); err != nil {
				if errors.Is(err, secrets.ErrNoCredential) {
					_, err := fmt.Fprintf(cmd.OutOrStdout(), "No stored key for %s.\n", provider)
					return err
				}
				return fmt.Errorf("auth logout: %w", err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Removed stored key for %s.\n", provider)
			return err
		},
	}
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which providers have a stored or environment API key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := newSecretsStore(cmd.ErrOrStderr())
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
			if _, err := fmt.Fprintln(tw, "PROVIDER\tSTATUS"); err != nil {
				return err
			}
			for _, provider := range secrets.KnownProviders {
				if _, err := fmt.Fprintf(tw, "%s\t%s\n", provider, providerStatusLabel(byProvider[provider])); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(tw, "ollama\t(no key required)"); err != nil {
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
// log key values", extended here to every FAZ 8 command's output).
func providerStatusLabel(st secrets.ProviderStatus) string {
	if st.Source != "" && st.Source != secrets.SourceNone {
		return fmt.Sprintf("set (%s)", st.Source)
	}
	if envVar, ok := firstSetEnvVar(st.Provider); ok {
		return fmt.Sprintf("set (env: %s)", envVar)
	}
	return "not set"
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
