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
		Use:               "init [bash|zsh|fish|powershell]",
		Short:             "Install shell integration hooks",
		Args:              translatedMaxArgs(newLoader, 1, i18n.MsgInitUsageError),
		ValidArgsFunction: completeFirstArgFromList(shellinitShellNames()),
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

			// GOOS=windows PowerShell is the one multi-profile case: it
			// targets EVERY installed PowerShell variant's own profile
			// (Windows PowerShell 5.1 AND PowerShell 7, whichever are
			// actually present) rather than RCPath's single goos-keyed
			// guess — see shellinit.ResolvePowerShellProfiles' doc
			// comment for the "pwsh gap" this closes. Every other
			// shell — including PowerShell on non-Windows, where only
			// pwsh has ever existed — keeps using RCPath's original
			// single-profile path unchanged, byte-for-byte.
			if shell == shellinit.PowerShell && deps.goos == "windows" {
				profiles, err := shellinit.ResolvePowerShellProfiles(cmd.Context(), deps.goos, deps.lookPath, deps.run)
				if err != nil {
					return fmt.Errorf("%s", tr.T(i18n.MsgInitPowerShellNoneFoundError))
				}
				if remove {
					return runInitRemovePowerShell(cmd, profiles, tr)
				}
				return runInitInstallPowerShell(cmd, profiles, assumeYes, tr)
			}

			path, ok, note := shellinit.RCPath(cmd.Context(), shell, deps.goos, deps.getenv, deps.lookPath, deps.run)
			if remove {
				return runInitRemove(cmd, shell, path, ok, note, deps.getenv, tr)
			}
			return runInitInstall(cmd, shell, path, ok, note, assumeYes, deps.getenv, tr)
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
// confirmation on stdin before writing. getenv is only consulted for
// shell == shellinit.Fish (installFishCompletionsIfApplicable's own
// resolution of FishCompletionsPath) — every other shell ignores it.
func runInitInstall(cmd *cobra.Command, shell shellinit.Shell, path string, ok bool, note string, assumeYes bool, getenv func(string) string, tr i18n.Translator) error {
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
		if err := installFishCompletionsIfApplicable(cmd, shell, getenv, tr); err != nil {
			return err
		}
		_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitAlreadyInstalled, path))
		return err
	}

	if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitPreview, path, block)); err != nil {
		return err
	}

	if !assumeYes {
		confirmed, err := confirmYesNo(cmd, tr.T(i18n.MsgInitConfirmPrompt, path), tr.Lang())
		if err != nil {
			return err
		}
		if !confirmed {
			// A decline covers the rc-file edit ONLY — completions are a
			// separate, self-contained, comrade-owned artifact, never
			// written here regardless: skipping
			// installFishCompletionsIfApplicable on this path is what
			// respects the "no" the user just gave.
			_, err := fmt.Fprintln(cmd.OutOrStdout(), tr.T(i18n.MsgInitAborted))
			return err
		}
	}

	if err := writeFileContent(path, updated); err != nil {
		return err
	}
	if err := installFishCompletionsIfApplicable(cmd, shell, getenv, tr); err != nil {
		return err
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitInstalled, path))
	return err
}

// installFishCompletionsIfApplicable writes comrade's fish completions
// script (shellinit.FishCompletionsScript) to its native lazy-load
// location (shellinit.FishCompletionsPath) whenever shell is Fish — a
// no-op for every other shell. Idempotency is a plain overwrite (no
// ApplyBlock-style merge/diff needed — see FishCompletionsScript's own
// doc comment), so this always writes the exact same fixed content and
// prints the exact same confirmation, every time it runs; there is no
// separate "already installed" branch to maintain here.
//
// Called from both of runInitInstall's own "the hook is in place" exit
// points (StatusAlreadyInstalled and a fresh successful write) — never
// from the !ok fallback (nothing was resolved to write into either
// case) or a declined confirmation (see runInitInstall's own comment on
// that path).
func installFishCompletionsIfApplicable(cmd *cobra.Command, shell shellinit.Shell, getenv func(string) string, tr i18n.Translator) error {
	if shell != shellinit.Fish {
		return nil
	}
	path, ok, _ := shellinit.FishCompletionsPath(getenv)
	if !ok {
		// FishCompletionsPath resolves from the exact same HOME/
		// XDG_CONFIG_HOME chain RCPath's own Fish branch just used to
		// reach this point at all (ok=true there is a precondition of
		// ever calling this function) — this branch is therefore
		// unreachable in practice, not a real failure mode worth its own
		// message; skip silently rather than block an already-successful
		// hook install on a completions nicety.
		return nil
	}
	if err := writeFileContent(path, shellinit.FishCompletionsScript()); err != nil {
		return err
	}
	_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitFishCompletionsInstalled, path))
	return err
}

