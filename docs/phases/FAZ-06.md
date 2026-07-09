# FAZ 06 — Executor + Üç Mod (auto / ask / info)

## Inherited building blocks — audit (STEP 0)

Three packages arrived already scaffolded, building, vet-clean, and with
passing tests, per this phase's brief: `internal/executor`, `internal/tui`,
`internal/audit`. Each was read against UYGULAMA_PLANI.md FAZ 6's spec and
CLAUDE.md's security rules before building anything on top of them.

- **`internal/executor`** — sound, no changes needed. `Run` builds `sh -c`
  (Unix) / `powershell -NoProfile -Command` (Windows) via an
  OS-injectable constructor (`newForGOOS`, so the Windows command-building
  branch is unit-tested on this Linux/WSL2 box); never prepends `sudo`/
  `runas` (verified: `TestRunNeverPrependsSudoOrElevation`); puts the
  child in its own process group (`setProcAttr`/Unix) so `killProcessGroup`
  kills the whole tree, not just the immediate `sh`, on both a per-step
  `Options.Timeout` and `ctx` cancellation — both proven to actually kill
  the process (a `sleep 5 && touch <marker>` never creates the marker)
  and to return promptly, not after the full sleep. Output capture is
  tail-truncated to 8KB per stream independent of live streaming. This
  already matched everything FAZ 6 needed; nothing was changed.
- **`internal/tui`** — sound, no changes needed. `Confirm` maps the exact
  TR letters (`mapKey`: e/h/d/a/t → Yes/No/Edit/Explain/All) via a pure,
  PTY-free-testable function; the bubbletea v2 `Update`/`View` wiring
  around it is a thin shell. `riskBadgeStyle`/`warningStyle`/
  `commandStyle`/`statusStyle` all degrade to a fully unstyled
  `lipgloss.NewStyle()` when `colorEnabled` is false — verified this holds
  by re-running `internal/tui`'s existing suite and reading every style
  function; `general.color=false` really does fully disable color, per
  CLAUDE.md's constraint. `Confirm` surfaces a canceled `ctx` as an error
  rather than hanging (`TestConfirmContextCanceledBeforeStartReturnsError`),
  which `internal/engine`'s `resolveAskChoice` relies on to abort
  gracefully on Ctrl-C mid-prompt. Nothing was changed.
