# FAZ 03 — Context Collector + Redaction

## What was delivered

### `internal/context` — environment context collector

- `Collector`: every OS/exec/env dependency (`GOOS`, `Getenv`, `LookPath`,
  `RunCommand`, `Geteuid`, `Getwd`, `UserHomeDir`, `Environ`) is a struct
  field, not a package-level global or a direct `os.*`/`runtime.GOOS`
  call inside the collection logic — so every branch, including the
  windows-only ones, is exercised in tests regardless of the OS the test
  binary actually runs on. `NewCollector()` wires the real OS.
- `Collect(ctx, Options) Context` never returns an error: every
  sub-collection degrades to its zero value on failure instead of
  aborting the whole snapshot (a missing shell version or unreadable
  history file is a shrug, not a fatal error for `comrade fix`/`do`).
- **Shell detection** (`DetectShell`): unix — basename of `$SHELL`;
  windows — `PSModulePath` present ⇒ `"powershell"`, else `"cmd"`, per
  CLAUDE.md's stated heuristic.
- **Shell version** (`ShellVersion`): best-effort `<shell> --version` (or
  PowerShell's `$PSVersionTable.PSVersion.ToString()`), first output line
  only, under a 500ms timeout derived from the caller's `ctx`. Any
  failure — unknown shell, missing binary, non-zero exit, timeout — is
  silent (`""`), never an error.
- **Admin detection** (`IsAdmin`): unix is exact (`geteuid()==0`,
  `known=true`). **Windows is honestly `(false, false)`** — reliable
  elevation detection needs the Windows token API
  (`OpenProcessToken`/`GetTokenInformation`), which would require a new
  go.mod dependency (e.g. `golang.org/x/sys/windows`) not currently in
  this project's stack for a single best-effort signal. `known=false`
  means "not checked", never "confirmed non-admin" — FAZ 5/6's
  elevated/destructive risk classification must not (and will not) treat
  this flag as a safety gate by itself; the safety engine's own
  denylist/risk-escalation rules are the actual gate, independent of this
  package.
- **Package manager detection** (`DetectPackageManagers`): `exec.LookPath`
  scan for `apt, dnf, pacman, zypper, brew, port, winget, scoop, choco`,
  returned in that fixed order (never PATH-scan order), so the result is
  stable across machines.
- **`last_command.json`** (`LastCommand`, `LastCommandPath`,
  `ReadLastCommand`): FAZ 3 owns and defines this file's shape —
  `{command, exit_code, stderr_tail, stdout_tail, timestamp, shell}` —
  and reads it; FAZ 4's shell hooks will be the only writer. Path:
  `$XDG_STATE_HOME/cli-comrade/last_command.json`, falling back to
  `~/.local/state/cli-comrade/last_command.json`; windows
  `%LOCALAPPDATA%\cli-comrade\last_command.json`. A missing file,
  unreadable file, or corrupt JSON all collapse to a single `ok=false` —
  callers never special-case `os.IsNotExist` vs. a JSON error.
  `LastCommand.Age(now)` reports elapsed time; the 10-minute staleness
  threshold from UYGULAMA_PLANI.md's FAZ 4 fallback chain is that future
  caller's decision, not this package's.
- **Opt-in history** (`ReadHistory`, gated by `Options.SendHistory` +
  `HistoryDepth`): bash (`~/.bash_history`, plain lines), zsh
  (`~/.zsh_history`, strips the `: <ts>:<dur>;` extended-history prefix),
  fish (`~/.local/share/fish/fish_history`, parses `- cmd: ` lines),
  PowerShell PSReadLine
  (`%APPDATA%\Microsoft\Windows\PowerShell\PSReadLine\ConsoleHost_history.txt`,
  plain lines). Best-effort per shell: an unrecognized shell or
  unreadable file returns `nil`, never an error. Only the last `depth`
  entries are returned.
- **Opt-in env names** (`EnvNames`, gated by `Options.SendEnvNames`):
  extracts sorted variable *names* from `os.Environ()`-shaped
  `"NAME=value"` strings — values are never touched or returned, per
  CLAUDE.md: "Env var içerikleri ASLA gönderilmez, sadece isimleri
  (opt-in)".
- `Options` is this package's own type (`SendHistory`, `HistoryDepth`,
  `SendEnvNames`) — it does **not** import `internal/config`. A future
  caller translates `config.ContextConfig` into `context.Options`,
  keeping this package usable independently of the config schema.

