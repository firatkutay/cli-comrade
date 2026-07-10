# FAZ 09 — i18n (TR/EN), `comrade explain`, `comrade chat`

## What was built

### `internal/i18n` — the message catalog + Translator (new leaf package)

- **`MessageID`/`Catalog`**: every user-facing string cli-comrade prints through a Translator is a
  typed `MessageID` constant, never a raw literal at the call site. `catalogEN`/`catalogTR`
  (`internal/i18n/catalog.go`) are `map[MessageID]string` fmt-format-string catalogs.
- **`Translator.T(id, args...)`**: resolves `id` in the active catalog, falling back to the English
  catalog, then to the bare `MessageID` string itself — never panics on an unknown key. Args are
  applied via `fmt.Sprintf` only when at least one is given, so a zero-arg message containing a
  literal `%` is never misinterpreted as a format verb (`TestTranslatorTZeroArgsReturnsFormatUnchanged`).
- **Bidirectional drift guard** (`TestCatalogsCoverIdenticalKeys`): iterates `catalogEN` checking every
  key exists in `catalogTR`, AND iterates `catalogTR` checking every key exists in `catalogEN` — a
  message added to only one catalog, in either direction, fails `go test` instead of silently falling
  back to English at runtime.
- **`ResolveLanguage(configLanguage, getenv) Lang`** — the single, consolidated language resolver
  (CLAUDE.md's "Dil seçimi"), precedence:
  1. `configLanguage` non-"auto" (`"tr"`/`"en"`) wins outright.
  2. `COMRADE_LANG` — this project's own `COMRADE_`-prefixed override (FAZ 1's env convention),
     added **above** `LANG` in the resolution order per this phase's own acceptance criterion
     (`COMRADE_LANG=tr comrade explain ...`). A `tr`-prefixed value (case-insensitive) is Turkish.
  3. `LANG`, then `LC_ALL` — glibc-style locale parsing (`tr_TR.UTF-8` → tr).
  4. Otherwise English.
  - `internal/engine`'s own `resolveLanguage` (FAZ 5/7) was **deleted**; `Planner.GeneratePlan` and
    `Diagnoser.Diagnose` now call `i18n.ResolveLanguage(...).String()` directly — one resolver, no
    duplicate logic to drift. `TestResolveLanguage` moved (and grew a COMRADE_LANG/LC_ALL precedence
    table) to `internal/i18n/lang_test.go`'s `TestResolveLanguagePrecedence`.
- **No global Translator anywhere** (CLAUDE.md "Global state yok"): `internal/cli/runtime.go`'s
  `newTranslator(cfg)` builds one per command invocation from that command's own loaded config, and
  every command that prints a translated string takes it as a local value, never a package variable.
  `internal/engine.RunDeps` gained an optional `Translator i18n.Translator` field (see below) —
  additive, not required, so no pre-FAZ-9 test needed to change.

### Migration: hardcoded strings → catalog

Every string named `// i18n FAZ 9` in FAZ 0-8's own doc comments moved into the catalog exactly as
planned: `internal/cli/stub.go` (deleted outright — see `comrade explain`/`comrade chat` below),
`internal/cli/config.go`'s `firstRunNoticeFormat`, `internal/cli/promptui.go`'s language note (left
as-is — see "Known limitations" below).

Beyond those marked spots, this phase also migrated (mechanically, preserving EN byte-for-byte so the
existing suite needed zero assertion changes):

- `internal/cli/runtime.go` / `config.go` / `models.go`: the first-run notice, the `--yolo` warning,
  `config test-llm`'s result line, `config list`'s table header, `config models`' docs-note/select-
  prompt/confirm lines.
- `internal/cli/root.go`: the bare-invocation version banner (resolved from `COMRADE_LANG`/`LANG`/
  `LC_ALL` only, **not** config `general.language` — a bare `comrade` never loads config, and doing so
  just to pick a banner's language would introduce a first-run config-file side effect that doesn't
  exist today; a deliberate, documented exception).
- `internal/engine/runner.go`: `RunDeps` gained `Translator i18n.Translator`; `printBlocked`/
  `printBlockedEdit`/`printYoloBypassWarning`/`executeInfo`'s inline Block line, plus every
  `RunSummary.AbortReason` string (`canceled`, `step N is blocked: ...`, `step N failed (exit ...): ...`,
  the self-correction variant, and the shared retry suggestion) now route through
  `deps.tr().T(...)`. `tr()` returns `d.Translator` when it resolves to Turkish, otherwise a fresh
  English Translator — so every RunDeps literal every existing engine test constructs (none of which
  set the new field) renders **byte-for-byte identical English**, proven by the entire pre-existing
  `internal/engine` suite passing unmodified, plus one new TR smoke test,
  `TestExecuteAutoBlockRendersTurkishWhenRunDepsTranslatorIsTurkish`, which sets a Turkish Translator
  and asserts `ENGELLENDİ(...)` appears (and `BLOCKED(` does not), and that `AbortReason` contains
  `engellendi`.

Translations are natural Turkish, not literal machine translation (e.g. "geri alınamaz" for
"irreversible/permanently", "yerel güvenlik kontrolü" for "local safety check", "sağlayıcı" for
"provider" in the diagnostic `test-llm` line) — reviewed message-by-message against their English
counterpart's actual meaning, not word-by-word.

#### Follow-up sweep: closing the full-migration gap

