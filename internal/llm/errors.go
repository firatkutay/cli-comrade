package llm

import (
	"errors"
	"fmt"
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
)

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
	default:
		return "error"
	}
}
