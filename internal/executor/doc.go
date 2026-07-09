// Package executor runs a single generated command on the host platform
// (sh -c on Unix, powershell -Command on Windows), streaming its
// stdout/stderr live while also capturing a tail-truncated copy, and
// reports its exit code, duration, and whether a per-step timeout or
// context cancellation (Ctrl-C) killed it. See executor.go for the full
// runtime-vs-compile-time platform-branching rationale.
package executor
