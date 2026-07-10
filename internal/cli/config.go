package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
)

// loaderFactory builds a fresh *config.Loader for a single command
// invocation. It is a func rather than a shared *config.Loader value so
// each subcommand resolves the config path (which depends on
// process-environment state) at the moment it actually runs.
type loaderFactory func() (*config.Loader, error)

// newConfigCmd builds the "comrade config" command tree: get, set, list,
// edit and path.
func newConfigCmd(newLoader loaderFactory) *cobra.Command {
	root := &cobra.Command{
		Use:   "config",
		Short: "View and edit cli-comrade configuration",
	}

	root.AddCommand(
		newConfigGetCmd(newLoader),
		newConfigSetCmd(newLoader),
		newConfigListCmd(newLoader),
		newConfigEditCmd(newLoader),
		newConfigPathCmd(newLoader),
		newConfigTestLLMCmd(newLoader),
		newConfigModelsCmd(newLoader),
	)

	return root
}

// newConfigTestLLMCmd sends a minimal "ping" completion through the full
// llm.Client (fallback chain included) built from the effective config,
// and prints the responding provider, model, and latency on success.
// Hidden per UYGULAMA_PLANI.md FAZ 2 item 6 — this is a diagnostic aid,
// not a user-facing feature to advertise in --help.
func newConfigTestLLMCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:    "test-llm",
		Short:  "Send a tiny test completion through the configured LLM provider chain",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			loader, err := newLoader()
			if err != nil {
				return err
			}

			cfg, created, err := loader.Load()
			if err != nil {
				return err
			}
			tr := newTranslator(*cfg)
			if created {
				if _, err := fmt.Fprint(cmd.ErrOrStderr(), tr.T(i18n.MsgFirstRunNotice, loader.Path())); err != nil {
					return err
				}
			}

			store, err := newSecretsStore(cmd.ErrOrStderr(), tr)
			if err != nil {
				return fmt.Errorf("test-llm: %w", err)
			}
			client, err := llm.New(*cfg, llm.WithKeyResolver(secretsKeyResolver(store)))
			if err != nil {
				return fmt.Errorf("test-llm: %w", err)
			}

			start := time.Now()
			resp, err := client.Complete(cmd.Context(), llm.CompletionRequest{
				Messages:  []llm.Message{{Role: "user", Content: "ping"}},
				MaxTokens: 16,
			})
			if err != nil {
				return fmt.Errorf("test-llm: %w", err)
			}
			latency := time.Since(start)

			_, err = fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgTestLLMResult,
				client.Name(), resp.Model, latency.Round(time.Millisecond)))
			return err
		},
	}
}

func newConfigGetCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print the effective value of a config key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			loader, err := newLoader()
			if err != nil {
				return err
			}
			if err := ensureLoaded(loader, cmd.ErrOrStderr()); err != nil {
				return err
			}
			cfg, _, err := loader.Load()
			if err != nil {
				return err
			}
			tr := newTranslator(*cfg)

			value, err := loader.Get(args[0])
			if err != nil {
				return translateConfigError(tr, err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), formatValue(value))
			return err
		},
	}
}

func newConfigSetCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Validate and persist a config key's value",
		// Args is deliberately NOT cobra.ExactArgs(2): DisableFlagParsing
		// (below) means cobra's own flag parser never runs, which is what
		// normally intercepts -h/--help BEFORE any Args validator even
		// sees the args — with ExactArgs(2) in place, "comrade config set
		// --help" (one arg) failed Args validation with cobra's raw
		// English "accepts 2 arg(s), received 1" and RunE was never even
		// reached, so this subcommand's own --help was completely
		// unreachable (QA D2). RunE below does its own -h/--help and
		// arg-count handling instead, translated per general.language.
		Args: cobra.ArbitraryArgs,
		// Config values can legitimately start with "-" (e.g. a bug in a
		// negative-int value that Validate should reject with a clear
		// message, not one pflag silently reinterprets as an unknown
		// shorthand flag). "set" takes exactly two fixed positional
		// arguments and defines no flags of its own, so disabling flag
		// parsing here is safe and lets any raw value through untouched.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
				return cmd.Help()
			}
			if len(args) != 2 {
				// bestEffortTranslator, NOT envOnlyTranslator: a wrong-arg-
				// count usage error is not the "validation must reject a
				// bad key/value before any filesystem side effect" case
				// the comment below is about — no real set attempt was
				// even made here, so loading config for the language is
				// fine (matches every other command's --help/usage-error
				// behavior, e.g. `comrade config get <bad-key>` already
				// loads config first too).
				return fmt.Errorf("%s", bestEffortTranslator(cmd, newLoader).T(i18n.MsgConfigSetUsageError))
			}
			key, raw := args[0], args[1]

			parsed, err := config.Validate(key, raw)
			if err != nil {
				// Deliberately NOT loadConfigWithNotice/newLoader().Load()
				// here: validation must reject a bad key/value before any
				// filesystem side effect (a first-run config file being
				// created) — envOnlyTranslator (the same
				// COMRADE_LANG/LANG/LC_ALL/Windows-locale-only resolution
				// every other "must report before config is ever loaded"
				// path in this tree already uses — see its own doc
				// comment in runtime.go) is what every other such path
				// already resolves language from, so this is one more
				// call site of an existing, documented pattern, not a new
				// one.
				return translateConfigError(envOnlyTranslator(), err)
			}

			loader, err := newLoader()
			if err != nil {
				return err
			}
			if err := ensureLoaded(loader, cmd.ErrOrStderr()); err != nil {
				return err
			}
			if err := loader.SetAndSave(key, parsed); err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s = %s\n", key, formatValue(parsed))
			return err
		},
	}
}

