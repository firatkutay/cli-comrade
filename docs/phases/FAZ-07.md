# FAZ 07 — `comrade fix` (Hata Çözme Akışı)

## What was built

### `internal/engine/diagnose.go` — `Diagnoser` (the diagnose-side counterpart of `Planner`)

- **`ErrorContext`**: `{Command, ExitCode, Stderr, Stdout, System contextpkg.Context}` — everything
  `Diagnose` needs about the failing command. `ExitCode` is `-1` when genuinely unknown (the paste-mode
  fallback, below, never observes a real one). `internal/engine` never reads `last_command.json` or an
  executor itself — `internal/cli`'s fallback chain builds one of these and hands it in, exactly the
  same layering `Planner`/`GeneratePlan` already uses for `comrade do`.
- **`Diagnosis`**: `{RootCause, Explanation string, Plan Plan}` — `Plan` reuses `engine.Plan`/`Step`
  verbatim, so `comrade fix` hands the exact same type to `engine.Execute` that `comrade do` does.
- **`Diagnoser.Diagnose(ctx, ErrorContext) (Diagnosis, error)`**: builds a `go:embed`'d system prompt
  (`prompts/diagnose_system.txt` + the Turkish block `prompts/diagnose_lang_tr.txt` when
  `general.language` resolves to `tr` — reusing `resolveLanguage` verbatim from `prompt.go` — + the
  few-shot grounding `prompts/diagnose_fewshot.txt` + `serializeErrorContext`'s rendering of the failing
  command/exit code/stderr/stdout tails and the same OS/shell/package-manager/admin block
  `serializeContext` already renders for plan generation), sends one request with
  `RequiredFields: ["root_cause", "explanation", "plan"]`, decodes into a `rawDiagnosis` whose `Plan`
  field is `rawPlan` **reused directly from `planner.go`** — so `toPlan` (also `planner.go`) builds and
  risk-parses the fix plan with zero duplicated conversion logic — and runs every resulting step through
  the same `safety.Engine` `Planner` uses. Deliberately does **not** run `GeneratePlan`'s own
  empty-`steps` corrective re-prompt: a diagnosis that legitimately found nothing to fix (the prompt's own
  "the command actually succeeded" escape hatch) is a valid response shape here, not a malformed one.
- Schema strictness proven directly: missing `root_cause` → error, `"plan": {}` (empty object) → error
  (the same `isEmptyJSONValue` rule every other required-field check in this codebase uses), a
  markdown-fenced response → handled (reuses `llm.ExtractJSON`/`ValidateInto`, not a second parser).

### `internal/engine/prompts/` — new embeds

- **`diagnose_system.txt`**: the JSON schema instruction (`root_cause`/`explanation`/`plan`), the
  explicit "name the real cause, not a generic restatement" and "a terminal beginner must be able to
  follow `explanation`" quality bar, the package-manager-aware install-step rule ("use a **detected**
  package manager, never guess one that isn't listed"), and the same OS/shell-targeting,
  risk-labeling, non-chaining, non-interactive-flag rules `plan_system.txt` already establishes for
  plan generation (kept consistent on purpose — same schema family, same rules, different task).
- **`diagnose_lang_tr.txt`**: the Turkish instruction block — `root_cause`/`explanation`/`plan.summary`/
  every step's `rationale` in Turkish; JSON keys and the `risk` value itself stay English, exactly
  mirroring `plan_lang_tr.txt`'s existing convention.
- **`diagnose_fewshot.txt`**: 16 worked examples — 8 common failure patterns (command not found +
  package-manager-aware install suggestion, permission denied, port already in use, ENOENT/no such
  file, Python `ModuleNotFoundError`, git merge conflict, DNS/proxy failure, PowerShell
  `ExecutionPolicy`), each given once in English and once in Turkish, so both languages have concrete
  `{root_cause, explanation, plan}`-shaped grounding to imitate. Explicitly labeled as
  never-copy-verbatim grounding, not a cache of literal answers.

### `internal/engine/verify.go` — `OfferVerification` (post-solution verification, FAZ 7 item 4)

