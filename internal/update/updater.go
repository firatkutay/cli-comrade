package update

import (
	"context"
	"fmt"
)

// SignatureStatus reports the outcome of Apply's checksums.txt
// signature-verification gate (MEDIUM#4) for a successful Apply call.
// This package never prints anything itself — it has no
// i18n.Translator — so it hands this status back to the caller
// (internal/cli/upgrade.go, which does have one) to decide what, if
// anything, to tell the user.
type SignatureStatus int

const (
	// SignatureStatusUnset is Result's zero value: Apply either failed
	// before reaching the signature gate, or no update was available at
	// all (there was nothing to verify).
	SignatureStatusUnset SignatureStatus = iota

	// SignatureStatusNotConfigured means the embedded cosign.pub is
	// still cosign.pub's build-time placeholder — signature
	// verification was impossible, so Apply fell back to its
	// pre-existing checksum-only verification. internal/cli's
	// upgrade.go renders this as an informational warning.
	SignatureStatusNotConfigured

	// SignatureStatusVerified means a real public key is embedded and
	// checksums.txt's signature verified successfully against it before
	// Apply trusted checksums.txt for anything.
	SignatureStatusVerified
)

// Result summarizes one Check or Apply outcome.
type Result struct {
	CurrentVersion  string
	LatestVersion   string // the release tag as published, e.g. "v0.2.0"
	ReleaseURL      string
	UpdateAvailable bool

	// SignatureStatus is only meaningful when UpdateAvailable is true
	// AND Apply returned a nil error — see SignatureStatus's own doc
	// comment. Check never sets this; it stays SignatureStatusUnset.
	SignatureStatus SignatureStatus
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

	// CosignPub overrides the embedded cosign.pub for tests only. nil or
	// empty means: use the embedded key — exactly like every production
	// caller (internal/cli's defaultUpgradeDeps never sets this).
	// Exported only so tests in OTHER packages (internal/cli) can drive
	// Apply's signature-verification gate against an explicit placeholder
	// or an ephemeral in-test key pair instead of whatever key is
	// actually embedded in the binary — production code must always
	// leave this nil.
	CosignPub []byte
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
// checksums.txt, verify checksums.txt's own signature and the archive's
// checksum against it, then extract the binary from it. It deliberately
// never touches the filesystem outside downloading into memory — the
// caller (internal/cli/upgrade.go) resolves the running executable's own
// path and calls ReplaceBinary itself, keeping this package's core logic
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

	// Security-critical (MEDIUM#4): anchor checksums.txt itself to the
	// embedded cosign public key before trusting anything it says. This
	// runs BEFORE VerifyChecksum on purpose — checksums.txt is only as
	// trustworthy as its own signature, and the archive is only as
	// trustworthy as checksums.txt; without this step, a compromised
	// release (or a compromised transport) could forge checksums.txt to
	// match a malicious archive and VerifyChecksum would happily agree.
	sigStatus, err := u.verifyChecksumsSignature(ctx, rel, checksumsData)
	if err != nil {
		return result, nil, err
	}
	result.SignatureStatus = sigStatus

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

// verifyChecksumsSignature is Apply's MEDIUM#4 gate in front of
// checksums.txt. It never prints anything — see SignatureStatus's doc
// comment — it only reports what happened, or a hard-failure error:
//
//   - If the embedded public key is still cosign.pub's build-time
//     placeholder (signingConfigured reports false), signature
//     verification is impossible. This is a deliberate rollout-safe
//     fallback, not a bug: it returns (SignatureStatusNotConfigured,
//     nil) so Apply proceeds to its existing checksum-only verification
//     exactly as it did before signing was ever wired in — a release
//     built before a real key is embedded keeps working.
//   - Once a real key is configured, this becomes a hard gate: a release
//     published without a ChecksumsSigFileName asset (wrapping
//     ErrMissingSignatureAsset), or with one that doesn't verify
//     (wrapping ErrSignatureInvalid), is refused outright. Either case
//     returns a non-nil error and Apply never reaches
//     VerifyChecksum/ExtractBinary, so the caller's ReplaceBinary is
//     never invoked on unsigned or mis-signed material.
func (u *Updater) verifyChecksumsSignature(ctx context.Context, rel Release, checksumsData []byte) (SignatureStatus, error) {
	pubPEM := u.CosignPub
	if len(u.CosignPub) == 0 {
		pubPEM = embeddedCosignPub
	}

	if !signingConfigured(pubPEM) {
		return SignatureStatusNotConfigured, nil
	}

	sigAsset, ok := rel.AssetByName(ChecksumsSigFileName)
	if !ok {
		return SignatureStatusUnset, fmt.Errorf("update: signed releases required but no signature asset found for %s: %w", ChecksumsSigFileName, ErrMissingSignatureAsset)
	}
	sigData, err := u.Downloader.Download(ctx, sigAsset.BrowserDownloadURL)
	if err != nil {
		return SignatureStatusUnset, fmt.Errorf("update: download %s: %w", ChecksumsSigFileName, err)
	}
	if err := verifyChecksumsSignatureWith(pubPEM, checksumsData, sigData); err != nil {
		return SignatureStatusUnset, fmt.Errorf("update: %s signature verification failed: %w", ChecksumsFileName, err)
	}
	return SignatureStatusVerified, nil
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