### `internal/redact` — secret masking

- `New(maskEmails, maskIPs bool) *Redactor` — **no `internal/config`
  import**, by design, so it can be imported from anywhere (most
  importantly `internal/llm`) without pulling in the config package's
  dependency graph.
- `(*Redactor).Apply(s string) string` masks, in this fixed, load-bearing
  order:
  1. **`private_key`** — full multiline PEM blocks
     (`-----BEGIN ... PRIVATE KEY----- ... -----END ... PRIVATE
     KEY-----`, any key type), first, since later single-line patterns
     could otherwise partially match inside a PEM body's base64.
  2. **credential kv** (`password=`, `passwd=`, `pwd=`, `token=`,
     `secret=`, `api_key=`, `apikey=`, case-insensitive, `=`/`:`
     separator, tagged `[REDACTED:credential]`) — before the generic
     `api_key` patterns, so `api_key=sk-XXXX` is tagged `credential`
     (keeping the key name visible) rather than the less informative
     `api_key`. Only the value is masked; the key name and separator are
     preserved as typed.
  3. **`bearer`** (`Authorization: Bearer <token>` keeps the header text;
     bare `Bearer <token>` needs an 8+ char token-shaped charset after
     it, so a lone trailing `"Bearer"` or a short following word like
     `"of"` in `"the Bearer of good news"` never false-positives) —
     before the generic `api_key` patterns, so a bearer token that
     happens to look like an API key keeps its bearer framing.
  4. **`api_key`** — `sk-`, `ghp_`, `gho_`, `AKIA`, `xox[baprs]-`, each
     with a leading `\b`. This is what keeps `sk-` from firing inside
     `"risk-free..."`: `\b` requires a word/non-word transition, and
     `"risk-free"`'s embedded `"sk-"` is preceded by the word character
     `"i"`, so no boundary exists there — no length threshold or
     explicit exclusion list needed.
  5. **`jwt`** — standalone three-segment `eyJ...` tokens not already
     consumed by bearer or credential-kv above.
  6. **`email`**, **`ip`** — optional, gated by the two constructor
     flags. IPv4 and a "simple" IPv6 form are covered;
     `127.0.0.1`/`0.0.0.0`/`::1` are **never** masked even when IP
     masking is enabled — redacting them would be noise and would break
     the Ollama "it's running on localhost" guidance the LLM needs to
     give.
- `Apply` is **idempotent**: none of the patterns match their own
  `[REDACTED:...]` output, proven by a dedicated test running `Apply`
  twice over a string hitting every mandatory + optional pattern and
  asserting the two results are equal.
- UTF-8 safety is inherited from Go's `regexp`/`ReplaceAllString`
  operating on the string as-is (no manual byte slicing); verified by a
  golden test mixing Turkish prose with a real secret.