// translateConfigError re-renders a config.UnknownKeyError/
// config.InvalidValueError (from config.Validate/Loader.Get/Source/Set)
// through tr, via errors.As rather than parsing config's own English
// Error() text — the QA-found fix for `comrade config set`/`get`'s
// validation errors bypassing i18n entirely. Any OTHER error (a
// filesystem/decode failure from loader.Load itself, already covered by
// this tree's existing, documented "~40 wrap-chain" i18n exception) is
// returned unchanged.
func translateConfigError(tr i18n.Translator, err error) error {
	var unknownKey *config.UnknownKeyError
	if errors.As(err, &unknownKey) {
		return fmt.Errorf("%s", tr.T(i18n.MsgConfigUnknownKey, unknownKey.Key, strings.Join(unknownKey.ValidKeys, ", ")))
	}

	var invalid *config.InvalidValueError
	if errors.As(err, &invalid) {
		switch invalid.Reason {
		case config.ReasonInvalidEnum:
			return fmt.Errorf("%s", tr.T(i18n.MsgConfigInvalidEnum, invalid.Raw, invalid.Key, strings.Join(invalid.Enum, ", ")))
		case config.ReasonNotBoolean:
			return fmt.Errorf("%s", tr.T(i18n.MsgConfigInvalidBool, invalid.Raw, invalid.Key))
		case config.ReasonNotInteger:
			return fmt.Errorf("%s", tr.T(i18n.MsgConfigInvalidInt, invalid.Raw, invalid.Key))
		case config.ReasonNotPositive:
			return fmt.Errorf("%s", tr.T(i18n.MsgConfigNotPositive, invalid.Raw, invalid.Key))
		case config.ReasonNotNonNegative:
			return fmt.Errorf("%s", tr.T(i18n.MsgConfigNotNonNegative, invalid.Raw, invalid.Key))
		}
	}

	return err
}

func newConfigListCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every config key, its effective value, and its source",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			loader, err := newLoader()
			if err != nil {
				return err
			}
			if err := ensureLoaded(loader, cmd.ErrOrStderr()); err != nil {
				return err
			}
			cfg, _, err := loader.Load()
			if err != nil {
				return err
			}
			tr := newTranslator(*cfg)

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(tw, tr.T(i18n.MsgConfigListHeader)); err != nil {
				return err
			}
			for _, key := range config.Keys() {
				value, err := loader.Get(key)
				if err != nil {
					return err
				}
				source, err := loader.Source(key)
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", key, formatValue(value), source); err != nil {
					return err
				}
			}
			return tw.Flush()
		},
	}
}

func newConfigEditCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open the config file in $EDITOR",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			loader, err := newLoader()
			if err != nil {
				return err
			}
			if err := ensureLoaded(loader, cmd.ErrOrStderr()); err != nil {
				return err
			}

			editor := resolveEditor()
			editCmd := exec.CommandContext(cmd.Context(), editor, loader.Path()) // #nosec G204 -- editor is EDITOR/vi/notepad, not attacker-controlled
			editCmd.Stdin = cmd.InOrStdin()
			editCmd.Stdout = cmd.OutOrStdout()
			editCmd.Stderr = cmd.ErrOrStderr()
			if err := editCmd.Run(); err != nil {
				return fmt.Errorf("run editor %q on %s: %w", editor, loader.Path(), err)
			}
			return nil
		},
	}
}

func newConfigPathCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			loader, err := newLoader()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), loader.Path())
			return err
		},
	}
}

// ensureLoaded loads loader's effective config (creating the file with
// defaults on first run) and prints the one-line first-run notice to w
// (the command's stderr, so `x=$(comrade config get key)`-style capture
// of stdout never has to filter it out) when this call is what created
// the file. It discards the loaded *config.Config: every caller here
// re-reads individual keys afterward through loader.Get/Source, so the
// struct itself isn't needed.
func ensureLoaded(loader *config.Loader, w io.Writer) error {
	cfg, created, err := loader.Load()
	if err != nil {
		return err
	}
	if created {
		tr := newTranslator(*cfg)
		if _, err := fmt.Fprint(w, tr.T(i18n.MsgFirstRunNotice, loader.Path())); err != nil {
			return err
		}
	}
	return nil
}

// resolveEditor picks the editor `comrade config edit` should launch:
// $EDITOR if set, otherwise a platform-appropriate fallback.
func resolveEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	return "vi"
}

// formatValue renders a value returned by config.Loader.Get for display:
// string slices as a comma-joined list (matching the comma-separated
// syntax `config set` accepts for the same keys), everything else via
// fmt's default formatting.
//
// config.Loader.Get returns whatever viper decoded a key as. A slice
// value freshly parsed by config.Validate (e.g. what `set` just wrote)
// comes back as []string, but a slice value read back out of the merged
// TOML config (defaults merged with the on-disk file) comes back as
// []interface{} (aka []any) — viper/go-toml never rebuilds it into a
// concrete []string. Both cases must render the same way, or `set` and
// the following `get`/`list` would show different formats for the same
// key (e.g. "a,b" right after `set`, but "[a b]" on the next `get`).
func formatValue(value any) string {
	switch v := value.(type) {
	case []string:
		return strings.Join(v, ",")
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = fmt.Sprint(item)
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprint(value)
	}
}
