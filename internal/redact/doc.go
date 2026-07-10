// Package redact masks secrets (API keys, tokens, passwords, private
// keys, JWTs, and optionally emails/IPs) out of any text before it
// leaves this process, per CLAUDE.md security rule #3 ("Redaction
// pipeline'ı bypass edilemez; LLM'e giden her payload redact'ten
// geçer"). It has zero dependency on internal/config by design: New
// takes plain bools instead of a config.Config so this package can be
// imported from anywhere — most importantly internal/llm, which wires
// it as a hardwired, non-injectable middleware in front of every
// connector (see docs/history/phases/FAZ-03.md).
package redact
