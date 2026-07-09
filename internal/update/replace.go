package update

import (
	"fmt"
	"os"
	"path/filepath"
)

// oldSuffix is appended to a Windows executable's own path when it can't
// delete or overwrite itself while running (see ReplaceBinary).
const oldSuffix = ".old"

// ReplaceBinary atomically replaces the executable at targetPath with
// newContent, matching the mode bits (chmod 0o755) every other platform
// needs to stay executable.
//
//   - Non-Windows: write newContent to a temp file in targetPath's own
//     directory, chmod it executable, then os.Rename it over targetPath.
//     Rename is atomic within one filesystem, and — unlike Windows — a
//     Unix/Linux/macOS process may safely replace the very file backing
//     its own running executable image; the OS keeps the original inode
//     open under the running process until it exits.
//   - Windows: a running process holds an exclusive lock on its own .exe
//     that forbids both deleting and overwriting it directly. The
//     standard workaround (rename-the-running-exe dance): rename
//     targetPath to targetPath+".old" (renaming, unlike deleting or
//     overwriting, IS permitted on the running exe), write newContent to
//     the now-free targetPath, and leave the ".old" file for
//     CleanupOldBinary to remove on a later run once nothing still holds
//     it open.
func ReplaceBinary(targetPath string, newContent []byte, goos string) error {
	if goos == "windows" {
		return replaceWindowsBinary(targetPath, newContent)
	}
	return replaceUnixBinary(targetPath, newContent)
}

func replaceUnixBinary(targetPath string, newContent []byte) error {
	dir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(dir, ".comrade-update-*.tmp")
	if err != nil {
		return fmt.Errorf("update: replace binary: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(newContent); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("update: replace binary: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("update: replace binary: close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil { // #nosec G302 -- tmpPath becomes the comrade executable itself via the rename below; it must keep its execute bit, unlike a data file
		return fmt.Errorf("update: replace binary: chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("update: replace binary: rename temp file into place: %w", err)
	}
	renamed = true
	return nil
}

func replaceWindowsBinary(targetPath string, newContent []byte) error {
	oldPath := targetPath + oldSuffix
	// Best-effort: a leftover .old from a previous update that
	// CleanupOldBinary never got a chance to remove must not block this
	// one.
	_ = os.Remove(oldPath)

	if err := os.Rename(targetPath, oldPath); err != nil {
		return fmt.Errorf("update: replace binary: rename running exe to %s: %w", oldPath, err)
	}

	if err := os.WriteFile(targetPath, newContent, 0o755); err != nil { // #nosec G306 -- targetPath is the comrade executable itself; it must keep its execute bit, unlike a data file
		// Best-effort rollback: restore the original exe so the install
		// isn't left with neither a working old nor new binary.
		_ = os.Rename(oldPath, targetPath)
		return fmt.Errorf("update: replace binary: write new exe: %w", err)
	}
	return nil
}

// CleanupOldBinary best-effort removes targetPath+".old", a leftover from
// a prior ReplaceBinary Windows rename dance whose original process may
// still have held it open at the time. Called on every command's
// startup path (see internal/cli's update-notification hook) so the
// leftover is cleared on the first run after the original process (and
// its file lock) has exited. Errors are deliberately swallowed — this is
// disk-hygiene cleanup, never something that should fail a command.
func CleanupOldBinary(targetPath string) {
	_ = os.Remove(targetPath + oldSuffix)
}
