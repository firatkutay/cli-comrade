# FAZ 11 — Sertleştirme ve Sürüm Adayı (v0.1.0-rc1)

## What was built

FAZ 11 is the release-candidate quality pass: no new product features,
just proving robustness (E2E scenarios, fuzz/edge hardening), fixing a
real cold-start performance bug it found along the way, tightening the
lint/vuln gate, and preparing (not cutting) the `v0.1.0-rc1` release.

## 1. End-to-end scenario tests

UYGULAMA_PLANI.md FAZ 11 item 1 names three Ubuntu/Linux scenarios (all
runnable in this sandbox), and three macOS/Windows scenarios (not
runnable here — no macOS/Windows host). The table below is honest about
which is which: every "Automated" row names a real, passing test; every
"Manual" row says so and gives the exact command a user must run on that
platform.

### Automated (this sandbox, Linux/Ubuntu)

| # | Scenario | Test | Result |
|---|---|---|---|
| 1 | apt-style "command not found" → `fix --info` suggests `apt-get install`, nothing runs | `internal/cli.TestFAZ11AptCommandNotFoundFixFlowInfoModeSuggestsInstallWithoutRunning` | PASS |
| 2 | "port already in use" → `fix --info` suggests find+free, nothing runs | `internal/cli.TestFAZ11PortAlreadyInUseFixFlowInfoModeSuggestsFreeingThePort` | PASS |
| 3 | "nginx kur ve başlat" in `--auto`: benign step runs, denylisted decoy step is Blocked | `internal/cli.TestFAZ11InstallAndStartNginxAutoModeRunsBenignStepAndBlocksDenylistedStep` | PASS |
| 3b | same shape at the `fix` layer, `--auto` (the one mode/decoy combination FAZ 7's own tests hadn't covered) | `internal/cli.TestFAZ11LLMSuggestsDenylistCommandBlockedAtFixLayerInAutoMode` | PASS |

Each of these builds the mock LLM response, drives the real `comrade`
cobra command tree (`execRootSplit`), and — for the `--auto` cases — runs
the benign step against the REAL `internal/executor` (not a fake),
confirmed independently via the audit log.

Pre-existing FAZ 6/7 coverage this phase did NOT need to duplicate (the
underlying mechanism is already proven, just not under a FAZ-11-specific
name): `TestDoDryRunRendersPlanTableAgainstMockProvider` and
`TestDoAutoModeRunsBenignStepAndBlocksDenylistedStepAgainstRealExecutor`
(do layer), `TestFixInfoModeSurfacesRootCauseExplanationAndPlan`,
`TestFixAutoModeExecutesPlanAndRunsPostSolutionVerification`, and
`TestFixAskModeRoutesBlockedStepToRunnerAndSkipsVerificationForDestructiveOriginal`
(fix layer) — plus, at the engine layer where `PromptUI` is
dependency-injected and testable without a real TTY,
`TestExecuteAutoForcesConfirmOnElevated` and
`TestExecuteAutoBypassesElevatedConfirmOnlyWithConfigAndYolo` (the latter
uses `sudo systemctl restart nginx` as its own example command) prove
the "elevated/destructive forces CONFIRM even in auto" half of item 1's
nginx scenario.

### Manual — deferred to platform (macOS / Windows)

These genuinely cannot run in this Linux sandbox (no macOS/Windows host,
no Homebrew/winget/PowerShell binary here). Marked manual, not faked.

