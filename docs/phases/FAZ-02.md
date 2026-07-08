# FAZ 02 — LLM Provider Layer

## What was delivered

- `internal/llm`: the CLAUDE.md `Provider` interface, verbatim
  (`Complete`/`Stream`/`Name`), plus the types every connector shares:
  `Message`, `CompletionRequest` (`System`, `Messages`, `MaxTokens`,
  `RequiredFields`), `CompletionResponse` (`Text`/`Model`/`Usage`/
  `StopReason`/`JSON`), `Usage`, and `Chunk` (`Text`/`Done`/`Err`).
  **Chunk error contract**: every connector sends zero or more
  `Done=false` text chunks, then exactly one `Done=true` chunk (`Err` set
  only on an abnormal stream end — a mid-stream provider error event, a
  transport failure, a decode failure), then closes the channel. There is
  no separate error channel; this single "final chunk carries the
  verdict" shape is what every connector and `Client.Stream` rely on.
- Four connectors, raw `net/http`, stdlib only — **zero new go.mod
  dependencies** (verified: `git diff go.mod go.sum` is empty for this
  phase):
  - `anthropic.go` — Messages API (`POST /v1/messages`), `x-api-key` +
    `anthropic-version: 2023-06-01` headers, SSE streaming
    (`content_block_delta`/`text_delta`, mid-stream `error` events,
    `529 overloaded_error` classified as retryable).
  - `openai_compat.go` — `POST {base_url}/chat/completions`, `Authorization:
    Bearer`; one connector for OpenAI/Mistral/Groq/GLM/Qwen/Kimi/
    OpenRouter/LM Studio, distinguished only by `base_url`/key. SSE
    streaming with the literal `data: [DONE]` sentinel.
  - `google.go` — Gemini `generateContent`/`streamGenerateContent`, model
    **path-encoded** via `url.PathEscape` (never a body field),
    `x-goog-api-key` header (never `?key=`, keeping keys out of
    URLs/logs). `RESOURCE_EXHAUSTED` in the error body is also treated as
    retryable, on top of the numeric status check.
  - `ollama.go` — `POST {base_url}/api/chat`, no auth; NDJSON streaming
    (one JSON object per line, `"done":true` on the last). Also exposes
    `ListModels` (`GET /api/tags`), used to resolve `llm.model` when it is
    empty — Ollama has no static default model, unlike the other three.
  - Every connector's type and constructor is **unexported**
    (`anthropicConnector`/`newAnthropicConnector`, etc.) — an external
    package cannot name the type or call the constructor. `Client`
    (`New(cfg)`) is the only public entry point into this package's
    network calls.
- `internal/llm/parse.go`: `ExtractJSON(raw)` strips a wrapping markdown
  fence (with or without a language tag), tolerates leading prose, and
  locates exactly one balanced top-level JSON object (respecting braces
  inside quoted strings); a second top-level object anywhere after the
  first is a rejected "multiple top-level JSON objects" error.
  `ValidateInto(raw, requiredFields, target)` layers on top: optional
  unmarshal into `target`, then checks every name in `requiredFields`
  against the extracted object's own top-level keys, rejecting a missing
  key, a JSON `null`, or an empty string/array/object (a `false` boolean
  or `0` number is *not* considered empty — presence is what's checked,
  not truthiness).
