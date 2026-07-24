package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/redact"
)

// clientAttempt is one entry in a Client's fallback chain: a connector
// paired with the provider name used for debug logging and error
// messages.
type clientAttempt struct {
	providerName string
	provider     Provider
}

// Client is the single public entry point into this package's connectors.
// Every completion or streaming request — the primary provider/model from
// config, and every configured fallback in order — flows through
// Client.Complete or Client.Stream. External packages cannot construct or
// call a connector directly: New(cfg) is the only way to obtain a
// Provider from this package (see docs/history/phases/FAZ-02.md's encapsulation
// rationale).
type Client struct {
	attempts []clientAttempt
	timeout  time.Duration

	// idleTimeout bounds the gap between two consecutive Stream chunks —
	// as opposed to timeout, which bounds the whole stream's duration.
	// Zero (the default, from llm.idle_timeout_seconds=0) disables it:
	// releaseOnClose then never starts an idle timer, reproducing this
	// package's pre-FAZ-6-idle-timeout behavior exactly. See
	// idleTimeoutDuration and releaseOnClose.
	idleTimeout time.Duration

	// redactor masks secrets out of every outgoing payload (System +
	// every Message.Content) before Complete/Stream ever reach a
	// connector. It is set exclusively by New, from cfg.Privacy — never
	// injectable from outside this package (there is no exported
	// setter and the field is unexported), so an external caller cannot
	// construct a Client with a nil/no-op redactor. See redactPayload
	// and docs/history/phases/FAZ-03.md for the non-bypassable-middleware
	// rationale (CLAUDE.md security rule #3).
	redactor *redact.Redactor
}

// compile-time assertion: Client itself satisfies Provider, so a caller
// that only needs "the configured LLM" (with fallback already handled)
// can depend on the Provider interface without caring that a Client sits
// behind it.
var _ Provider = (*Client)(nil)

// KeyResolver resolves provider's API key from wherever a caller wants to
// look — an OS keychain, a file fallback, environment variables, or any
// combination. It is the seam FAZ 8's internal/cli package uses to wire a
// keychain/file-backed lookup (via internal/secrets) ahead of
// resolveAPIKey's env-only logic, without this package ever importing
// internal/secrets: New's default resolver is resolveAPIKey itself (see
// WithKeyResolver), so a caller that never passes WithKeyResolver — every
// existing test in this package included — gets exactly the FAZ 2
// env-only behavior, unchanged.
type KeyResolver func(provider string) (string, error)

// Option configures New. The zero value of clientOptions (no options
// given) reproduces New's pre-FAZ-8 behavior exactly.
type Option func(*clientOptions)

type clientOptions struct {
	keyResolver KeyResolver
}

// WithKeyResolver overrides the KeyResolver New's connectors use to
// resolve an API key, in place of the package-default resolveAPIKey. A
// nil resolver is ignored (the default is kept) so a caller can pass a
// possibly-nil resolver through without an extra branch.
func WithKeyResolver(resolver KeyResolver) Option {
	return func(o *clientOptions) {
		if resolver != nil {
			o.keyResolver = resolver
		}
	}
}

// New builds a Client from cfg: the primary attempt is
// cfg.LLM.Provider+cfg.LLM.Model (or that provider's default model, if
// Model is empty), followed by one attempt per "provider/model" entry in
// cfg.LLM.Fallback, in the order given. It returns an error only for a
// structurally invalid entry (unknown provider name) — a missing API key
// is deferred to attempt time (see resolveAPIKey and Complete/Stream) so
// that one unconfigured fallback candidate never prevents constructing a
// Client whose primary provider works fine.
//
// opts is variadic so every pre-FAZ-8 call site (New(cfg), with no
// options) keeps compiling and behaving identically; see WithKeyResolver
// for the one option this package currently defines.
func New(cfg config.Config, opts ...Option) (*Client, error) {
	options := clientOptions{keyResolver: resolveAPIKey}
	for _, opt := range opts {
		opt(&options)
	}

	httpClient := &http.Client{}

	entries := make([]string, 0, 1+len(cfg.LLM.Fallback))
	entries = append(entries, cfg.LLM.Provider+"/"+cfg.LLM.Model)
	entries = append(entries, cfg.LLM.Fallback...)

	attempts := make([]clientAttempt, 0, len(entries))
	for _, entry := range entries {
		providerName, model := splitProviderModel(entry)
		provider, err := buildProvider(providerName, model, cfg, httpClient, options.keyResolver)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, clientAttempt{providerName: providerName, provider: provider})
	}

	return &Client{
		attempts:    attempts,
		timeout:     httpTimeout(cfg.LLM.TimeoutSeconds),
		idleTimeout: idleTimeoutDuration(cfg.LLM.IdleTimeoutSeconds),
		redactor:    redact.New(cfg.Privacy.RedactEmails, cfg.Privacy.RedactIPs),
	}, nil
}