| Scenario | Platform | Exact command to run | Expected behavior |
|---|---|---|---|
| Homebrew error (e.g. a formula install failure) | macOS | `comrade fix` right after a failed `brew install <formula>` | `comrade` diagnoses the brew error (permission on `/usr/local`, a missing cask tap, a checksum mismatch, etc.) and offers a fix plan; `--info` prints it without running anything |
| File-permission error | macOS | `comrade fix` after e.g. `cp foo /usr/local/bin/` fails with `Permission denied` | Diagnosis names the permission issue and suggests `sudo` or `chmod`/ownership fix, correctly classified `elevated`/`write` by `internal/safety` |
| `ExecutionPolicy` error | Windows (PowerShell) | `comrade fix` after a `.ps1` script fails with `... cannot be loaded because running scripts is disabled on this system` | Diagnosis names the ExecutionPolicy restriction and suggests `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned` (or equivalent), classified `elevated` |
| winget install | Windows | `comrade winget ile <paket> kur` (free-text dispatch to `do`) | Plan step uses `winget install <package>`, classified `write`/`elevated` per `internal/safety`'s package-manager-install escalation rule (already unit-tested against `winget` specifically — see `internal/safety/engine_test.go`'s `TestEvaluateEscalationPackageInstall`) |
| PATH problem | Windows | `comrade fix` after `'<tool>' is not recognized as an internal or external command` | Diagnosis suggests the correct install command AND/OR a PATH fix (`$Env:PATH` / System Properties), in `--info` mode first |

These need a real macOS/Windows host to actually exercise the OS-specific
error text, package managers, and `internal/executor`'s
`powershell -NoProfile -Command` branch end-to-end — the user should run
each once per platform before/around a real `v0.1.0` release. Golden/
unit-level coverage of the underlying mechanics (PowerShell command
building, package-manager escalation rules, Windows process-tree kill)
already exists from earlier phases and is unaffected by this gap.

## 2. Fuzz / edge hardening

| Case | Test | Result |
|---|---|---|
| ~1.28MB of stdout (far beyond `maxCaptureBytes`) | `internal/executor.TestFAZ11RunCapsOneMegabyteOfStdoutWithoutUnboundedGrowth` | PASS — `Result.Stdout` stays ≤ 8KB, tail preserved, head truncated; the live-streamed copy is NOT capped (proving the cap is specific to `Result`) |
| Invalid UTF-8 byte sequence in captured stdout | `internal/executor.TestFAZ11RunNonUTF8StdoutDoesNotPanic` | PASS — no panic, captured bytes pass through as a normal (if non-UTF-8) Go string |
| Invalid UTF-8 byte sequence through the redaction pipeline | `internal/redact.TestFAZ11ApplyInvalidUTF8BytesDoesNotPanic` | PASS — no panic, secrets on either side of the invalid bytes are still masked (complements FAZ 3's `TestApplyUTF8Safe`, which only covers well-formed multi-byte UTF-8, a different concern) |
| Offline / unreachable cloud provider → friendly message | `internal/llm.TestFAZ11AnthropicUnreachableProducesFriendlyOfflineError`, `...OpenAICompatUnreachableProducesFriendlyOfflineError`, `...GoogleUnreachableProducesFriendlyOfflineError` | PASS — a transport-level failure (`*url.Error`) is replaced with a message naming the provider, classified via `errors.Is(err, ErrOffline)`, never a bare `dial tcp: ...` string |
| Whole fallback chain offline → suggest Ollama | `internal/llm.TestFAZ11ClientSuggestsOllamaFallbackWhenWholeChainIsOffline` | PASS |
| ...but not when Ollama is already configured | `internal/llm.TestFAZ11ClientDoesNotSuggestOllamaWhenAlreadyConfigured` | PASS |
| ...and not for a non-offline failure (a real 400 response) | `internal/llm.TestFAZ11ClientDoesNotSuggestOllamaForNonOfflineFailure` | PASS |
| LLM suggests a denylisted command → Blocked, `do` layer | `internal/cli.TestFAZ11InstallAndStartNginxAutoModeRunsBenignStepAndBlocksDenylistedStep` (+ pre-existing `TestDoAutoModeRunsBenignStepAndBlocksDenylistedStepAgainstRealExecutor`) | PASS |
| LLM suggests a denylisted command → Blocked, `fix` layer | `internal/cli.TestFAZ11LLMSuggestsDenylistCommandBlockedAtFixLayerInAutoMode` (+ pre-existing `TestFixAskModeRoutesBlockedStepToRunnerAndSkipsVerificationForDestructiveOriginal`) | PASS |

### Bugs this hardening pass found and fixed (root cause, not worked around)

None of the fuzz/edge cases above surfaced a bug — the 8KB tail cap,
UTF-8 tolerance, and denylist enforcement were all already correct from
earlier phases. **The offline/no-network handling itself WAS a real gap**,
not a pre-existing bug but a genuinely missing behavior FAZ 11 item 2
asked for: `anthropic`/`openai_compat`/`google` had no reachability
wrapping at all (only `ollama` did, from FAZ 8) — a real network outage
against a cloud provider surfaced Go's raw `*url.Error` text
(`Post "https://api.anthropic.com/v1/messages": dial tcp: lookup
api.anthropic.com: no such host`) instead of an actionable message, and
there was no "try Ollama instead" suggestion anywhere. Fixed in
`internal/llm/errors.go` (new `ErrOffline` sentinel + shared
`wrapReachabilityError`, wired into all three cloud connectors'
`doRequest`) and `internal/llm/client.go` (`Client.Complete`/`Stream`'s
new `finalChainError` appends the Ollama suggestion exactly when the
whole chain failed offline and Ollama isn't already one of the
configured attempts).

## 3. Performance: cold start

### The bug this run found and root-cause-fixed

Measuring `comrade --version`/`--help`/`config path` (all three do NO
LLM call) initially showed **~600ms**, two orders of magnitude over the
<100ms target — on this repo's own native filesystem (`/tmp`, ext4, not
`/mnt/c` DrvFs), ruling out the documented DrvFs I/O caveat as the cause.

