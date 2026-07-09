// Package audit records every command internal/engine's Runner actually
// executed as one JSONL entry per step, and lets `comrade history` read
// them back. It never logs stdout/stderr content (may carry secrets the
// redaction pipeline never even saw, since a step's raw execution output
// is a local concern, not something sent to any LLM) — only the command
// text itself, which is exactly what the user already saw and approved.
package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Entry is one executed step's audit record — CLAUDE.md security rule #4's
// exact field list: timestamp, mode, command, risk class, exit code,
// duration, plus Request (the free-text request that produced the plan
// this step came from), which UYGULAMA_PLANI.md FAZ 6 item 4 also
// requires.
type Entry struct {
	Timestamp  time.Time `json:"timestamp"`
	Request    string    `json:"request"`
	Command    string    `json:"command"`
	Risk       string    `json:"risk"`
	Mode       string    `json:"mode"`
	ExitCode   int       `json:"exit_code"`
	DurationMs int64     `json:"duration_ms"`
}

// Logger appends Entry records to a single JSONL file and supports
// listing/retention cleanup over that same file. It holds no global
// state — path is fixed at construction (NewLogger), per CLAUDE.md's
// dependency-injection rule.
type Logger struct {
	path string
}

// NewLogger builds a Logger writing to path, creating path's parent
// directory (but not the file itself — Append creates it lazily on first
// write) if it does not already exist.
func NewLogger(path string) (*Logger, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("audit: create directory %s: %w", dir, err)
	}
	return &Logger{path: path}, nil
}

// Path returns the JSONL file path this Logger reads from and writes to.
func (l *Logger) Path() string {
	return l.path
}

// Append serializes entry as one JSON line and appends it to the audit
// file, opening it with O_APPEND so concurrent/interleaved writes from
// multiple short-lived `comrade` invocations never interleave mid-line or
// clobber each other's data — O_APPEND's move-to-end-then-write is atomic
// per write(2) call for a single line's worth of bytes on every platform
// this project targets.
func (l *Logger) Append(entry Entry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("audit: marshal entry: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("audit: open %s: %w", l.path, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("audit: write entry: %w", err)
	}
	return nil
}

// ReadAll reads every entry currently in the audit file, oldest first. A
// missing file is not an error: it reports zero entries, exactly like a
// freshly-installed `comrade` that has never executed anything yet. A
// line that fails to parse as JSON is skipped rather than aborting the
// whole read — a single corrupted line (e.g. a partial write from a crash
// mid-Append) should not hide every other, valid entry from `comrade
// history`.
func (l *Logger) ReadAll() ([]Entry, error) {
	f, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("audit: open %s: %w", l.path, err)
	}
	defer func() { _ = f.Close() }()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("audit: read %s: %w", l.path, err)
	}
	return entries, nil
}

// ApplyRetention drops every entry older than retentionDays (relative to
// now) by rewriting the audit file with only the surviving entries —
// UYGULAMA_PLANI.md FAZ 6 item 4's "retention_days temizliği açılışta lazy
// çalışır" requirement. A non-positive retentionDays disables cleanup
// entirely (retention is a user-configurable opt-out, not something this
// package forces). Called once per `comrade` invocation, at startup,
// before the entry(ies) that invocation itself will append — it is
// documented as an intentionally simple, unconditional read/filter/rewrite
// on every run (no separate "have I already cleaned up today" state file)
// since the audit log's expected size is small (rewriting it here costs
// microseconds, not something worth its own caching layer — see
// docs/phases/FAZ-06.md).
func (l *Logger) ApplyRetention(retentionDays int, now time.Time) error {
	if retentionDays <= 0 {
		return nil
	}

	entries, err := l.ReadAll()
	if err != nil {
		return err
	}

	cutoff := now.AddDate(0, 0, -retentionDays)
	kept := entries[:0]
	for _, e := range entries {
		if !e.Timestamp.Before(cutoff) {
			kept = append(kept, e)
		}
	}
	if len(kept) == len(entries) {
		return nil
	}

	return l.rewrite(kept)
}

// rewrite atomically replaces the audit file's contents with entries: it
// writes to a temp file in the same directory, then os.Rename's it into
// place, so a reader never observes a partially-rewritten file mid-way
// through cleanup — the same atomic-write pattern
// internal/context.WriteLastCommand uses.
func (l *Logger) rewrite(entries []Entry) error {
	dir := filepath.Dir(l.path)
	tmp, err := os.CreateTemp(dir, ".audit-*.jsonl.tmp")
	if err != nil {
		return fmt.Errorf("audit: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	w := bufio.NewWriter(tmp)
	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			_ = tmp.Close()
			return fmt.Errorf("audit: marshal entry: %w", err)
		}
		if _, err := w.Write(append(data, '\n')); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("audit: write temp file: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("audit: flush temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("audit: close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("audit: chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, l.path); err != nil {
		return fmt.Errorf("audit: rename temp file into place: %w", err)
	}
	renamed = true
	return nil
}

// auditFileName is the JSONL file's name within the resolved state
// directory.
const auditFileName = "audit.jsonl"

// PathFor resolves the audit.jsonl path for goos, using getenv for
// environment lookups — the same injectable-goos pattern as
// config.ResolvePath / context.LastCommandPath, so the windows branch is
// testable regardless of the OS the test binary actually runs on.
//
//   - windows: %LOCALAPPDATA%\cli-comrade\audit.jsonl
//   - otherwise: $XDG_STATE_HOME/cli-comrade/audit.jsonl, falling back to
//     ~/.local/state/cli-comrade/audit.jsonl when XDG_STATE_HOME is unset
func PathFor(goos string, getenv func(string) string) (string, error) {
	if goos == "windows" {
		localAppData := getenv("LOCALAPPDATA")
		if localAppData == "" {
			return "", fmt.Errorf("resolve audit.jsonl path: LOCALAPPDATA environment variable is not set")
		}
		return localAppData + `\cli-comrade\` + auditFileName, nil
	}

	if xdg := getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "cli-comrade", auditFileName), nil
	}

	home := getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("resolve audit.jsonl path: HOME environment variable is not set")
	}
	return filepath.Join(home, ".local", "state", "cli-comrade", auditFileName), nil
}

// DefaultPath resolves the audit.jsonl path for the OS and environment
// this process is actually running under.
func DefaultPath() (string, error) {
	return PathFor(runtime.GOOS, os.Getenv)
}
