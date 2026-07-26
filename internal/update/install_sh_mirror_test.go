package update

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestInstallShEmbedsExactCosignPub is the bidirectional drift guard for
// COSIGN_PUB's cross-language mirror (GitHub issue #28): scripts/install.sh
// embeds its own literal copy of the project's cosign public key (a plain
// shell script has no go:embed equivalent), so it and cosign.pub are now
// the same secret held in two places with no shared source of truth tying
// them together — a key rotation applied to one side alone would leave
// the other silently authenticating against a stale key. Same shape as
// internal/envkeys/managed_mirror_test.go's TestNPMDispatcherEnvSignalMatchesGoConstants,
// applied to a byte-identical secret instead of a name:value pair.
//
// This reads scripts/install.sh's actual source (not a copy/fixture) and
// asserts it contains the exact bytes this package already has embedded
// via go:embed (embeddedCosignPub) — the real cosign.pub content, not a
// re-read of the file, so a change to either cosign.pub itself or to how
// this package embeds it is covered by the same assertion.
func TestInstallShEmbedsExactCosignPub(t *testing.T) {
	installShPath := filepath.Join("..", "..", "scripts", "install.sh")
	src, err := os.ReadFile(installShPath) // #nosec G304 -- fixed, repo-relative path to this repo's own source file, not attacker-controlled input
	require.NoError(t, err, "scripts/install.sh must be readable from internal/update's own test at %s — did it move?", installShPath)

	require.True(t, bytes.Contains(src, embeddedCosignPub),
		"scripts/install.sh's embedded COSIGN_PUB has drifted out of sync with internal/update/cosign.pub — "+
			"they must stay byte-identical (the install.sh copy is only readable as a literal since a shell "+
			"script has no go:embed equivalent). Update install.sh's COSIGN_PUB block to match cosign.pub's "+
			"exact current bytes.")
}

// TestInstallPs1EmbedsExactCosignPub is install.ps1's counterpart to
// TestInstallShEmbedsExactCosignPub above (GitHub issue #43): install.ps1
// carries the same byte-identical copy of the project's cosign public key
// (as a PowerShell here-string literal — a plain .ps1 script has no
// equivalent of Go's embed directive either), so the same drift risk
// applies: a key rotation applied to cosign.pub without also updating
// install.ps1 would leave install.ps1 silently authenticating against a
// stale key.
//
// Line-ending normalization is not a loophole here, it's a correctness
// requirement: .gitattributes forces scripts/install.ps1 to CRLF line
// endings ("*.ps1 text eol=crlf") while cosign.pub (and this package's
// embedded embeddedCosignPub, via Go's embed directive) stay LF per the
// repo's default ("* text=auto eol=lf") — a raw byte search would
// spuriously report drift on
// line-ending convention alone, never on actual key content. Both sides are
// normalized identically before comparing, so this still asserts the PEM
// bytes are identical — it cannot mask a real content change, only the
// line-ending convention each file is independently required to use.
func TestInstallPs1EmbedsExactCosignPub(t *testing.T) {
	installPs1Path := filepath.Join("..", "..", "scripts", "install.ps1")
	src, err := os.ReadFile(installPs1Path) // #nosec G304 -- fixed, repo-relative path to this repo's own source file, not attacker-controlled input
	require.NoError(t, err, "scripts/install.ps1 must be readable from internal/update's own test at %s — did it move?", installPs1Path)

	normalizedSrc := bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	normalizedCosignPub := bytes.ReplaceAll(embeddedCosignPub, []byte("\r\n"), []byte("\n"))

	require.True(t, bytes.Contains(normalizedSrc, normalizedCosignPub),
		"scripts/install.ps1's embedded $CosignPub has drifted out of sync with internal/update/cosign.pub — "+
			"they must stay byte-identical (modulo line-ending convention; the install.ps1 copy is only readable "+
			"as a literal since a PowerShell script has no go:embed equivalent). Update install.ps1's $CosignPub "+
			"here-string to match cosign.pub's exact current bytes.")
}
