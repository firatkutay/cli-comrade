package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// loaderFactory builds a fresh *config.Loader for a single command
// invocation. It is a func rather than a shared *config.Loader value so
// each subcommand resolves the config path (which depends on
// process-environment state) at the moment it actually runs.
type loaderFactory func() (*config.Loader, error)

// firstRunNoticeFormat is the single hardcoded English line printed the
// first time cli-comrade creates a config file for the user. FAZ 9 moves
// this into the i18n catalog like every other user-facing string.
const firstRunNoticeFormat = "Created default config at %s\n"

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
	)

	return root
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

			value, err := loader.Get(args[0])
			if err != nil {
				return err
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
		Args:  cobra.ExactArgs(2),
		// Config values can legitimately start with "-" (e.g. a bug in a
		// negative-int value that Validate should reject with a clear
		// message, not one pflag silently reinterprets as an unknown
		// shorthand flag). "set" takes exactly two fixed positional
		// arguments and defines no flags of its own, so disabling flag
		// parsing here is safe and lets any raw value through untouched.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, raw := args[0], args[1]

			parsed, err := config.Validate(key, raw)
			if err != nil {
				return err
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

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(tw, "KEY\tVALUE\tSOURCE"); err != nil {
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
	_, created, err := loader.Load()
	if err != nil {
		return err
	}
	if created {
		if _, err := fmt.Fprintf(w, firstRunNoticeFormat, loader.Path()); err != nil {
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
