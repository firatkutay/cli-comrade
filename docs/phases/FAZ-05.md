# FAZ 05 — Engine: Plan Üretimi + Risk Sınıflandırma

## What was delivered

### `internal/safety` — LLM-independent rule engine

- **`RiskClass`** (`RiskRead < RiskWrite < RiskNetwork < RiskElevated <
  RiskDestructive`, an ascending severity ordinal so `Engine.Evaluate` can
  implement "never lower the declared risk" with plain integer `max`) +
  **`ParseRiskClass`**, the single source of the five canonical lowercase
  spellings (`"read"`/`"write"`/`"network"`/`"elevated"`/`"destructive"`)
  both `String()` and `ParseRiskClass` draw from, so they can never drift.
  An unrecognized string is always an error — `ParseRiskClass` never
  guesses a default; `internal/engine`'s plan parser is the one that
  chooses to fail closed to `RiskDestructive` on that error.
- **`Decision{Action, Reason, EffectiveRisk, MatchedRule}`** and
  **`Action`** (`Allow`/`Confirm`/`Block`) — `Engine.Evaluate`'s result
  type. `MatchedRule` names whichever denylist/escalation rule drove the
  decision (empty for a plain `Allow` with nothing escalated), kept for
  audit/debug logging and for `comrade do --dry-run`'s rendered `BLOCKED(
  <reason>)` cells.
- **`Engine`** (`NewEngine(cfg config.Config)`): compiles
  `cfg.Safety.DenylistExtra` once at construction (see Decisions below for
  the invalid-pattern-skip behavior) and holds the fixed, package-level
  built-in denylist and escalation rule sets. `Evaluate(command,
  declared)`:
  1. Built-in denylist, then user denylist — first match wins and returns
     `Block` unconditionally, `declared` is not even consulted.
  2. Escalation rules — `EffectiveRisk` starts at `declared` and is raised
     (never lowered) to the highest risk any matching rule implies.
  3. `EffectiveRisk` of `destructive` or `elevated` maps to `Confirm`;
     anything lower maps to `Allow`.
- **Denylist rules** (`internal/safety/denylist.go`): `rm -rf /` and its
  `-fr`/`-r -f`/`--recursive --force`/`~`/`$HOME`/`${HOME}`/`/*` variants
  (token-based, matched by `rm`'s path basename so `/bin/rm`/`/usr/bin/rm`
  count too, and with the target canonicalized by `normalizeRootTarget`
  first — see the post-review hardening section below), `mkfs`/
  `mkfs.<fstype>`, `dd ... of=/dev/<any real disk device>` (broadened past
  a fixed device-name-prefix allowlist — see below), `diskpart` + `clean`
  (both words, case-insensitive), a PowerShell drive-root recursive delete
  recognized across the whole Remove-Item alias family (`Remove-Item`,
  `Remove-ItemProperty`, `ri`, `rd`, `rmdir`, `del`, `erase`, `rm` — see
  below), `format <drive>:`, the classic fork bomb (`:(){ :|:& };:`,
  whitespace-tolerant), `> /dev/<any real disk device>` redirection.
- **Escalation rules** (`internal/safety/escalation.go`), ten total: `rm
  -r`/`-f` (any combined short or long flag) → destructive; the
  Remove-Item alias family with `-Recurse`/`-Force` (or an abbreviated/
  cmd.exe-legacy equivalent) targeting anything at all → destructive;
  `chmod -R 777` (either flag order) → destructive; disk-write
  redirects/`of=/dev/*` against any real device, broadened identically to
  the denylist's version of the same check → destructive; registry
  `Remove-Item`/`Remove-ItemProperty` on `HKLM:`/`HKCU:` → destructive;
  `killall`/`taskkill /F` → elevated; `iptables -F`/`netsh advfirewall
  reset` → elevated; `git push --force`/`-f` → destructive;
  `sudo`/`runas`/`-Verb RunAs` → elevated; package-manager
  `install`/`add` → write; `curl`/`wget`/`Invoke-WebRequest`/
  `Invoke-RestMethod`/`apt(-get) update|upgrade` → network.
- **Tests**: 236 table-driven leaf sub-cases (well over the ≥60 required)
  plus 41 standalone test functions, spanning both Unix and PowerShell
  command families across every decision type — every denylist entry hit
  + its documented near-miss, every escalation rule hit + non-hit, the
  upward-only property (declared `destructive` stays `destructive` on a
  benign command; declared `read` + `rm -r` becomes `destructive`),
  `denylist_extra` (custom pattern blocks; invalid pattern skipped with
  exactly one stderr warning, verified by capturing `os.Stderr`),
  `ParseRiskClass`'s unknown-value rejection, and every probe from the
  post-review hardening pass below (quote-fragility, the broadened disk
  device family, the PowerShell Remove-Item alias set, and the
  root-delete near-equivalents).

### Post-review hardening pass (independent security review, same phase)

An independent security review of this phase's first pass returned
`CHANGES_REQUIRED` with three blockers and two majors/mediums — all
addressed by root-causing the underlying matching mechanism rather than
patching each individual probe:

- **Quote-fragility (BLOCKER 1) — every matcher now runs against a single
  normalized form, not the raw command.** The original code ran the
  disk/dd/redirect/mkfs regexes against Evaluate's raw input, so a single
  stray quote defeated them: `dd if=/dev/zero of='/dev/sda'` contains no
  literal `of=/dev/sda` substring once the quotes are counted, so it fell
  through to `Allow`. The `rm` rules were incidentally robust only because
  they already ran on quote-stripped tokens. **Fix**: `normalizeCommand`
  (`internal/safety/tokenize.go`) strips every `"`/`'`/`` ` `` character
  *anywhere* in the string (not just at token edges — an edge-only trim
  cannot clean `of='/dev/sda'`, since the opening quote sits mid-token,
  right after `of=`) and collapses whitespace, and `Engine.Evaluate` calls
  it exactly once, up front, before either denylist ever runs or any
  escalation rule is checked — so every rule, regex-based or token-based,
  gets this hardening for free instead of needing its own quote-tolerant
  pattern. Proven by `TestEvaluateQuoteFragilityIsFixedByNormalization`
  (`of='/dev/sda'`, `of="/dev/sda"`, `mkfs.ext4 '/dev/sdb'`,
  `rm -rf '/'`, and a redirect, all → `Block`) plus a direct
  `TestNormalizeCommand` unit test pinning the exact transform.
- **Disk-device family was an allowlist of name prefixes (BLOCKER 2 /
  MEDIUM 5) — replaced with a denylist of known-safe pseudo-devices.**
  The original `dd`/redirect rules only recognized `sd`/`hd`/`nvme`/
  `xvd`/`vd`-prefixed device names, so `> /dev/nvme0n1` (wait — that one
  *did* match) but `> /dev/vda`, `> /dev/xvda`, `> /dev/mmcblk0`,
  `> /dev/disk0` (macOS), and `dd ... of=/dev/loop0` all reached `Allow`.
  CLAUDE.md's own denylist entry names no specific device family in the
  first place. **Fix**: `hasRealDiskDeviceReference`
  (`internal/safety/denylist.go`) now treats *any* `/dev/<name>` reference
  as a real disk **except** a small, explicit safe-pseudo-device set
  (`null`, `zero`, `tty`/`ttyN`, `random`, `urandom`, `full`, `stdin`,
  `stdout`, `stderr`, `fd/*`, `pts/*`) — an allowlist of the few devices
  that can *never* be a persistent disk, rather than a prefix-list of
  disk names that is always one new device-naming convention (as
  `mmcblk`/`nvme`/`xvd`/`disk` already demonstrated) away from a false
  negative. The denylist's redirect/`dd of=` rules and the escalation
  rule's disk-write check both now share this exact same broadened
  concept (`isDiskRedirect`/`isDdRealDiskWrite`). Proven by
  `TestEvaluateDenylistBlocksBroadDiskDeviceFamilyOnRedirect`/`...OnDdOf`
  (nvme0n1/vda/xvda/mmcblk0/disk0 → `Block`) and
  `TestEvaluateSafePseudoDevicesAreNeverTreatedAsDisks`/
  `TestHasRealDiskDeviceReference` (`/dev/null`, `/dev/urandom`,
  `/dev/tty1`, `/dev/fd/3`, `/dev/pts/0` → never treated as a disk).
- **PowerShell Remove-Item aliases reached Allow (BLOCKER 3) — the
  denylist and escalation rule now recognize the whole alias family, not
  just the literal cmdlet name.** `ri -r -fo C:\`, `rd /s /q C:\`,
  `del`/`erase`/`rmdir` at a drive root all reached `Allow`, since the
  original rule matched only the literal string `Remove-Item`. **Fix**:
  `removeItemAliasWords` (`internal/safety/denylist.go`) is the set
  `{remove-item, remove-itemproperty, ri, rd, rmdir, del, erase, rm}`,
  matched case-insensitively and by path basename; `hasRecurseIshFlag`/
  `hasForceIshFlag` accept an unambiguous, case-insensitive **prefix
  abbreviation** of `-Recurse`/`-Force` (`-r`, `-rec`, `-fo`, ... —
  mirroring PowerShell's own parameter-name-abbreviation behavior) *or*
  the cmd.exe legacy `/s` (subdirectories)/`/q` (quiet) flags `rd`/`del`
  inherit from cmd.exe. The same alias set and flag-acceptance logic now
  drives both `isRemoveItemAliasDriveRootDelete` (denylist: alias +
  recurse-ish flag + drive-root target → `Block`) and
  `isRemoveItemAliasEscalation` (escalation: alias + recurse-ish OR
  force-ish flag + *any* target → `destructive`), replacing the old
  literal-`Remove-Item`-only regex entirely rather than adding a parallel
  alias-only rule next to it. Proven by
  `TestEvaluateDenylistBlocksRemoveItemAliasesAtDriveRoot` (every alias +
  every accepted flag spelling, including `rd /s /q C:\`, at a drive root
  → `Block`; `ri .\build`/`rd C:\` with no recurse-ish flag → `Allow`),
  `TestEvaluateEscalationRemoveItemAliasAnyTarget`, and direct
  `TestIsRemoveItemAliasWord`/`TestHasAbbreviatedFlag` unit tests.
- **Root-delete near-equivalents were near-misses instead of Blocks
  (MAJOR 4).** `rm -rf //`, `rm -rf /.`, `rm -rf ~/`, `rm -rf $HOME/`,
  `/bin/rm -rf /`, and PowerShell's `rm -Recurse -Force C:\` all reached
  only `Confirm` (or, for the basename case, `Allow` outright), since the
  target-equality check was an exact string match and the command-name
  check was an exact `"rm"` match. **Fix**: (a) `normalizeRootTarget`
  (`internal/safety/denylist.go`) canonicalizes a `/`-rooted target with
  the standard library's **`path.Clean`** before the `rootTargets`
  lookup, rather than an ad-hoc set of hand-picked cases — `path.Clean`
  is real path-cleaning logic, so it collapses every "resolves to root"
  spelling in one pass (repeated slashes, a `.` segment anywhere, *and*
  a `..` segment that can't climb above root), is never fooled by a
  glob-suffixed kept target (`path.Clean("/*") == "/*"`, since `*` is an
  ordinary path segment), and never touches a genuine near-miss
  (`path.Clean("/tmp/x") == "/tmp/x"`). `~`/`$HOME`/`${HOME}` aren't path
  syntax, so `path.Clean` doesn't understand them at all — their
  trailing-slash/trailing-`/.` normalization stays hand-written, exactly
  as before, applied only when the target does *not* start with `/`; (b)
  `isRmRootDelete` matches `rm` by `path.Base`, so `/bin/rm`/`/usr/bin/rm`
  count; (c) `rm` is already a member of `removeItemAliasWords` (see
  BLOCKER 3), so a PowerShell-style `rm -Recurse -Force C:\` at a drive
  root is caught by `isRemoveItemAliasDriveRootDelete` the same way
  `ri`/`rd` are. Proven by `TestEvaluateDenylistBlocksRmRootNearEquivalents`
  (all six → `Block`) and `TestEvaluateRmRootNearMissesStillOnlyEscalate`
  (`rm -rf ./build`, `rm -rf /tmp/x`, `rm -rf ~/project` still only
  escalate, confirming the fix didn't overreach into real near-misses)
  plus a direct `TestNormalizeRootTarget` unit test.
  - **Residual found on re-verification, fixed the same way.** The first
    hardening pass's `normalizeRootTarget` was still an ad-hoc single-pass
    string transform (collapse repeated slashes via regex, then strip one
    trailing `/.` or one trailing `/`) rather than `path.Clean`, so it
    missed `..` segments entirely: `rm -rf /..`, `rm -rf /./`,
    `rm -rf /.//`, and `rm -rf /../..` all still reached only `Confirm`.
    Since the underlying target-canonicalization problem is exactly what
    `path.Clean` already solves correctly (and the stdlib had been
    available the whole time — `path` was already imported for
    `path.Base`), the root-cause fix was replacing the ad-hoc `/`-rooted
    branch with `path.Clean` outright rather than adding a fourth
    hand-picked case for `..`. Proven by
    `TestEvaluateDenylistBlocksRmRootDotDotVariants` (all four → `Block`,
    plus the `./build`/`/tmp/x`/`~/project` near-miss set re-asserted in
    the same test) and four new `TestNormalizeRootTarget` cases
    (`/..`, `/./`, `/.//`, `/../..` → `/`).
- **Dry-run table showed the LLM's declared risk, not the safety engine's
  verdict (MEDIUM 6).** `renderPlan` (`internal/cli/do.go`) now always
  renders `step.Decision.EffectiveRisk`, never the raw `step.Risk` the
  model produced: an `Allow` row shows the plain effective-risk name, a
  `Confirm` row shows `CONFIRM(<effective risk>)` (so an escalation is
  visible even when it isn't severe enough to `Block`), and a `Block` row
  shows `BLOCKED(<reason>)` exactly as before. Proven by the exact-value
  `TestRenderPlanShowsEffectiveRiskNotDeclaredRisk` (three synthetic steps
  whose `Decision.EffectiveRisk` deliberately disagrees with `step.Risk`
  in both directions) and updated `TestDoDryRunRendersPlanTableAgainstMockProvider`
  assertions (`CONFIRM(elevated)` for the two `sudo` steps).
- **E2E fixture strengthened (MINOR 7).** The `do_test.go` mock plan's
  decoy `rm -rf /` step is now labeled `"risk": "read"` (previously
  `"destructive"`) — the strongest form of the independence proof: the
  denylist `Block` doesn't even consult the declared risk, so it must
  fire even when the model's own label is actively, maximally wrong in
  the benign direction, not just "close but under-labeled".

### `internal/engine` — plan generation

- **`Planner`** (`NewPlanner(client Completer, cfg config.Config)`): DI'd
  over a minimal **`Completer`** interface (`Complete(ctx,
  llm.CompletionRequest) (llm.CompletionResponse, error)`) defined in this
  package so tests mock it without depending on `*llm.Client`'s full
  surface — any real `*llm.Client` satisfies it as-is. A `safety.Engine`
  is built once at construction (not per-call) from the same `cfg`.
- **`GeneratePlan(ctx, request string, sysCtx contextpkg.Context) (Plan,
  error)`**: builds the system prompt (see below), requests a completion
  with `RequiredFields: []string{"summary"}` (see Decisions for why
  `"steps"` is deliberately excluded), decodes the JSON response, and:
  - an empty `steps` array triggers exactly one automatic corrective
    re-prompt (`emptyStepsCorrection`); still empty after that is a hard
    error.
  - more steps than `safety.max_auto_steps` triggers exactly one
    automatic consolidate re-prompt (`consolidateCorrectionFormat`); if
    the result (or the original, if the retry itself errored) still
    exceeds the limit, the plan is hard-truncated with a bracketed
    warning marker prepended to `Summary` rather than erroring.
  - any step whose `risk` field doesn't parse via `safety.ParseRiskClass`
    is defaulted to `RiskDestructive` (fail-closed) and a bracketed note
    is appended to `Summary`.
  - every step is then run through `Planner`'s `safety.Engine` and
    annotated with its `Decision` — `Plan`/`Step` carry `Decision`
    alongside `Command`/`Rationale`/`Risk`/`Reversible`, so a caller never
    has to re-derive it.
  - a step with an empty `"command"` is a hard error (not one of the two
    documented re-prompt cases — see Decisions).
- **System prompts** (`internal/engine/prompts/`, `go:embed`'d):
  `plan_system.txt` is the English core instruction — exact JSON shape,
  OS/shell targeting (Unix always targets POSIX `sh` even when the user's
  interactive shell is fish, since commands run via `sh -c`; Windows
  always targets PowerShell), the five risk labels with one-line
  definitions and a "label conservatively, pick the higher class when in
  doubt" rule, the reversibility flag's meaning, "never chain more than
  one risky operation into a single step — split it", "prefer the
  detected package manager", and "never emit an interactive/TUI command;
  add a safe non-interactive flag where one exists". `plan_lang_tr.txt` is
  appended only when the resolved language is `tr`: it instructs the
  model to write `summary`/`rationale` in Turkish while keeping every JSON
  key name, the `risk` value, and the `command` text itself unchanged.
  `serializeContext` (in `prompt.go`) appends OS/architecture
  (`runtime.GOARCH`, read directly — not a `contextpkg.Context` field, see
  Decisions)/shell/working directory/detected package managers/admin
  status, always in English (grounding for the model, never shown to the
  user), and never `History`/`EnvNames`/`LastCommand`.
- **Language resolution** (`resolveLanguage`): `general.language` wins
  outright when it is `"tr"` or `"en"`; `"auto"` (the schema default, and
  an empty string, treated the same) falls through to `$LANG` — a
  `tr`-prefixed value resolves `"tr"`, anything else (including unset)
  resolves `"en"`.

### `comrade do <request...> --dry-run` (hidden)

- New hidden cobra command (`internal/cli/do.go`), wired into the root
  command tree. Without `--dry-run` it returns
  `errDryRunRequired` ("execution arrives in a later phase; use
  --dry-run"), printed to stderr by `main.go` and exiting 1 — no plan
  generation, no network call, is attempted at all in that path.
- With `--dry-run`: loads the effective config (printing the first-run
  notice to stderr exactly like every other subcommand), builds a real
  `llm.Client`, collects a real `contextpkg.Context` via
  `contextpkg.NewCollector()`, runs `engine.NewPlanner(client,
  *cfg).GeneratePlan`, and renders the result with `renderPlan`: the
  summary line, a blank line, then a `text/tabwriter`-aligned `STEP |
  COMMAND | RISK | REVERSIBLE | RATIONALE` table. A step the safety
  engine `Block`ed renders `BLOCKED(<reason>)` in its RISK column instead
  of the plain risk name; everything else shows its risk name plainly
  (its `Decision.Action` may still be `Confirm` — FAZ 6's mode logic is
  what actually acts on that, not this phase).

## Decisions / deviations

- **Conservative, fail-closed command matching — the quoted-echo false
  positive is accepted on purpose.** `normalizeCommand` strips every quote
  character anywhere in the string and collapses whitespace, and
  `tokenizeCommand` then splits the result on whitespace, without any real
  shell-grammar awareness. This means `echo "rm -rf /"` normalizes/
  tokenizes identically to a bare `rm -rf /` and is Blocked — a false
  positive CLAUDE.md's safety mandate explicitly prefers over the
  alternative false negative: missing an actually-dangerous invocation
  routed through `sh -c "..."`/`bash -c '...'`. `TestEvaluateDenylistBlocksRmRootDeleteVariants`'s
  `"quoted echo false positive is accepted (fail-closed)"` case pins this
  choice as a regression test, not an accident.
- **Denylist vs. escalation for "same-shaped" commands is deliberately
  split by target specificity.** `rm -rf /tmp/x` and `Remove-Item
  -Recurse C:\Users\foo` carry the same dangerous *flags* as their
  denylisted cousins but target something other than the whole
  root/home/drive — they escalate to `destructive` (via `Confirm`) rather
  than `Block`. (An earlier draft of this document used `dd ...
  of=/dev/loop0` as a third example of this split; the post-review
  hardening pass's MEDIUM 5 fix folded loopback devices into the
  broadened "any real disk" concept, so that specific command now Blocks
  — see the hardening section above. The rm/Remove-Item near-miss split
  is unaffected and still exactly the near-miss behavior UYGULAMA_PLANI.md
  FAZ 5 calls for.) Covered by dedicated test cases per rule (e.g.
  `TestEvaluateRmRootNearMissesStillOnlyEscalate`,
  `TestEvaluateDenylistBlocksRemoveItemRecurseDriveRoot`'s subdirectory
  case).
- **Invalid `denylist_extra` pattern: skip + exactly one stderr warning,
  never a construction error or panic.** A user's config is not something
  `NewEngine` should be able to crash the whole plan-generation path over;
  the warning is printed once, at construction (regexes are compiled
  exactly once, not per-`Evaluate` call), and
  `TestNewEngineSkipsInvalidUserDenylistPatternWithOneStderrWarning`
  captures `os.Stderr` to prove both the one-warning-at-construction
  behavior and that `Evaluate` itself never re-warns.
- **`requestRawPlan`'s `RequiredFields` is `["summary"]`, not
  `["summary", "steps"]`.** `internal/llm/parse.go`'s `isEmptyJSONValue`
  treats an empty JSON array as "missing" for `ValidateInto`'s
  required-field check. Requiring `"steps"` there would turn a legitimate
  (if unhelpful) `{"summary": "...", "steps": []}` response into a hard
  `llm.ErrParseFailure` inside `llm.Client` itself, before `GeneratePlan`
  ever got a chance to run its own documented empty-steps re-prompt
  (UYGULAMA_PLANI.md FAZ 5 item 3). Requiring only `"summary"` lets that
  response through so `GeneratePlan`'s own `len(raw.Steps) == 0` check —
  not `llm.Client`'s generic one — is what triggers the re-prompt.
  Discovered by `TestGeneratePlanEmptyStepsTriggersOneCorrectiveReprompt`
  actually failing against the naive `["summary","steps"]` version before
  this fix.
- **An empty `"command"` in a step is a hard error, not a third re-prompt
  path.** UYGULAMA_PLANI.md FAZ 5 only names two malformed-response
  recoveries (empty `steps`, too many `steps`); a step with no command at
  all is rare enough in practice (every `internal/llm` connector returns
  non-empty text, and an instructed model reliably fills `"command"`)
  that surfacing it straight to the caller is preferable to inventing a
  third bespoke recovery path this phase wasn't asked for.
- **Unknown risk label fails closed to `RiskDestructive` — the "note" is
  a bracketed marker appended to `Plan.Summary`**, not a new `Step` field
  or a change to the LLM-authored `Rationale` text (which is meant to
  stay exactly what the model wrote, in the user's language). Same
  mechanism, and the same reasoning, as the truncation marker.
- **Architecture is read directly via `runtime.GOARCH`, not added to
  `contextpkg.Context`.** UYGULAMA_PLANI.md's context block wording
  mentions "OS, arch, shell, cwd, package managers, admin", but
  `internal/context.Context` (FAZ 3) has no `Arch` field, and this phase's
  scope explicitly excludes touching `internal/context`. Architecture is a
  build-time constant, not something that needs OS-process collection, so
  `internal/engine/prompt.go`'s `serializeContext` reads `runtime.GOARCH`
  inline instead.
- **Import direction is strictly one-way**: `internal/engine` depends on
  `internal/llm`, `internal/safety`, `internal/context`, `internal/config`;
  `internal/safety` depends only on `internal/config` and the standard
  library; neither of those (nor `internal/llm`/`internal/context`) import
  `internal/engine` back. `internal/cli` sits above `internal/engine` and
  is the one package allowed to import it, to wire up `comrade do`.
- **`comrade do` is hidden and requires `--dry-run`, on purpose.** This
  phase performs no execution whatsoever — FAZ 6 replaces this file's
  `RunE` with the real auto/ask/info execution loop and un-hides the
  command (likely renaming/repurposing it per UYGULAMA_PLANI.md FAZ 6 item
  3's root-command fallback). `TestDoIsHiddenFromHelp` pins the
  hidden-for-now state so it doesn't silently leak into `--help` before
  FAZ 6 is ready for it.
- **Manual, real-LLM acceptance item** (tracked in `docs/PROGRESS.md`,
  not automated here): running `./comrade do "docker kur" --dry-run"`
  against a real, configured API key and eyeballing that the live model's
  plan is sane for the target OS. The `httptest`-mocked end-to-end test
  below proves the whole pipeline (prompt → parse → safety engine →
  render) wires together correctly and deterministically; it cannot prove
  a real model's plan quality.

## Gate

- `go vet ./...` — clean.
- `/home/firfir/go/bin/golangci-lint run` — 0 issues.
- `go test ./... -count=1` — all packages pass, including the four test
  files in `internal/safety` (`risk_test.go`, `engine_test.go`,
  `hardening_test.go`: 236 table-driven sub-cases + 41 standalone tests
  after the post-review hardening pass and its residual follow-up fix,
  up from an initial 99+27), `internal/engine`'s
  `planner_test.go` (mock-`Completer`-driven unit tests covering the
  happy path, both re-prompt recoveries, the unknown-risk fail-closed
  path, the empty-command error, error propagation, and language
  resolution), and `internal/cli`'s `do_test.go` (the `httptest`
  mock-provider end-to-end test, the exact-value `renderPlan` unit test,
  the mandatory-`--dry-run` guard, and the hidden-from-help check).
- `go test -race ./internal/safety/ ./internal/engine/ ./internal/cli/` —
  clean, no data races, run specifically over the three packages this
  hardening pass touched.
- `make build` — succeeds.
- `make cross` — succeeds for all five targets (linux/amd64,
  linux/arm64, darwin/amd64, darwin/arm64, windows/amd64).

### `comrade do "install docker" --dry-run` — real binary, mock provider

The compiled `./comrade` binary, run for real against a small local
`net/http` server standing in for an `openai_compat` endpoint (started via
`go run`, listening on `127.0.0.1:18322`, answering a canned three-step
"docker kur" plan for any `POST /chat/completions`):

```
$ export COMRADE_PROVIDER=openai_compat
$ export COMRADE_LLM_OPENAI_COMPAT_BASE_URL=http://127.0.0.1:18322
$ export COMRADE_OPENAI_COMPAT_API_KEY=demo-key
$ ./comrade do "install docker" --dry-run
Created default config at <tmp>/.config/cli-comrade/config.toml
Docker kurulur ve başlatılır.

STEP  COMMAND                             RISK                                                                 REVERSIBLE  RATIONALE
1     sudo apt-get install -y docker.io   CONFIRM(elevated)                                                    false       Docker paketini kurar.
2     sudo systemctl enable --now docker  CONFIRM(elevated)                                                    true        Docker servisini etkinleştirir ve başlatır.
3     rm -rf /                            BLOCKED(matches denylist rule: rm -rf / (or ~ / $HOME root delete))  false       Modelin asla üretmemesi gereken bir deneme.
```

(Re-run after the post-review hardening pass — MEDIUM 6 — and with the
decoy step's fixture risk relabeled to `"read"`, MINOR 7; see that
section above.) Steps 1 and 2 render `CONFIRM(elevated)`: the safety
engine's own `EffectiveRisk`/`Action`, not a bare echo of the model's
`"elevated"` label — the table's whole purpose is to surface that second
check, not redisplay the model's claim. Step 3 — a `rm -rf /` decoy the
mock model labeled `"read"` (the most adversarial label possible for this
probe) — is rendered `BLOCKED(...)` by `internal/safety.Engine`, proving
the LLM-independent second check holds even when the model's own output
is actively, maximally wrong, not merely under-cautious. This exact
scenario is committed as `TestDoDryRunRendersPlanTableAgainstMockProvider`
in `internal/cli/do_test.go`, using an `httptest.Server` instead of a
hand-run one so it runs deterministically under `go test`.