- **Post-review fix**: `credentialKVPattern`'s value group now matches
  `"[^"]*"|'[^']*'|[^\s,;)}]+` instead of a plain `\S+`. A bare `\S+`
  value stops at the first whitespace, so `password="a b c"` only
  masked `"a` and leaked ` b c"` onto the wire — an independent review
  caught this. The value alternation now tries a double-quoted string,
  then a single-quoted string, then a delimiter-bounded bare token (the
  bare branch excludes trailing `,;)}` — but deliberately *not* `]`,
  since the `[REDACTED:credential]` marker itself ends in `]` and
  excluding it would leave a stray `]` behind on a second `Apply` pass;
  see `redact.go`'s pattern comment for the full idempotency trace).
  Golden tests added: `password="a b c"`, `token='x y'`, a
  quote-immediately-followed-by-comma round-trip proving the fix is
  idempotent, plus the existing false-positive suite (`tokens=5`, etc.)
  re-verified green.

### Non-bypassable middleware wiring (`internal/llm`)

- `Client` gained an unexported `redactor *redact.Redactor` field, set
  **only** inside `New(cfg)` from `cfg.Privacy.RedactEmails`/
  `cfg.Privacy.RedactIPs`. There is no exported setter and no way to
  construct a `Client` from outside this package other than `New(cfg)`
  (per FAZ 2's encapsulation), so an external caller cannot pass a
  nil/no-op redactor in — the field simply isn't reachable.
- `Client.Complete` and `Client.Stream` — the sole entry points into this
  package's connectors, since the connector constructors are unexported
  (FAZ 2) — both call a new `redactPayload(req)` unconditionally as their
  first action, before anything else touches `req`. It returns a copy of
  `req` with `System` and every `Message.Content` passed through
  `c.redactor.Apply`. Combined with the connectors' unexported
  constructors, there is no compile-reachable path from a caller of this
  package to a connector's wire call that skips redaction.
- **Post-review hardening**: `redactPayload` now treats `c.redactor ==
  nil` as **fail-closed**, not a no-op — it lazily assigns
  `redact.New(false, false)` so every mandatory pattern family
  (`api_key`/`jwt`/`private_key`/`credential`/`bearer`) still applies
  even on the internal struct-literal construction path this package's
  own fallback-chain tests use to bypass `New` and stub connectors
  directly. A `*Client` can now never send an unredacted payload
  regardless of how it was constructed, not just via the public API.
- **Proof tests**: `TestClientCompleteRedactsPayloadBeforeReachingConnector`
  (`internal/llm/client_redact_test.go`) builds a real `Client` via
  `New(cfg)` against an `ollama` provider pointed at an `httptest` server
  that captures the raw outgoing request body. The payload's `System`
  carries a fake `api_key=sk-...` and a message carries `password=...`;
  the test asserts the captured body contains `[REDACTED:credential]`
  and does **not** contain either raw secret, while an email in the same
  payload survives intact when `redact_emails=false` — proving both the
  bypass-proof wiring and that the config flag is actually honored
  end-to-end. A companion test flips `redact_emails=true` and asserts the
  email is masked too.
  `TestClientStreamRedactsPayloadBeforeReachingConnector` mirrors this
  for `Client.Stream`, driving a real `openai_compat` connector against
  an `httptest` server speaking actual Server-Sent Events (`data: ...`
  frames + the `[DONE]` sentinel): the channel is drained to completion,
  then the single captured request body is asserted to contain
  `[REDACTED:credential]` for both the `System` and message secrets with
  neither raw secret present.

## Decisions / deviations

- **Windows admin detection kept honest, not approximated.** The task
  spec explicitly rejected heuristics like "try writing to an
  admin-only path" as unacceptable. Rather than adding a new dependency
  for one best-effort signal, `IsAdmin` on windows always returns
  `(false, false)` — the `known` bool is the load-bearing part of the
  contract, and every caller must check it before treating `isAdmin` as
  meaningful.
- **`internal/context`'s `Options` type does not import `internal/config`.**
  Only `internal/redact` was explicitly required to stay
  config-independent, but the same reasoning applies to `context`: it
  keeps the collector testable and reusable without pulling in viper's
  dependency graph. The FAZ 5/6 caller that wires `config.Config` into
  this collector will do the two-field translation itself.
- **`credentialKVPattern` intentionally shadows `apiKeyPatterns` for
  `key=value` forms.** E.g. `api_key=sk-XXXX` ends up tagged
  `[REDACTED:credential]`, not `[REDACTED:api_key]` — this was a
  judgment call favoring "the key name told us it was a credential" over
  "the value's shape told us it was an API key" when both signals are
  present; see `redact.go`'s `Apply` doc comment for the full ordering
  rationale.
- **IPv6 pattern is deliberately simple**, per the task's own wording —
  it covers common full/compressed hextet forms plus the bare `::1`
  loopback, not every RFC 4291 shorthand. Sufficient for the "don't leak
  a real internal address" grounding use case; not a general-purpose
  IPv6 validator.

## Gate

- `go vet ./...` — clean.
- `/home/firfir/go/bin/golangci-lint run` — 0 issues.
- `go test ./... -count=1` — all packages pass (114 new test functions
  across `internal/context`, `internal/redact`, and the two new
  `internal/llm` middleware-proof tests; every other existing package's
  suite unaffected).
- `make build` — succeeds.
- `make cross` — succeeds (linux/amd64, linux/arm64, darwin/amd64,
  darwin/arm64, windows/amd64), proving `internal/context` and
  `internal/redact` compile clean on all three target OSes.