- `internal/llm/client.go`: `Client`, built by `New(cfg config.Config)`
  from `cfg.LLM.Provider`+`cfg.LLM.Model` (the primary attempt) followed
  by one attempt per `"provider/model"` entry in `cfg.LLM.Fallback`.
  `Client` itself satisfies `Provider` (`Name()` returns the primary
  attempt's provider name) so a caller that only needs "the configured
  LLM, fallback already handled" can depend on the interface. Per-attempt
  timeout (`llm.timeout_seconds`, default 60s for a non-positive value) is
  applied via `context.WithTimeout` around the caller's own `ctx`, for
  both `Complete` and `Stream`.
  - **Fallback classification**: `Complete`/`Stream` try each attempt in
    order. `ErrAuthRejected` (HTTP 401/403 — a credential the provider's
    API itself rejected) stops the chain immediately and is returned as
    the final error, never trying the next attempt. Every other failure —
    timeout/context-deadline, network error, 429/5xx/529, a missing API
    key, or a JSON parse/validation failure (`ErrParseFailure`) — is
    retried against the next attempt. `Stream`'s fallback applies only to
    the initial handshake (a non-nil error from a connector's `Stream`
    before any chunk is produced); once a channel is returned, its
    contents are never retried mid-flight — a later failure surfaces
    through that channel's final `Chunk.Err`, per the Chunk contract
    above.
  - `COMRADE_DEBUG=1` logs one line per attempt to stderr:
    `[llm] provider=<name> model=<model-or-blank> result=<class> latency=<dur>`.
- `internal/llm/keys.go`: `resolveAPIKey(provider)` checks
  `COMRADE_<PROVIDER>_API_KEY` first, then the provider's well-known
  vendor env var(s) (`ANTHROPIC_API_KEY`; `OPENAI_API_KEY`; `GEMINI_API_KEY`
  then `GOOGLE_API_KEY`). Ollama needs no key and has no entry. A missing
  key never fails `Client` construction — see Decisions below.
- `internal/llm/models.go`: default models used when `llm.model` is
  empty — `claude-haiku-4-5` (anthropic), `gpt-5.4-mini` (openai_compat),
  `gemini-3.5-flash` (google), all verified 2026-07. Ollama has no static
  default; the first entry from `/api/tags` is used, with a "pull a model
  first" error when none are installed.
- `comrade config test-llm` (`Hidden: true`): sends a tiny `{role: user,
  content: "ping"}` completion through the full `Client` (fallback chain
  included), printing `provider=... model=... latency=...` on success or
  the wrapped, helpful error otherwise.
- Tests: table-driven httptest fakes for all four connectors (request
  shape — path, headers, body incl. system-prompt placement and Gemini's
  path-encoded model; response parsing — text/usage/stop-reason; error
  mapping — 401→auth-rejected, 429/5xx/529→overloaded), one streaming
  test per connector (concatenated deltas, clean channel close) plus the
  Anthropic mid-stream `error`-event case, `parse.go` edge cases (fenced/
  unfenced/prose-prefixed JSON, two objects, broken JSON, missing
  field), Ollama's `ListModels` (fixture parse + empty-list guidance
  error), the `Client` fallback scenarios (500→secondary succeeds with
  both servers hit; auth error stops the chain; parse failure falls
  back), and a `comrade config test-llm` command test wired against an
  httptest server via `llm.openai_compat.base_url`.

## Decisions & deviations

- **JSON strategy: system-prompt instruction + `parse.go`, not native
  structured-output params.** Anthropic's `output_config.format`,
  OpenAI's `response_format`, Gemini's `responseSchema`, and Ollama's
  `format` all exist, but the vendor fleet behind `openai_compat` is not
  uniform — LM Studio, for one, rejects `type: json_object` outright.
  Rather than special-case per-vendor native support inside one
  connector, this phase asks for JSON purely via the caller's system
  prompt and validates/extracts it uniformly in `parse.go`. **Future
  work**: once `openai_compat`'s vendor matrix is characterized (FAZ 8+),
  revisit native structured outputs as an opt-in per-connector
  optimization — it would reduce parse-failure-triggered fallback
  attempts, but is out of scope here.
- **A missing API key never fails `Client` construction — it is
  deferred to attempt time.** `buildProvider` resolves each attempt's key
  at `New()` time (so key lookup happens once, not per-request), but a
  provider missing its key becomes a `missingKeyProvider` placeholder
  instead of erroring `New()` out entirely, and instead of being baked
  into the connector as an empty credential (which would send an empty
  header and come back as a misleading `ErrAuthRejected` from the wire).
  `missingKeyProvider.Complete`/`Stream` return the precise
  `*KeyMissingError` the moment that attempt is actually tried — and,
  since it wraps `ErrAPIKeyMissing` (not `ErrAuthRejected`), the fallback
  loop treats it as retryable, moving on to the next attempt. This isn't
  spelled out explicitly in UYGULAMA_PLANI.md item 4/5; the alternative
  (fail `New()` immediately on any unconfigured entry, including
  fallbacks) would make a `llm.fallback` list unusable unless *every*
  listed provider's key was already set, defeating the point of having a
  fallback chain for providers configured opportunistically (e.g. a local
  Ollama fallback with no key requirement at all, alongside a
  not-yet-configured cloud fallback). `ErrAuthRejected` is reserved
  strictly for a credential the provider's API rejected over the wire
  (401/403), matching the plan's literal wording ("Non-retryable:
  401/403 (auth)").
- **`Stream`'s fallback boundary is the initial handshake only.** Once a
  connector's `Stream` returns a live channel, `Client` does not retry
  mid-stream — there is no way to "undo" partial output already handed to
  the caller. A connector-level failure after the handshake (e.g. a
  mid-stream Anthropic `error` event) surfaces through that channel's
  final `Chunk{Done: true, Err: ...}`, exactly as it would without a
  fallback chain in front of it. This is the interpretation taken for
  "fallback chain" applying to streaming; UYGULAMA_PLANI.md's fallback
  section is not explicit about streaming's exact retry boundary.
  `releaseOnClose` in `client.go` ties the per-attempt timeout context's
  `cancel()` to the full stream's drain (not the initial connect), so a
  long stream isn't torn down partway through by a context whose
  `cancel` fired too early.
