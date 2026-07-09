package secrets

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// fileFallbackWarning is printed to stderr exactly once, the first time
// any Store method actually runs against the file backend — never when
// the keychain backend is active. It intentionally does not repeat the
// full explanation (that lives in fileHeader, inside the file itself,
// where it stays visible every time someone opens the file later).
const fileFallbackWarning = "cli-comrade: no OS keychain available on this machine; storing API keys base64-obfuscated (NOT encrypted) in a 0600 file instead. See that file's header comment for details.\n"

// fileHeader is written verbatim at the top of the credentials file on
// every write, so the "base64 is encoding, not encryption" warning is
// visible to anyone who opens the file directly, not just on first use.
const fileHeader = `# cli-comrade credential fallback file.
#
# WARNING: the values below are base64-ENCODED, NOT ENCRYPTED. Anyone who
# can read this file can recover every API key in it. cli-comrade writes
# here only because no OS keychain (macOS Keychain, Windows Credential
# Manager, Linux Secret Service) was reachable on this machine; prefer
# ` + "`comrade auth login <provider>`" + ` again once one is available. This file
# is created with 0600 permissions (owner read/write only) — never loosen
# them, and never commit or share this file.
`

// fileBackend is Store's fallback backend, used when
// detectKeychainAvailable reports no OS keychain is reachable. Each
// provider's key is stored as one "provider = base64(key)" line in a
// single 0600 file at path — base64 only encodes the key so the file is
// not raw plaintext at a glance; it is NOT encryption (see fileHeader and
// CLAUDE.md security rule #2).
type fileBackend struct {
	path string
}

func (b *fileBackend) kind() Source { return SourceFile }

func (b *fileBackend) get(provider string) (string, bool, error) {
	entries, err := b.readAll()
	if err != nil {
		return "", false, err
	}
	encoded, ok := entries[provider]
	if !ok {
		return "", false, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", false, fmt.Errorf("secrets: file fallback: provider %q: corrupt value in %s: %w", provider, b.path, err)
	}
	return string(raw), true, nil
}

func (b *fileBackend) set(provider, key string) error {
	entries, err := b.readAll()
	if err != nil {
		return err
	}
	entries[provider] = base64.StdEncoding.EncodeToString([]byte(key))
	return b.writeAll(entries)
}

func (b *fileBackend) delete(provider string) error {
	entries, err := b.readAll()
	if err != nil {
		return err
	}
	if _, ok := entries[provider]; !ok {
		return ErrNoCredential
	}
	delete(entries, provider)
	return b.writeAll(entries)
}

// readAll reads and parses b.path, returning an empty (non-nil) map when
// the file does not exist yet — that is the normal "nothing stored yet"
// state, not an error. When the file does exist, it also repairs the
// file's permissions back to 0600 if they have drifted (e.g. a hand-edit,
// or an umask that loosened them at some point) before parsing it.
func (b *fileBackend) readAll() (map[string]string, error) {
	data, err := os.ReadFile(b.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("secrets: read credentials file %s: %w", b.path, err)
	}
	if err := repairPerms(b.path); err != nil {
		return nil, err
	}
	return parseCredentialsFile(data), nil
}

// writeAll rewrites b.path from scratch with entries. The file is opened
// with O_CREATE and mode 0600 in the same call that creates it — not
// created with a looser mode and chmod'd afterward — so there is no
// window during which the file exists on disk with broader-than-owner
// permissions.
func (b *fileBackend) writeAll(entries map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(b.path), 0o700); err != nil {
		return fmt.Errorf("secrets: create credentials directory for %s: %w", b.path, err)
	}
	f, err := os.OpenFile(b.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("secrets: open credentials file %s: %w", b.path, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString(serializeCredentialsFile(entries)); err != nil {
		return fmt.Errorf("secrets: write credentials file %s: %w", b.path, err)
	}
	return nil
}

// repairPerms restores path's permission bits to 0600 if they have
// drifted. It is a no-op on windows: os.Chmod there only maps mode's
// 0200 (owner-writable) bit onto the file's read-only attribute, so a
// Unix-style 0600 check would be meaningless — Windows access control is
// governed by the file's ACL (inherited, by default, as owner-only from
// %APPDATA%), not POSIX permission bits.
func repairPerms(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("secrets: stat credentials file %s: %w", path, err)
	}
	if info.Mode().Perm() != 0o600 {
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("secrets: repair permissions on %s: %w", path, err)
		}
	}
	return nil
}

// parseCredentialsFile parses fileHeader-style "# comment" and
// "provider = base64value" lines. Malformed lines (no "=") are silently
// skipped rather than erroring, matching config.Loader's own
// hand-edited-file tolerance elsewhere in this project.
func parseCredentialsFile(data []byte) map[string]string {
	entries := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		entries[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return entries
}

// serializeCredentialsFile renders entries as fileHeader followed by one
// sorted "provider = base64value" line per entry, so writeAll's output is
// deterministic (byte-for-byte reproducible for the same entries),
// regardless of map iteration order.
func serializeCredentialsFile(entries map[string]string) string {
	providers := make([]string, 0, len(entries))
	for p := range entries {
		providers = append(providers, p)
	}
	sort.Strings(providers)

	var b strings.Builder
	b.WriteString(fileHeader)
	for _, p := range providers {
		fmt.Fprintf(&b, "%s = %s\n", p, entries[p])
	}
	return b.String()
}
