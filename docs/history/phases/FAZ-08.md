# FAZ 08 — Keychain, `comrade auth`, Model Yönetimi

## What was built

### `internal/secrets` — credential store abstraction (new package)

- **`Store` interface**: `Get(ctx, provider) (key, source, err)`, `Set(ctx, provider, key) error`,
  `Delete(ctx, provider) error`, `Status(ctx) ([]ProviderStatus, error)`. `Source` is one of
  `keychain`/`file`/`env`/`none` — `Store` itself never returns `env` (it has no knowledge of
  environment variables at all); that value exists purely so `internal/cli`'s `auth status` can label
  a Store answer and its own env-var check with one shared type.
- **`KnownProviders`**: every `config.ProviderNames()` value except `ollama` (which needs no
  credential) — derived, not hand-copied, per this project's derive-or-guard rule. New
  `config.ProviderNames()` exports the `llm.provider` enum `keyDefs` already defines, so the two
  cannot drift silently (`TestKnownProvidersIsEveryConfigProviderExceptOllama` pins the exact list on
  both sides).
- **`NewStore(credentialsPath, stderr) Store`**: picks exactly one backend, once, via
  `detectKeychainAvailable()` — a read-only `keyring.Get` probe against a reserved account name that
  is never written. `keyring.ErrNotFound` (or no error) means the backend itself is reachable, just
  empty for that account → keychain backend. Any other error (a Linux Secret Service/D-Bus connection
  failure on a headless machine being the expected case, but the check does not special-case that one
  error) → file backend. This mirrors go-keyring's own test seam: `keyring.MockInit()` /
  `keyring.MockInitWithError(err)` control which branch every test actually exercises, so no test in
  this codebase ever touches the real OS keychain.
- **`keychainBackend`**: one go-keyring entry per provider, service `"cli-comrade"`, account =
  provider name.
- **`fileBackend`**: a single file at `<config dir>/credentials` (same directory `config.toml` lives
  in — see below), one `provider = base64(key)` line per provider, plus a header comment that spells
  out "base64 is encoding, not encryption" every time the file is opened directly. The file is created
  with `os.OpenFile(..., O_CREATE, 0600)` — 0600 from the moment of creation, never a looser mode
  chmod'd afterward — and `readAll` repairs the permission bits back to 0600 on every read if they've
  drifted (hand-edit, unusual umask). A one-time warning prints to stderr (via `sync.Once`) the first
  time *any* Store method actually runs against the file backend — never when the keychain backend is
  active.
- **`internal/config` additions**: `ResolveDir`/`DefaultDir` (the config *directory*, refactored out of
  `ResolvePath`/`DefaultPath` so both config.toml and the credentials file resolve against the exact
  same directory-resolution rules) and `ProviderNames()` (see above).

### `internal/llm` — the `KeyResolver` decoupling seam

