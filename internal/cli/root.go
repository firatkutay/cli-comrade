package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// NewRootCmd builds the "comrade" root command. version is injected at
// build time via -ldflags; it defaults to "dev" for local, non-release
// builds. Running "comrade" with no arguments prints the version followed
// by the standard cobra help output.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:     "comrade",
		Short:   "comrade is a cross-platform AI CLI companion for the terminal",
		Version: version,
		// cmd/comrade/main.go already prints Execute()'s returned error
		// exactly once to stderr; without these two, cobra would ALSO
		// print its own "Error: ..." line and a full Usage: block for
		// every subcommand error, tripling up on a single failure.
		// Cobra checks these flags on whichever command actually ran
		// OR on root — set here, they cover every subcommand in this
		// tree, including ones added below, with no per-subcommand
		// opt-in required.
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "comrade version %s\n\n", cmd.Version); err != nil {
				return err
			}
			return cmd.Help()
		},
	}
	root.SetVersionTemplate("comrade version {{.Version}}\n")

	// newLoader is resolved lazily, once per subcommand invocation, rather
	// than once here: the config file path depends on environment
	// variables (XDG_CONFIG_HOME, APPDATA) that tests set per-case, and
	// resolving eagerly would bake in whatever the environment looked
	// like at process startup instead of at command-execution time. This
	// is the "Loader constructed ... passed to subcommands" dependency
	// injection CLAUDE.md calls for: no package-level viper/config state
	// anywhere in this tree.
	newLoader := func() (*config.Loader, error) {
		return config.NewLoader("")
	}

	root.AddCommand(
		newFixCmd(),
		newExplainCmd(),
		newChatCmd(),
		newConfigCmd(newLoader),
		newInitCmd(defaultInitDeps()),
		newHistoryCmd(),
		newHookCmd(),
		newDoCmd(newLoader),
	)

	return root
}
