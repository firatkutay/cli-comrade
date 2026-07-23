package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/update"
)

// upgradeDeps bundles every OS/network touchpoint `comrade upgrade`
// needs, exactly like initDeps (init.go) bundles init's own — newLoader
// wires the real ones in NewRootCmd via defaultUpgradeDeps; tests
// construct their own upgradeDeps directly, injecting fakes for
// fetcher/downloader/executable/replace so no test ever reaches the real
// network or touches a real running executable.
type upgradeDeps struct {
	version    string
	goos       string
	goarch     string
	fetcher    update.ReleaseFetcher
	downloader update.AssetDownloader
	executable func() (string, error)
	replace    func(targetPath string, content []byte, goos string) error
}

// defaultUpgradeDeps wires upgradeDeps to the real operating system and
// network this process is actually running under, for build-time version
// string.
func defaultUpgradeDeps(version string) upgradeDeps {
	return upgradeDeps{
		version:    version,
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
		fetcher:    &update.GitHubClient{},
		downloader: update.HTTPDownloader{},
		executable: os.Executable,
		replace:    update.ReplaceBinary,
	}
}

// newUpgradeCmd builds the "comrade upgrade" command (docs/history/UYGULAMA_PLANI.md
// FAZ 10 item 3): check GitHub Releases for a newer published version
// than this binary's own build-time version and, unless --check is
// given, download, checksum-verify, and install it in place.
func newUpgradeCmd(newLoader loaderFactory, deps upgradeDeps) *cobra.Command {
	var checkOnly bool

	cmd := &cobra.Command{
		Use:               "upgrade",
		Short:             "Check for or install a newer released version of comrade",
		Args:              translatedNoArgs(newLoader),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, tr, err := loadConfigWithNotice(cmd, newLoader)
			if err != nil {
				return err
			}

			// A leftover comrade.exe.old from a prior Windows self-update
			// is cleaned up opportunistically here too (in addition to
			// the passive per-command hook in updatenotice.go) — running
			// `comrade upgrade` again is exactly the moment a user is
			// most likely to be doing OS-level housekeeping around this
			// binary.
			if exePath, exeErr := deps.executable(); exeErr == nil {
				update.CleanupOldBinary(exePath)
			}

			if update.IsDevBuild(deps.version) {
				return fmt.Errorf("%s", tr.T(i18n.MsgUpgradeDevBuildError))
			}

			u := &update.Updater{
				Fetcher:    deps.fetcher,
				Downloader: deps.downloader,
				GOOS:       deps.goos,
				GOARCH:     deps.goarch,
			}

			if checkOnly {
				result, err := u.Check(cmd.Context(), deps.version)
				if err != nil {
					return translateUpgradeFetchError(cmd, tr, "upgrade --check", err)
				}
				return printUpgradeCheckResult(cmd, tr, result)
			}

			result, binary, err := u.Apply(cmd.Context(), deps.version)
			if err != nil {
				return translateUpgradeFetchError(cmd, tr, "upgrade", err)
			}
			if !result.UpdateAvailable {
				_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgUpgradeUpToDate, result.CurrentVersion))
				return err
			}

			// update.Updater never prints anything itself (it has no
			// i18n.Translator) — it only reports the signature-gate
			// outcome (MEDIUM#4) via result.SignatureStatus, so this is
			// the one place that renders it for the user.
			if result.SignatureStatus == update.SignatureStatusNotConfigured {
				if _, err := fmt.Fprint(cmd.ErrOrStderr(), tr.T(i18n.MsgUpgradeSignatureNotConfigured)); err != nil {
					return err
				}
			}

			if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgUpgradeDownloading, result.LatestVersion)); err != nil {
				return err
			}

			exePath, err := deps.executable()
			if err != nil {
				return fmt.Errorf("upgrade: resolve running executable path: %w", err)
			}
			exePath = resolveRealExecutablePath(cmd, tr, exePath)
			if err := deps.replace(exePath, binary, deps.goos); err != nil {
				return fmt.Errorf("upgrade: %w", err)
			}

			_, err = fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgUpgradeInstalled, result.LatestVersion))
			return err
		},
	}

	cmd.Flags().BoolVar(&checkOnly, "check", false, enUsageDefault(i18n.MsgFlagCheck))
	return cmd
}