Root cause, found via `GODEBUG=inittrace=1`: `github.com/atotto/
clipboard` v0.1.4 (a transitive dependency of `charm.land/bubbles/v2/
textinput`, used by `internal/tui`'s confirm prompt AND `comrade chat` —
i.e. reachable from every command, not just chat) runs its Unix build's
`func init()` unconditionally at package-load time: up to five
sequential `exec.LookPath` calls (`xclip`, `xsel`, `termux-clipboard-*`
×2, `clip.exe`, `powershell.exe`) to detect which clipboard tool is
available, **before `main()` even starts** — paid by every `comrade`
invocation, whether or not it ever touches the clipboard. On this
sandbox's own WSL2 shell, `$PATH` has ~124 entries, 69 of them
Windows-side directories reached over the 9p/DrvFs bridge (Git for
Windows, PowerShell, Python launchers, etc.) — each `LookPath` scan
walks the whole `PATH` until it finds (or exhausts) the binary name, so
five scans × a mostly-slow-per-entry `PATH` cost ~600ms, confirmed via
`strace`/`GODEBUG=inittrace=1` isolating exactly this one `init()` call
at `github.com/atotto/clipboard @2.2ms, 600ms clock`.

No newer upstream release exists to pick up a fix (`v0.1.4` is
`atotto/clipboard`'s latest tag; `charm.land/bubbles/v2` is already
pinned at its own latest, `v2.1.1`, which still imports it). The
`windows`/`darwin`/`plan9` builds of this same package do NOT have this
problem (no `init()`-time `exec.LookPath` loop there) — only the Unix
build does.

**Fix**: a locally vendored, behavior-preserving fork
(`third_party/atotto-clipboard/`, wired in via a `go.mod` `replace
github.com/atotto/clipboard => ./third_party/atotto-clipboard`) whose
only change (`clipboard_unix.go`) is moving that same probing logic out
of `func init()` and into a `sync.Once`-guarded `detectCommands()`
called lazily from `getPasteCommand`/`getCopyCommand`/`readAll`/
`writeAll` — the exact same detection, run at most once, but only on
first *actual* clipboard use instead of unconditionally at process
start. Public API (`Primary`, `Unsupported`, `ReadAll`, `WriteAll`) is
byte-for-byte identical; `windows`/`darwin`/`plan9` build files are
copied verbatim (untouched) so cross-compilation still needs them for
`make cross`'s other 4 targets. BSD license/copyright notice preserved
in the vendored `LICENSE` file.

### Measured numbers

Before (this sandbox, native `/tmp` ext4 — NOT `/mnt/c` DrvFs):

```
$ time /tmp/comrade-test --version >/dev/null
real  0m0.602s
$ time /tmp/comrade-test --help >/dev/null
real  0m0.601s
$ time /tmp/comrade-test config path >/dev/null
real  0m0.599s
```

After the `third_party/atotto-clipboard` fix:

