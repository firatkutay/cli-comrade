// Package secrets stores and resolves LLM provider API keys outside of
// config.toml, per CLAUDE.md security rule #2 ("API key'ler asla config
// dosyasına plaintext yazılmaz"): an OS keychain (macOS Keychain, Windows
// Credential Manager, Linux Secret Service, via zalando/go-keyring) is
// the primary backend, with a 0600 file at
// "<config dir>/credentials" — obfuscated with base64, NOT encrypted —
// as the fallback when no keychain is available (e.g. headless Linux
// with no Secret Service).
//
// This package deliberately has zero dependency on internal/llm: it is
// consumed from internal/cli, which wires a Store-backed
// llm.KeyResolver into the llm.Client it constructs (see
// docs/phases/FAZ-08.md and llm.WithKeyResolver's doc comment) — keeping
// the dependency arrow cli -> {llm, secrets}, never llm -> secrets.
package secrets
