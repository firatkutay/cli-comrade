package cli

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/context"
)

// newHookCmd builds the hidden "comrade hook" command group: internal
// entry points shell hooks invoke, never meant for direct interactive
// use (Hidden, so it never appears in --help output).
func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "hook",
		Short:  "Internal hooks invoked by shell integration (not for direct use)",
		Hidden: true,
	}
	cmd.AddCommand(newHookRecordCmd())
	return cmd
}

// newHookRecordCmd builds "comrade hook record": the sole writer of
// last_command.json, invoked by every internal/shellinit hook snippet
// after each prompt. See internal/shellinit's doc comment and
// docs/phases/FAZ-04.md for why shell scripts exec this instead of
// hand-assembling the JSON themselves.
func newHookRecordCmd() *cobra.Command {
	var (
		shellName string
		exitCode  int
		command   string
	)

	cmd := &cobra.Command{
		Use:    "record",
		Short:  "Record the last executed shell command (invoked by shell hooks)",
		Hidden: true,
		Args:   cobra.NoArgs,
		// RunE always returns nil: a broken write here must never break
		// the user's shell prompt (every hook snippet also independently
		// swallows this command's own failure, but the invariant holds
		// here too, in case a hook is ever invoked without that guard).
		RunE: func(cmd *cobra.Command, _ []string) error {
			recordLastCommand(cmd, shellName, exitCode, command)
			return nil
		},
	}

	cmd.Flags().StringVar(&shellName, "shell", "", "Name of the shell invoking this hook")
	cmd.Flags().IntVar(&exitCode, "exit", 0, "Exit code of the last command")
	cmd.Flags().StringVar(&command, "command", "", "The last command's text")
	return cmd
}

// recordLastCommand writes last_command.json for the given shell/exit
// code/command text. Any failure (path resolution or the write itself)
// is swallowed silently, and only surfaced on cmd's stderr when
// COMRADE_DEBUG is set — this hook must never make a shell prompt
// noisy or fail a user's terminal session.
func recordLastCommand(cmd *cobra.Command, shellName string, exitCode int, command string) {
	path, err := context.LastCommandPath(runtime.GOOS, os.Getenv)
	if err == nil {
		err = context.WriteLastCommand(path, context.LastCommand{
			Command:   command,
			ExitCode:  exitCode,
			Timestamp: time.Now().UTC(),
			Shell:     shellName,
		})
	}
	if err != nil && os.Getenv("COMRADE_DEBUG") != "" {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "comrade hook record: %v\n", err)
	}
}
