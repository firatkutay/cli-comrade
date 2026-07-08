package cli

import "github.com/spf13/cobra"

// newFixCmd stubs the error-diagnosis flow. Real behavior lands in FAZ 7.
func newFixCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fix",
		Short: "Diagnose and fix the last failed command",
		RunE: func(cmd *cobra.Command, _ []string) error {
			printNotReady(cmd.OutOrStdout(), "fix")
			return nil
		},
	}
}

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

// newInitCmd stubs shell integration setup. Real behavior lands in FAZ 4.
func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [bash|zsh|fish|powershell]",
		Short: "Install shell integration hooks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			printNotReady(cmd.OutOrStdout(), "init")
			return nil
		},
	}
}

// newHistoryCmd stubs the audit-log viewer. Real behavior lands in FAZ 6.
func newHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "Show recently executed commands",
		RunE: func(cmd *cobra.Command, _ []string) error {
			printNotReady(cmd.OutOrStdout(), "history")
			return nil
		},
	}
}