- **`internal/audit`** — sound, no changes needed functionally. `Entry`
  carries exactly CLAUDE.md security rule #4's fields (timestamp, mode,
  command, risk, exit code, duration) plus `Request` — and, critically,
  **no** `Stdout`/`Stderr` field at all, so a captured command's output
  (which never passed through the redaction pipeline, since it's a local
  concern, not something sent to an LLM) can never leak into the audit
  log by construction, not just by convention. `ApplyRetention` does an
  atomic temp-file-then-`os.Rename` rewrite, safe against a reader
  observing a half-written file. One **lint-only** fix: two
  `defer f.Close()` calls (`Append`/`ReadAll`) tripped `errcheck` under
  this repo's pinned `golangci-lint` config — changed to
  `defer func() { _ = f.Close() }()`, matching the exact idiom already
  used everywhere else in this codebase (`internal/llm`'s four connectors,
  all `defer func() { _ = resp.Body.Close() }()`). No behavior changed.
  (A second, unrelated pre-existing lint failure was fixed the same way in
  `internal/tui/confirm_test.go`'s `TestConfirmContextCanceledBeforeStartReturnsError`.)

## Go 1.25 floor bump (user-approved, reverses FAZ 0)

`go.mod`/`CLAUDE.md` were already updated in the working tree before this
phase started (`go 1.25.0` / `toolchain go1.26.4`, `charm.land/bubbletea/v2
v2.0.8` + `bubbles/v2 v2.1.1` + `lipgloss/v2 v2.0.5`, all MIT, all
transitively pinned in `go.sum`). This reverses FAZ 0's original Go 1.23+
floor — noted here for the record since it's a floor-constraint change,
not because this phase performed it (it didn't: no `go get`/`go mod edit`
was run in this phase, per the task's explicit constraint).

## What was built (the missing orchestration layer)

### `internal/engine/mode.go` — `Mode` + precedence

- **`Mode`** (`ModeAuto`/`ModeAsk`/`ModeInfo`) + `String()`/`ParseMode`,
  mirroring `safety.RiskClass`'s single-source-of-truth pattern.
- **`ResolveMode(flagValue, envValue, configValue string) (Mode, error)`**:
  a pure, cobra-independent function implementing UYGULAMA_PLANI.md FAZ 6
  item 2's exact precedence — flag wins, then `COMRADE_MODE`, then config
  `general.mode` — tested by `TestResolveModePrecedence`'s three cases.
  `internal/cli` is the only caller, and collapses its three mutually
  exclusive `--auto`/`--ask`/`--info` bools into `flagValue` itself
  (`executionFlags.modeFlagValue`, in `internal/cli/flags.go`) before
  calling this.

### `internal/engine/runner.go` — the mode loop (the product's heart)

- **`Choice`** (`ChoiceYes`/`ChoiceNo`/`ChoiceEdit`/`ChoiceExplain`/
  `ChoiceAll`) is this package's *own* type, not a re-export of
  `tui.PromptChoice` — so `internal/engine` never has to import
  `internal/tui`'s interactive bubbletea machinery (`Confirm`,
  `PromptStep`) at all. It **does** still use `internal/tui`'s
  non-interactive rendering helpers (`RiskBadge`/`PrintStatus`/
  `PrintWarning`/`PrintExplanation`) directly, since those take only an
  `io.Writer`/`bool`/`string` and carry no engine-shaped coupling in
  either direction — reusing them avoids re-inventing the exact same
  color-degradation logic a second time. `internal/cli/promptui.go` is the
  one place a concrete bubbletea `PromptChoice`/`PromptStep` is converted
  to/from this package's `Choice`/`Step`.
- **`PromptUI` interface** (`Confirm(ctx, Step) (Choice, editedCommand
  string, err error)`, `Explain(ctx, Step) (string, error)`) — the
  consumer-side seam every ask-mode/forced-auto-confirm test scripts
  against; `internal/cli/promptui.go`'s `tuiPromptUI` is the one real
  implementation.
- **`CommandExecutor`/`AuditSink`** — equally minimal consumer-side
  interfaces around `*executor.Executor`/`*audit.Logger`; both concrete
  types satisfy them with no adapter needed.
- **`RunDeps`** bundles every injected dependency (executor, `*safety.
  Engine`, `Completer`, `PromptUI`, `AuditSink` — nilable, disables
  audit — stdout/stderr, color, the two `confirm_*` config bools, `Yolo`,
  per-step timeout, the original request text, an injectable clock).
- **`Execute(ctx, plan, mode, deps) (RunSummary, error)`** dispatches to
  one of three mode functions:
  - **info** (`executeInfo`): prints every step numbered, with its risk
    badge (or `BLOCKED(reason)`) and rationale; **never** calls
    `deps.Executor.Run` even once.
  - **ask** (`executeAsk`): per non-Blocked step, `resolveAskChoice` drives
    the confirm loop — `[e]`/`[h]`/`[t]` return immediately; `[a]` fetches
    +prints an explanation and re-loops the same step; `[d]` re-evaluates
    the edited command through `deps.Safety.Evaluate` **before** ever
    running it — a Block on the edit refuses and re-loops (never runs);
    otherwise the loop re-shows the confirm prompt for the *edited* step
    ("confirm edited version"). `[t]` sets a local
    `autoApproveRemaining` flag that skips the prompt for every later
    step whose `EffectiveRisk` is read/write/network — a later
    destructive/elevated step still goes through `resolveAskChoice`
    individually, since the skip condition explicitly excludes
    `>= RiskElevated`.
  - **auto** (`executeAuto`): every step below `RiskElevated` runs
    immediately with one printed status line (no prompt, ever);
    destructive/elevated drops to the *same* `resolveAskChoice` used by
    ask mode (a `[t]ümü` there is simply treated like `[e]vet` for that
    one step — auto mode has no "remaining" state to set, since it
    already never prompts for low-risk steps), **unless** the
    config+`--yolo` bypass fires for that exact risk class
    (`confirm_destructive=false && --yolo`, or the elevated equivalent),
    which prints the mandatory red warning line and proceeds without
    ever calling `PromptUI.Confirm`.
  - **Block never executes, in any mode, unconditionally** — every mode
    function checks `step.Decision.Action == safety.Block` *before*
    either the confirm-loop or the yolo-bypass branch is even reached; a
    Block is structurally unreachable from both, not merely
    untriggered. In auto mode a Block also aborts every remaining step
    (see Decisions).
  - **Self-correction** (`executeStepWithSelfCorrection`): on a failed
    attempt (nonzero exit — a `TimedOut` result counts as a failure too;
    a `Canceled` one never does, and stops immediately with no
    correction attempted), asks `deps.LLM` for one corrected replacement
    command (a small, dedicated system prompt — `selfCorrectionSystemPrompt`
    — requesting the same per-step JSON shape plan generation uses),
    re-evaluates *that* revision through `deps.Safety` before ever
    running it (a Block on the revision stops immediately, reporting the
    last real failure — the revision itself never executes), and retries
    — up to `selfCorrectionMaxAttempts = 3` total **across the whole
    plan run**, not per step. Every attempt that actually reaches
    `deps.Executor.Run` — including every failing one — gets its own
    audit entry, since each one really executed on the host.
  - **Ctrl-C**: `Execute`'s loops check `ctx.Err()` at the top of every
    iteration and also observe a canceled `executor.Result.Canceled`
    from a step already in flight; either path sets `RunSummary.Aborted`
    with reason `"canceled"` and stops — `internal/cli/do.go` wires the
    actual `signal.NotifyContext(cmd.Context(), os.Interrupt)` that feeds
    this `ctx`.
  - **`RunSummary`** enumerates every step's fate (`Executed`/`Skipped`/
    `Blocked`), including steps the loop never reached after an abort
    (`fillSkippedTail` — the summary is a complete accounting, not just
    "here's what happened before we gave up").

