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
