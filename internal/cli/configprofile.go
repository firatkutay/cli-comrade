package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/tui"
)

// newConfigProfileCmd builds the "comrade config profile" command tree:
// list, show, use, add, remove, set. Mirrors newConfigCmd/newAuthCmd's own
// parent-command pattern (translatedUnknownSubcommand + a RunE that just
// prints help) exactly.
func newConfigProfileCmd(newLoader loaderFactory) *cobra.Command {
	root := &cobra.Command{
		Use:   "profile",
		Short: "Manage named config profiles",
		Args:  translatedUnknownSubcommand(newLoader),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.AddCommand(
		newConfigProfileListCmd(newLoader),
		newConfigProfileShowCmd(newLoader),
		newConfigProfileUseCmd(newLoader),
		newConfigProfileAddCmd(newLoader),
		newConfigProfileRemoveCmd(newLoader),
		newConfigProfileSetCmd(newLoader),
	)

	return root
}

// profileUsageError builds the shared MsgConfigProfileUsageError,
// translated, for a wrong-arity `config profile <subcommand>` invocation
// — see that MessageID's own doc comment (i18n/catalog.go) for why this
// is one shared, parameterized message rather than one dedicated
// MessageID per subcommand.
func profileUsageError(cmd *cobra.Command, newLoader loaderFactory, argHint string) error {
	return fmt.Errorf("%s", bestEffortTranslator(cmd, newLoader).T(i18n.MsgConfigProfileUsageError, cmd.CommandPath(), argHint))
}

// translateProfileError re-renders a config.ProfileNotFoundError/
// ProfileExistsError/InvalidProfileNameError/ProfileKeyNotAllowedError
// (or a config.UnknownKeyError/InvalidValueError from the underlying
// config.Validate/ValidateProfileKey call) through tr, via errors.As —
// the same errors.As-at-the-CLI-boundary pattern translateConfigError
// (config.go) already established. Any other error is returned unchanged.
func translateProfileError(tr i18n.Translator, err error) error {
	var notFound *config.ProfileNotFoundError
	if errors.As(err, &notFound) {
		return fmt.Errorf("%s", tr.T(i18n.MsgConfigProfileNotFound, notFound.Name))
	}
	var exists *config.ProfileExistsError
	if errors.As(err, &exists) {
		return fmt.Errorf("%s", tr.T(i18n.MsgConfigProfileAlreadyExists, exists.Name))
	}
	var invalidName *config.InvalidProfileNameError
	if errors.As(err, &invalidName) {
		return fmt.Errorf("%s", tr.T(i18n.MsgConfigProfileInvalidName, invalidName.Name))
	}
	var keyNotAllowed *config.ProfileKeyNotAllowedError
	if errors.As(err, &keyNotAllowed) {
		return fmt.Errorf("%s", tr.T(i18n.MsgConfigProfileKeyNotAllowed, keyNotAllowed.Key))
	}
	return translateConfigError(tr, err)
}

// printProfileSafetyOverrideWarning prints P-5's mandatory HIGHLIGHTED
// warning (MsgConfigProfileSafetyOverrideWarning, rendered through
// tui.PrintWarning exactly like the --yolo warning) whenever profile
// overrides any safety.* key — shared by `profile use` and `profile
// show`, the two commands P-5 requires it for. A no-op when
// config.ProfileSafetyOverrides(profile) is empty.
func printProfileSafetyOverrideWarning(cmd *cobra.Command, cfg config.Config, tr i18n.Translator, name string, profile map[string]any) error {
	overrides := config.ProfileSafetyOverrides(profile)
	if len(overrides) == 0 {
		return nil
	}
	msg := tr.T(i18n.MsgConfigProfileSafetyOverrideWarning, name, strings.Join(overrides, ", "))
	return tui.PrintWarning(cmd.ErrOrStderr(), msg, resolveColorEnabled(cfg, os.Environ(), cmd.ErrOrStderr()))
}

func newConfigProfileListCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:               "list",
		Short:             "List every defined config profile",
		Args:              translatedNoArgs(newLoader),
		ValidArgsFunction: cobra.NoFileCompletions,
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

			names := make([]string, 0, len(cfg.Profiles))
			for name := range cfg.Profiles {
				names = append(names, name)
			}
			sort.Strings(names)

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(tw, tr.T(i18n.MsgConfigProfileListHeader)); err != nil {
				return err
			}
			for _, name := range names {
				marker := ""
				if name == cfg.General.Profile {
					marker = "*"
				}
				count := len(config.ProfileKeys(cfg.Profiles[name]))
				if _, err := fmt.Fprintf(tw, "%s\t%s\t%d\n", name, marker, count); err != nil {
					return err
				}
			}
			return tw.Flush()
		},
	}
}

func newConfigProfileShowCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "show [<name>]",
		Short: "Print a profile's own key/value overrides (defaults to the active profile)",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) <= 1 {
				return nil
			}
			return profileUsageError(cmd, newLoader, "[<name>]")
		},
		ValidArgsFunction: cobra.NoFileCompletions,
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

			name := cfg.General.Profile
			if len(args) == 1 {
				name = args[0]
			}
			profile, ok := cfg.Profiles[name]
			if !ok {
				return translateProfileError(tr, &config.ProfileNotFoundError{Name: name})
			}

			heading := i18n.MsgConfigProfileShowInactive
			if name == cfg.General.Profile {
				heading = i18n.MsgConfigProfileShowActive
			}
			if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(heading, name)); err != nil {
				return err
			}
			for _, key := range config.ProfileKeys(profile) {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s = %s\n", key, formatProfileValue(profile, key)); err != nil {
					return err
				}
			}

			return printProfileSafetyOverrideWarning(cmd, *cfg, tr, name, profile)
		},
	}
}

func newConfigProfileUseCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Activate a defined config profile",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return nil
			}
			return profileUsageError(cmd, newLoader, "<name>")
		},
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
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

			profile, ok := cfg.Profiles[name]
			if !ok {
				return translateProfileError(tr, &config.ProfileNotFoundError{Name: name})
			}

			if err := loader.SetAndSave("general.profile", name); err != nil {
				return err
			}

			if err := printProfileSafetyOverrideWarning(cmd, *cfg, tr, name, profile); err != nil {
				return err
			}

			_, err = fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgConfigProfileActivated, name))
			return err
		},
	}
}

func newConfigProfileAddCmd(newLoader loaderFactory) *cobra.Command {
	var fromCurrent bool
	cmd := &cobra.Command{
		Use:   "add <name> [--from-current]",
		Short: "Create a new, empty config profile",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return nil
			}
			return profileUsageError(cmd, newLoader, "<name> [--from-current]")
		},
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
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

			var seed map[string]any
			if fromCurrent {
				seed = currentLLMSectionSeed(*cfg)
			}

			if err := loader.CreateProfile(name, seed); err != nil {
				return translateProfileError(tr, err)
			}

			_, err = fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgConfigProfileAdded, name))
			return err
		},
	}
	cmd.Flags().BoolVar(&fromCurrent, "from-current", false, enUsageDefault(i18n.MsgFlagProfileFromCurrent))
	return cmd
}

// currentLLMSectionSeed snapshots cfg's current file-level [llm] section
// into a flat "dotted key" -> value seed map, exactly the shape
// Loader.CreateProfile's seed parameter expects — `comrade config profile
// add --from-current`'s "copy current file-level [llm] values" behavior.
func currentLLMSectionSeed(cfg config.Config) map[string]any {
	return map[string]any{
		"llm.provider":               cfg.LLM.Provider,
		"llm.model":                  cfg.LLM.Model,
		"llm.fallback":               cfg.LLM.Fallback,
		"llm.timeout_seconds":        cfg.LLM.TimeoutSeconds,
		"llm.idle_timeout_seconds":   cfg.LLM.IdleTimeoutSeconds,
		"llm.max_tokens":             cfg.LLM.MaxTokens,
		"llm.openai_compat.base_url": cfg.LLM.OpenAICompat.BaseURL,
		"llm.ollama.base_url":        cfg.LLM.Ollama.BaseURL,
	}
}

func newConfigProfileRemoveCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a config profile",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return nil
			}
			return profileUsageError(cmd, newLoader, "<name>")
		},
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
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

			if err := loader.RemoveProfile(name); err != nil {
				return translateProfileError(tr, err)
			}

			_, err = fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgConfigProfileRemoved, name))
			return err
		},
	}
}

func newConfigProfileSetCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "set <name> <key> <value>",
		Short: "Validate and persist a key's value inside a config profile",
		// Mirrors newConfigSetCmd's own DisableFlagParsing rationale
		// (config.go): a profile's value can legitimately start with "-",
		// and this subcommand takes exactly three fixed positional
		// arguments and defines no flags of its own.
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
				return cmd.Help()
			}
			if len(args) != 3 {
				return profileUsageError(cmd, newLoader, "<name> <key> <value>")
			}
			name, key, raw := args[0], args[1], args[2]

			parsed, err := config.ValidateProfileKey(key, raw)
			if err != nil {
				// Deliberately envOnlyTranslator, not loader.Load(): the
				// same "reject before any filesystem side effect" reasoning
				// newConfigSetCmd's own RunE documents for the top-level
				// `config set`.
				return translateProfileError(envOnlyTranslator(), err)
			}

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

			if err := loader.SetProfileKey(name, key, parsed); err != nil {
				return translateProfileError(tr, err)
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s.%s = %s\n", name, key, formatValue(parsed))
			return err
		},
	}
}

// formatProfileValue renders profile's own value at key (a dotted path
// relative to profile's root) for `comrade config profile show`, reusing
// formatValue (config.go) for the actual formatting once the raw value
// is looked up by walking profile's nesting.
func formatProfileValue(profile map[string]any, key string) string {
	parts := strings.Split(key, ".")
	var cur any = profile
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[part]
	}
	return formatValue(cur)
}