// httpTimeout wraps timeoutSeconds (llm.timeout_seconds) as a
// time.Duration, defaulting to 60s for a non-positive config value so a
// misconfigured/zero timeout never turns into an instant-cancel context.
func httpTimeout(timeoutSeconds int) time.Duration {
	if timeoutSeconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(timeoutSeconds) * time.Second
}

// idleTimeoutDuration wraps idleTimeoutSeconds (llm.idle_timeout_seconds)
// as a time.Duration. Unlike httpTimeout, a non-positive value here means
// "disabled" (returns 0) rather than falling back to a default duration —
// 0 is idle_timeout_seconds's documented default, and disabling the
// feature entirely is the whole point of that default.
func idleTimeoutDuration(idleTimeoutSeconds int) time.Duration {
	if idleTimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(idleTimeoutSeconds) * time.Second
}

// splitProviderModel splits a "provider/model" fallback entry into its
// two parts. A bare "provider" with no "/model" suffix yields model == ""
// (the provider's default is used).
func splitProviderModel(entry string) (provider, model string) {
	parts := strings.SplitN(entry, "/", 2)
	provider = parts[0]
	if len(parts) == 2 {
		model = parts[1]
	}
	return provider, model
}

// buildProvider constructs the connector for one fallback-chain entry.
// API key resolution happens here, at construction time, for providers
// that need one. A missing key never fails Client construction: the
// entry becomes a *missingKeyProvider instead, so the precise
// "which env var to set" *KeyMissingError only ever surfaces when
// Complete/Stream actually reach that attempt — where it is treated like
// any other retryable failure, never as ErrAuthRejected (that sentinel is
// reserved for a credential the provider's API itself rejected over the
// wire, not one that was never sent).
//
// For openai_compat/ollama specifically, this is also SAST finding #3's
// point-of-use enforcement: config.CheckBaseURL rejects (a hard error,
// failing Client construction — unlike config.validateLoadedConfig at
// config-load time, which only warns) a reject-class base_url — non-
// http(s) scheme, no host, or a cloud-metadata/link-local host — for the
// entry actually being built here, whether it's the primary provider or a
// configured fallback. This is the moment the API key would otherwise be
// handed to a connector holding that base_url, so it is the right place
// to refuse, and it is safe to fail Client construction entirely here
// (unlike Load()): `comrade config set`/`config edit`/`config get` never
// call New at all, so they stay available to repair the value even when
// every do/fix/chat/explain invocation now refuses to build a client.
func buildProvider(providerName, model string, cfg config.Config, httpClient *http.Client, resolveKey KeyResolver) (Provider, error) {
	switch providerName {
	case "anthropic":
		if model == "" {
			model = defaultAnthropicModel
		}
		key, err := resolveKey("anthropic")
		if err != nil {
			return &missingKeyProvider{name: providerName, err: err}, nil
		}
		return newAnthropicConnector(key, model, httpClient), nil

	case "openai_compat":
		if model == "" {
			model = defaultOpenAICompatModel
		}
		if _, err := config.CheckBaseURL("llm.openai_compat.base_url", cfg.LLM.OpenAICompat.BaseURL); err != nil {
			return nil, fmt.Errorf("llm: %s: %w", providerName, err)
		}
		key, err := resolveKey("openai_compat")
		if err != nil {
			return &missingKeyProvider{name: providerName, err: err}, nil
		}
		return newOpenAICompatConnector(key, model, cfg.LLM.OpenAICompat.BaseURL, httpClient), nil

	case "google":
		if model == "" {
			model = defaultGoogleModel
		}
		key, err := resolveKey("google")
		if err != nil {
			return &missingKeyProvider{name: providerName, err: err}, nil
		}
		return newGoogleConnector(key, model, httpClient), nil

	case "ollama":
		// model may legitimately be "" here — resolved lazily against
		// /api/tags on first use (see ollamaConnector.resolveModel).
		if _, err := config.CheckBaseURL("llm.ollama.base_url", cfg.LLM.Ollama.BaseURL); err != nil {
			return nil, fmt.Errorf("llm: %s: %w", providerName, err)
		}
		return newOllamaConnector(model, cfg.LLM.Ollama.BaseURL, httpClient), nil

	default:
		return nil, fmt.Errorf("llm: unknown provider %q", providerName)
	}
}