```
$ time /tmp/comrade-final --version >/dev/null
real  0m0.005s
$ time /tmp/comrade-final --help >/dev/null
real  0m0.004s
$ time /tmp/comrade-final config path >/dev/null
real  0m0.004s
```

**~4-5ms — well under the <100ms target**, a ~130x improvement. Also
proven as a regression backstop (not a re-statement of the precise
target, which would flake on a loaded/shared CI runner):
`internal/cli.TestFAZ11ColdStartStaysWellUnderOneSecond` builds the real
binary and asserts `--version`/`--help`/`config path` each stay under a
deliberately generous 500ms ceiling — comfortably above normal CI
variance, comfortably below the ~600ms-class regression this run found
and fixed.

Other candidates the task asked to check were already fine, verified by
reading the code (no eager execution for these commands):
`newLoader` in `internal/cli/root.go` is a closure, resolved once per
subcommand invocation — never called at all for a bare `--version`/
`--help` flag (cobra handles both without invoking `RunE`); `config
path` calls it but only to read a path string, no full config parse;
the package-manager `LookPath` scan and context collection
(`internal/context.Collector`) only run inside `do`/`fix`'s own
pipeline, never from `--version`/`--help`/`config`; the update-check
(`maybeNotifyUpdate`) is `PersistentPostRunE`, runs AFTER a command's
own `RunE` returns successfully (not before), is skipped outright for a
`dev` build (`update.IsDevBuild`), and is bounded to 3s only when a
check is actually due (throttled to at most once per `CheckInterval`).

### Distribution consequence of the fix: `go install @version` is no longer usable

The fork fix's `replace github.com/atotto/clipboard =>
./third_party/atotto-clipboard` directive in `go.mod` has one real
downstream effect: `go install github.com/firatkutay/cli-comrade/cmd/
comrade@<version>` (the by-module-path-and-version form, with no
main-module context) is no longer supported. Verified against Go's own
reference (go.dev/ref/mod) and reproduced directly against this
project's own toolchain (go1.26.5) rather than assumed: `go install`'s
`@version` form requires "the module containing packages named on the
command line ... must not contain directives (`replace` and `exclude`)
that would cause it to be interpreted differently if it were the main
module" — and `replace` directives "only apply in the main module's
go.mod file and are ignored in other modules." A minimal repro (a
throwaway module with a local-path `replace`, served through a
hand-built file-based module proxy) confirms Go **hard-errors, it does
not silently ignore the replace**:

```
$ GOPROXY="file://.../proxy" GOSUMDB=off go install "example.com/mainmod/cmd/foo@v0.0.1"
go: example.com/mainmod/cmd/foo@v0.0.1 (in example.com/mainmod@v0.0.1):
	The go.mod file for the module providing named packages contains one or
	more replace directives. It must not contain directives that would cause
	it to be interpreted differently than if it were the main module.
