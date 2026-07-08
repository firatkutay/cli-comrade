package cli

import (
	"fmt"

	"github.com/spf13/cobra"
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "comrade version %s\n\n", cmd.Version); err != nil {
				return err
			}
			return cmd.Help()
		},
	}
	root.SetVersionTemplate("comrade version {{.Version}}\n")

	root.AddCommand(
		newFixCmd(),
		newExplainCmd(),
		newChatCmd(),
		newConfigCmd(),
		newInitCmd(),
		newHistoryCmd(),
	)

	return root
}
