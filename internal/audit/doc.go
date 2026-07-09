// Package audit records every command internal/engine's Runner actually
// executed as one JSONL entry (timestamp, request, command, risk class,
// mode, exit code, duration) for later inspection via "comrade history".
// It never logs stdout/stderr content — only the command text itself,
// which is exactly what the user already saw and approved.
package audit