```

So the fix is safe either way — there is no silent-degradation failure
mode where a user's `go install @version` "succeeds" without the
cold-start fix; it fails loudly and immediately instead. `docs/
INSTALL.md`'s "Kaynaktan derleme"/"Build from source" sections (both
TR and EN) and `README.md`'s cross-references were updated to stop
advertising `go install ...@latest` and instead show the working
alternative — `git clone` + `go build -o comrade ./cmd/comrade` (or
`go install ./cmd/comrade` run from inside the checkout) — which DOES
honor the local `replace`, because the checkout itself is the main
module in that invocation. `KNOWN_LIMITATIONS.md` gained a matching
bilingual entry.

**The packaged distribution paths are unaffected**, confirmed by
actually running them, not just reasoning about them: goreleaser builds
every archive/`.deb`/`.rpm`/brew-cask/scoop/winget artifact by running
an ordinary `go build` from inside this checkout (never `go install
@version`), so the main module's own `replace` directive applies
normally. Proof: `goreleaser build --single-target --snapshot --clean`
(the same v2.16.0 binary already pinned) succeeds, and the resulting
binary's `--version` runs in ~3-4ms — the fast path, not the ~600ms
upstream-`init()` path — directly confirming the vendored fork was
linked into the actual goreleaser-built artifact, not just the
`make build`/`make cross` outputs already measured above.

## 4. Strict `golangci-lint` profile + `govulncheck`

### New linters added (`.golangci.yml`)

On top of the existing `misspell`/`unconvert`/`unused`/`ineffassign`:
`gosec`, `errorlint`, `gocritic`, `revive`, `bodyclose`, `noctx`,
`unparam`, `prealloc` — a curated set (not `enable-all`), each with a
one-line justification in the config for why it's relevant to THIS
project (raw shell exec, raw HTTP connectors, CLAUDE.md's `%w`-wrapping
mandate). Two narrow test-file exclusions, also justified inline:
`gosec` (test fixtures deliberately construct the exact denylisted
commands `internal/safety` exists to catch) and `noctx` (a few
`httptest`-server-only request builders where a context is genuinely
immaterial).

**35 issues surfaced, all fixed** (0 remaining, confirmed with
`--max-issues-per-linter=0 --max-same-issues=0` to rule out the
default result-capping hiding anything):

| Linter | Count | Fix applied |
|---|---|---|
| `gosec` | 17 | Real hardening: tightened `0o755`→`0o750` directory / `0o644`→`0o600` file permissions on `audit.jsonl`, `last_command.json`, `config.toml`, and the update-check state file (all may carry command text or state a local, single-owner CLI has no reason to expose to other local users). Justified `// #nosec` exceptions (matching this codebase's existing convention) for: the binary-replacement `WriteFile`/`Chmod` calls in `internal/update` that must keep `+x` (they write the new `comrade` executable itself); `os.ReadFile`s of this process's OWN fixed XDG-state paths (shell history, `last_command.json`, update-check state) — never attacker-controlled; the `RunCommand`/editor `exec.Command` calls that already only ever invoke a small, fixed set of trusted binaries; two i18n catalog false positives (`G101`, the message-catalog literal containing the *word* "password") |
| `bodyclose` | 4 | False positives (confirmed by reading each): all four are a `Stream` method whose `resp.Body` IS closed on both paths — immediately on a non-200 status, or by the streaming goroutine's own `defer` once it finishes reading — which `bodyclose`'s static analysis doesn't see as a guaranteed close. `//nolint:bodyclose` with that exact reasoning at each of the 4 sites (`anthropic.go`, `google.go`, `ollama.go`, `openai_compat.go`) |
| `revive` | ~30 (capped/re-surfaced in batches) | Unused test-closure parameters renamed to `_` (dozens, across every LLM connector's test file); every `MsgHelpShortXxx`/`MsgFlagXxx`/full-sentence-error `MessageID` constant in `internal/i18n/catalog.go` that lacked an individual doc comment (an established convention every OTHER constant in that file already followed) got one |
| `unparam` | 4 | Real simplifications: `blockStep()`'s always-identical `command` parameter removed (hardcoded, 8 call sites updated); `baseDeps()`'s always-discarded third return value (`*bytes.Buffer` for stderr) removed (37 call sites updated); `executeInfo` simplified to no longer return an always-`RunSummary{}`(zero value) — the caller (`Execute`) now returns it directly, since there was nothing to summarize; `execRootSplit`'s `version` parameter kept (justified `//nolint:unparam` — it mirrors `NewRootCmd`'s own real, production-varying parameter; every CURRENT test happens to pass `"dev"`, which is incidental, not a reason to strip a parameter a real caller varies) |
| `noctx` | 1 | Justified exception: `internal/executor`'s `exec.Command` (not `CommandContext`) is deliberate — `Run` manages `ctx` cancellation itself so it can kill the whole process GROUP via `killProcessGroup`, not just the direct child that `exec.CommandContext`'s automatic SIGKILL would reach |
| `gocritic`, `errorlint`, `prealloc` | 0 | Already clean |

Full command: `/home/firfir/go/bin/golangci-lint run` → `0 issues.`

### `govulncheck`

Pinned `golang.org/x/vuln/cmd/govulncheck@v1.4.0` (2026-06-17 —
`v1.5.0`, 2026-06-25, is only 14 days old at the time of this run, one
day short of the 15-day version-selection floor; `v1.4.0` is the newest
eligible release).