- **`scanSSE` (`sse.go`) is one shared parser for three of the four
  connectors.** Anthropic, openai_compat, and Google all speak
  line-oriented `data: {json}` SSE frames; the only real difference is
  what each connector's callback does with the decoded JSON (dispatch on
  a `type` field vs. index into `choices`/`candidates`, and — for
  openai_compat only — recognizing the literal `data: [DONE]` sentinel
  via the shared `errSSEDone` signal). Ollama's streaming is NDJSON, not
  SSE, and uses a plain `bufio.Scanner` instead.
- **`ExtractJSON`'s "reject a second top-level object" check is a simple
  presence test, not a second full parse.** After locating and validating
  the first balanced `{...}` object, any further `{` in the remaining
  text is treated as a second object and rejected — this is intentionally
  conservative (a stray `{` in trailing prose that isn't actually JSON
  would also trigger it) but matches the literal test case in
  UYGULAMA_PLANI.md ("çift JSON") and keeps the implementation a single
  linear scan rather than a second brace-matching pass just to rule out
  false positives that haven't come up in practice.
- **`googleConnector` carries a `baseURL` field defaulting to the real
  `googleAPIBase` constant** (rather than hardcoding the constant into
  `url()`), purely so tests can point it at an `httptest` server while
  still exercising the same path-building/path-encoding logic used
  against the real API. `anthropicConnector` already had an equivalent
  seam from FAZ 0/1 conventions; this phase applies the same pattern to
  Google for consistency.
- **`context.Context` first parameter, no exceptions in this package.**
  Every connector method that does network I/O (`Complete`, `Stream`,
  `ListModels`) takes `ctx` first, per CLAUDE.md's rule — this package is
  nothing but network I/O, so there is no carve-out like FAZ 1's
  file-I/O-has-no-context-aware-stdlib-variant exception.
- **Keychain integration deferred to FAZ 8, structurally.**
  `resolveAPIKey`'s `(string, error)` signature and the `providerEnvVars`
  table are the only two things FAZ 8 needs to touch to prepend an OS
  keychain lookup ahead of the environment-variable checks — no caller in
  this package needs to change.

## Acceptance evidence

- `go vet ./...` — clean.
- `$(go env GOPATH)/bin/golangci-lint run` — `0 issues.` (one round of
  `staticcheck` S1016 fixes: `anthropicMessage(m)`/`openAIMessage(m)`/
  `ollamaMessage(m)` type conversions replace field-by-field struct
  literals where the source and destination struct shapes are
  identical.)
- `go test ./...` — all packages pass. Per-package count: `internal/cli`
  25 `--- PASS` lines, `internal/config` 36, `internal/llm` 54 (parse.go
  edge cases, all four connectors' request/response/error/streaming
  tests, the Anthropic mid-stream error case, Ollama's `ListModels`, and
  the `Client` fallback scenarios — 500→secondary with both servers hit,
  auth-does-not-fallback, parse-failure-does-fallback, all-fail wraps the
  *last* attempt's error).
- `make build` → `./comrade`. Manual, network-level verification against
  the real built binary (not just `go test`), in an isolated
  `$HOME`/`$XDG_CONFIG_HOME`:
  - No key configured: `./comrade config test-llm` → exit 1,
    `test-llm: llm: all providers failed: anthropic: no API key found for provider "anthropic"; set one of: COMRADE_ANTHROPIC_API_KEY, ANTHROPIC_API_KEY`.
  - `./comrade config --help` does not list `test-llm` (confirms
    `Hidden: true` took effect on the real binary, not just in-test).
  - End-to-end success against a real (if minimal) HTTP server: a
    Python `http.server`-based fake OpenAI-compatible endpoint on
    `127.0.0.1:8934`, asserting the exact `Bearer manual-test-key` header
    and `model` field it received; `comrade config set llm.provider
    openai_compat` + `comrade config set llm.openai_compat.base_url
    http://127.0.0.1:8934` + `COMRADE_OPENAI_COMPAT_API_KEY=manual-test-key
    comrade config test-llm` → `provider=openai_compat
    model=gpt-5.4-mini latency=10ms`, exit 0.
  - `COMRADE_DEBUG=1` against the same no-key scenario prints
    `[llm] provider=anthropic model= result=auth_missing latency=1ms` to
    stderr before the final error, confirming the attempt log fires and
    is classified correctly.
- `git diff --stat go.mod go.sum` — empty. Zero new dependencies added
  this phase, as required (raw `net/http` only).

## Future work (noted, not done this phase)

- Native structured-output params (`output_config.format`/
  `response_format`/`responseSchema`/`format`) as a per-connector,
  opt-in optimization once `openai_compat`'s vendor support matrix is
  characterized — see "JSON strategy" above.
- OS keychain (`zalando/go-keyring`) ahead of the environment-variable
  checks in `resolveAPIKey` (FAZ 8).
- Exposing `ollamaConnector.ListModels` through `Client` for `comrade
  config`'s model picker (FAZ 8) — it exists and is tested now, but has
  no public entry point yet since nothing outside this package needs it
  this phase.
