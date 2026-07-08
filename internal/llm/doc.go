// Package llm defines the provider-agnostic Provider interface (per
// CLAUDE.md's "LLM Provider Mimarisi") plus one unexported connector per
// backend (anthropic, openai_compat, google, ollama) and the Client that
// resolves config into a fallback chain of connectors.
//
// Connector constructors are unexported by design: the only way an
// external package can talk to an LLM backend is through New(cfg), which
// returns a *Client. This is what lets FAZ 3's redaction pipeline (and
// any future cross-cutting concern) sit in front of every request without
// relying on every call site to remember to use it — there is no other
// path to the network in this package.
package llm