// missingKeyProvider is a placeholder Provider standing in for a
// fallback-chain entry whose API key could not be resolved at Client
// construction time. It defers the *KeyMissingError to the moment
// Complete/Stream actually try this attempt, so the error carries
// accurate context and the fallback loop can retry the next attempt
// exactly as it would for any other failure — a *KeyMissingError is
// never sent over the wire as an empty credential (which would otherwise
// come back as a misleading ErrAuthRejected from the provider itself).
type missingKeyProvider struct {
	name string
	err  error
}

func (p *missingKeyProvider) Name() string { return p.name }

func (p *missingKeyProvider) Complete(context.Context, CompletionRequest) (CompletionResponse, error) {
	return CompletionResponse{}, p.err
}

func (p *missingKeyProvider) Stream(context.Context, CompletionRequest) (<-chan Chunk, error) {
	return nil, p.err
}

// Name reports the primary (first) provider name this Client was
// constructed with.
func (c *Client) Name() string {
	if len(c.attempts) == 0 {
		return ""
	}
	return c.attempts[0].providerName
}

// Complete tries each attempt in the fallback chain in order, returning
// the first success. An attempt's API key is (re-)resolved here, before
// the connector runs, so a KeyMissingError is reported with an accurate,
// current message and — like every other non-auth-rejected failure — is
// retried against the next attempt rather than aborting the whole chain.
// A 401/403 (ErrAuthRejected) stops the chain immediately without trying
// any further attempt, per docs/history/UYGULAMA_PLANI.md FAZ 2 item 4.
func (c *Client) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	if len(c.attempts) == 0 {
		return CompletionResponse{}, fmt.Errorf("llm: no provider configured")
	}
	req = c.redactPayload(req)

	var lastErr error
	for _, attempt := range c.attempts {
		start := time.Now()
		resp, err := c.tryComplete(ctx, attempt, req)
		latency := time.Since(start)

		if err == nil {
			logAttempt(attempt.providerName, resp.Model, "ok", latency)
			return resp, nil
		}

		logAttempt(attempt.providerName, "", errClass(err), latency)
		lastErr = wrapAttemptError(attempt.providerName, err)

		if errors.Is(err, ErrAuthRejected) {
			return CompletionResponse{}, lastErr
		}
	}
	return CompletionResponse{}, c.finalChainError(lastErr)
}

// wrapAttemptError prefixes err's message with providerName for
// Complete/Stream's returned chain error, UNLESS err's own Error() text
// already starts with that exact "providerName: " prefix. Two error
// classes returned by this package's connectors already embed the
// provider name as a leading prefix themselves: *StatusError (every
// connector's non-2xx HTTP response — see errors.go's Error() method) and
// the reachability wrap wrapReachabilityError/wrapOllamaReachabilityError
// build for a transport-level failure (offline/unreachable). Applying
// this same "%s: " prefix unconditionally on top of either produced a
// doubled "openai_compat: openai_compat: http 401: ..." (and the
// anthropic/google/ollama equivalents). Every other error class this
// package returns (e.g. *KeyMissingError, ErrParseFailure) has no such
// embedded prefix and still gets it added here, exactly as before.
func wrapAttemptError(providerName string, err error) error {
	if strings.HasPrefix(err.Error(), providerName+": ") {
		return err
	}
	return fmt.Errorf("%s: %w", providerName, err)
}

