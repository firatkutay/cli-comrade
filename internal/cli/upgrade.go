package cli

import (
	"errors"
	"fmt"
	"os"
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

			if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgUpgradeDownloading, result.LatestVersion)); err != nil {
				return err
			}

			exePath, err := deps.executable()
			if err != nil {
				return fmt.Errorf("upgrade: resolve running executable path: %w", err)
			}
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
// Any error that is NEITHER of those two (e.g. a later Apply-specific
// failure — no matching release asset for this platform, a checksum
// mismatch, a failed binary replace, or resultFor's version-string parse
// error) falls through unchanged, wrapped with prefix exactly like
// before this fix — those already carry their own reasonably specific
// detail and are unrelated to D3's fetch-step bug.
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
