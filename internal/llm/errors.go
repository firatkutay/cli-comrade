package llm

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Sentinel errors classifying why a connector attempt failed. Client's
// fallback loop (and any caller) uses errors.Is against these instead of
// matching on strings or HTTP status codes directly.
var (
	// ErrAPIKeyMissing means resolveAPIKey found no credential for a
	// provider in any of its known environment variables.
	ErrAPIKeyMissing = errors.New("llm: no API key configured for provider")

	// ErrAuthRejected means the provider's API returned 401/403 for a
	// request that did carry a credential. Non-retryable: Client stops
	// the fallback chain immediately on this error instead of trying the
	// next configured attempt (UYGULAMA_PLANI.md FAZ 2 item 4).
	ErrAuthRejected = errors.New("llm: API key rejected by provider")

	// ErrOverloaded covers HTTP 429 (rate limited) and 5xx/529
	// (overloaded/server error) responses. Retryable: Client tries the
	// next attempt in the fallback chain.
	ErrOverloaded = errors.New("llm: provider rate-limited or overloaded")

	// ErrParseFailure means the provider's response text could not be
	// extracted/validated as the JSON shape the caller declared via
	// CompletionRequest.RequiredFields. Retryable.
	ErrParseFailure = errors.New("llm: failed to parse structured response")

	// ErrOffline classifies a transport-level failure reaching a cloud
	// provider's API — DNS resolution failure, connection refused, or a
	// dial/TLS timeout — as opposed to a non-2xx HTTP response from a
	// server that WAS reached (that is ErrOverloaded/ErrAuthRejected/a
	// plain *StatusError instead). UYGULAMA_PLANI.md FAZ 11 item 2's "ağ
	// yokken ... anlaşılır offline mesajı": wrapReachabilityError below
	// is what turns Go's raw *url.Error (e.g. "dial tcp: lookup
	// api.anthropic.com: no such host") into a message a terminal
	// beginner can act on, and Client.Complete/Stream use errors.Is
	// against this sentinel to decide whether to append a "try Ollama
	// instead" suggestion once every configured attempt has failed.
	// Retryable, exactly like ErrOverloaded — a transient/offline network
	// condition, not a rejection.
	ErrOffline = errors.New("llm: could not reach provider (network unreachable)")

	// ErrIdleTimeout means a Stream went longer than llm.idle_timeout_seconds
	// without producing a chunk — distinct from the whole-stream
	// llm.timeout_seconds deadline, which surfaces as ctx.Err() (deadline
	// exceeded) instead. Only ever produced when idle_timeout_seconds is
	// configured above its 0 (disabled) default; see releaseOnClose in
	// client.go, this package's sole enforcement point for it.
	ErrIdleTimeout = errors.New("llm: stream idle timeout")
)

// wrapReachabilityError recognizes a transport-level failure (err is, or
// wraps, a *url.Error — the shape *http.Client.Do returns for exactly
// this class of failure, never for a non-2xx HTTP response) and replaces
// it with a message naming provider and baseURL, wrapping both ErrOffline
// (so callers can errors.Is against it) and the original err (so no
// diagnostic detail — e.g. the underlying DNS error text — is lost).
// Anything else (a non-2xx response, a body-read failure, ...) passes
// through unchanged: this is deliberately narrower than
// wrapOllamaReachabilityError's ollama-specific "run `ollama serve`"
// phrasing, since anthropic/openai_compat/google are always remote
// services a user cannot "start" themselves — Client is what adds the
// "try Ollama" suggestion, once, after the whole fallback chain fails
// this way (see errClass and Client.Complete/Stream in client.go).
func wrapReachabilityError(provider, baseURL string, err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("%s: could not reach %s — check your network connection (%w): %w", provider, baseURL, ErrOffline, err)
	}
	return err
}

// StatusError is returned by every connector for a non-2xx HTTP response.
// It wraps one of the sentinels above via Unwrap (Sentinel is nil for a
// status this package does not specially classify, e.g. 400) so callers
// can errors.Is against the sentinel while errors.As still recovers the
// provider name, status code, and raw message.
type StatusError struct {
	Provider   string
	StatusCode int
	Message    string
	Sentinel   error
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s: http %d: %s", e.Provider, e.StatusCode, e.Message)
}

// Unwrap lets errors.Is(err, ErrAuthRejected)/errors.Is(err, ErrOverloaded)
// see through a *StatusError to its classification, when one applies.
func (e *StatusError) Unwrap() error {
	return e.Sentinel
}

// KeyMissingError is returned by resolveAPIKey when no credential could be
// found for Provider. EnvVars lists every environment variable name that
// was checked, in priority order, so the error message tells the user
// exactly which one to set.
type KeyMissingError struct {
	Provider string
	EnvVars  []string
}

func (e *KeyMissingError) Error() string {
	return fmt.Sprintf("no API key found for provider %q; set one of: %s", e.Provider, strings.Join(e.EnvVars, ", "))
}

// Unwrap lets errors.Is(err, ErrAPIKeyMissing) see through a
// *KeyMissingError.
func (e *KeyMissingError) Unwrap() error {
	return ErrAPIKeyMissing
}

// errClass returns a short, stable classification string for err, used
// only in the COMRADE_DEBUG=1 fallback-attempt log (see logAttempt).
func errClass(err error) string {
	switch {
	case errors.Is(err, ErrAuthRejected):
		return "auth_rejected"
	case errors.Is(err, ErrAPIKeyMissing):
		return "auth_missing"
	case errors.Is(err, ErrOverloaded):
		return "overloaded"
	case errors.Is(err, ErrParseFailure):
		return "parse_failure"
	case errors.Is(err, ErrOffline):
		return "offline"
	case errors.Is(err, ErrIdleTimeout):
		return "idle_timeout"
	default:
		return "error"
	}
}
