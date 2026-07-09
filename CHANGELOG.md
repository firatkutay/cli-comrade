# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- FAZ 10: packaging and distribution — a complete `goreleaser` v2
  pipeline, `comrade upgrade` (hand-rolled, checksum-verified
  self-update), a passive weekly version notice, and full install/
  config/security/troubleshooting docs. `.goreleaser.yaml` now produces,
  in addition to FAZ 0's archives+checksums: a Homebrew **Cask** (not the
  deprecated-as-of-goreleaser-v2.16 `brews`/Formula shape —
  `homebrew_casks:`, `firatkutay/homebrew-tap`), a Scoop bucket manifest
  (`firatkutay/scoop-bucket`), a winget manifest
  (`FiratKutay.comrade`), and `.deb`/`.rpm` packages via `nfpm`
  (maintainer, MIT license, `utils` section) — plus an explicit
  `release: github: {owner, name}` block (this repo has no git remote
  configured) and a commented-out `signs:` cosign block documenting the
  key-provisioning steps needed to enable it later. New
  `.github/workflows/release.yml`: on `v*` tag push, `actions/checkout`
  (`fetch-depth: 0` — goreleaser refuses a shallow clone) +
  `actions/setup-go` + `anchore/sbom-action` (a source CycloneDX SBOM,
  `sbom-source.cdx.json` — this repo ships no container image, so there
  is no separate container SBOM to also generate) + `goreleaser-action`,
  every step pinned by full commit SHA with a version comment.
  New `internal/update` leaf package (stdlib only — `net/http`,
  `archive/tar`/`archive/zip`, `compress/gzip`, `crypto/sha256` — no new
  dependency): `GitHubClient`/`HTTPDownloader` (both injectable via the
  `ReleaseFetcher`/`AssetDownloader` interfaces so every test uses a
  fake, never the real network), `ArchiveName`/`BinaryName`/
  `ChecksumsFileName` (the Go-code mirror of goreleaser's own
  `name_template`), `VerifyChecksum` (SHA-256 against the release's own
  `checksums.txt` — mandatory before ANY extraction/install),
  `ExtractBinary` (tar.gz/zip), `IsNewer`/`IsDevBuild` (a minimal,
  hand-rolled version comparator — not full SemVer precedence, sufficient
  for this project's own `vX.Y.Z` tags), `ReplaceBinary` (atomic rename
  on Unix; the Windows rename-running-exe-to-`.old` dance, since a
  running process can't overwrite or delete its own `.exe`) +
  `CleanupOldBinary`, and `CheckState`/`ShouldCheck`/`StatePathFor` (the
  weekly-throttle state file, alongside `audit.jsonl`/
  `last_command.json` under the platform state dir — never inside
  `config.toml`). New `comrade upgrade` (`internal/cli/upgrade.go`):
  `--check` only reports; without it, downloads, checksum-verifies, and
  atomically replaces the running binary, refusing outright on a `dev`
  (unversioned local) build. New `general.update_check` config key
  (default `true`) gating a passive, at-most-once-per-week "a new version
  is available" notice printed to stderr at the end of any command
  (`root.PersistentPostRunE`, added to `NewRootCmd`/`newRootCmd` — the
  latter an unexported, fetcher-injectable variant used only by tests
  that need to exercise a full successful/failed background check
  without ever reaching the real GitHub API) — silent on any failure
  (offline, API error), bounded by a 3s timeout, and skipped for a bare
  `comrade` invocation, `comrade upgrade` itself, a `dev` build, or
  `update_check=false`. `docs/phases/FAZ-10.md` has the full design
  rationale (why hand-rolled over a self-update library, the exact
  goreleaser/action version-pin decisions and why).
- FAZ 10: a bidirectional release-name drift guard
  (`internal/cli/release_names_test.go`,
  `TestReleaseArchiveNamingIsConsistentAcrossGoreleaserInstallScriptsAndUpdatePackage`)
  renders `.goreleaser.yaml`'s own `archives[].name_template` (via
  `text/template`, not a hand-copied string) and cross-checks it against
  `scripts/install.sh`, `scripts/install.ps1`, and
  `internal/update.ArchiveName`/`BinaryName`/`ChecksumsFileName` — a Go
  test (wired into the existing `go test ./...` CI step, not a
  duplicate/parallel script) that fails on ANY of the four drifting from
  the other three, in either direction. `scripts/install.sh` also gained:
  a `wget` fallback (`fetch_url`/`fetch_url_to_file` dispatch on whichever
  of `curl`/`wget` `require_downloader` actually finds, erroring with a
  clear message if neither is present) and a `sudo` fallback when neither
  `~/.local/bin` nor `/usr/local/bin` is writable.