### `internal/audit` integration + `comrade history`

- `internal/cli/do.go`'s `buildAuditSink` builds a real `*audit.Logger`
  from `audit.DefaultPath()` when `audit.enabled` (config default
  `true`), runs `ApplyRetention(cfg.Audit.RetentionDays, time.Now())`
  once per invocation (a cleanup failure is reported to stderr, never
  fatal), and returns `nil` (disabling audit logging) when
  `audit.enabled=false`.
- **`comrade history`** (`internal/cli/history.go`, replacing the FAZ 0
  stub): a `TIME/MODE/RISK/EXIT/COMMAND` `tabwriter` table by default;
  `--json` prints one compact JSON object per line (`encoding/json`'s
  `Encoder`, re-marshaled from `audit.Entry` — not a byte-for-byte file
  dump, see Decisions); `--limit N` (default 20) keeps only the N
  most-recent entries. **Read-only**: it never calls `ApplyRetention` or
  writes to the audit file in any way — `TestHistoryIsReadOnlyNeverRewritesAuditFile`
  pins this by comparing the file's contents byte-for-byte before/after a
  `history` invocation.

### `internal/cli` — wiring

- **`internal/cli/flags.go`**: `executionFlags` (`--dry-run`/`--auto`/
  `--ask`/`--info`/`--yolo`) + `addExecutionFlags(cmd)`, shared by both
  `do` and root, so both register their own independent `*cobra.Command`-
  local copy (no flag-object aliasing, no persistent-flag propagation
  surprises).