// runInitRemove implements comrade init --remove. getenv is only
// consulted for shell == shellinit.Fish, mirroring runInitInstall.
func runInitRemove(cmd *cobra.Command, shell shellinit.Shell, path string, ok bool, note string, getenv func(string) string, tr i18n.Translator) error {
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
		if err := removeFishCompletionsIfApplicable(cmd, shell, getenv, tr); err != nil {
			return err
		}
		_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitNotInstalled, path))
		return err
	}

	if err := writeFileContent(path, updated); err != nil {
		return err
	}
	if err := removeFishCompletionsIfApplicable(cmd, shell, getenv, tr); err != nil {
		return err
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitRemoved, path))
	return err
}

// removeFishCompletionsIfApplicable deletes comrade's fish completions
// file (installFishCompletionsIfApplicable's own target path) whenever
// shell is Fish — a no-op for every other shell. Deletion is
// unconditional and idempotent regardless of whether the hook BLOCK
// itself was found (a completions file surviving a hook-block removal,
// or vice-versa, is exactly the drift this guards against) — a missing
// completions file is simply not reported (removeFileIfExists' own
// "not there" case), never an error.
func removeFishCompletionsIfApplicable(cmd *cobra.Command, shell shellinit.Shell, getenv func(string) string, tr i18n.Translator) error {
	if shell != shellinit.Fish {
		return nil
	}
	path, ok, _ := shellinit.FishCompletionsPath(getenv)
	if !ok {
		return nil
	}
	removed, err := removeFileIfExists(path)
	if err != nil {
		return err
	}
	if !removed {
		return nil
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitFishCompletionsRemoved, path))
	return err
}

// removeFileIfExists deletes path, reporting whether a file was actually
// there to delete — a missing file is not an error, mirroring
// readFileOrEmpty's own "missing is not an error" convention elsewhere
// in this file.
func removeFileIfExists(path string) (bool, error) {
	err := os.Remove(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("init: remove %s: %w", path, err)
}

// pendingPSInstall is one PowerShell profile runInitInstallPowerShell has
// already decided needs a write (ApplyBlock returned StatusInstalled or
// StatusUpgraded) — collected during the preview pass so the single
// combined confirmation only has to be asked once, then applied to every
// pending profile in a second pass.
type pendingPSInstall struct {
	profile shellinit.PSProfile
	updated string
}

// runInitInstallPowerShell implements comrade init powershell's
// GOOS=windows multi-variant install path: every profile in profiles
// (one per installed PowerShell variant — see shellinit.
// ResolvePowerShellProfiles) is evaluated independently via the same
// ApplyBlock idempotency machinery runInitInstall uses for a single
// profile, but the y/N confirmation is asked only ONCE, covering every
// profile that actually needs a write — not once per profile — so a
// two-variant machine is not prompted twice for what is, from the
// user's perspective, a single "comrade init powershell" invocation.
//
// A profile whose OK is false (its variant's binary was found but its
// own $PROFILE could not be resolved) is reported and skipped — it never
// blocks the other profile(s) from being processed normally.
func runInitInstallPowerShell(cmd *cobra.Command, profiles []shellinit.PSProfile, assumeYes bool, tr i18n.Translator) error {
	block, err := shellinit.Block(shellinit.PowerShell)
	if err != nil {
		return err
	}

	var pending []pendingPSInstall
	for _, p := range profiles {
		if !p.OK {
			if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitPSVariantUnresolved, p.Variant.Label(), p.Note)); err != nil {
				return err
			}
			continue
		}

		existing, err := readFileOrEmpty(p.Path)
		if err != nil {
			return err
		}

		updated, status, err := shellinit.ApplyBlock(existing, shellinit.PowerShell)
		if err != nil {
			return err
		}

		if status == shellinit.StatusAlreadyInstalled {
			if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitPSVariantAlreadyInstalled, p.Variant.Label(), p.Path)); err != nil {
				return err
			}
			continue
		}

		if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitPreview, p.Path, block)); err != nil {
			return err
		}
		pending = append(pending, pendingPSInstall{profile: p, updated: updated})
	}

	if len(pending) == 0 {
		return nil
	}

	if !assumeYes {
		confirmed, err := confirmYesNo(cmd, tr.T(i18n.MsgInitConfirmPromptMulti), tr.Lang())
		if err != nil {
			return err
		}
		if !confirmed {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), tr.T(i18n.MsgInitAborted))
			return err
		}
	}

	for _, pi := range pending {
		if err := writeFileContent(pi.profile.Path, pi.updated); err != nil {
			return err
		}
		if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitPSVariantInstalled, pi.profile.Variant.Label(), pi.profile.Path)); err != nil {
			return err
		}
	}
	return nil
}