- **`OfferVerification(ctx, RunDeps, Mode, originalCommand string) error`**: evaluates `originalCommand`
  through `deps.Safety` with `safety.RiskRead` as the **floor** declared risk (there is no LLM-declared
  risk for an original failing command — the floor is never trusted on its own; `Engine.Evaluate`'s
  denylist/escalation rules independently re-derive the real effective risk straight from the command
  text, so a floor of `RiskRead` can never hide real destructiveness). Skipped entirely, in every mode,
  when the verdict is `Block` or `RiskDestructive` — FAZ 7 item 4's "destructive değilse" gate — and
  silently skipped for an empty command too (paste mode never captured one, or there simply wasn't one).
  - **info**: prints `"Suggested verification: <command>"`. Never executes.
  - **ask**: reuses `resolveAskChoice` **verbatim** — the exact same `[e]vet`/`[h]ayır`/`[d]üzenle`/
    `[a]çıkla`/`[t]ümü` loop a real plan step gets, including re-evaluating an edited command through
    safety before ever running it.
  - **auto**: runs the command directly with no prompt for every risk class **except** `elevated` —
    CLAUDE.md's non-negotiable elevated-confirmation requirement applies to a verification re-run
    exactly like it does to a real plan step, so an elevated original command drops to the same
    confirm loop ask mode uses; `destructive` is already excluded above.
  - The actual re-run (`runVerification`) reports success/failure with a one-line status
    (`"verification: <cmd> succeeded"` / `"... still fails (exit N)"`) and — non-negotiably, per
    CLAUDE.md security rule #4 — is audited via the exact same `appendAudit` helper `Execute` itself
    uses for every other executed command. This re-run is never exempted from the audit log just
    because it happens after the main plan finished.

### `internal/cli/fix.go` — the command, wiring `Diagnoser` + the fallback chain into FAZ 6's `Execute`

- **`acquireErrorContext`** implements UYGULAMA_PLANI.md FAZ 4/7's fallback chain, in order:
  1. `comrade fix -- <command...>` (`explicitCommand` non-empty, parsed via `cmd.ArgsLenAtDash()`):
     run it via `captureByRunning`, ignoring `last_command.json` entirely.
  2. `comrade fix --rerun`: re-run the recorded `last_command.json` entry via `captureByRunning`.
     Errors immediately (no silent fallback) if there is no recorded entry at all.
  3. A **fresh** (`< 10 minutes`, `lastCommandFreshness`) `last_command.json` entry whose
     `exit_code != 0`: used directly, with **no** re-execution — its captured stderr/stdout tails are
     already exactly what the FAZ 4 shell hook recorded.
  4. Otherwise — stale (`>= 10 min`) or successful (`exit_code == 0`) — a one-line notice is printed to
     stderr explaining why the recorded entry was *not* silently reused, and the chain falls through to
     interactive **paste mode**.
- **`captureByRunning`** (shared by `--rerun` and `-- <command>`): classifies the command through
  `safetyEngine.Evaluate(command, safety.RiskRead)` **before** ever touching the executor — the same
  floor-only-never-trusted-alone reasoning `OfferVerification` uses — and refuses to execute at all
  when the verdict is `Block` or `RiskDestructive`, printing why and falling through to `pasteMode`
  instead (UYGULAMA_PLANI.md FAZ 7 item 2: re-running a failed `rm -rf` "to capture its error" would be
  catastrophic). Otherwise runs it via the injected `engine.CommandExecutor` and captures the real
  exit code/stderr/stdout.
- **`pasteMode`**: prompts for the failing command (one line) then its error output, terminated by a
  blank line or EOF; sets `ExitCode: -1` (the documented "unknown" sentinel) since a pasted transcript
  never carries a real exit code.
- **`runFix`**: `setupCLIRuntime` (new, shared with `runDo` — see below) → collect system context →
  `acquireErrorContext` → `Diagnoser.Diagnose` → print `RootCause` + `Explanation` (**every** mode, per
  FAZ 7 item 1c, so the user understands the failure before either reading the plan (info) or being
  asked to approve it (ask/auto)) → (optionally `--dry-run`'s `renderPlan`, reused unchanged from
  `do.go`) → resolve mode (identical `--auto`/`--ask`/`--info` > `COMRADE_MODE` > config precedence) →
  build the exact same `RunDeps` `do.go` builds → `engine.Execute` → `printRunSummary` (reused
  unchanged) → `engine.OfferVerification` (only once the run reached a clean end: info always, since
  nothing can abort there; ask/auto only when `!summary.Aborted`).
- **`internal/cli/runtime.go`** (new): `setupCLIRuntime` factors the
  load-config/first-run-notice/`--yolo`-warning/`llm.New` sequence that was previously inlined in
  `runDo` — now shared verbatim by `runDo` and `runFix`, since both are "load config, maybe build an
  LLM client, then run the FAZ 5/6 plan+execute machinery" pipelines and this part never differed
  between them. `runDo`'s own behavior/output is unchanged (verified: its existing test suite passes
  unmodified).

