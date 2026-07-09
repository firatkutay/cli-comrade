package cli

import "github.com/spf13/cobra"

// newFixCmd lands in fix.go as of FAZ 7 (real error-diagnosis flow).

// newExplainCmd stubs the command-explanation flow. Real behavior lands in
// FAZ 9.
func newExplainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explain [command]",
		Short: "Explain what a command does, without running it",
		RunE: func(cmd *cobra.Command, _ []string) error {
			printNotReady(cmd.OutOrStdout(), "explain")
			return nil
		},
	}
}

// newChatCmd stubs the interactive chat session. Real behavior lands in
// FAZ 9.
func newChatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "chat",
		Short: "Start an interactive, context-preserving chat session",
		RunE: func(cmd *cobra.Command, _ []string) error {
			printNotReady(cmd.OutOrStdout(), "chat")
			return nil
		},
	}
}