// resolveRealExecutablePath resolves exePath through any symlinks
// (LOW#8): package managers such as Homebrew install the actual binary
// under a versioned cellar path and symlink it into PATH, so
// os.Executable() can return the symlink's path rather than the real
// file. deps.replace/update.ReplaceBinary renames/overwrites whatever
// path it's given — pointed at a symlink, that would either clobber the
// symlink itself with a plain file or leave it dangling, either way
// breaking the install. Resolving first makes ReplaceBinary operate on
// the real backing file instead, exactly like a manual reinstall would.
//
// On EvalSymlinks failure (a dangling symlink, or exePath just not
// existing as a real filesystem entry) this warns to cmd's stderr and
// falls back to the original, unresolved path rather than aborting the
// upgrade — the previous, symlink-naive behavior.
func resolveRealExecutablePath(cmd *cobra.Command, tr i18n.Translator, exePath string) string {
	resolvedPath, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), tr.T(i18n.MsgUpgradeSymlinkResolveWarning, exePath, err))
		return exePath
	}
	return resolvedPath
}

// translateUpgradeFetchError re-renders a failure from u.Check/u.Apply's
// very first step (fetching the latest release from GitHub) into a
// clean, i18n'd message instead of surfacing GitHub's own raw response
// detail to the user (QA D3): update.ErrReleaseNotFound (this repository
// has no published release yet — a 404, GitHub's own actual response for
// that case) gets its own dedicated message; any OTHER
// update.ErrFetchFailed (network unreachable, a non-200/non-404 status,
// a malformed response body) gets one concise, generic wrapper — the
// underlying Go error's full text (which may include a short truncated
// response-body snippet; see update/github.go) is never shown to the
// user, only written to cmd's stderr when COMRADE_DEBUG is set, matching
// hook.go's own established COMRADE_DEBUG-gated-detail convention
// elsewhere in this tree.
//
// It also renders MEDIUM#4's two signature-gate hard-failures the same
// way: update.ErrMissingSignatureAsset (signing is configured but this
// release published no checksums.txt.sig) and update.ErrSignatureInvalid
// (checksums.txt's signature didn't verify) each get their own clean,
// translated message instead of the raw internal error text.
//
// Any error that is NEITHER of those four (e.g. no matching release
// asset for this platform, a checksum mismatch, a failed binary replace,
// or resultFor's version-string parse error) falls through unchanged,
// wrapped with prefix exactly like before this fix — those already carry
// their own reasonably specific detail and are unrelated to D3's
// fetch-step bug.
func translateUpgradeFetchError(cmd *cobra.Command, tr i18n.Translator, prefix string, err error) error {
	if errors.Is(err, update.ErrReleaseNotFound) {
		return fmt.Errorf("%s", tr.T(i18n.MsgUpgradeNoReleaseFound))
	}
	if errors.Is(err, update.ErrFetchFailed) {
		if os.Getenv("COMRADE_DEBUG") != "" {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", prefix, err)
		}
		return fmt.Errorf("%s", tr.T(i18n.MsgUpgradeFetchFailed))
	}
	if errors.Is(err, update.ErrMissingSignatureAsset) {
		return fmt.Errorf("%s", tr.T(i18n.MsgUpgradeSignatureMissing))
	}
	if errors.Is(err, update.ErrSignatureInvalid) {
		return fmt.Errorf("%s", tr.T(i18n.MsgUpgradeSignatureInvalid))
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

// printUpgradeCheckResult renders `comrade upgrade --check`'s one-line
// report: either the up-to-date message or the newer-version-available
// message, matching the exact two outcomes update.Updater.Check reports.
func printUpgradeCheckResult(cmd *cobra.Command, tr i18n.Translator, result update.Result) error {
	if !result.UpdateAvailable {
		_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgUpgradeUpToDate, result.CurrentVersion))
		return err
	}
	_, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgUpgradeNewerAvailable, result.LatestVersion, result.CurrentVersion, result.ReleaseURL))
	return err
}
