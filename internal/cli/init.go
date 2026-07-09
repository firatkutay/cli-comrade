package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/context"
	"github.com/firatkutay/cli-comrade/internal/shellinit"
)

// initDeps bundles every OS-environment dependency "comrade init" needs,
// so tests can exercise every branch — including the windows-only
// PowerShell one — regardless of the OS the test binary actually runs
// on. newInitCmd(defaultInitDeps()) wires the real OS in NewRootCmd;
// tests construct their own initDeps directly.
type initDeps struct {
	goos     string
	getenv   func(string) string
	lookPath func(string) (string, error)
	run      shellinit.CommandRunner
}

// defaultInitDeps wires initDeps to the real operating system this
// process is running under.
func defaultInitDeps() initDeps {
	return initDeps{
		goos:     runtime.GOOS,
		getenv:   os.Getenv,
		lookPath: exec.LookPath,
		run:      context.RunCommand,
	}
}

// newInitCmd builds the "comrade init" command: installing, printing,
// or removing the shell-integration block documented in
// internal/shellinit.
func newInitCmd(deps initDeps) *cobra.Command {
	var (
		printOnly bool
		remove    bool
		assumeYes bool
	)

	cmd := &cobra.Command{
		Use:   "init [bash|zsh|fish|powershell]",
		Short: "Install shell integration hooks",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if printOnly && remove {
				return errors.New("init: --print and --remove are mutually exclusive")
			}

			shellName, err := resolveShellArg(args, deps)
			if err != nil {
				return err
			}
			shell, err := shellinit.ParseShell(shellName)
			if err != nil {
				return err
			}

			if printOnly {
				block, err := shellinit.Block(shell)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), block)
				return err
			}

			path, ok, note := shellinit.RCPath(cmd.Context(), shell, deps.goos, deps.getenv, deps.lookPath, deps.run)
			if remove {
				return runInitRemove(cmd, path, ok, note)
			}
			return runInitInstall(cmd, shell, path, ok, note, assumeYes)
		},
	}

	cmd.Flags().BoolVar(&printOnly, "print", false, "Print the shell snippet only; make no file changes")
	cmd.Flags().BoolVar(&remove, "remove", false, "Remove the cli-comrade block from the shell rc/profile file")
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "Assume yes and skip the confirmation prompt")
	return cmd
}

// resolveShellArg returns the shell name to act on: args[0] if given,
// otherwise the current shell as detected from the environment. An
// empty detection result, or a detected shell comrade init does not
// support (e.g. "cmd" on Windows), is an error asking the user to name
// one explicitly rather than guessing.
func resolveShellArg(args []string, deps initDeps) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	detected := context.DetectShell(deps.goos, deps.getenv)
	if detected == "" {
		return "", errors.New("init: could not detect your shell; run e.g. \"comrade init bash\" explicitly")
	}
	if _, err := shellinit.ParseShell(detected); err != nil {
		return "", fmt.Errorf("init: detected shell %q is not supported; run \"comrade init bash|zsh|fish|powershell\" explicitly", detected)
	}
	return detected, nil
}

// runInitInstall implements comrade init's default (non-print,
// non-remove) path: show the block that would be added and the target
// rc file, then — unless assumeYes short-circuits it — ask for a y/N
// confirmation on stdin before writing.
func runInitInstall(cmd *cobra.Command, shell shellinit.Shell, path string, ok bool, note string, assumeYes bool) error {
	block, err := shellinit.Block(shell)
	if err != nil {
		return err
	}

	if !ok {
		_, err := fmt.Fprintf(cmd.OutOrStdout(),
			"%s\n\nCould not automatically locate a profile file to edit (%s).\nAdd the block above to your PowerShell profile manually.\n",
			block, note)
		return err
	}

	existing, err := readFileOrEmpty(path)
	if err != nil {
		return err
	}

	updated, status, err := shellinit.ApplyBlock(existing, shell)
	if err != nil {
		return err
	}

	if status == shellinit.StatusAlreadyInstalled {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "cli-comrade shell integration is already installed in %s\n", path)
		return err
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "The following will be added to %s:\n\n%s\n\n", path, block); err != nil {
		return err
	}

	if !assumeYes {
		confirmed, err := confirmYesNo(cmd, fmt.Sprintf("Add cli-comrade shell integration to %s? [y/N] ", path))
		if err != nil {
			return err
		}
		if !confirmed {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "Aborted; no changes made.")
			return err
		}
	}

	if err := writeFileContent(path, updated); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Installed cli-comrade shell integration in %s\n", path)
	return err
}

// runInitRemove implements comrade init --remove.
func runInitRemove(cmd *cobra.Command, path string, ok bool, note string) error {
	if !ok {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Nothing to remove: could not locate a profile file (%s).\n", note)
		return err
	}

	existing, err := readFileOrEmpty(path)
	if err != nil {
		return err
	}

	updated, removed := shellinit.RemoveBlock(existing)
	if !removed {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "cli-comrade shell integration is not installed in %s; nothing to do.\n", path)
		return err
	}

	if err := writeFileContent(path, updated); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Removed cli-comrade shell integration from %s\n", path)
	return err
}

// confirmYesNo prints prompt to cmd's stdout and reads a single line
// from cmd's stdin, returning true only for a (case-insensitive) "y" or
// "yes" response.
func confirmYesNo(cmd *cobra.Command, prompt string) (bool, error) {
	if _, err := fmt.Fprint(cmd.OutOrStdout(), prompt); err != nil {
		return false, err
	}
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("init: read confirmation: %w", err)
	}
	line = strings.TrimSpace(line)
	return strings.EqualFold(line, "y") || strings.EqualFold(line, "yes"), nil
}

// readFileOrEmpty reads path's content, treating a missing file as
// empty content rather than an error — an rc file that does not exist
// yet is the common case for a first-time comrade init.
func readFileOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is a well-known shell rc/profile location, not attacker-controlled input
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("init: read %s: %w", path, err)
	}
	return string(data), nil
}

// writeFileContent writes content to path, creating path's parent
// directory first if needed (e.g. fish's ~/.config/fish/ on a machine
// that has never run fish).
func writeFileContent(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("init: create directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { // #nosec G306 -- an rc/profile file is meant to be user-readable, matching its pre-existing permissions convention
		return fmt.Errorf("init: write %s: %w", path, err)
	}
	return nil
}