- **`KeyResolver func(provider string) (string, error)`** + **`Option`**/**`WithKeyResolver`**:
  `New(cfg, opts ...Option)` — variadic, so every pre-FAZ-8 `New(cfg)` call site (this package's own
  test suite included) compiles and behaves identically, using `resolveAPIKey` (env-only) as the
  default resolver. `buildProvider` now takes the resolver as a parameter instead of calling
  `resolveAPIKey` directly. This keeps the dependency arrow `cli -> {llm, secrets}` — `internal/llm`
  never imports `internal/secrets`.
- **`ResolveEnvKey`/`ProviderEnvVars`**: exported forms of the previously-unexported
  `resolveAPIKey`/`providerEnvVars`, so `internal/cli`'s secrets-backed resolver can delegate to the
  exact same env-only fallback logic, and `auth status` can display (never resolve from) the env var
  names it checks.
- **`ListOllamaModels`/`ListOpenAICompatModels`**: package-level wrappers around each connector's
  (new, for openai_compat) `ListModels` method, for `comrade config models`'s live queries — no
  `llm.Client`/API key needed for Ollama.
- **`KnownAnthropicModels`/`KnownGoogleModels`** + `AnthropicModelsDocsURL`/`GoogleModelsDocsURL`: a
  static, hand-maintained snapshot (matching the existing `defaultAnthropicModel`/`defaultGoogleModel`
  constants) for the two providers with no public unauthenticated "list models" endpoint this CLI
  queries.
- **Ollama reachability**: `wrapOllamaReachabilityError` recognizes a `*url.Error` (the shape
  `*http.Client.Do` returns for a connection refused/timeout/DNS failure) inside any error from
  `doRequest`/`ListModels` and replaces it with an actionable message — UYGULAMA_PLANI.md FAZ 8 item
  5's "Ollama çalışmıyor görünüyor; `ollama serve` ..." — applied at the one shared `doRequest` call
  site, so `Complete`, `Stream`, and `ListModels`/`comrade config models` all get it for free.

### `internal/cli` — `comrade auth`, `comrade config models`, resolver wiring

- **`secretsstore.go`**: `newSecretsStore(stderr) (secrets.Store, error)` (resolves
  `config.DefaultDir()` + `"credentials"`) and `secretsKeyResolver(store) llm.KeyResolver` — checks
  `store.Get` first, falls through to `llm.ResolveEnvKey` only when the store found nothing (or
  itself errored). `runtime.go`'s `setupCLIRuntime` (shared by `do`/`fix`) and `config.go`'s
  `test-llm` now both wire this resolver into every `llm.Client` they build — the actual resolution
  order changed by this phase, `comrade do`/`comrade fix`/`comrade config test-llm` included.
- **`auth.go`**: `comrade auth login/logout/status`.
  - `login <provider>`: rejects `ollama` and any unknown provider before prompting; reads the key via
    an injectable `passwordReader` (production value `golang.org/x/term.ReadPassword`; tests inject a
    fake so no real TTY is needed); trims and rejects an empty key; **stores the key, then** sends a
    single scoped test completion (`pingProvider` — a one-off `llm.Client` built from the user's real
    effective config, with `Fallback` cleared and the just-entered key wired via a constant
    `KeyResolver`, so it pings the right `base_url` but never touches the resolver chain or the store
    a second time) and reports success (model + latency) or failure without treating a failed ping as
    a command error.
  - `logout <provider>`: `store.Delete`; reports "no stored key" (not an error) on
    `secrets.ErrNoCredential`.
  - `status`: `store.Status()` for `keychain`/`file`/`none`, falling back per-provider to
    `llm.ProviderEnvVars` for an `env: <VAR>` label when the store has nothing — never printing a key
    value anywhere.
- **`models.go`**: `comrade config models` — `fetchModelsForProvider` dispatches on `cfg.LLM.Provider`
  (static list for anthropic/google, `secretsKeyResolver`-backed live query for openai_compat, live
  `/api/tags` for ollama), prints a numbered menu (+ a docs-link note for the static-snapshot
  providers), reads one line from stdin (`readModelChoice` — single-shot, not a re-prompt loop: FAZ 8
  item 4 explicitly leaves the UX open and flags "easier to test" as the deciding factor), and
  persists the choice via `loader.SetAndSave("llm.model", ...)`.

## Decisions

- **Login stores the key even when the ping fails.** An offline user, a transient provider outage, or
  a slightly-wrong-but-fixable `base_url` must not be blocked from saving a key they believe is
  correct. The ping's only job is to report a signal, not to gate persistence — the printed message
  makes clear the key *is* stored and the failure might be transient, the key, or the network.
- **`KeyResolver` is a function type threaded through `New`'s variadic `opts`, not a new constructor.**
  This preserves every pre-FAZ-8 `New(cfg)` call site (including this package's entire existing test
  suite) unchanged, while giving `internal/cli` a seam to wire in keychain/file resolution ahead of
  the env-only default — without `internal/llm` ever importing `internal/secrets`.
- **`secrets.KnownProviders` is derived from `config.ProviderNames()` minus `"ollama"`, not
  hand-copied.** `config.ProviderNames()` is a one-line addition exporting `keyDefs`'s existing
  `llm.provider` enum; a hand-copied literal list in `internal/secrets` would have been an unguarded
  mirror the moment `comrade config set llm.provider`'s enum ever changed.
- **`comrade config models`'s selection prompt is single-shot (error on an invalid choice), not a
  re-prompt loop.** The task item explicitly leaves this open and flags a plain numbered prompt as
  "easier to test" than a bubbletea list; a single `bufio.Reader.ReadString('\n')` plus `strconv.Atoi`
  is fully deterministic to test via `cmd.SetIn` and needs no extra UI dependency.
- **Anthropic/Google's model lists are a static, docs-linked snapshot, not a live query.** Neither
  provider exposes a public, unauthenticated "list models" endpoint this CLI can call the way Ollama's
  `/api/tags` or an OpenAI-compatible `/models` can; the printed docs URL (`AnthropicModelsDocsURL`/
  `GoogleModelsDocsURL`) is the explicit acknowledgment that this list can go stale.
- **The file fallback's "not encrypted" warning lives in two places for two different audiences**: a
  one-time stderr message (for whoever is running the command right now) and a permanent header
  comment inside the file itself (for whoever opens the file directly, possibly much later, having
  never seen the stderr warning).

## Manual verification (real binary, real headless-sandbox keychain probe, mock Ollama server)

Built via `make build`; run with `HOME`/`XDG_CONFIG_HOME` pointed at a fresh temp directory per the
usual isolation pattern. This sandbox has no reachable D-Bus Secret Service, so every run below
exercises the **real** `detectKeychainAvailable()` probe hitting the **real** (not mocked)
`secretServiceProvider`, which genuinely fails here — proving the file-fallback path end-to-end, not
just under `keyring.MockInitWithError`.

**`comrade auth status`, fresh, no keys anywhere:**

```
$ ./comrade auth status
cli-comrade: no OS keychain available on this machine; storing API keys base64-obfuscated (NOT encrypted) in a 0600 file instead. See that file's header comment for details.
PROVIDER       STATUS
anthropic      not set
openai_compat  not set
google         not set
ollama         (no key required)
```

**`comrade auth status` reflecting an environment-variable key (no keychain/file entry at all):**

```
$ ANTHROPIC_API_KEY=sk-demo-env-value ./comrade auth status
cli-comrade: no OS keychain available on this machine; ...
PROVIDER       STATUS
anthropic      set (env: ANTHROPIC_API_KEY)
...
```

No `credentials` file was created by this run — an env-only resolution never touches the store.

**`comrade auth login` without a real TTY** (this sandbox has none; the interactive flow itself is
proven by `TestAuthLoginStoresKeyAndReportsPingSuccess`/`...PingFails`/`...RejectsEmptyKey` with an
injected fake `passwordReader` — see Gate below):

```
$ echo "some-key" | ./comrade auth login anthropic
Enter API key for anthropic: auth login: read key: inappropriate ioctl for device
```

This is the expected, documented failure mode of `golang.org/x/term.ReadPassword` against a non-TTY
stdin — exactly why the password reader is injectable in tests instead of exercised for real here.

**`comrade config models` against a live mock Ollama `/api/tags`** (a standalone `http.server`
returning two model names):

```
$ ./comrade config set llm.provider ollama
$ ./comrade config set llm.ollama.base_url http://127.0.0.1:8934
$ printf "2\n" | ./comrade config models
1) llama3.1
2) mistral
Select a model number: llm.model = mistral

$ ./comrade config get llm.model
mistral
```

**`comrade config models` against an unreachable Ollama** (same provider/base_url pointed at a port
nothing listens on):

```
$ ./comrade config models
config models: ollama does not appear to be running at http://127.0.0.1:19999; start it with `ollama serve`, or set llm.ollama.base_url to the correct address (ollama: request: Get "http://127.0.0.1:19999/api/tags": dial tcp 127.0.0.1:19999: connect: connection refused)
```

## Gate

- `go vet ./...` — clean.
- `/home/firfir/go/bin/golangci-lint run ./...` — 0 issues.
- `gofmt -l .` — clean (no diffs).
- `go test ./... -count=1` — all packages pass, including:
  - `internal/secrets` (new): 17 test functions — keychain Set/Get/Delete/Status round trips (via
    `keyring.MockInit()`), file-fallback round trips (via `keyring.MockInitWithError`), the 0600
    permission test (`TestFileFallbackCreatesFileWith0600Permissions` — runs under `t.TempDir()`,
    which resolves to native `/tmp` on this WSL2 sandbox, not the `/mnt/c` DrvFs mount this checkout
    lives under, so the assertion observes real POSIX bits, not DrvFs's synthetic always-777; skipped
    on windows, where POSIX permission bits aren't meaningful), permission-repair-on-read, base64
    round-trip + not-plaintext-on-disk, the one-time-warning test, corrupt-value error handling, and
    `detectKeychainAvailable`'s three branches.
  - `internal/config`: `TestProviderNamesMatchesLLMProviderEnum`, `TestResolveDir*`/
    `TestResolvePathIsResolveDirPlusConfigFileName` (new).
  - `internal/llm`: `TestNewWithKeyResolver*` (2), `TestResolveEnvKey*`/`TestProviderEnvVars*`/
    `TestKnown*Models*` (7, new `keys_test.go`), `TestOllamaCompleteUnreachable*`/
    `TestListOllamaModels*` (3, reachability + wrapper), `TestOpenAICompatListModels*`/
    `TestListOpenAICompatModels*` (4).
  - `internal/cli`: `auth_test.go` (12 tests — login reject/store/ping-success/ping-fail/empty-key,
    logout remove/no-op, status not-set/env/keychain-precedence/file-source/never-leaks-key-values),
    `models_test.go` (7 tests — anthropic/google static, ollama live + unreachable, openai_compat live
    via env key, out-of-range, non-numeric), `secretsstore_test.go`
    (`TestSecretsKeyResolverPrecedence`, table test: store beats `COMRADE_*` env beats vendor env beats
    missing).
- `go test ./internal/secrets/... ./internal/cli/... -race -count=1` — clean, no data races.
- `make build` / `make cross` (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64) —
  all five succeed; go-keyring's per-OS files (`keyring_unix.go` for linux — no cgo required on this
  build-tag combination —, `keyring_darwin.go`, `keyring_windows.go` + `danieljoos/wincred`) all
  cross-compiled cleanly with `CGO_ENABLED` unset (the default for cross-compilation).
- No key value ever appears in any command's stdout/stderr:
  `TestAuthStatusNeverPrintsKeyValues` (a sentinel key set via both the store and an env var, then
  `auth status`'s full stdout+stderr asserted not to contain it) and
  `TestAuthLoginStoresKeyAndReportsPingSuccess`'s own `assert.NotContains(..., "sk-test-key-123")`.

## Dependencies

- `github.com/zalando/go-keyring v0.2.8` (MIT) — pulls `github.com/godbus/dbus/v5 v5.2.2` (BSD-3, Linux
  Secret Service) and `github.com/danieljoos/wincred v1.2.3` (MIT, Windows Credential Manager) as
  transitive, platform-gated dependencies.
- `golang.org/x/term v0.44.0` (BSD-3) — the newest version whose registry publish date is ≥ 15 days
  before today (2026-06-08; `v0.45.0` was published 2026-07-08, one day before this phase, and was
  skipped per this project's version-selection rule).
- Both added as direct requires in `go.mod`; `go.sum` updated via `go mod tidy` (diff reviewed —
  exactly these two direct deps plus their two expected transitive, platform-specific dependencies).

## Risks / follow-ups

- **`comrade auth login`'s real interactive flow (a genuine TTY) was not exercised against the actual
  compiled binary** in this phase — `golang.org/x/term.ReadPassword` fundamentally requires a real
  terminal file descriptor, which no CI/sandbox shell provides. The injectable `passwordReader` seam
  exists specifically so the login flow (prompt → store → ping → report) is still proven end-to-end,
  just with a fake reader standing in for the TTY read. This is a standard, unavoidable limitation for
  any no-echo terminal prompt, not something specific to this implementation.
- **The static Anthropic/Google model lists will go stale.** They are hand-maintained, matching this
  codebase's existing `defaultAnthropicModel`/`defaultGoogleModel` constants as of 2026-07; the printed
  docs link is the explicit, permanent acknowledgment of that staleness risk rather than a promise to
  keep them current automatically.
- i18n (FAZ 9) will need to route every new hardcoded string this phase added (`auth`'s
  prompts/messages, `config models`'s menu/prompt text, the ollama-reachability message) through the
  eventual catalog, exactly like every other phase's hardcoded English strings.
- `comrade explain`/`comrade chat` remain FAZ 9 stubs, untouched by this phase.
