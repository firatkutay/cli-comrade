// Package shellinit builds and manages the "comrade init" shell
// integration block for bash, zsh, fish, and PowerShell.
//
// Every hook snippet below (internal/shellinit/snippets/*) is a
// per-prompt guard that, on every failed-or-not command, execs the
// comrade binary itself — "comrade hook record --shell <name> --exit
// <code> --command <text>" — rather than hand-assembling
// last_command.json's JSON in shell script. Shell-escaping arbitrary
// command text (embedded quotes, unicode, newlines) into a JSON literal
// from inside bash/zsh/fish/PowerShell string syntax is unsafe and
// differs per shell; delegating the encoding to the compiled binary
// (encoding/json, one code path, one set of tests) keeps the JSON
// correct for any command text and identical across every shell. See
// internal/cli/hook.go for the "hook record" subcommand and
// internal/context.WriteLastCommand for the atomic write.
//
// None of the snippets capture stderr/stdout: CLAUDE.md rejects a
// global stderr-tee as unreliable across bash/zsh, so only the command
// text, exit code, shell name, and timestamp are recorded here.
// "comrade fix --rerun" (FAZ 7) captures stderr itself, by controllably
// re-running the command through internal/executor — see
// docs/history/phases/FAZ-04.md for the full rationale.
//
// ApplyBlock/RemoveBlock operate on rc-file content as plain strings
// (no file I/O), delimited by the exact marker lines MarkerBegin/
// MarkerEnd, so a second "comrade init" run is idempotent: unchanged
// content is left alone, changed content (an older snippet version) is
// replaced in place, and a missing block is appended.
package shellinit
