package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/secrets"
)

// maxReachBodyBytes bounds how many bytes ReachCheck reads from the
// keyless probe response — only ever needed to count ollama's /api/tags
// model list, never anything larger. Mirrors update/github.go's own
// maxReleaseJSONBytes-style defensive cap against a misbehaving endpoint.
const maxReachBodyBytes = 1 << 20 // 1 MiB

// ReachCheck GETs the active provider's keyless health endpoint
// (llm.HealthEndpoint) — no credential is ever sent by this step. ANY
// HTTP status back (401/404 included) counts as reachable; only a
// transport-level failure (DNS/dial/TLS/timeout) is a Fail. ollama gets
// one extra rule: a 200 response with zero locally-pulled models is a
// Warn, fix `ollama pull <model>`.
//
// When deps.Live is true and the provider is not ollama (ollama needs no
// credential to reject), it additionally resolves a key exactly like
// `comrade auth login`'s own ping does (Store first, then known
// environment variables) and sends ONE real, minimal authenticated
// request via deps.LivePing — a 401/403 (llm.ErrAuthRejected) is a Fail
// naming the rejection; any other failure is a Warn (the key might still
// be fine).
func ReachCheck(ctx context.Context, deps Deps) Result {
	provider := deps.Cfg.LLM.Provider
	if deps.ConfigErr != nil || provider == "" {
		return Result{Severity: SeveritySkip}
	}

	url, ok := llm.HealthEndpoint(provider, deps.Cfg)
	if !ok {
		return Result{Severity: SeveritySkip, Summary: i18n.MsgDoctorReachSkip, SummaryArgs: []any{provider}}
	}
	if deps.HTTP == nil {
		return Result{Severity: SeveritySkip}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{Severity: SeverityFail, Summary: i18n.MsgDoctorReachFail, SummaryArgs: []any{provider}, Detail: err.Error()}
	}
	resp, err := deps.HTTP.Do(req)
	if err != nil {
		return Result{Severity: SeverityFail, Summary: i18n.MsgDoctorReachFail, SummaryArgs: []any{provider}, Detail: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	if provider == "ollama" && resp.StatusCode == http.StatusOK {
		if ollamaHasNoModels(resp.Body) {
			return Result{Severity: SeverityWarn, Summary: i18n.MsgDoctorReachOllamaNoModels, Fix: "ollama pull llama3.1"}
		}
	} else {
		// The body is only ever inspected for ollama's model-count rule
		// above; every other provider's response is otherwise unused —
		// still drain it so the connection can be reused, matching every
		// connector in internal/llm's own io.Copy-to-discard convention.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxReachBodyBytes))
	}

	if deps.Live && provider != "ollama" {
		return reachCheckLive(ctx, deps, provider)
	}

	return Result{Severity: SeverityOK, Summary: i18n.MsgDoctorReachOK, SummaryArgs: []any{provider}}
}

// ollamaHasNoModels reads body (already bounded by maxReachBodyBytes) as
// Ollama's GET /api/tags response shape and reports whether its "models"
// array is empty. A decode failure is treated as "not empty" (never
// escalate a body-parsing hiccup into ReachCheck's own Warn) since a
// malformed body here is a genuinely different, more surprising problem
// than "no models pulled yet" — this function's only job is to answer
// that one specific question, not to fully validate the response.
func ollamaHasNoModels(body io.Reader) bool {
	var parsed struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(body, maxReachBodyBytes)).Decode(&parsed); err != nil {
		return false
	}
	return len(parsed.Models) == 0
}

// reachCheckLive is ReachCheck's --live tail: resolve a key for provider
// (Store first, then known environment variables — the same precedence
// `comrade auth login`'s own ping uses), then send ONE real completion
// through deps.LivePing.
func reachCheckLive(ctx context.Context, deps Deps, provider string) Result {
	key, err := resolveKeyForLive(ctx, deps, provider)
	if err != nil {
		return Result{
			Severity:    SeverityFail,
			Summary:     i18n.MsgDoctorKeyMissing,
			SummaryArgs: []any{provider},
			Fix:         "comrade auth login " + provider,
		}
	}
	if deps.LivePing == nil {
		return Result{Severity: SeverityOK, Summary: i18n.MsgDoctorReachOK, SummaryArgs: []any{provider}}
	}

	_, latency, pingErr := deps.LivePing(ctx, deps.Cfg, provider, key)
	if pingErr != nil {
		if errors.Is(pingErr, llm.ErrAuthRejected) {
			return Result{Severity: SeverityFail, Summary: i18n.MsgDoctorReachLiveRejected, SummaryArgs: []any{provider}, Detail: pingErr.Error()}
		}
		return Result{Severity: SeverityWarn, Summary: i18n.MsgDoctorReachLiveFailed, SummaryArgs: []any{provider}, Detail: pingErr.Error()}
	}
	return Result{Severity: SeverityOK, Summary: i18n.MsgDoctorReachLiveOK, SummaryArgs: []any{provider, latency.String()}}
}

// resolveKeyForLive resolves provider's API key exactly like
// internal/cli's secretsKeyResolver does (Store first, then
// llm.ResolveEnvKey's own known-environment-variable fallback) — a small,
// deliberate duplication of that tiny function rather than an
// internal/cli import, which would create an import cycle (internal/cli
// already imports internal/doctor for the check registry).
func resolveKeyForLive(ctx context.Context, deps Deps, provider string) (string, error) {
	if deps.Store != nil {
		if key, source, err := deps.Store.Get(ctx, provider); err == nil && source != secrets.SourceNone {
			return key, nil
		}
	}
	return llm.ResolveEnvKey(provider)
}
