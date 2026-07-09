package cli

import (
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

// newUpgradeCmd builds the "comrade upgrade" command (UYGULAMA_PLANI.md
// FAZ 10 item 3): check GitHub Releases for a newer published version
// than this binary's own build-time version and, unless --check is
// given, download, checksum-verify, and install it in place.
func newUpgradeCmd(newLoader loaderFactory, deps upgradeDeps) *cobra.Command {
	var checkOnly bool

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Check for or install a newer released version of comrade",
		Args:  cobra.NoArgs,
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
					return fmt.Errorf("upgrade --check: %w", err)
				}
				return printUpgradeCheckResult(cmd, tr, result)
			}

			result, binary, err := u.Apply(cmd.Context(), deps.version)
			if err != nil {
				return fmt.Errorf("upgrade: %w", err)
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