// runInitRemovePowerShell implements comrade init powershell --remove's
// GOOS=windows multi-variant path: every profile in profiles is
// processed independently via RemoveBlock, with no confirmation prompt —
// matching runInitRemove's single-profile behavior exactly, just applied
// once per installed variant instead of once for a single goos-guessed
// binary.
func runInitRemovePowerShell(cmd *cobra.Command, profiles []shellinit.PSProfile, tr i18n.Translator) error {
	for _, p := range profiles {
		if !p.OK {
			if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitPSVariantUnresolved, p.Variant.Label(), p.Note)); err != nil {
				return err
			}
			continue
		}

		existing, err := readFileOrEmpty(p.Path)
		if err != nil {
			return err
		}

		updated, removed := shellinit.RemoveBlock(existing)
		if !removed {
			if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitPSVariantNotInstalled, p.Variant.Label(), p.Path)); err != nil {
				return err
			}
			continue
		}

		if err := writeFileContent(p.Path, updated); err != nil {
			return err
		}
		if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgInitPSVariantRemoved, p.Variant.Label(), p.Path)); err != nil {
			return err
		}
	}
	return nil
}

// confirmYesNo prints prompt to cmd's stdout and reads a single line from
// cmd's stdin, returning true only for an affirmative response in lang
// (isAffirmative) — the rendered prompt itself already tells the user which
// letter to type (e.g. TR's MsgInitConfirmPrompt renders "[e/H]", EN's
// renders "[y/N]"), so acceptance must be driven by the SAME lang or a
// TR user typing the very letter the prompt showed them would be silently
// rejected. Anything else, including an empty line (bare Enter), stays
// default-NO — same fail-closed default confirmYesNo always had.
func confirmYesNo(cmd *cobra.Command, prompt string, lang i18n.Lang) (bool, error) {
	if _, err := fmt.Fprint(cmd.OutOrStdout(), prompt); err != nil {
		return false, err
	}
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("init: read confirmation: %w", err)
	}
	return isAffirmative(lang, strings.TrimSpace(line)), nil
}

// isAffirmative reports whether line is lang's affirmative confirmation
// answer: TR accepts (case-insensitive) "e"/"evet", any other language
// (including LangEN) accepts "y"/"yes" — mirrors internal/tui/confirm.go's
// mapKey per-language key-set discipline, minus mapKey's cross-language
// inversion hazard (there is no TR key here that collides with an EN
// negative, so no extra guard is needed beyond the language switch itself).
func isAffirmative(lang i18n.Lang, line string) bool {
	if lang == i18n.LangTR {
		return strings.EqualFold(line, "e") || strings.EqualFold(line, "evet")
	}
	return strings.EqualFold(line, "y") || strings.EqualFold(line, "yes")
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
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("init: create directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { // #nosec G306 -- an rc/profile file is meant to be user-readable, matching its pre-existing permissions convention
		return fmt.Errorf("init: write %s: %w", path, err)
	}
	return nil
}