// finalChainError wraps lastErr — the last configured attempt's own
// (already provider-prefixed) failure — as Complete/Stream's final
// returned error once every attempt in the chain has failed, appending
// docs/history/UYGULAMA_PLANI.md FAZ 11 item 2's "Ollama varsa ona düşme önerisi" when
// two conditions both hold: lastErr is (or wraps) ErrOffline — a
// transport-level failure, not a rejection or malformed response the
// suggestion wouldn't fix — and no attempt already configured is Ollama
// (suggesting a user add what they already have configured would be
// useless noise, and would also misattribute a failure that occurred
// AFTER trying Ollama).
func (c *Client) finalChainError(lastErr error) error {
	err := fmt.Errorf("llm: all providers failed: %w", lastErr)
	if errors.Is(lastErr, ErrOffline) && !c.hasOllamaAttempt() {
		return fmt.Errorf("%w; if you have Ollama running locally, add \"ollama\" to llm.fallback (or set llm.provider=ollama) to keep working offline", err)
	}
	return err
}

// hasOllamaAttempt reports whether any configured attempt in c.attempts
// is the ollama provider.
func (c *Client) hasOllamaAttempt() bool {
	for _, attempt := range c.attempts {
		if attempt.providerName == "ollama" {
			return true
		}
	}
	return false
}

// redactPayload returns a copy of req with System and every message's
// Content passed through c.redactor, so no connector — anthropic,
// openai_compat, google, or ollama — ever receives an unredacted
// secret. Complete and Stream are the sole entry points into this
// package's connectors (the connector constructors are unexported per
// FAZ 2), and both call this unconditionally before doing anything
// else with req, so there is no code path from a caller to a connector
// that skips redaction (CLAUDE.md security rule #3;
// docs/history/phases/FAZ-03.md).
//
// c.redactor is nil only for a *Client built by struct literal instead
// of New (this package's own tests do this to stub connectors
// directly, bypassing the public API entirely) — that path is not
// reachable by an external caller, but redactPayload still fails
// closed rather than skipping redaction: a nil redactor lazily becomes
// redact.New(false, false), which keeps every mandatory pattern family
// (api_key/jwt/private_key/credential/bearer) active even though the
// optional email/IP flags default off without a real cfg.Privacy to
// read them from. This guarantees Complete/Stream never send an
// unredacted payload, regardless of how the Client was constructed.
func (c *Client) redactPayload(req CompletionRequest) CompletionRequest {
	if c.redactor == nil {
		c.redactor = redact.New(false, false)
	}
	req.System = c.redactor.Apply(req.System)
	if len(req.Messages) > 0 {
		redacted := make([]Message, len(req.Messages))
		for i, m := range req.Messages {
			redacted[i] = Message{Role: m.Role, Content: c.redactor.Apply(m.Content)}
		}
		req.Messages = redacted
	}
	return req
}

// tryComplete runs one attempt: it applies the per-attempt timeout (honoring
// the caller's ctx via context.WithTimeout), calls the connector, and — when
// the request declared RequiredFields — extracts/validates the response text
// as JSON, surfacing a validation failure as ErrParseFailure so Complete's
// loop retries the next attempt instead of returning malformed output.
func (c *Client) tryComplete(ctx context.Context, attempt clientAttempt, req CompletionRequest) (CompletionResponse, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := attempt.provider.Complete(timeoutCtx, req)
	if err != nil {
		return CompletionResponse{}, err
	}

	if len(req.RequiredFields) > 0 {
		doc, verr := ValidateInto(resp.Text, req.RequiredFields, nil)
		if verr != nil {
			return CompletionResponse{}, fmt.Errorf("%w: %w", ErrParseFailure, verr)
		}
		resp.JSON = doc
	}

	return resp, nil
}

// Stream tries each attempt in the fallback chain in order, exactly like
// Complete, but only for the initial handshake: once a connector's
// Stream successfully returns a channel, that channel's contents are
// never retried mid-flight — a stream failure after the first chunk
// surfaces through that channel's final Chunk.Err, per this package's
// Chunk contract, not through Stream's return value.
func (c *Client) Stream(ctx context.Context, req CompletionRequest) (<-chan Chunk, error) {
	if len(c.attempts) == 0 {
		return nil, fmt.Errorf("llm: no provider configured")
	}
	req = c.redactPayload(req)

	var lastErr error
	for _, attempt := range c.attempts {
		timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)

		ch, err := attempt.provider.Stream(timeoutCtx, req)
		if err == nil {
			logAttempt(attempt.providerName, "", "ok", 0)
			return releaseOnClose(timeoutCtx, ch, cancel, c.idleTimeout), nil
		}
		cancel()

		logAttempt(attempt.providerName, "", errClass(err), 0)
		lastErr = wrapAttemptError(attempt.providerName, err)

		if errors.Is(err, ErrAuthRejected) {
			return nil, lastErr
		}
	}
	return nil, c.finalChainError(lastErr)
}