**First run found a real, fixable issue**: `GO-2026-5856` (Encrypted
Client Hello privacy leak in `crypto/tls`, a Go **standard-library**
CVE — `govulncheck` catches these too, not just third-party module
CVEs, since it resolves the exact toolchain `go.mod` pins), fixed in
`go1.26.5`; this repo's `go.mod` was still on `toolchain go1.26.4`.
**Fixed** by bumping `go.mod`'s `toolchain` directive to `go1.26.5`
(`GOTOOLCHAIN=auto` downloaded it automatically on the next build) — a
one-line, zero-risk dependency fix, not a workaround.

```
$ /home/firfir/go/bin/govulncheck ./...
No vulnerabilities found.
```

(Before the fix, the same command reported `GO-2026-5856` against
`internal/llm/ollama.go`, `internal/audit/audit.go`, and
`internal/tui/confirm.go` — all reachable through `net/http`/`tls`,
confirming the exposure was real, not a false positive from an unused
code path.)

### CI wiring

`.github/workflows/ci.yml`'s `lint` job now runs the strict profile
(no change needed there — `golangci-lint-action` already reads
`.golangci.yml`). New `vulncheck` job: `actions/setup-go` (reads
`go.mod`'s toolchain directive, so it picks up `go1.26.5` too) +
`go install golang.org/x/vuln/cmd/govulncheck@v1.4.0` + `govulncheck
./...` — a separate job (not a step tacked onto `lint`/`build-test`) so
a vuln-DB regression is unambiguous in the Actions UI, gating every PR
and push to `main`.

## 5. CHANGELOG + known limitations

`CHANGELOG.md`'s accumulated `[Unreleased]` (FAZ 0–10) plus a new FAZ 11
entry are now under `## [0.1.0-rc1] - 2026-07-09`, with an empty
`[Unreleased]` kept on top for whatever comes after. `KNOWN_LIMITATIONS.md`
(new, bilingual TR/EN, repo root) is the RC's honest known-issues list —
see that file for the full, itemized accounting; **no git tag was
created** (tagging triggers the real release pipeline, and the
`homebrew-tap`/`scoop-bucket`/`winget-pkgs` target repos don't exist yet
— cutting the tag is explicitly a user decision, not this phase's).

## Verification (this run)

- `go vet ./...` — clean.
- `/home/firfir/go/bin/golangci-lint run` (new strict profile) — `0
  issues.` (double-checked with `--max-issues-per-linter=0
  --max-same-issues=0`).
- `go test ./... -count=1` — all 14 packages `ok`.
- `go test -race ./...` — all 14 packages `ok`, no race detected.
- `/home/firfir/go/bin/govulncheck ./...` — `No vulnerabilities found.`
  (after the `go1.26.5` toolchain bump; see above for what it found
  before).
- `make build` — succeeds (`./comrade`, `dev` version).
- `make cross` — all 5 targets (`linux/amd64`, `linux/arm64`,
  `darwin/amd64`, `darwin/arm64`, `windows/amd64`) build successfully,
  including the vendored `third_party/atotto-clipboard` fork's
  platform-specific build-tagged files.
- `/home/firfir/go/bin/goreleaser check` (v2.16.0, already pinned) —
  `1 configuration file(s) validated`.

## Known limitations / deferred work

See `KNOWN_LIMITATIONS.md` (repo root) for the full, bilingual RC
known-issues list. Summary: real macOS/Windows platform runtime
(executor process-tree kill on Windows, PowerShell hooks, OS keychain)
and the three manual E2E scenarios above remain manual/deferred (no
macOS/Windows host in this sandbox, unchanged from earlier phases);
real-LLM acceptance runs still need a live API key; cosign signing,
the `homebrew-tap`/`scoop-bucket`/`winget-pkgs` repos, and the real
`v0.1.0-rc1` tag are all still pending a user decision (FAZ 10's own
deferred items, unchanged); `anthropic`/`google`'s model lists are a
static, docs-linked snapshot (FAZ 8); the ~40 `"doing X: %w"`
wrap-chain errors and the handful of other documented i18n exceptions
remain untranslated by design (FAZ 9).