`fix` reuses FAZ 5/6 machinery in full: no execution loop, safety gating, mode resolution, or prompt UI
is reimplemented anywhere in this phase. `comrade fix` is exactly "acquire error context + diagnose +
(reuse `Runner`)", matching the task's explicit constraint.

## Decisions

- **Declared-risk floor for a command with no LLM-declared risk.** Both `captureByRunning` (pre-rerun
  safety check) and `OfferVerification` (post-solution verification) need a safety verdict for a
  command that was never part of an LLM-produced plan — there is no `step.Risk` to pass as `declared`.
  Both pass `safety.RiskRead` as the floor. This is safe specifically because `safety.Engine.Evaluate`'s
  contract is "declared is a floor an escalation rule may raise, never lower" — the denylist and all ten
  escalation rules independently re-derive the effective risk from the command text itself, regardless
  of what floor was declared. A `RiskRead` floor on `rm -rf /tmp/x` still ends up `RiskDestructive` via
  the "rm -r/-f" escalation rule; it is not a weaker check than a real plan step gets, just a
  differently-sourced starting point.
- **`--rerun` / `-- <command>` refuse before executing, then fall through to paste mode — never abort
  outright.** A refused destructive command is not a dead end: the user still gets to describe the
  failure via paste mode in the same invocation, rather than being told to re-run `comrade fix` a
  second time with different flags.
- **Post-solution verification's mode-specific behavior is asymmetric with `Execute`'s own by design**:
  ask mode reuses the exact confirm loop, but auto mode only forces a confirm for `elevated` (not
  every non-benign risk) — because `destructive` is already excluded entirely by the "offer" gate
  itself, so `elevated` is the only remaining risk class CLAUDE.md's non-negotiable confirmation rule
  still applies to at this point.
- **Diagnose does not run `GeneratePlan`'s empty-steps corrective re-prompt.** For plan generation, an
  empty `steps` array is *always* an underspecified answer to "give me a plan for this request" and
  deserves one automatic retry. For diagnosis, an empty `steps` array can be the *correct* answer
  ("this command actually already succeeded" or "I can't determine an actionable fix") — retrying would
  be asking the model to manufacture a plan it explicitly said didn't exist.
- **`fix`'s `--dry-run` reuses `do.go`'s `renderPlan` unchanged**, printed after the root
  cause/explanation instead of instead of them, giving `comrade fix --dry-run` the same
  print-the-safety-annotated-table-without-executing behavior `comrade do --dry-run` has, for free.

## Manual verification (real binary, mock provider — pasted output)

Built via `make build`; run against a small standalone `net/http` mock server standing in for an
`openai_compat` `/chat/completions` endpoint (`http://127.0.0.1:8971`). State/config dirs isolated per
run via `HOME`/`XDG_CONFIG_HOME`/`XDG_STATE_HOME` pointed at a fresh temp directory (no real key
required — this is the scripted-mock scenario the task calls for; the real-LLM acceptance run is the
one remaining **manual** item, tracked in `docs/PROGRESS.md`).