- FAZ 10: `docs/INSTALL.md`, `docs/CONFIGURATION.md` (every config key,
  default, and `COMRADE_...` env override in one table), `docs/
  SECURITY.md`, and `docs/TROUBLESHOOTING.md` — each bilingual (TR
  section then EN section, matching `README.md`'s own convention).
  `README.md` gained one-line per-OS install commands and links to all
  four docs.

- FAZ 9: full TR/EN i18n, `comrade explain`, and `comrade chat`. New
  `internal/i18n` leaf package: a `MessageID`-keyed `Catalog` (113 entries,
  English + Turkish), a `Translator` (`T(id, args...)`, DI'd per command —
  no global state) falling back en→bare-id on a missing key, and the
  single, consolidated `ResolveLanguage(configLanguage, getenv)` (config
  `general.language` > `COMRADE_LANG` > `LANG`/`LC_ALL` > en) that
  replaces `internal/engine`'s own now-deleted duplicate resolver. A
  bidirectional test (`TestCatalogsCoverIdenticalKeys`) guards the two
  catalogs against drift. Every command-output/prompt string in
  `stub.go` (deleted), `runtime.go`/`config.go`/`models.go`/`root.go`,
  `do.go` (the plan table + run summary), `fix.go` (notices + paste-mode
  prompts + new Root cause:/Explanation: headings), `auth.go` (every
  login/logout/status prompt and label), `history.go` (table header +
  a new friendly empty-log message), and `init.go` (every install/remove
  prompt) now routes through the catalog — EN output preserved
  byte-for-byte, several already-tested unexported helpers'
  signatures threaded with a `Translator` where that was the correct fix.
  cobra `--help`/usage text for all 21 commands, AND every one of their
  11 unique per-flag descriptions (new `internal/cli/help.go`, overriding
  root's `HelpFunc`/`UsageFunc` to re-translate every command's `Short`
  by `CommandPath()`, and every matching flag's `Usage` by name, before
  cobra renders — flag registrations now pass `enUsageDefault(id)`
  instead of a raw literal), is localized too, so a `--help` block never
  mixes languages. The ~12 standalone, full-sentence
  `fmt.Errorf`/`errors.New` user-facing errors (as opposed to the ~40
  `"doing X: %w"` internal wrap chains, deliberately left untranslated —
  CLAUDE.md's own established convention) are migrated as well, via a
  new `envOnlyTranslator()` for the handful that must report before any
  config load. The static-scanner drift guard
  (`internal/cli/catalog_coverage_test.go`) now scans BOTH `fmt.Print*`
  text and flag-registration descriptions, and has exactly ONE allowlist
  entry (`hook.go`'s `COMRADE_DEBUG`-gated hot-path diagnostic line and
  its 3 internal-only flag descriptions) — every other hardcoded-string
  exception (CLAUDE.md's mandated confirm-prompt option letters, cobra
  `Use` tokens, `fmt.Errorf` wrap chains) is either an explicit,
  individually-justified invariant or structurally outside the scanner's
  reach, documented in `docs/phases/FAZ-09.md`, not silently exempted.
  New `comrade explain <command...>`: a two-layer,
  NEVER-executing command explanation — an authoritative local
  `safety.Engine` warning, plus a new `internal/engine.Explainer` LLM
  breakdown (`{summary, parts, risk_note}`, mirroring
  `Planner`/`Diagnoser`'s shape) — in the user's resolved language. New
  `comrade chat`: a bubbletea v2 interactive session (scrollback
  `viewport` + `textinput`, matching `confirm.go`'s v2 style) with
  in-memory-only history and `/mode`/`/clear`/`/save <file>`/
  `/do <request>`/`/help`/`/exit` slash commands — history is NEVER
  written to disk except an explicit `/save`; `/do` routes through the
  exact same plan→safety→execute pipeline `comrade do` uses, under the
  session's active mode. All slash-command parsing and session-state
  logic is pure and unit-tested with no TTY.

- FAZ 8: keychain-backed credential storage, `comrade auth`, and
  `comrade config models`. New `internal/secrets` package: a `Store`
  interface (`Get`/`Set`/`Delete`/`Status`) backed by the OS keychain
  (`zalando/go-keyring` — macOS Keychain, Windows Credential Manager,
  Linux Secret Service) with a 0600-file fallback
  (`<config dir>/credentials`, base64-obfuscated — explicitly documented
  as NOT encryption, in both the file's own header and a one-time stderr
  warning on first use) when no keychain is reachable; the active backend
  is chosen once, by a read-only probe, at construction. `internal/llm`
  gained a `KeyResolver` seam (`Client.New(cfg, opts ...Option)`,
  `WithKeyResolver`) so `internal/cli` can wire a keychain/file-then-env
  resolver into every `llm.Client` it builds, without `internal/llm` ever
  importing `internal/secrets` — the package's own tests are unchanged
  and still exercise only the env-only default resolver. New top-level
  `comrade auth login/logout/status`: `login` reads the key with a
  no-echo prompt (`golang.org/x/term.ReadPassword`, injectable in tests),
  stores it, then sends a live one-completion "ping" through the
  provider — the key is stored regardless of whether the ping succeeds,
  so an offline user or a transient provider outage is never blocked from
  saving a key believed correct; `logout` removes a stored key;
  `status` reports every provider's source (`keychain`/`file`/
  `env: <VAR>`/`not set`) without ever printing a key value; `ollama` is
  rejected from `login`/`logout` (it needs no credential). New
  `comrade config models`: lists the active provider's models — a live
  `GET /api/tags` for `ollama`, a live `GET {base_url}/models` for
  `openai_compat` (new `ListModels`/`ListOpenAICompatModels`, resolved
  through the same keychain/file/env chain), and a static, docs-linked
  snapshot for `anthropic`/`google` (new `KnownAnthropicModels`/
  `KnownGoogleModels`) — as a numbered menu, persisting the selection to
  `llm.model` via the existing `Loader.SetAndSave` path. Ollama
  connection failures (refused/timeout) across `Complete`/`Stream`/
  `ListModels` now surface a friendly "`ollama serve`" message instead of
  a bare transport error. See docs/phases/FAZ-08.md.
- FAZ 7: `comrade fix` — the main use-case, error-diagnosis flow (replacing the
  FAZ 0 stub). `internal/engine`: new `Diagnoser.Diagnose(ctx, ErrorContext)
  (Diagnosis, error)` — a `go:embed`'d diagnose system prompt (root-cause/
  explanation/plan JSON schema, package-manager-aware install suggestions, a
  16-example TR/EN few-shot grounding block covering command-not-found,
  permission denied, port-in-use, ENOENT, Python `ModuleNotFoundError`, git
  merge conflicts, DNS/proxy failures, and PowerShell `ExecutionPolicy`) reusing
  `Planner`'s `rawPlan`/`toPlan`/language-resolution machinery verbatim for the
  plan portion, so the fix plan is safety-annotated exactly like `comrade do`'s.
  New `OfferVerification(ctx, RunDeps, Mode, command)` implements post-solution
  verification: offers to re-run the original failing command once the fix
  plan completes cleanly (info suggests it, ask prompts via the same confirm
  loop a real plan step gets, auto runs it directly except for `elevated`,
  which still confirms) — skipped entirely when the command is independently
  classified destructive/Blocked, in any mode; the re-run is itself audited
  like any other executed command. `internal/cli/fix.go`: the fallback chain
  (a fresh, failed `last_command.json` entry used directly; `--rerun` or
  `-- <command>` re-executes and captures fresh output, refusing outright —
  falling through to interactive paste mode instead — when the command is
  independently classified destructive; otherwise interactive paste mode reads
  a pasted command + error transcript from stdin) feeds `Diagnoser`, then hands
  the resulting plan to the exact same `engine.Execute`/mode-resolution/audit
  machinery `comrade do` uses — no execution logic is duplicated. New
  `internal/cli/runtime.go` factors the load-config/first-run-notice/
  `--yolo`-warning/`llm.New` sequence, now shared verbatim by `do` and `fix`.
  Proven end-to-end against a real compiled binary and an `httptest`/standalone
  mock `openai_compat` server: the `pyton --version` (typo'd `python3`)
  acceptance scenario surfaces the right root cause/explanation/install plan in
  info mode, and a destructive `rm -rf <dir>` last command is refused before
  ever reaching the executor (independently confirmed: the target directory's
  marker file survives) — see docs/phases/FAZ-07.md.
- FAZ 6: executor + three behavior modes (auto/ask/info) — the product's
  core execution loop. `internal/engine`: `Mode` (`auto`/`ask`/`info`) +
  `ResolveMode(flag, env, config)` implementing the exact flag >
  `COMRADE_MODE` > config precedence; `Execute(ctx, plan, mode, deps)
  (RunSummary, error)` dispatching to `info` (prints the plan, executes
  nothing), `ask` (per-step confirm via a decoupled `PromptUI` interface:
  `[e]vet`/`[h]ayır` run/skip, `[d]üzenle` re-evaluates the edited
  command through `safety.Engine` *before* ever running it — refusing and
  re-prompting on a newly-Blocked edit — `[a]çıkla` fetches an LLM
  explanation and re-prompts, `[t]ümü` auto-approves remaining
  read/write/network steps while still individually confirming
  destructive/elevated ones), and `auto` (sequential execution with a
  one-line status per step; destructive/elevated steps force the same
  confirm prompt unless `safety.confirm_destructive`/`confirm_elevated`
  is disabled *and* `--yolo` is set, which prints a mandatory red warning
  and proceeds — Block itself is never `--yolo`-bypassable in any mode,
  and a Block aborts the rest of an auto-mode plan). A failed step
  (nonzero exit, including timeout) triggers up to 3 total
  self-correction round-trips per run (ask the LLM for a revised command,
  re-evaluate it through safety before ever running it, retry), then
  stops with a summary and suggestion. Ctrl-C (`signal.NotifyContext`)
  cancels the run cleanly: the in-flight step's process group is killed
  by `internal/executor`, remaining steps are recorded skipped, and a
  full summary is printed. `internal/audit` is wired end-to-end: one
  JSONL entry per step that actually executed (never a Blocked/skipped
  one, never stdout/stderr content), lazy `retention_days` cleanup once
  per invocation; new `comrade history` (replacing the FAZ 0 stub) reads
  it back as a table or `--json`, `--limit N` (default 20), read-only.
  `comrade do <request...>` is now the real, no-longer-hidden pipeline
  (`--dry-run` unchanged from FAZ 5; new `--auto`/`--ask`/`--info`/
  `--yolo` flags); the root command now falls back to `do` for any arg
  vector that doesn't match a known subcommand (`comrade docker kur`
  works directly). New `[executor]` config section
  (`step_timeout_seconds`, default 300) for internal/executor's per-step
  timeout. Proven end-to-end against a real compiled binary, a real
  `*executor.Executor`, and an `httptest`-mocked `openai_compat` plan
  server: a benign step actually runs while a `rm -rf /` step the model
  mislabeled "read" is Blocked and never reaches the executor — see
  docs/phases/FAZ-06.md.
- FAZ 5: engine (plan generation) + safety rule engine — `internal/safety`:
  an LLM-independent, config-driven `Engine.Evaluate(command, declared)`
  implementing CLAUDE.md's five-class `RiskClass` (read/write/network/
  elevated/destructive, ascending severity) and `Action`
  (Allow/Confirm/Block). Every matcher runs against a single
  `normalizeCommand`'d form (all quote characters stripped, whitespace
  collapsed) rather than the raw string, so a stray quote can never defeat
  a rule. A hardcoded denylist (`rm -rf /`/`~`/`$HOME` variants and their
  near-equivalents — `//`, `/.`, trailing slashes, an absolute path to
  `rm` — canonicalized before matching; `mkfs`; `dd`/redirect writes to
  any real disk device, an explicit safe-pseudo-device allowlist
  (`null`/`zero`/`tty*`/`random`/`urandom`/`full`/`stdin`/`stdout`/
  `stderr`/`fd/*`/`pts/*`) aside; `diskpart`+`clean`; a PowerShell
  drive-root recursive delete recognized across the whole Remove-Item
  alias family (`Remove-Item`, `Remove-ItemProperty`, `ri`, `rd`,
  `rmdir`, `del`, `erase`, `rm`), accepting abbreviated
  (`-r`/`-rec`/`-fo`/...) and cmd.exe-legacy (`/s`/`/q`) flag spellings;
  `format <drive>:`; the classic fork bomb) always returns Block, plus
  `safety.denylist_extra` user regexes (an invalid one is skipped with
  one stderr warning at construction, never a crash); ten escalation
  rules (`rm -r`/`-f`, the same Remove-Item alias family with any
  target, `chmod -R 777`, disk-write redirects, registry `Remove-Item*`
  on `HKLM:`/`HKCU:`, `killall`/`taskkill /F`, `iptables -F`/`netsh
  advfirewall reset`, `git push --force`, `sudo`/`runas`/`-Verb RunAs`,
  package-manager installs, network verbs) only ever raise a command's
  effective risk, never lower it — a step the LLM declared `destructive`
  stays `destructive` even when nothing else matches. 236 table-driven
  sub-cases across both Unix and PowerShell command families (up from an
  initial 99 after an independent security review's `CHANGES_REQUIRED`
  hardening pass, plus a residual `path.Clean`-based fix on
  re-verification — see docs/phases/FAZ-05.md), including the documented
  near-miss set (`rm -rf ./build`/`rm -rf ~/project` escalate but never
  Block) and the deliberately-accepted `echo "rm -rf /"` quoted false
  positive (fail-closed by design).
  `internal/engine`: `Planner.GeneratePlan` builds a `go:embed`'d system
  prompt (English core instruction + a Turkish block injected per
  `general.language`'s auto/LANG resolution) carrying the OS/arch/shell/
  cwd/package-manager/admin context (never env values), requests a
  single structured-JSON completion through a minimal `Completer`
  interface any `*llm.Client` satisfies, recovers from an empty `steps`
  array or a `max_auto_steps` overrun with exactly one automatic
  corrective re-prompt each (hard error / hard-truncate-with-a-summary-
  marker respectively if the retry doesn't fix it), fails closed to
  `RiskDestructive` for any step whose `risk` label doesn't parse, and
  runs every step through `safety.Engine` before returning — the LLM's
  declared risk is never the last word. New hidden `comrade do
  <request...> --dry-run` (execution itself is FAZ 6's job; without
  `--dry-run` this phase refuses to run at all) renders the plan as a
  `STEP|COMMAND|RISK|REVERSIBLE|RATIONALE` table showing the safety
  engine's own verdict — `CONFIRM(<effective risk>)` or
  `BLOCKED(<reason>)` — never the model's raw declared risk, proven
  end-to-end against an `httptest` mock `openai_compat` server whose
  canned "docker kur" plan includes a `sudo apt-get install` (elevated)
  step and a `rm -rf /` decoy step the model itself labels `read` and
  that still renders Blocked.
- FAZ 4: shell integration — `comrade init [bash|zsh|fish|powershell]`
  (replacing the FAZ 0 stub) installs an idempotent, marker-delimited
  hook block (`internal/shellinit`, `go:embed`'d per-shell snippets)
  into the shell's rc/profile file (`~/.bashrc`, `~/.zshrc` respecting
  `ZDOTDIR`, `~/.config/fish/config.fish` respecting
  `XDG_CONFIG_HOME`, or a PowerShell `$PROFILE` actually resolved by
  invoking `pwsh`/`powershell` — never guessed); `--print` shows the
  snippet only, `--remove` uninstalls it, `--yes` skips the
  confirmation prompt. Every hook execs a new hidden `comrade hook
  record --shell <name> --exit <code> --command <text>` subcommand
  instead of hand-assembling JSON in shell script (unsafe for arbitrary
  command text — see docs/phases/FAZ-04.md), which atomically writes
  `last_command.json` via a new `context.WriteLastCommand` (temp file +
  rename). Hooks intentionally record only command/exit code/timestamp/
  shell, never stderr/stdout (FAZ 7's `comrade fix --rerun` owns
  capturing that, by re-running the command directly). New
  `scripts/install.sh` / `scripts/install.ps1` download, checksum-verify,
  and install the latest (or pinned) release binary, then suggest
  `comrade init`.
- FAZ 3: context collector + redaction — `internal/context`: DI-friendly
  `Collector`/`Collect(ctx, Options) Context` gathering OS/arch, shell
  type + best-effort version, working/home dir, admin/root status
  (windows honestly reports "not checked" rather than guessing),
  detected package managers (`apt`/`dnf`/`pacman`/`zypper`/`brew`/`port`/
  `winget`/`scoop`/`choco`), `last_command.json` reader (format now
  defined here; FAZ 4's shell hooks will write it), and opt-in shell
  history (bash/zsh/fish/PowerShell PSReadLine) + env-var *names*
  (never values). `internal/redact`: `Redactor.Apply` masks API keys
  (`sk-`/`ghp_`/`gho_`/`AKIA`/`xox[baprs]-`), JWTs, PEM private-key
  blocks, `password=`/`token=`/etc. credential kv pairs (value only,
  key name kept visible), and `Authorization: Bearer` tokens — plus
  optional email/IP masking (never masking `127.0.0.1`/`0.0.0.0`/`::1`).
  Wired as a **non-bypassable middleware** in `internal/llm.Client`:
  `Complete`/`Stream` redact `System` + every message's `Content` before
  any connector call, hardwired from `cfg.Privacy` inside `New(cfg)` with
  no external way to inject a no-op redactor — proven by an `httptest`
  based test asserting a real secret never reaches the wire.
- FAZ 2: LLM provider layer — `internal/llm`: the CLAUDE.md `Provider`
  interface plus four unexported connectors talking raw `net/http` (no
  SDKs, zero new go.mod dependencies): `anthropic` (Messages API, SSE
  streaming, `529 overloaded_error` handling), `openai_compat` (one
  connector for OpenAI/Mistral/Groq/GLM/Qwen/Kimi/OpenRouter/LM Studio,
  distinguished by `base_url`), `google` (Gemini `generateContent`,
  path-encoded model, `x-goog-api-key` header), `ollama` (`/api/chat` +
  `/api/tags`-backed `ListModels` for its dynamic default model).
  Connector constructors are unexported; `llm.New(cfg)` building a
  `*Client` is the only way to reach the network from this package.
  `internal/llm/parse.go`'s `ExtractJSON`/`ValidateInto` strip markdown
  fences and validate a caller-declared set of required JSON fields in
  the model's response text. `Client` resolves `llm.provider`+`llm.model`
  plus `llm.fallback` into an ordered attempt chain: 401/403 stops the
  chain immediately, everything else (timeout, network error, 429/5xx/
  529, parse failure, missing API key) retries the next attempt, logged
  to stderr per attempt under `COMRADE_DEBUG=1`. API keys resolve from
  `COMRADE_<PROVIDER>_API_KEY` then each provider's well-known env var.
  New hidden command `comrade config test-llm` sends a ping completion
  through the full fallback chain and prints provider/model/latency.
- FAZ 1: config system — `internal/config` (viper-backed TOML schema,
  `~/.config/cli-comrade/config.toml` / `%APPDATA%\cli-comrade\config.toml`
  path resolution with `XDG_CONFIG_HOME` support, first-run default-file
  creation, `COMRADE_` env overrides including the named
  `COMRADE_MODE`/`COMRADE_PROVIDER`/`COMRADE_MODEL` aliases); real
  `comrade config` command tree (`get`/`set`/`list`/`edit`/`path`)
  replacing the FAZ 0 stub, with type/enum-validated `set` and an
  aligned `list` table showing each key's source (default/file/env).
- FAZ 0: project skeleton — Go module, `internal/`/`cmd/` directory layout,
  cobra root command (`comrade --version` / `--help`), stub subcommands
  (`fix`, `explain`, `chat`, `config`, `init`, `history`), Makefile
  (`build`/`test`/`lint`/`vet`/`cross`/`tools`), `.golangci.yml`, GitHub
  Actions CI (build/test/lint across ubuntu/macos/windows), base
  `.goreleaser.yaml`, README, LICENSE.