// releaseOnClose forwards every Chunk from ch to a new channel, calling
// cancel only once the underlying stream is fully drained and ch is
// closed. This ties the per-attempt timeout context's lifetime to the
// whole stream's duration (not just the initial connect) without leaking
// it the moment Stream returns.
//
// Every receive from ch and every forward to out is guarded against
// ctx (== the same timeoutCtx passed to the connector's own Stream call):
// if the caller abandons out without draining it (a Ctrl-C disconnect)
// and then cancels the context that owns ctx, this goroutine returns
// instead of blocking forever on out<-chunk — the mirror image of the
// ctx.Done() guard sendChunk gives every connector's own producer
// goroutine. Without this, an abandoned-but-uncancelled out would still
// eventually unblock once the connector's producer notices ctx.Done() and
// stops sending into ch, but this forwarding goroutine would otherwise be
// the one left blocked on out<-chunk in the meantime.
//
// idleTimeout, when non-zero, bounds the gap between two consecutive
// chunks arriving on ch (including the gap before the very first one) —
// as opposed to ctx's own deadline, which bounds the whole stream. This
// is this package's only enforcement point for llm.idle_timeout_seconds:
// every connector's producer goroutine still just sends into ch exactly
// as before, unaware of idle timeouts entirely, since a single central
// timer here — reset on every chunk this goroutine forwards — is
// equivalent and far simpler than duplicating a per-chunk deadline into
// each of the four connectors' own read loops. idleTimeout == 0 (the
// llm.idle_timeout_seconds default) disables this entirely: idleCh stays
// nil, so that case in the select below can never fire, reproducing this
// package's exact pre-idle-timeout behavior.
//
// The per-chunk reset below is a bare idleTimer.Reset(idleTimeout), with
// no Stop()+drain first. That is deliberate, not an oversight: this
// module is go 1.25, and as of Go 1.23 a Timer's channel is synchronous
// (unbuffered) and Reset on its own is documented to guarantee no stale
// receive — see `go doc time.Timer.Reset`. The pre-1.23 Stop-then-drain
// idiom is actively WRONG on a 1.23+ timer here: if idleTimer fires in
// the same instant a chunk also arrives and this select happens to pick
// the chunk case, the fire is never received by <-idleCh (select only
// serves one ready case), Stop() then reports false, and a
// `<-idleTimer.C` drain would block forever — no value was ever queued
// on an unbuffered channel nobody received from — leaking exactly this
// goroutine. Don't reintroduce the drain.
func releaseOnClose(ctx context.Context, ch <-chan Chunk, cancel context.CancelFunc, idleTimeout time.Duration) <-chan Chunk {
	out := make(chan Chunk)
	go func() {
		defer close(out)
		defer cancel()

		var idleTimer *time.Timer
		var idleCh <-chan time.Time
		if idleTimeout > 0 {
			idleTimer = time.NewTimer(idleTimeout)
			defer idleTimer.Stop()
			idleCh = idleTimer.C
		}

		for {
			select {
			case chunk, ok := <-ch:
				if !ok {
					return
				}
				if idleTimer != nil {
					idleTimer.Reset(idleTimeout)
				}
				if !sendChunk(ctx, out, chunk) {
					return
				}
			case <-idleCh:
				sendChunk(ctx, out, Chunk{Done: true, Err: fmt.Errorf("llm: %w: no chunk received for %s", ErrIdleTimeout, idleTimeout)})
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// sendChunk delivers chunk on ch, but gives up and returns false the
// moment ctx is done — so a stream-reader goroutine whose consumer
// abandoned the channel (a Ctrl-C disconnect propagated as a cancelled
// context) never blocks forever on an unbuffered send nobody will ever
// read. Every connector's Stream goroutine (anthropic, google, ollama,
// openai_compat) and releaseOnClose's forwarding goroutine route every
// Chunk send through this helper — see docs/history/PROGRESS.md's FAZ 6
// hardening note this closes out.
func sendChunk(ctx context.Context, ch chan<- Chunk, chunk Chunk) bool {
	select {
	case ch <- chunk:
		return true
	case <-ctx.Done():
		return false
	}
}