**`comrade fix --info`, FAZ 7's own named acceptance scenario** — a fresh `last_command.json` entry
recording a failed `pyton --version` (a typo'd `python3`) is diagnosed, and info mode prints the root
cause, the plain-language explanation, the numbered fix plan, and the offered verification command:

```
$ ./comrade fix --info
The command "pyton" does not exist; it looks like a typo for python3, which is also not installed.

Your computer doesn't recognize pyton. It's likely a typo of python3, and that isn't installed on this machine yet.
Install python3 with apt, then check its version.

1. [elevated] sudo apt-get install -y python3
   Installs Python 3 using the detected package manager.
2. [read] python3 --version
   Confirms python3 now works.

Suggested verification: pyton --version
```

**Destructive `--rerun` refusal** — `last_command.json` records a failed `rm -rf <tempdir>/dangerzone`
(a directory seeded with a marker file); `comrade fix --rerun --info` is run with paste-mode input
piped to stdin:

```
$ printf 'echo pasted-command\nsome pasted error text\n\n' | ./comrade fix --rerun --info
refusing to re-run "rm -rf <tempdir>/dangerzone": it is classified destructive; paste the command and its error output instead.
No recent failed command available. Paste the failing command, then its error output (end with a blank line):
Command: Error output (end with a blank line):
The command "pyton" does not exist; it looks like a typo for python3, which is also not installed.
...
Suggested verification: echo pasted-command

$ ls -la <tempdir>/dangerzone/
total 8
drwxr-xr-x 2 firfir firfir 4096 ... .
drwxr-xr-x 5 firfir firfir 4096 ... ..
-rw-r--r-- 1 firfir firfir    0 ... marker.txt
```

The marker file survives — independent, real-filesystem proof that the destructive `rm -rf` last
command was refused **before** it ever reached the executor, exactly as
`TestAcquireErrorContextRefusesDestructiveRerun` asserts at the function level (see Gate below).

## Gate

- `go vet ./...` — clean.
- `/home/firfir/go/bin/golangci-lint run ./...` — 0 issues.
- `go test ./... -count=1` — all packages pass, including 9 new test functions in
  `internal/engine/diagnose_test.go`, 9 in `internal/engine/verify_test.go`, 10 in
  `internal/cli/fix_test.go`, plus `internal/cli/root_test.go`'s stub-list test updated (`"fix"`
  removed, matching the existing pattern for `do`/`history`/`config`/`init`).
- `go test ./internal/engine/... -race -count=1` — clean, no data races.
- `make build` / `make cross` (all five targets) — succeed.
- Destructive-rerun-block and stale-rejection tests, by name:
  - `TestAcquireErrorContextRefusesDestructiveRerun` — `--rerun` against a `rm -rf <dir>` last command:
    `recordingExecutor.calls` stays empty, `stderr` contains `"refusing to re-run"`, the chain falls
    through to (scripted) paste mode.
  - `TestAcquireErrorContextRerunWithoutLastCommandIsAnError` — `--rerun` with no recorded entry at all
    is a clear error, not a silent fallback.
  - `TestFixStaleLastCommandFallsThroughToPasteMode` — a `last_command.json` entry `> 10` minutes old
    is never silently reused (`stderr` contains `"more than 10 minutes old"`; falls to paste mode).
  - `TestFixSuccessfulLastCommandFallsThroughToPasteMode` — a fresh entry with `exit_code == 0` is
    never treated as something to fix (`stderr` contains `"exited successfully"`).
  - `TestOfferVerificationSkippedWhenOriginalCommandIsDestructive` — engine-level, all three modes:
    a destructive original command is never offered for verification and the executor is never called.
  - `TestFixAskModeRoutesBlockedStepToRunnerAndSkipsVerificationForDestructiveOriginal` — cli-level:
    proves the same skip end-to-end (`stdout` never contains `"verification"`).

## Risks / follow-ups

- **Manual real-LLM acceptance** (a live `comrade fix` run against `pyton --version` with a real
  Anthropic/OpenAI-compatible key) is a genuinely manual item — no automated test can assert on a real
  model's actual response quality. This phase's task scope excludes `docs/PROGRESS.md`, so this item is
  recorded here for the team lead to carry into that file's "Notlar" section; every other acceptance
  criterion in this phase is proven automatically (mock-LLM end-to-end tests) or by the pasted
  real-binary-against-mock-server runs above.
- Ask-mode's interactive confirm loop for `comrade fix` itself is **not** re-driven through a real
  bubbletea program at the `internal/cli` level in this phase's test suite — consistent with FAZ 6's
  own precedent (`internal/cli/do_test.go` also only exercises `--dry-run`/`--auto` end-to-end, never a
  real interactive ask-mode session). `OfferVerification`'s ask-mode behavior (prompt, run on
  `[e]vet`, skip on `[h]ayır`) is proven at the `internal/engine` level with a scripted `fakePrompt`
  (`TestOfferVerificationAskModePromptsAndRunsOnYes`/`...SkipsOnNo`), and `internal/tui`'s own suite
  headlessly drives the real bubbletea program independently — the same layering FAZ 6 already
  established and documented.
- `comrade explain`/`comrade chat` remain FAZ 9 stubs, untouched.
- i18n (FAZ 9) will need to route every new hardcoded string this phase added (the fallback-chain
  notices, the paste-mode prompts, the verification suggestion/status lines, the diagnose/few-shot
  prompt text) through the eventual catalog — all funneled through single functions/constants for
  exactly that mechanical migration, per CLAUDE.md's i18n note.