- **`internal/cli/promptui.go`**: `tuiPromptUI`, the sole adapter wiring
  `internal/tui.Confirm` (bubbletea) and an `engine.Completer` (the LLM,
  for `[a]çıkla`'s explanation) into `engine.PromptUI`.
- **`internal/cli/do.go`**: `comrade do <request...>` is now the real
  pipeline — load config → (warn on `--yolo`) → build `llm.Client` →
  collect context → `Planner.GeneratePlan` → **either** `--dry-run`'s
  `renderPlan` (unchanged from FAZ 5) **or** resolve the mode, build
  `executor.New`/`safety.NewEngine`/`tuiPromptUI`/the audit sink, wrap
  `cmd.Context()` in `signal.NotifyContext(..., os.Interrupt)`, and call
  `engine.Execute`. Prints a final `"N executed, M skipped, K blocked"`
  summary line (plus the abort reason, if any) after ask/auto — not after
  info, which already is its own complete printed output. `do` is no
  longer `Hidden`: it is FAZ 6's real, documented entry point.
- **Root fallback** (`internal/cli/root.go`): `root.Args =
  cobra.ArbitraryArgs` (see Decisions for exactly why this specific
  field, and only this field, is what's needed) + `rootFlags :=
  addExecutionFlags(root)` + `root.RunE` now branches: zero args → the
  existing version+help banner (unchanged); any args → `runDo(cmd,
  newLoader, strings.Join(args, " "), rootFlags)`. Known subcommands
  (`fix`/`explain`/`chat`/`config`/`init`/`history`/`hook`/`completion`/
  `help`) are matched by cobra's own `Find` *before* ever falling through
  to root, so they route exactly as before; `--help`/`--version` are
  intercepted by cobra's own flag handling ahead of any dispatch.
- **README.md**: checked for a "Go 1.23+" mention to update to "Go
  1.25+" per this phase's brief — there is none anywhere in
  `README.md` (it documents user-facing vision/modes/build commands
  only, never a Go version), so there was nothing to change. `go.mod`/
  `CLAUDE.md` already carry the bumped floor. `docs/phases/FAZ-00.md`'s
  own historical "1.23" note was left untouched either way, per this
  phase's explicit scope (prior phase docs are a historical record, not
  living documentation).

## Post-review hardening pass (independent security review, same phase)

An independent security review of this phase's first pass returned
`CHANGES_REQUIRED` with one CRITICAL finding: the ask-mode `[d]üzenle`
edit path could be used to bypass the denylist entirely, in both ask and
auto modes.

- **Denylist bypass via edit-then-approve (CRITICAL) — the resolved
  step's Decision is now re-checked at every caller, not just the
  original.** `resolveAskChoice`'s `[d]üzenle` case already
  re-evaluated an edited command through `safety.Evaluate` and printed
  `BLOCKED(...)` + re-looped when the edit itself was unsafe — but that
  re-loop showed the *same* confirm prompt again, and if the user then
  pressed `[e]vet` (or `[t]ümü`) on that same, still-Blocked step, the
  function's `default:` case returned it as an ordinary
  `(choice, step, true, nil)` result with no signal that anything was
  wrong. Both callers (`executeAsk`, `resolveAutoGate`'s caller
  `executeAuto`) only ever checked `step.Decision.Action == safety.Block`
  once, at the *top* of the loop, against the plan's *original*
  Decision — never against the Decision a mid-loop edit produced — so
  `step = resolved` followed by `runAndRecord` ran the edited `rm -rf /`
  outright. Reproduced exactly as the review described: a benign `ls`
  step, edited to `rm -rf /`, then `[e]vet`, executed for real in both
  ask (the default mode) and auto. **Fix**: `executeAsk` and
  `executeAuto` each now re-check `resolved`/`confirmed`'s own
  `Decision.Action` immediately after `resolveAskChoice`/
  `resolveAutoGate` returns, *regardless of which choice the user
  picked* — a Block found there is recorded `OutcomeBlocked`
  (`printBlocked` + `continue` in ask; the same plus
  `summary.Aborted`/abort-on-block in auto) and the loop never reaches
  `runAndRecord` for it. This makes the invariant hold on the *resolved*
  step, not merely the plan's original one. **Belt-and-suspenders**:
  `executeStepWithSelfCorrection` itself now also refuses to run a
  Blocked step as a final guard (returns an error immediately, before
  ever touching `deps.Executor`), independent of both caller checks —
  defense-in-depth in case a future caller ever forgets its own check.
  `Execute`'s doc comment was corrected: it previously claimed the
  Block invariant was enforced by a single structural check; it now
  documents both checkpoints (top-of-loop against the original Decision,
  post-resolve against the edited one) plus the final executor-level
  guard.
- **Regression tests** (the gap the original suite had: the existing
  `TestExecuteAskEditThenBlockRefusesAndReprompts` only ever scripted
  `{Edit:"rm -rf /"}, {No}` — never `{Edit:"rm -rf /"}, {Yes}`, which is
  exactly the sequence that bypassed the fix):
  - `TestExecuteAskEditThenYesOnBlockedEditNeverRuns` — ask mode,
    `{ChoiceEdit:"rm -rf /"}, {ChoiceYes}` → `fakeExecutor.callCount() ==
    0`, step recorded `OutcomeBlocked`.
  - `TestExecuteAskEditThenAllOnBlockedEditNeverRuns` — ask mode,
    `{ChoiceEdit:"rm -rf /"}, {ChoiceAll}`, followed by a second,
    unrelated step — proves both that the executor never runs anything
    for the blocked step AND that `[t]ümü` on a Blocked edit does not
    leak an "approve all remaining" state onto the next step (which is
    still individually prompted).
  - `TestExecuteAutoEditThenYesOnBlockedEditNeverRuns` /
    `...AllOnBlockedEditNeverRuns` — the same two scripts against a
    destructive step in auto mode (dropping into `resolveAutoGate`'s
    identical confirm loop), asserting `callCount() == 0` and
    `summary.Aborted` with a `"blocked"` reason, matching auto's
    abort-on-block design decision.
  - `TestExecuteStepWithSelfCorrectionRefusesToRunBlockedStepDirectly` —
    the belt-and-suspenders guard's own direct unit test, calling
    `executeStepWithSelfCorrection` with a Blocked step directly
    (bypassing every caller-level check) and asserting it errors without
    ever reaching the executor.
  - `TestExecuteAskEditThenBlockRefusesAndReprompts` (pre-existing) was
    updated: its `{Edit:"rm -rf /"}, {No}` script now correctly asserts
    `OutcomeBlocked` rather than `OutcomeSkipped` — the fix makes this
    outcome more accurate too (the command was genuinely unsafe, not
    merely declined), not just newly safe.
- **README.md check (MINOR)**: re-confirmed `README.md` mentions no Go
  version anywhere (only a `### Build`/`### Kurulum` section listing bare
  `make` commands) and has no dedicated build-from-source/developer
  section to add one to — no change made, per the review's own
  "if it genuinely mentions no Go version, that's fine" instruction.

## Mode-behavior decisions

- **Auto-abort-on-block.** A Blocked step in auto mode stops the entire
  remaining plan (`RunSummary.Aborted = true`, every later step recorded
  `Skipped`) rather than silently skipping just that one step and
  continuing. Auto mode's whole premise is "the agent can be trusted to
  proceed unattended" — a plan containing an actually-denylisted command
  is evidence that premise doesn't currently hold for *this* plan, so
  stopping and surfacing that to the user (rather than pressing on with
  whatever's left) is the safer default. Ask mode, by contrast, only
  skips the one Blocked step and keeps going, since the user is already
  attending every step there.
- **Self-correction cap = 3, global not per-step**, matching
  UYGULAMA_PLANI.md's explicit "en fazla 3 self-correction denemesi"
  wording read as a whole-run budget (mirroring the reference "BI
  projesi" pattern the plan cites) rather than three attempts *per
  failing step* — a plan with several failing steps shares one budget,
  so one persistently-broken step can't silently consume the entire
  run's correction allowance across every other step too (moot in
  practice, since the loop aborts on the first unrecovered failure
  anyway — but it's the more conservative reading either way).
- **Elevated steps never get auto-sudo'd** — this was already
  `internal/executor.buildCommand`'s existing, audited-sound behavior
  (see Inherited building blocks above); FAZ 6 adds nothing here beyond
  routing elevated steps through the same confirm-or-yolo-bypass gate as
  destructive ones. If a plan step needs `sudo` and the model didn't put
  `sudo` in the command text itself, it fails with the OS's own
  permission error — this package never escalates on the user's behalf.
- **`--yolo` bypass**: printed as a **mandatory** red warning banner on
  *every* invocation with `--yolo` set (CLAUDE.md security rule #6),
  independent of whether the config-side bypass conditions actually let
  it skip anything this particular run — plus a *second*,
  per-occurrence red warning line each time the bypass actually fires
  for a specific destructive/elevated step. Both are tested independently
  (`TestExecuteAutoBypassesDestructiveConfirmOnlyWithConfigAndYolo`/
  `...ElevatedConfirmOnlyWithConfigAndYolo` for the per-step warning;
  `runDo`'s own `--yolo` check for the per-invocation one).
- **Root-fallback tradeoff, stated plainly**: setting `root.Args =
  cobra.ArbitraryArgs` means a genuine subcommand typo (`comrade fx`)
  no longer gets cobra's helpful "unknown command, did you mean...?"
  suggestion — it silently free-text-dispatches to `do("fx")` instead,
  which will produce *some* plan (or a clearly-worded LLM/provider
  error) rather than a crisp "unknown command". This is the deliberate,
  documented tradeoff UYGULAMA_PLANI.md FAZ 6 item 3 asks for
  ("`comrade docker kur` çalışsın"); it is the same UX tradeoff other
  natural-language-first CLIs make.

## bubbletea v2 headless-test approach

Every ask-mode/auto-forced-confirm test in `internal/engine/runner_test.go`
scripts a fake `PromptUI` (`fakePrompt`, a scripted, ordered `Choice`
queue) instead of driving a real bubbletea `Program` — `internal/engine`
never imports `internal/tui`'s interactive pieces at all (see the
`Choice`/`PromptUI` note above), so there is nothing bubbletea-shaped to
even drive at this layer. `internal/tui`'s *own* suite (inherited,
unchanged this phase) is what headlessly exercises the real
`tea.Program` via `tea.WithInput`/`tea.WithOutput` redirected to
in-memory buffers — see `TestConfirmRunsHeadlessProgramAndReturnsChoice`.
This phase's own new end-to-end proof
(`TestDoAutoModeRunsBenignStepAndBlocksDenylistedStepAgainstRealExecutor`)
goes one level higher still: the real compiled pipeline, a real
`*executor.Executor`, and a real (fake-server-backed) `llm.Client`, with
no bubbletea prompt reached at all (the only destructive/elevated-shaped
step in its fixture is Blocked outright, never reaching a confirm).

## Windows deferred-runtime note

Unchanged from the inherited `internal/executor` audit: `setProcAttr`/
`killProcessGroup`'s Windows branch (`executor_windows.go`) kills only the
direct child process, not further-nested grandchildren PowerShell itself
spawned — a real Windows process-tree-kill (`CREATE_NEW_PROCESS_GROUP` +
a job object) is a documented future item, not implemented here. Every
Windows-specific test (`TestRunOnWindowsRunsPowerShellForReal`) is
unit-guarded (`t.Skip` when not actually running on `windows` or when
`powershell` isn't on `PATH`) rather than exercised in this Linux/WSL2
development environment; the project's CI matrix (UYGULAMA_PLANI.md FAZ
11) does include a real `windows` runner where this test activates for
real.

## Other decisions / deviations

- **`executor.step_timeout_seconds` (new `[executor]` config section,
  default 300)**: UYGULAMA_PLANI.md FAZ 6 item 1 requires a per-step
  timeout "adım başına config'den" but never named a specific key —
  `internal/config/schema.go`'s `ExecutorConfig` + `validate.go`'s
  matching `KeyDef` fill that gap. Verified bidirectionally consistent
  with the existing drift guards (`TestKeyDefsMatchConfigStruct`,
  `TestKeyDefsMatchDefaultConfigTOML`), plus a new pinned-default
  assertion in `TestDefaultMatchesPlanExactly`.
  `RunDeps.StepTimeout` is wired straight from it in `runDo`.
- **`comrade history --json`** re-marshals `audit.Entry` values (one
  compact JSON object per line) rather than dumping the audit file's raw
  bytes — "ham çıktı" (raw output) is read as "unformatted structured
  output" (as opposed to the human table), not literally "bytes as
  stored on disk", since a byte-dump would make `--limit` meaningless
  (it operates on parsed entries, which is also what `ReadAll` already
  gives every other caller).
- **`PromptUI`/`Choice` deliberately do not import `internal/tui`'s
  interactive types** (see the runner.go section above) — this is the
  one layering choice this phase treated as load-bearing rather than
  incidental, since it's what makes every ask/auto-mode test in
  `internal/engine` fast, PTY-free, and independent of bubbletea's own
  behavior.
- **A second `safety.Engine` instance is built in `runDo`**
  (`safety.NewEngine(*cfg)`, wired into `RunDeps.Safety`), separate from
  the one `engine.Planner` already builds internally for
  `GeneratePlan`'s own per-step annotation. `safety.Engine` is pure,
  stateless computation over `cfg.Safety.DenylistExtra` (compiled once
  at construction) — the tiny duplicated compile cost was judged cheaper
  than exposing `Planner`'s private engine just to share one instance.

## Manual verification (real binary, mock provider — pasted output)

Built via `make build`; run against a small local `net/http` mock
standing in for an `openai_compat` endpoint. State/config dirs isolated
via `HOME`/`XDG_CONFIG_HOME`/`XDG_STATE_HOME` env vars pointed at a temp
directory.

**`comrade docker kur --info`** (root-fallback dispatch → `do` → info
mode — proves both the fallback routing *and* info mode's
print-only/never-execute behavior in one shot):

```
$ ./comrade docker kur --info
Created default config at <tmp>/.config/cli-comrade/config.toml
Docker kurulur.

1. [read] echo docker-would-install-here
   Docker kurulum simulasyonu
2. [elevated] sudo systemctl enable --now docker
   Docker servisini baslatir
3. BLOCKED(matches denylist rule: rm -rf / (or ~ / $HOME root delete)): rm -rf /
   model asla uretmemeli
```

(Risk badges render as ANSI-colored inline blocks in a real terminal —
`[read]`/`[elevated]` above are the plain-text equivalent of what a
`cat`-piped capture shows; `general.color=true` is the schema default.)

**`comrade config list`** (proves a known subcommand still routes
normally, not through the fallback):

```
$ ./comrade config list | head -5
KEY                     VALUE  SOURCE
audit.enabled           true   file
audit.retention_days    90     file
context.history_depth   5      file
context.send_env_names  false  file
```

**`comrade docker kur --auto`** (real execution: the benign step
actually runs; the elevated step forces an interactive confirm — piped
`"h"` declines it; the denylisted step is Blocked and aborts the run),
followed by **`comrade history`**/**`comrade history --json`** reading
back the resulting audit trail:

```
$ echo "h" | ./comrade docker kur --auto
-> running: [read] echo docker-would-install-here
docker-would-install-here
3. BLOCKED(matches denylist rule: rm -rf / (or ~ / $HOME root delete)): rm -rf /

1 executed, 1 skipped, 1 blocked
aborted: step 3 is blocked: matches denylist rule: rm -rf / (or ~ / $HOME root delete)
comrade do: step 3 is blocked: matches denylist rule: rm -rf / (or ~ / $HOME root delete)

$ ./comrade history
TIME                       MODE  RISK  EXIT  COMMAND
2026-07-09T05:01:06+03:00  auto  read  0     echo docker-would-install-here

$ ./comrade history --json
{"timestamp":"2026-07-09T05:01:06.379217712+03:00","request":"docker kur","command":"echo docker-would-install-here","risk":"read","mode":"auto","exit_code":0,"duration_ms":1}
```

Exactly one audit entry — the benign step that actually ran — never the
Blocked one, matching `TestExecuteAppendsOneAuditEntryPerExecutedStep`'s
engine-level proof and this phase's own end-to-end
`TestDoAutoModeRunsBenignStepAndBlocksDenylistedStepAgainstRealExecutor`.

## Gate

- `go vet ./...` — clean.
- `/home/firfir/go/bin/golangci-lint run ./...` — 0 issues (after the two
  inherited pre-existing `errcheck` fixes noted above).
- `go test ./... -count=1` — all packages pass, including 21 new test
  functions in `internal/engine/runner_test.go`, 6 in
  `internal/engine/mode_test.go`, a new `internal/cli/history_test.go`
  (5 test functions), and new/updated cases in
  `internal/cli/{do,root,config}_test.go`.
- `go test -race ./internal/executor/... ./internal/engine/... -count=1`
  — clean, no data races (the Ctrl-C/concurrent-cancellation paths in
  both packages are exactly what this targets).
- `make build` — succeeds.
- `make cross` — succeeds for all five targets (linux/amd64,
  linux/arm64, darwin/amd64, darwin/arm64, windows/amd64), confirming the
  bubbletea/lipgloss/bubbles dependency graph cross-compiles cleanly.
- The four security-invariant tests, by name:
  1. `TestExecuteBlockNeverExecutesInAskMode` +
     `TestExecuteBlockNeverExecutesInAutoModeEvenWithYolo` — Block never
     executes in any mode, including auto+`--yolo`.
  2. `TestExecuteAutoForcesConfirmOnDestructive` /
     `...OnElevated` +
     `TestExecuteAutoBypassesDestructiveConfirmOnlyWithConfigAndYolo` /
     `...ElevatedConfirmOnlyWithConfigAndYolo` +
     `TestExecuteAutoBypassRequiresBothConfigFlagAndYolo` —
     destructive/elevated always confirm in auto except the explicit
     config-flag+`--yolo` bypass, proven from both directions.
  3. `TestExecuteAskEditThenBlockRefusesAndReprompts` — an edited ask-mode
     command is re-evaluated by safety before running (edit
     benign→`rm -rf /`→Blocked, never run, re-prompted). **Post-review
     hardening added four more, closing the actual bypass this one test
     alone did not catch**: `TestExecuteAskEditThenYesOnBlockedEditNeverRuns`,
     `TestExecuteAskEditThenAllOnBlockedEditNeverRuns`,
     `TestExecuteAutoEditThenYesOnBlockedEditNeverRuns`,
     `TestExecuteAutoEditThenAllOnBlockedEditNeverRuns` — all four assert
     `fakeExecutor.callCount() == 0` for the edit→Block→`[e]vet`/`[t]ümü`
     sequence, in both ask and auto modes. Plus
     `TestExecuteStepWithSelfCorrectionRefusesToRunBlockedStepDirectly`
     for the belt-and-suspenders executor-level guard.
  4. `TestRunNeverPrependsSudoOrElevation` (inherited, re-verified
     unchanged) — the executor never auto-sudo's an elevated command.

## Risks / follow-ups

- `internal/executor`'s Windows process-tree-kill is still single-process
  (see Windows deferred-runtime note) — a real job-object-based fix is
  future work, not blocking this phase.
- `comrade fix`/`explain`/`chat` remain FAZ 7/9 stubs, untouched.
- i18n (FAZ 9) will need to route every new hardcoded string this phase
  added (`printRunSummary`, the info-mode step lines, the self-correction/
  explain system prompts, the `--yolo` warning text) through the eventual
  catalog — all funneled through single functions/constants exactly for
  that mechanical migration, per CLAUDE.md's i18n note.