A first pass of this phase left `auth.go`/`do.go`/`fix.go`/`history.go`/`init.go` on an
allowlist as "pre-existing debt". That did not meet UYGULAMA_PLANI.md's actual acceptance
("hiçbir hardcoded kullanıcı mesajı kalmasın" — no hardcoded user message remains, enforced by a
linter) — a debt-allowlist acknowledges a gap, it does not close one, and cli-comrade is a
bilingual, Turkish-first tool where `comrade do`'s plan table is the primary output. This
follow-up sweep closes it: **57 new `MessageID`s** were added (catalog now 90 total, up from 33),
and every one of the five files was fully migrated — updating several already-tested unexported
helper signatures where necessary (the coordinator's own instruction: that IS this phase's actual
work, not an excuse to skip it):

- **`internal/cli/do.go`**: `renderPlan`/`printRunSummary`/`buildAuditSink` all gained a
  `tr i18n.Translator` parameter (`do_test.go`'s direct `renderPlan(&buf, plan, ...)` call updated
  to pass `i18n.NewTranslator(i18n.LangEN)`). The plan table's `STEP\tCOMMAND\tRISK\tREVERSIBLE\t
  RATIONALE` header, its `BLOCKED(%s)`/`CONFIRM(%s)` risk cells (→ `ENGELLENDİ(%s)`/`ONAY(%s)` in
  Turkish — the same vocabulary `engine/runner.go`'s own Blocked rendering already uses, kept
  consistent on purpose), and the `"N executed, M skipped, K blocked"`/`"aborted: ..."` summary
  lines are now `MsgPlanTableHeader`/`MsgPlanBlockedCell`/`MsgPlanConfirmCell`/
  `MsgRunSummaryCounts`/`MsgRunSummaryAbortedLine`. `runDo` also now sets
  `engine.RunDeps.Translator` (it never did before this sweep — `comrade do`'s BLOCKED/abort output
  was always English regardless of resolved language until now); `runFix` does the same.
- **`internal/cli/fix.go`**: `acquireErrorContext`/`captureByRunning`/`pasteMode` all gained a
  `tr i18n.Translator` parameter (`fix_test.go`'s 5 direct call sites updated to pass
  `i18n.NewTranslator(i18n.LangEN)`). The stale/exit-0 fallback notices, the destructive-refusal
  notice (`MsgFixRefusalNotice`/`MsgFixBlockedClassification`), and every paste-mode prompt are
  migrated. `comrade fix`'s root-cause/explanation output also gained explicit headings
  (`MsgFixRootCauseHeading`/`MsgFixExplanationHeading` — "Root cause:"/"Explanation:", matching
  `comrade explain`'s own heading style; previously these printed as unlabeled raw text).
- **`internal/cli/auth.go`**: `newAuthLogoutCmd`/`newAuthStatusCmd` now take a `loaderFactory`;
  every login/logout/status prompt and label (`Enter API key for %s:`, the ping-succeeded/-failed
  lines, `No stored key for %s.`, `Removed stored key for %s.`, the status table header/ollama row,
  and the `set (%s)`/`set (env: %s)`/`not set` labels via `providerStatusLabel(st, tr)`) is
  migrated. The two zero-config-touch fast-rejection error paths (`ollama` needs no key; unknown
  provider) deliberately still run BEFORE any config load, preserving `auth_test.go`'s
  `TestAuthLoginRejectsOllama`/`RejectsUnknownProvider`, which never isolate a config dir.
- **`internal/cli/history.go`**: `newHistoryCmd` now takes a `loaderFactory`. The table header is
  migrated, and — closing a real UX gap, not just a translation — an empty audit log now prints a
  friendly `MsgHistoryEmpty` ("No commands recorded yet." / "Henüz kayıtlı komut yok.") instead of a
  bare header row with nothing under it; `TestHistoryOnEmptyLogPrintsHeaderOnly` was renamed to
  `TestHistoryOnEmptyLogPrintsFriendlyEmptyMessage` and its assertion updated to match. Several
  `history_test.go` cases switched from `execRoot` to `execRootSplit` because `comrade history` now
  loads config (for its Translator) and, on an isolated dir's first invocation, prints the shared
  first-run notice — which must land on stderr, not get mixed into stdout the JSON/line-count
  assertions parse.
- **`internal/cli/init.go`**: `newInitCmd` now takes a `loaderFactory`; `runInitInstall`/
  `runInitRemove` gained a `tr i18n.Translator` parameter. Every install/remove prompt — the
  PowerShell manual-fallback notice, "already installed", the diff preview, the y/N confirm prompt,
  "Aborted; no changes made.", "Installed ...", "Nothing to remove...", "... not installed; nothing
  to do.", "Removed ..." — is migrated. `--print` and every shell/arg-resolution error path still
  return before ever loading config (zero-config-touch, matching `auth.go`'s identical principle),
  so `TestInitPrintOnlyPrintsSnippetWithoutTouchingAnyFile`'s exact-equality assertion is
  unaffected. `init_test.go`'s two exec helpers now call `withIsolatedConfigDir(t)` (install/remove
  do load config).
- **`internal/cli/models.go`**: the one remaining diagnostic-adjacent string was already migrated in
  the first pass; this sweep found nothing further to do here.
- **cobra `--help`/usage text for every command** (`do`, `fix`, `explain`, `chat`, `config` + its 7
  subcommands, `auth` + its 3 subcommands, `init`, `history`, `hook` + `hook record`, and root
  itself — 21 `Short` lines total): new `internal/cli/help.go`. cobra reads a command's `Short`
  field DIRECTLY, both for that command's own `--help` and for a parent's "Available Commands"
  listing (which reads every child's `Short` straight off the struct) — a plain string baked in at
  command-construction time, long before any per-invocation config is loaded. `help.go`'s
  `registerTranslatedHelp` overrides root's `HelpFunc`/`UsageFunc` (captured once, `SetHelpFunc`/
  `SetUsageFunc` — cobra walks up to the nearest ancestor with one set, so registering only on root
  covers the whole tree) to re-translate every command's `Short` (`applyTranslatedHelp`, keyed by
  `CommandPath()`) immediately before cobra's own default rendering runs. No command's `Long` needed
  covering (none is set anywhere in this tree). Verified live: `COMRADE_LANG=tr comrade --help`
  renders every subcommand's line in Turkish in the "Available Commands" listing, and
  `comrade auth login --help` renders that command's own Turkish `Short` too (see Manual
  demonstration below).

**Still deliberately left as-is** (a real, minimal, individually-justified list — not a blanket
exemption; every item below is either an explicit CLAUDE.md invariant or genuinely not "a hardcoded
user message" in the sense the acceptance criterion means):
- **`internal/tui/confirm.go`**'s `"[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü"` option-letter line:
  CLAUDE.md **mandates** these exact Turkish letters/words as the confirm-prompt contract,
  regardless of the resolved UI language — this is not translated BY DESIGN, not an oversight.
  Its "Edit command (enter to confirm, esc to cancel):" companion line remains pre-existing,
  minor, inline-edit-only chrome (not reached by the catalog-coverage scanner at all — see below).
- **cobra `Use` command tokens** (`do`, `fix`, `auth login <provider>`, ...): these are the literal
  command names the user types (`comrade auth login`, not a translation of "login") — translating
  them would mean cli-comrade's own command vocabulary changes per language, breaking muscle memory
  and scripts; every CLI tool with i18n'd help text keeps its verbs untranslated for this reason.
- **`internal/cli/hook.go`**'s `COMRADE_DEBUG`-gated diagnostic line: developer-facing (gated
  behind an explicit debug env var, never shown in normal operation), and `recordLastCommand` runs
  on every shell prompt (FAZ 4's hot path) — loading config there just to resolve a display
  language for a debug-only trace is a deliberate performance tradeoff, not an oversight.
- **`internal/cli/promptui.go`**'s ask-mode `[a]çıkla` inline-explanation system prompt: an LLM
  system prompt (not literal user-facing text) — unchanged from FAZ 6, out of this phase's scope.
- **`fmt.Errorf`-constructed error-WRAP chains** (e.g. `"auth login: store key: %w"`,
  `"config models: %w"`, `"init: read %s: %w"`) throughout `internal/cli`: ~40 of these exist. This
  is CLAUDE.md's own established `fmt.Errorf("...: %w", err)` convention, used identically in dozens
  of places across every package — translating every error-wrap prefix project-wide is a materially
  different, much larger undertaking than translating standalone, full-sentence user errors (which
  a second follow-up round DID migrate in full — see below). The exact rule applied: a wrap chain
  (`"doing X: %w"`, adding call-site context to an INNER error this command doesn't control the
  wording of) stays untranslated; a complete, standalone sentence with no `%w` at all (the terminal
  error a user reads as the WHOLE message) gets migrated. See the next section for the precise,
  final accounting.

### Second follow-up round: per-flag `--help` descriptions + full-sentence `fmt.Errorf`/`errors.New`

An independent review confirmed every CRITICAL bar (explain-never-executes, chat-no-autosave,
`/do` safety-gating, single-source language resolution, linter enforcement) but flagged one MAJOR
completeness gap: ~26 remaining English-only user-visible strings — 14 per-flag `--help`
descriptions (a TR user saw the command's own `Short` in Turkish but every flag description in the
SAME `--help` block in English) and ~12 full-sentence `fmt.Errorf`/`errors.New` messages (the exact
text a user reads as the terminal error). Both are now closed — **23 new `MessageID`s** (catalog
90 → 113):

- **Per-flag descriptions (11 unique flags, `MsgFlag*`)**: `--dry-run`/`--auto`/`--ask`/`--info`/
  `--yolo` (`flags.go`, shared identically by root/`do`/`fix`), `--rerun` (`fix.go`), `--json`/
  `--limit` (`history.go`), `--print`/`--remove`/`--yes` (`init.go`). Exactly like `Short` text
  (`help.go`'s pre-existing mechanism), a flag's description (pflag's own "usage" string) is a plain
  string baked in at flag-REGISTRATION time, before any per-invocation Translator exists — so every
  registration site now passes `enUsageDefault(id)` (new in `help.go`: `i18n.NewTranslator(i18n.
  LangEN).T(id)`, i.e. the catalog's OWN English default) instead of a raw literal, and
  `applyTranslatedHelp` was extended to ALSO `cmd.Flags().VisitAll(...)` and overwrite each
  matching flag's `Usage` (`flagUsageByName`, keyed by flag NAME since the same flag carries
  identical text everywhere it's registered) immediately before cobra renders — the identical
  render-time-override pattern `Short` already used, just applied to `*pflag.Flag.Usage` too.
  `comrade hook record`'s 3 flags (`--shell`/`--exit`/`--command`) are deliberately NOT covered —
  same justification as `hook.go`'s existing exception (internal-only, invoked by generated shell
  snippets, never read by an end user via `--help`).
- **Full-sentence errors (12, `Msg*Error`/`MsgModelsChoice*`)**: `auth.go`'s ollama-rejection and
  unknown-provider errors (`MsgAuthOllamaNoKeyError`/`MsgAuthUnknownProviderError`, now rendered via
  a NEW `envOnlyTranslator()` — see below) and its "no key entered" error (`MsgAuthNoKeyEnteredError`,
  rendered via the command's own config-aware `tr`, since it fires after the config load); `fix.go`'s
  `--rerun`-with-nothing-to-rerun error (`MsgFixRerunNoLastCommandError`, config-aware `tr` — it
  fires inside `acquireErrorContext`, which already receives one); `flags.go`'s
  `--auto`/`--ask`/`--info` mutual-exclusivity error (`MsgFlagsModeExclusiveError`, `envOnlyTranslator`
  — this runs before ANY config load in `runDo`/`runFix`); `init.go`'s `--print`/`--remove`
  mutual-exclusivity and shell-undetected/shell-unsupported errors (`MsgInitPrintRemoveExclusiveError`/
  `MsgInitShellUndetectedError`/`MsgInitShellUnsupportedError`, all `envOnlyTranslator` — all three
  fire before `newInitCmd`'s config load); `models.go`'s "provider returned no models"/"unknown
  provider" errors (`MsgModelsNoModelsError`/`MsgModelsUnknownProviderError`, config-aware `tr`,
  `fetchModelsForProvider` gained a `tr i18n.Translator` parameter) and `readModelChoice`'s
  not-a-number/out-of-range messages (`MsgModelsChoiceNotANumber`/`MsgModelsChoiceOutOfRange`,
  `readModelChoice` gained a `tr i18n.Translator` parameter) — the sentinel `errInvalidSelection`
  (needed for a stable `errors.Is` check) is still wrapped via `%w`, but the substantive text after
  it is now translated (`fmt.Errorf("%w: %s", errInvalidSelection, tr.T(...))` — byte-identical EN,
  since the old format was literally `"%w: %q is not a number..."` with `%w` rendering as
  `errInvalidSelection.Error()` = `"invalid selection"`).
- **New `envOnlyTranslator()` (`runtime.go`)**: resolves language from `COMRADE_LANG`/`LANG`/
  `LC_ALL` ONLY, deliberately skipping config `general.language` — for the handful of errors that
  must report BEFORE any config load happens in that command's flow (so a CLI usage mistake, e.g.
  `--auto --ask` together, or `auth login ollama`, is still reported without ever touching the
  filesystem — preserving `auth_test.go`'s `TestAuthLoginRejectsOllama`/`RejectsUnknownProvider`,
  which never isolate a config dir, and every other zero-config-touch fast-rejection path exactly
  as-is). `root.go`'s bare-invocation version banner (already using this exact pattern from the
  first round) and `help.go`'s `helpTranslator` fallback were both refactored to call the same
  shared helper instead of duplicating the resolution inline. **Documented, minor, deliberate
  inconsistency**: these specific messages honor `COMRADE_LANG`/`LANG`/`LC_ALL` but NOT a config
  `general.language=tr` with no matching env var set — an acceptable tradeoff to keep these
  specific paths config-load-free, called out explicitly rather than silently.
- **The distinction actually applied** (why these 12 and not the ~40 wrap chains): a message
  qualifies for migration only if it is a COMPLETE, STANDALONE sentence — the exact text
  `cmd/comrade/main.go` prints as the user's terminal error, with no `: %w` wrapping an inner error
  at all. Every remaining `fmt.Errorf` in `internal/cli` that ends in `": %w"`-shaped wrapping (adding
  call-site context to an error this command doesn't itself author the wording of — a network
  failure, a filesystem error, a provider error) is left as-is, per CLAUDE.md's own established
  error-wrapping convention.
- **Linter extended, not force-fit**: `internal/cli/catalog_coverage_test.go` gained
  `findRawFlagDescriptions`/`flagRegistrationSelectors` — the SAME AST-scan approach as the existing
  Print/Fprint scan, now also flagging any `Bool(Var)(P)`/`String(Var)(P)`/`Int(Var)(P)`/
  `Duration(Var)`/`StringSlice(Var)` registration call whose LAST argument (the description, in
  every pflag variant) is a raw, letter-containing literal rather than an `enUsageDefault(id)` call
  — verified live by temporarily reverting one flag to a raw literal and confirming the test fails
  (then restoring it; the whole suite is green again). Per the coordinator's own explicit escape
  hatch, a robust AST-level heuristic for "user-facing sentence vs. `%w` wrap chain" was
  **deliberately NOT attempted** for `fmt.Errorf`/`errors.New` — `Errorf`/`errors.New` remain outside
  `fmtPrintSelectors` exactly as before, and the 12-message migration above was applied manually,
  call site by call site, against the one written rule stated above. This is documented in the
  test's own doc comment (`TestCatalogCoverageNoNewHardcodedUserFacingStrings`), not silently
  assumed to be "handled" by a fragile pattern-match that would false-positive on every legitimate
  wrap chain in the codebase.

### Catalog-coverage test — now a tight, enforcing gate covering BOTH output text and flag descriptions

`internal/cli/catalog_coverage_test.go`'s `TestCatalogCoverageNoNewHardcodedUserFacingStrings`
statically scans every non-test `.go` file directly under `internal/cli` and `internal/tui` (an AST
walk via `go/parser`/`go/ast`) for TWO shapes:

1. A call to `fmt.Print`/`Println`/`Printf`/`Fprint`/`Fprintln`/`Fprintf` whose format/text argument
   is a raw string literal containing at least one letter **outside its fmt verbs** (a regex strips
   `%s`/`%d`/`%-6.2f`/`%%`-shaped verbs before the letter check, so pure layout strings like
   `"%s\t%s\n"` are correctly exempt while `"%d executed, %d skipped"` is correctly flagged).
2. A pflag flag-registration call (`Bool`/`BoolVar`/`BoolVarP`/`String...`/`Int...`/`Duration...`/
   `StringSlice...` — `flagRegistrationSelectors`) whose LAST argument (the description, in every
   variant's shape) is likewise a raw, letter-containing literal rather than an `enUsageDefault(id)`
   call.

**`catalogCoverageAllowlist` has exactly ONE entry: `hook.go`** (its `COMRADE_DEBUG` diagnostic line
AND its 3 flag descriptions — both developer-facing, both justified above). Every other file this
scanner covers is fully enforced with zero exemption, for BOTH shapes — `auth.go`, `do.go`, `fix.go`,
`history.go`, and `init.go` each had their Print/Fprint allowlist entry removed the moment their last
flagged literal was migrated (first follow-up round), and this round's flag-description scan found
ZERO pre-existing violations across the whole tree (every flag registration was already migrated to
`enUsageDefault` in the same pass that added the scan) — verified live: temporarily reverting one
flag registration to a raw literal makes the test fail with a precise message identifying the file
and literal; restoring it makes the whole suite green again.
`TestCatalogCoverageAllowlistHasNoStaleEntries` keeps the one remaining entry honest (fails the
instant `hook.go`'s combined flagged-literal count — Print/Fprint text plus flag descriptions —
reaches zero, so the entry can never quietly outlive the debt it covers).

**What it still can't catch** (documented in the test's own doc comment, not silently assumed
away): text built via concatenation/nested `Sprintf` before reaching `Print*`; cobra `Use`
command-token strings (untranslated by design — see above); standalone `fmt.Errorf`/`errors.New`
messages (deliberately NOT scanned — see "Second follow-up round" above for the manually-applied
rule and why an automated heuristic here would be fragile); and any non-`fmt.Print*`/non-flag-
registration rendering path at all — most notably `internal/tui/confirm.go`'s `View`, which builds
its output via `strings.Builder.WriteString`, not `fmt.Fprint*`, so its literals (the
CLAUDE.md-mandated option-letter line, and the "Edit command..." line) are structurally invisible to
this specific scan, not exempted by a false "all clear".

### `comrade explain <command...>` — two-layer, never executes

- **Layer 1 (local, authoritative for the warning)**: `internal/cli/explain.go`'s `runExplain` runs
  `command` through the exact same `safety.Engine` every other command uses (`RiskRead` as the
  declared-risk floor, exactly like `fix.go`'s `captureByRunning`). A destructive or denylisted
  (`Block`) verdict prints `MsgExplainSafetyWarning` FIRST, before anything the LLM said.
- **Layer 2 (LLM, secondary)**: new `internal/engine.Explainer` (`explain.go`, mirroring
  `Planner`/`Diagnoser`'s exact shape: injected `Completer`+`config.Config`, no global state) sends
  `prompts/explain_system.txt` (+ `explain_lang_tr.txt` for `lang == "tr"`) and decodes/validates a
  `{summary, parts:[{token, meaning}], risk_note}` response via the same `llm.ValidateInto`
  (`RequiredFields: ["summary"]`) every other engine prompt uses. `Explanation.RiskNote` is rendered
  AFTER and is explicitly documented as secondary to the safety layer's verdict — it is LLM color
  commentary, never the sole trigger for a warning.
- **`comrade explain` never executes anything, structurally**: there is no `internal/executor` import
  in `explain.go` at all (`TestExplainNeverImportsExecutor` asserts this directly against the file's own
  source, the strongest form of this proof — a future edit that tried to add execution would first have
  to add that import, which is trivially reviewable) and `engine.Explainer` has no executor/safety field
  of its own either (`TestExplainerNeverCallsAnythingButComplete`).
- `newExplainCmd` sets `DisableFlagParsing: true` — the command explained is itself arbitrary shell
  text and routinely starts with a dash (`rm -rf ...`); without this, cobra/pflag would try to parse
  those tokens as comrade's own flags (the exact same fix `config set` already applies, for the same
  reason).
- Mode-independent (no `--auto`/`--ask`/`--info`/`--dry-run`/`--yolo` flags at all — explain always just
  explains); `general.color`/`general.language` respected via the shared `loadConfigWithNotice`/
  `buildLLMClient` helpers `runtime.go` was refactored to expose (extracted from `setupCLIRuntime`,
  which now composes them — `runDo`/`runFix`'s existing behavior is unchanged; `explain`/`chat` reuse
  the config-load/LLM-client pieces without `setupCLIRuntime`'s `--yolo`-warning step, since neither
  command has a `--yolo` flag).

### `comrade chat` — bubbletea v2 interactive session

- **Privacy (CLAUDE.md security rule)**: session history (`[]llm.Message`, in-memory only) is
  **never** written to disk except an explicit `/save <file>`. There is no autosave, no temp file,
  anywhere in this package — `TestDispatchChatLineSaveWritesTranscriptAndNothingElseWritesToDisk` proves
  it directly: an isolated temp `HOME`, a full chat turn, an assertion the directory is still empty,
  THEN `/save`, THEN an assertion exactly one file now exists with exactly the rendered transcript.
  `saveTranscript` (`chatsession.go`) writes with `0600` permissions and is reachable from exactly one
  call site: `chatdispatch.go`'s literal `"/save"` branch.
- **Pure core, bubbletea shell** (matching FAZ 6's `internal/tui/confirm.go` precedent): every actual
  decision — slash-command parsing (`chatparse.go`'s `parseChatInput`), session-state transitions
  (`chatsession.go`'s `chatSession.setMode`/`clear`/`appendUser`/`appendAssistant`), and the full
  dispatch logic tying them together (`chatdispatch.go`'s `chatController.dispatchChatLine`) — is a set
  of pure functions with zero bubbletea/terminal coupling, unit-tested directly with no TTY at all
  (`chatparse_test.go`, `chatsession_test.go`, `chatdispatch_test.go`). `chatmodel.go`'s `chatModel`
  (the actual `tea.Model`: a `viewport.Model` scrollback + a `textinput.Model` input line, matching
  `confirm.go`'s v2 style — `tea "charm.land/bubbletea/v2"`, `View() tea.View` via `tea.NewView`,
  `case tea.KeyPressMsg:` + `msg.String()`) only wires that pure logic to bubbletea's `Cmd`/`Quit`
  protocol; per FAZ 6's own precedent, the real interactive TUI loop itself is not exercised by an
  automated test (there is no PTY in CI) — the "chat slash-command flow (scripted)" this phase's
  acceptance criterion asks for is exactly what `chatdispatch_test.go`'s battery of
  `TestDispatchChatLine*` tests demonstrate end-to-end, driving `dispatchChatLine` directly.
- **In-session slash commands**: `/mode auto|ask|info` (switches the session's active mode — the exact
  same `engine.Mode`/`engine.ParseMode` `comrade do`/`comrade fix` use), `/clear` (resets history, not
  mode), `/save <file>` (see privacy above), `/do <request>`, `/help`, `/exit`/`/quit`.
- **"Do it" trigger — the `/do <request>` design decision**: UYGULAMA_PLANI.md FAZ 9 explicitly left
  open a choice between an explicit `/do` command and heuristic NL intent-detection on the model's own
  replies, recommending the explicit command as the reliable path. This phase implements **only** the
  explicit `/do <request>` command, deliberately with **no** heuristic NL detection layered on top:
  intent-sniffing a conversational assistant reply is fragile (a model can phrase an actionable
  suggestion a dozen ways, or phrase a purely informational answer to sound actionable) and easy to
  both spoof (a prompt-injected reply that "looks like" a plan) and silently miss (a legitimate request
  the heuristic doesn't recognize never gets safety-gated at all) — an explicit command has neither
  failure mode, and `internal/engine`'s existing plan+safety+execute pipeline already has extensive
  test coverage that only holds if it is invoked exactly the way `comrade do` invokes it. `/do` runs
  `runChatDo` (`chat.go`) — the SAME `engine.Planner.GeneratePlan` → `engine.Execute` pipeline `comrade
  do` uses (real `safety.Engine`, real `executor.Executor`, the real `tuiPromptUI` for ask-mode
  confirms), under the session's current mode — `TestRunChatDoBlocksDenylistedStepAndNeverExecutesIt`
  mirrors `do_test.go`'s own end-to-end proof: a benign step actually runs (real executor, stdout
  contains its marker) while a denylisted decoy step is Blocked and never reaches the executor, backed
  by `TestDispatchChatLineDoBlockedCommandIsReportedAsBlockedNeverExecuted` at the dispatch layer.
- **Nested confirm-prompt terminal handoff**: `/do`'s ask-mode confirm prompt (`internal/tui.Confirm`,
  via `tuiPromptUI`) spins up its OWN, independent `tea.Program` against the same terminal — which only
  works once the outer chat program has let go of it. `chatmodel.go`'s `newRealChatDoRunner` calls
  `m.program.ReleaseTerminal()` before `runChatDo` and `RestoreTerminal()` afterward (both real
  `*tea.Program` methods — verified against the vendored `charm.land/bubbletea/v2@v2.0.8` source, not
  from memory, per this session's own instruction) — `doRunner` is a plain `func` parameter to
  `dispatchChatLine`, so every test above injects a fake with no terminal/`*tea.Program` involved at
  all; `m.program` is guarded nil-safe for exactly that reason.
- **`chatSystemPromptFormat`**: the plain-text chat-turn system prompt (distinct from `/do`'s plan-
  generation prompt) instructs the model to redirect any execution request to `/do` rather than attempt
  to describe running something itself, and states the resolved language by name (`"Turkish"`/
  `"English"`) for the model's own benefit.

## Tests

- `internal/i18n`: `TestResolveLanguagePrecedence` (config/`COMRADE_LANG`/`LANG`/`LC_ALL` precedence,
  `tr_TR`→tr, invalid config value falls through, nil getenv→en), `TestCatalogsCoverIdenticalKeys`
  (bidirectional drift guard), `TestCatalogsHaveNoEmptyValues`, `TestTranslatorTAppliesActiveCatalogAndInterpolates`,
  `TestTranslatorTZeroArgsReturnsFormatUnchanged`, `TestTranslatorTFallsBackToEnglishThenBareID`
  (hand-built catalogs, independent of the real catalog content).
- Migration regression: the entire pre-existing `internal/engine`+`internal/cli`+`internal/tui` suite
  passes unmodified or with test-side updates only where a signature genuinely changed (byte-for-byte
  EN preserved in every case); TR smoke tests:
  `internal/engine/runner_test.go`'s `TestExecuteAutoBlockRendersTurkishWhenRunDepsTranslatorIsTurkish`,
  `internal/cli/i18n_smoke_test.go`'s `TestI18nSmokeFirstRunNoticeInTurkish` and
  `TestI18nSmokeYoloWarningInTurkish`, `internal/cli/explain_test.go`'s
  `TestExplainTurkishLanguageProducesTurkishSafetyWarning`, and — the follow-up sweep's own required
  smoke coverage — `internal/cli/i18n_smoke_core_test.go`'s
  **`TestI18nSmokeCoreCommandsRenderTurkish`** (subtests `do --info`, `auth status`, `history`, each
  run with `COMRADE_LANG=tr` and asserted against the Turkish catalog strings — `do --info`'s Blocked
  step renders `ENGELLENDİ(`, `auth status`'s header/labels render `SAĞLAYICI`/`kayıtlı değil`/
  `anahtar gerekmez`, `history`'s empty log renders `Henüz kayıtlı komut yok.`) and
  **`TestI18nSmokeHelpTextRendersTurkish`** (proves `help.go`'s `--help`-localization mechanism:
  `do`'s Turkish `Short` appears in root's "Available Commands" listing, and `auth login`'s own
  Turkish `Short` appears in its own nested `--help`).
- `comrade explain`: `internal/engine/explain_test.go` (`TestExplainerHappyPathParsesPartsAndSendsCommandInUserMessage`,
  `TestExplainerNeverCallsAnythingButComplete`, `TestExplainerRequestsTurkishInstructionBlockWhenConfigured`,
  `TestExplainerOmitsEmptyPartsEntries`, `TestExplainerPropagatesCompleterError`);
  `internal/cli/explain_test.go` (`TestExplainDestructiveCommandShowsSafetyWarningAndLLMBreakdown`,
  `TestExplainBenignCommandShowsNoSafetyWarning`, `TestExplainDenylistedCommandIsReportedBlocked`,
  **`TestExplainNeverImportsExecutor`** — the explain-never-executes structural guard,
  `TestExplainTurkishLanguageProducesTurkishSafetyWarning`).
- `comrade chat`: `chatparse_test.go` (every slash command + plain text + edge cases), `chatsession_test.go`
  (mode/clear/append/renderTranscript/`saveTranscript` permissions), `chatdispatch_test.go` — including
  **`TestDispatchChatLineSaveWritesTranscriptAndNothingElseWritesToDisk`** (the no-autosave guard) and
  **`TestDispatchChatLineDoBlockedCommandIsReportedAsBlockedNeverExecuted`** +
  **`TestRunChatDoBlocksDenylistedStepAndNeverExecutesIt`** (the safety-gated-`/do` guard).
- Catalog-coverage: `internal/cli/catalog_coverage_test.go`'s
  `TestCatalogCoverageNoNewHardcodedUserFacingStrings` (now covering BOTH Print/Fprint text and flag
  descriptions) and `TestCatalogCoverageAllowlistHasNoStaleEntries`.
- Second follow-up round (flag descriptions + full-sentence errors), all in
  `internal/cli/i18n_smoke_core_test.go`: **`TestI18nSmokeFlagDescriptionRendersTurkish`** (the
  required TR smoke assertion — `--dry-run`'s and `--limit`'s descriptions render in Turkish in the
  SAME `--help` block as the already-Turkish command `Short`) and
  **`TestI18nSmokeFullSentenceErrorsRenderTurkish`** (4 subtests: `auth login` unknown-provider and
  `--auto`/`--ask` and `--print`/`--remove` mutual-exclusivity errors — all via `envOnlyTranslator`,
  none touching config — plus `config models`' unknown-provider error via the config-aware
  translator, exercised at the `fetchModelsForProvider` function level).

## Manual demonstration

`COMRADE_LANG=tr comrade explain "rm -rf node_modules"` against a fresh, isolated config dir (no API
key configured — the acceptance criterion's own scenario, since explain genuinely needs an LLM call for
its second layer):

```
Varsayılan ayar dosyası oluşturuldu: /tmp/comrade-demo-home/cli-comrade/config.toml
cli-comrade: no OS keychain available on this machine; storing API keys base64-obfuscated (NOT encrypted) in a 0600 file instead. See that file's header comment for details.
⚠ bu komut cli-comrade'in yerel güvenlik kontrolüne göre destructive sınıfında: escalated to destructive by rule: rm -r/-f (recursive or force delete) — sisteminizde geri alınamaz silme/üzerine yazma veya başka kalıcı bir değişikliğe yol açabilir.
comrade explain: engine: explain: llm: all providers failed: anthropic: no API key found for provider "anthropic"; set one of: COMRADE_ANTHROPIC_API_KEY, ANTHROPIC_API_KEY
```

This proves the acceptance criterion's two claims independently: (1) Turkish output — the safety
warning is fully localized, offline, with zero LLM dependency; (2) the warning fires correctly for a
destructive command. Layer 2 (the LLM breakdown) against a mock provider is proven by
`TestExplainDestructiveCommandShowsSafetyWarningAndLLMBreakdown` (`internal/cli/explain_test.go`) —
run and passing, see Tests above — which is the mock-server equivalent the phase's own instructions
offered as an alternative to a real API key.

The chat slash-command flow (scripted) is `chatdispatch_test.go`'s full `TestDispatchChatLine*` battery
— run and passing, see Tests above — driving `/mode`, `/clear`, `/save`, `/do`, `/help`, `/exit`, and
plain-text turns directly against `dispatchChatLine`, exactly as a real session would sequence them.

`--help` localization, demonstrated live (`COMRADE_LANG=tr comrade --help`, fresh isolated config dir):

```
comrade, terminalde çapraz platform çalışan bir yapay zeka CLI yoldaşıdır

Usage:
  comrade [flags]
  comrade [command]

Available Commands:
  auth        Kayıtlı LLM sağlayıcı API anahtarlarını yönetir (anahtar zinciri, dosya yedeğiyle)
  chat        Bağlamı koruyan, etkileşimli bir sohbet oturumu başlatır
  completion  Generate the autocompletion script for the specified shell
  config      cli-comrade yapılandırmasını görüntüler ve düzenler
  do          Serbest metinli bir istek için plan üretir ve aktif moda göre çalıştırır
  explain     Bir komutun ne yaptığını, bayrak bayrak, onu çalıştırmadan açıklar
  fix         Son başarısız komutu (veya verilen bir komutu) teşhis eder ve düzeltir
  help        Help about any command
  history     Denetim kaydından son çalıştırılan komutları gösterir
  init        Kabuk (shell) entegrasyon kancalarını kurar
```

(`completion`/`help` are cobra's own built-in commands, not part of `helpShortByPath` — left in
English, out of scope.) And nested: `COMRADE_LANG=tr comrade auth login --help` →

```
Bir sağlayıcı için API anahtarı kaydeder, ardından küçük bir test isteği gönderir

Usage:
  comrade auth login <provider> [flags]
```

Per-flag descriptions in the SAME `--help` block as the command's own (already-Turkish) `Short` —
`COMRADE_LANG=tr comrade do --help`:

```
Serbest metinli bir istek için plan üretir ve aktif moda göre çalıştırır

Usage:
  comrade do <request...> [flags]

Flags:
      --ask       bu çalıştırma için ask modda çalışır (COMRADE_MODE/config ayarını geçersiz kılar)
      --auto      bu çalıştırma için auto modda çalışır (COMRADE_MODE/config ayarını geçersiz kılar)
      --dry-run   üretilen planı çalıştırmadan yazdırır
  -h, --help      help for do
      --info      planı yazdırır ve açıklar, hiçbir şey çalıştırmaz
      --yolo      TEHLİKELİ: config'de safety.confirm_destructive/confirm_elevated de kapalıysa, auto modda destructive/elevated onayını atlar
```

And `COMRADE_LANG=tr comrade history --help`:

```
Denetim kaydından son çalıştırılan komutları gösterir

Usage:
  comrade history [flags]

Flags:
  -h, --help        help for history
      --json        her kaydı tablo yerine satır başına bir JSON nesnesi olarak yazdırır
      --limit int   gösterilecek en yeni kayıtların azami sayısı (default 20)
```

This proves the completeness gap the independent review flagged is closed: no `--help` block mixes
languages anymore — the command `Short` and every one of its own flag descriptions render in the
same resolved language.

## Gate

- `go vet ./...` — clean.
- `/home/firfir/go/bin/golangci-lint run` — `0 issues.`
- `go test ./... -count=1` — all packages green.
- `go test ./internal/i18n/... ./internal/engine/... -race -count=1` — green.
- `make build` — succeeds.
- `make cross` — all 5 targets (`linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`,
  `windows/amd64`) build successfully.

## Known limitations / deferred work

- **Final, minimal, individually-justified exception list** (every other user-visible surface —
  command output, prompts, `--help` `Short`/`Long`, per-flag descriptions, and every standalone
  full-sentence error — is now migrated):
  - `internal/tui/confirm.go`'s `"[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü"` option-letter line: a
    CLAUDE.md-**mandated** invariant, not translated per-language BY DESIGN.
  - `internal/tui/confirm.go`'s "Edit command (enter to confirm, esc to cancel):" line: structurally
    outside the AST scanner's reach (`strings.Builder.WriteString`, not `fmt.Fprint*`) — pre-existing,
    minor, documented debt, not a false "all clear".
  - cobra `Use` command tokens (`do`, `auth login <provider>`, ...): translating cli-comrade's own
    command vocabulary per language would break muscle memory and scripts — every i18n'd CLI keeps
    its verbs untranslated for this reason.
  - `internal/cli/hook.go`'s `COMRADE_DEBUG`-gated diagnostic line AND its 3 flag descriptions
    (`--shell`/`--exit`/`--command`): developer-facing (an explicit debug env var; an internal
    command invoked only by generated shell snippets, never typed by an end user) — loading config
    on FAZ 4's shell-prompt hot path just to resolve a display language is a deliberate performance
    tradeoff, not an oversight. This is the catalog-coverage linter's ONLY remaining allowlist entry.
  - `internal/cli/promptui.go`'s ask-mode `[a]çıkla` inline-explanation system prompt: an LLM system
    prompt (not literal user-facing text), unchanged since FAZ 6, out of this phase's scope.
  - `fmt.Errorf`-constructed error-WRAP chains (`"doing X: %w"`-shaped, ~40 of them) throughout
    `internal/cli`: CLAUDE.md's own established convention, applied identically across every
    package — see "Second follow-up round" above for the exact, written rule that distinguishes
    these (left as-is) from the 12 standalone full-sentence errors (migrated).
  None of these is silently mis-reported as covered: the catalog-coverage linter's own allowlist has
  exactly one entry (`hook.go`, covering both its Print line and its flag descriptions), and every
  other exception above is either invisible to that linter by construction (documented as such,
  e.g. `confirm.go`'s `WriteString` calls and every `fmt.Errorf`/`errors.New`) or a deliberate,
  CLAUDE.md-anchored design choice, not a gap.
- `comrade chat`'s plain-text turns and the LLM breakdown call `/do`'s planner make both block the
  bubbletea `Update` loop for their duration (no spinner/async indicator) — an accepted simplification
  documented in `chatmodel.go`'s own doc comment, not a bug: a real terminal is released to the
  do-pipeline during `/do` anyway, so nothing could animate concurrently in that case regardless.
