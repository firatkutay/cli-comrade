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
	"github.com/firatkutay/cli-comrade/internal/i18n"
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
// internal/shellinit. newLoader is only consulted for the install/remove
// paths' translated output (see runInitInstall/runInitRemove) — --print
// and every argument-resolution error return before ever touching it, so
// `comrade init --print`/a bad shell name never load or create a config
// file as a side effect (auth_test.go-style zero-config-touch fast
// rejection, matching the exact same principle applied to `comrade auth
// login`'s ollama/unknown-provider checks).
func newInitCmd(deps initDeps, newLoader loaderFactory) *cobra.Command {
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
				return fmt.Errorf("%s", envOnlyTranslator().T(i18n.MsgInitPrintRemoveExclusiveError))
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

			_, tr, err := loadConfigWithNotice(cmd, newLoader)
			if err != nil {
				return err
			}

			path, ok, note := shellinit.RCPath(cmd.Context(), shell, deps.goos, deps.getenv, deps.lookPath, deps.run)
			if remove {
				return runInitRemove(cmd, path, ok, note, tr)
			}
			return runInitInstall(cmd, shell, path, ok, note, assumeYes, tr)
		},
	}

	cmd.Flags().BoolVar(&printOnly, "print", false, enUsageDefault(i18n.MsgFlagPrint))
	cmd.Flags().BoolVar(&remove, "remove", false, enUsageDefault(i18n.MsgFlagRemove))
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, enUsageDefault(i18n.MsgFlagYes))
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
		return "", fmt.Errorf("%s", envOnlyTranslator().T(i18n.MsgInitShellUndetectedError))
	}
	if _, err := shellinit.ParseShell(detected); err != nil {
		return "", fmt.Errorf("%s", envOnlyTranslator().T(i18n.MsgInitShellUnsupportedError, detected))
	}
	return detected, nil
}

// runInitInstall implements comrade init's default (non-print,
// non-remove) path: show the block that would be added and the target
// rc file, then — unless assumeYes short-circuits it — ask for a y/N
// confirmation on stdin before writing.
func runInitInstall(cmd *cobra.Command, shell shellinit.Shell, path string, ok bool, note string, assumeYes bool, tr i18n.Translator) error {
	block, err := shellinit.Block(shell)
	if err != nil {
		return err
	}

	if !ok {
		_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitPowerShellManualFallback, block, note))
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
		_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitAlreadyInstalled, path))
		return err
	}

	if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitPreview, path, block)); err != nil {
		return err
	}

	if !assumeYes {
		confirmed, err := confirmYesNo(cmd, tr.T(i18n.MsgInitConfirmPrompt, path))
		if err != nil {
			return err
		}
		if !confirmed {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), tr.T(i18n.MsgInitAborted))
			return err
		}
	}

	if err := writeFileContent(path, updated); err != nil {
		return err
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitInstalled, path))
	return err
}

// runInitRemove implements comrade init --remove.
func runInitRemove(cmd *cobra.Command, path string, ok bool, note string, tr i18n.Translator) error {
	if !ok {
		_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitRemoveNoProfile, note))
		return err
	}

	existing, err := readFileOrEmpty(path)
	if err != nil {
		return err
	}

	updated, removed := shellinit.RemoveBlock(existing)
	if !removed {
		_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitNotInstalled, path))
		return err
	}

	if err := writeFileContent(path, updated); err != nil {
		return err
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitRemoved, path))
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
