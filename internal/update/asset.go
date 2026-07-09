package update

import "strings"

// ProjectName is goreleaser's project_name (.goreleaser.yaml) — distinct
// from RepoName (github.go): the GitHub repository is "cli-comrade", but
// every built artifact (the binary itself, and the leading field of the
// archive name_template) uses the shorter "comrade" project name.
const ProjectName = "comrade"

// ChecksumsFileName is the name of the checksums manifest goreleaser
// attaches to every release (.goreleaser.yaml's checksum.name_template)
// — the file scripts/install.sh, scripts/install.ps1, and this
// package's own VerifyChecksum all download and verify an archive
// against. A change to .goreleaser.yaml's checksum.name_template MUST be
// mirrored here; internal/cli's release-name drift-guard test checks
// this constant against the literal value install.sh/install.ps1 use.
const ChecksumsFileName = "checksums.txt"

// ArchiveName renders the exact archive filename goreleaser produces for
// one platform build — the Go-code mirror of .goreleaser.yaml's
// archives[].name_template ("{{ .ProjectName }}_{{ .Version }}_{{ .Os
// }}_{{ .Arch }}") plus its format/format_overrides (tar.gz everywhere,
// zip for windows). version must already have any leading "v" stripped
// (see StripVersionPrefix) — goreleaser's own {{ .Version }} template
// variable is the tag with its leading "v" removed, exactly like
// scripts/install.sh's version_number="${version#v}".
//
// internal/cli's release-name drift-guard test statically compares this
// function's literal field order/separators against both
// .goreleaser.yaml's name_template and the archive-name construction in
// scripts/install.sh and scripts/install.ps1 — change any one of the
// four without the other three and that test fails.
func ArchiveName(project, version, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return project + "_" + version + "_" + goos + "_" + goarch + "." + ext
}

// BinaryName returns the executable's file name inside its archive for
// goos — "comrade" everywhere except Windows, where the archived binary
// carries the platform's usual ".exe" suffix.
func BinaryName(goos string) string {
	if goos == "windows" {
		return "comrade.exe"
	}
	return "comrade"
}

// StripVersionPrefix removes a single leading "v" from a version string
// (e.g. a GitHub release tag "v0.2.0" -> "0.2.0"), matching goreleaser's
// own {{ .Version }} template variable and scripts/install.sh's
// version_number="${version#v}". A version with no leading "v" is
// returned unchanged.
func StripVersionPrefix(version string) string {
	return strings.TrimPrefix(version, "v")
}
