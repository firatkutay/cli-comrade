package update

import (
	"context"
	"fmt"
)

// Result summarizes one Check or Apply outcome.
type Result struct {
	CurrentVersion  string
	LatestVersion   string // the release tag as published, e.g. "v0.2.0"
	ReleaseURL      string
	UpdateAvailable bool
}

// Updater bundles everything `comrade upgrade` needs to decide whether a
// newer release exists and, if so, fetch and verify it — every
// network-facing dependency is injected (ReleaseFetcher, AssetDownloader)
// so tests exercise the whole flow against fakes, never the real GitHub
// API or a real download.
type Updater struct {
	Fetcher    ReleaseFetcher
	Downloader AssetDownloader
	GOOS       string
	GOARCH     string
}

// Check resolves the latest published release and reports whether it is
// newer than currentVersion, without downloading or installing anything
// — the `--check`-only path.
func (u *Updater) Check(ctx context.Context, currentVersion string) (Result, error) {
	rel, err := u.Fetcher.LatestRelease(ctx)
	if err != nil {
		return Result{}, err
	}
	return u.resultFor(currentVersion, rel)
}

// Apply performs the full self-update up to (but not including) writing
// to disk: fetch the latest release, confirm it actually is newer than
// currentVersion, download the matching platform archive and
// checksums.txt, verify the archive's checksum against that manifest,
// then extract the binary from it. It deliberately never touches the
// filesystem outside downloading into memory — the caller
// (internal/cli/upgrade.go) resolves the running executable's own path
// and calls ReplaceBinary itself, keeping this package's core logic
// independent of "where am I installed".
//
// When the latest release is not newer than currentVersion, Apply
// returns the Result (UpdateAvailable: false) and a nil binary/nil error
// — there is nothing to install, and that is not a failure.
func (u *Updater) Apply(ctx context.Context, currentVersion string) (Result, []byte, error) {
	rel, err := u.Fetcher.LatestRelease(ctx)
	if err != nil {
		return Result{}, nil, err
	}
	result, err := u.resultFor(currentVersion, rel)
	if err != nil {
		return Result{}, nil, err
	}
	if !result.UpdateAvailable {
		return result, nil, nil
	}

	archiveName := ArchiveName(ProjectName, StripVersionPrefix(rel.TagName), u.GOOS, u.GOARCH)
	archiveAsset, ok := rel.AssetByName(archiveName)
	if !ok {
		return result, nil, fmt.Errorf("update: release %s has no asset named %s (unsupported platform %s/%s?)", rel.TagName, archiveName, u.GOOS, u.GOARCH)
	}
	checksumsAsset, ok := rel.AssetByName(ChecksumsFileName)
	if !ok {
		return result, nil, fmt.Errorf("update: release %s has no %s asset", rel.TagName, ChecksumsFileName)
	}

	archiveData, err := u.Downloader.Download(ctx, archiveAsset.BrowserDownloadURL)
	if err != nil {
		return result, nil, err
	}
	checksumsData, err := u.Downloader.Download(ctx, checksumsAsset.BrowserDownloadURL)
	if err != nil {
		return result, nil, err
	}

	// Security-critical: never proceed to extraction/installation on an
	// archive whose bytes don't match the release's own published
	// checksums.txt.
	if err := VerifyChecksum(archiveData, checksumsData, archiveName); err != nil {
		return result, nil, err
	}

	binary, err := ExtractBinary(archiveData, archiveName, BinaryName(u.GOOS))
	if err != nil {
		return result, nil, err
	}
	return result, binary, nil
}

func (u *Updater) resultFor(currentVersion string, rel Release) (Result, error) {
	newer, err := IsNewer(currentVersion, rel.TagName)
	if err != nil {
		return Result{}, err
	}
	return Result{
		CurrentVersion:  currentVersion,
		LatestVersion:   rel.TagName,
		ReleaseURL:      rel.HTMLURL,
		UpdateAvailable: newer,
	}, nil
}
