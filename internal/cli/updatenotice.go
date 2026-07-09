package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/update"
)

// updateNoticeNetworkTimeout bounds the background GitHub check
// maybeNotifyUpdate performs when a check is due — UYGULAMA_PLANI.md FAZ
// 10 item 4's "never block or slow the command" requirement, applied as
// a short, bounded worst case (at most once per CheckInterval) rather
// than a true fire-and-forget goroutine: a goroutine that outlives
// Execute() has no guaranteed chance to print before the process exits,
// which would make the notice unreliable; a short synchronous timeout
// keeps the guarantee simple at the cost of a small, rare delay.
const updateNoticeNetworkTimeout = 3 * time.Second

// maybeNotifyUpdate is root.PersistentPostRunE's body (see NewRootCmd):
// after any subcommand finishes successfully, decide whether to print
// UYGULAMA_PLANI.md FAZ 10 item 4's "a new version is available" line.
// Every failure mode here — config load, state read/write, the network
// call itself — is silent: this is a best-effort convenience notice,
// never something that should turn a successful command into a failure
// or spam stderr with its own diagnostics.
//
// version is the build-time version (cmd/comrade/main.go's -ldflags
// -X main.version=...); a "dev" build is skipped entirely (see
// update.IsDevBuild) — there is no meaningful "newer than dev" release
// comparison. fetcher is injected by the caller (newRootCmd) rather than
// constructed here, so tests can exercise the full successful-check path
// against a fake instead of the real GitHub API.
func maybeNotifyUpdate(cmd *cobra.Command, newLoader loaderFactory, version string, fetcher update.ReleaseFetcher) {
	if update.IsDevBuild(version) {
		return
	}

	loader, err := newLoader()
	if err != nil {
		return
	}
	cfg, _, err := loader.Load()
	if err != nil {
		return
	}
	if !cfg.General.UpdateCheck {
		return
	}

	// Windows self-update leaves a comrade.exe.old lock-file leftover
	// behind (internal/update.ReplaceBinary's rename dance) that can
	// only be removed once the original process holding it open has
	// exited — "on next run" per UYGULAMA_PLANI.md FAZ 10 item 3.
	// Attempting this on every command, not only `comrade upgrade`
	// itself, is what actually satisfies "next run" for a user who
	// upgraded and later runs any other comrade command first.
	if runtime.GOOS == "windows" {
		cleanupOldBinaryBestEffort()
	}

	statePath, err := update.DefaultStatePath()
	if err != nil {
		return
	}
	st := update.ReadState(statePath)
	now := time.Now()
	if !update.ShouldCheck(now, st) {
		return
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), updateNoticeNetworkTimeout)
	defer cancel()

	rel, fetchErr := fetcher.LatestRelease(ctx)

	newState := update.CheckState{LastCheckedAt: now}
	if fetchErr == nil {
		newState.LatestKnownVersion = rel.TagName
	}
	// Persist the attempt regardless of success: an offline machine
	// must not retry (and pay updateNoticeNetworkTimeout) on every
	// single command for the whole time it stays offline — it gets
	// throttled to once per CheckInterval exactly like a successful
	// check does. Best-effort; a write failure here is silent too.
	_ = update.WriteState(statePath, newState)

	if fetchErr != nil {
		return
	}

	newer, cmpErr := update.IsNewer(version, rel.TagName)
	if cmpErr != nil || !newer {
		return
	}

	tr := newTranslator(*cfg)
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), tr.T(i18n.MsgUpdateAvailableNotice, rel.TagName, version))
}

// cleanupOldBinaryBestEffort resolves the running executable's own path
// and removes any leftover ".old" file next to it (update.CleanupOldBinary),
// swallowing any resolution error — this is disk hygiene, never something
// that should affect command behavior.
func cleanupOldBinaryBestEffort() {
	exePath, err := executableForCleanup()
	if err != nil {
		return
	}
	update.CleanupOldBinary(exePath)
}

// executableForCleanup is os.Executable, indirected through a package
// variable so tests can override it — mirrors how upgradeDeps.executable
// is injected for the same reason in upgrade.go.
var executableForCleanup = os.Executable
