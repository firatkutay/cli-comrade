package update

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// VerifyChecksum reports whether data's SHA-256 digest matches the entry
// for fileName inside checksumsTxt — the same checksums.txt goreleaser's
// checksum block produces and scripts/install.sh's `sha256sum -c`/
// scripts/install.ps1's Get-FileHash already verify against. checksumsTxt
// is expected in the standard `sha256sum` output format: one
// "<hex-digest>  <filename>" line per artifact.
//
// This is the security-critical step CLAUDE.md's supply-chain posture
// and docs/history/UYGULAMA_PLANI.md FAZ 10 both require before ever replacing the
// running binary: a downloaded archive is NEVER installed without first
// being checksum-verified against the release's own published manifest.
func VerifyChecksum(data, checksumsTxt []byte, fileName string) error {
	want, err := findChecksum(checksumsTxt, fileName)
	if err != nil {
		return err
	}

	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("update: checksum mismatch for %s: expected %s, got %s", fileName, want, got)
	}
	return nil
}

// findChecksum scans checksumsTxt line by line for an entry naming
// fileName, returning its hex digest.
func findChecksum(checksumsTxt []byte, fileName string) (string, error) {
	for _, line := range strings.Split(string(checksumsTxt), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		digest := fields[0]
		// sha256sum's format prefixes the filename with "*" for binary
		// mode on some platforms; strip it before comparing.
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == fileName {
			return digest, nil
		}
	}
	return "", fmt.Errorf("update: no checksum entry found for %s", fileName)
}
