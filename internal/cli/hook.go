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
//
// RunE is set (rather than left nil) purely so `comrade hook --help`
// (and a bare `comrade hook`) render a sane, non-empty "Usage:" line
// (QA D5): cobra's default usage template only ever prints "Usage:\n
// {{.UseLine}}" when the command is Runnable (c.Run/c.RunE set) OR
// "Usage:\n  {{.CommandPath}} [command]" when it HasAvailableSubCommands
// — and hook's only child, "hook record", is ALSO Hidden (see
// newHookRecordCmd — deliberately, TestHookRecordIsHiddenFromHelp pins
// that), so neither branch fired and both lines rendered blank. Un-
// hiding "hook record" instead was considered and rejected: it is
// invoked only by generated shell snippets, never meant to be
// discoverable even by someone deliberately probing "comrade hook
// --help". This RunE is never reached in real shell-hook usage (every
// snippet always calls "comrade hook record ...", never bare "comrade
// hook") — it exists solely to make Runnable() true; its body is
// identical to what cobra already does automatically for a bare
// invocation of a command WITH visible subcommands (print help, exit 0).
func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "hook",
		Short:  "Internal hooks invoked by shell integration (not for direct use)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
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
