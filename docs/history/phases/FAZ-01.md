# FAZ 01 — Config System

## What was delivered

- `internal/config`: viper-backed TOML configuration.
  - `schema.go`: the `Config` struct tree (`General`/`LLM`/`Safety`/
    `Context`/`Privacy`/`Audit`, plus `LLM.OpenAICompat`/`LLM.Ollama`)
    with `mapstructure` tags matching UYGULAMA_PLANI.md's FAZ 1 schema
    exactly. `defaultConfigTOML` is the single source of truth for every
    key's default value and the on-disk layout written on first run —
    it is the literal TOML block from the plan. `Default()` parses that
    constant rather than duplicating the same 22 defaults as Go
    literals a second time.
  - `paths.go`: `ResolvePath(goos, getenv)` — Windows:
    `%APPDATA%\cli-comrade\config.toml`; everything else:
    `$XDG_CONFIG_HOME/cli-comrade/config.toml`, falling back to
    `~/.config/cli-comrade/config.toml`. Takes `goos`/`getenv` as
    parameters (rather than reading `runtime.GOOS`/`os.Getenv` inline)
    so every branch — including the Windows one — is unit-testable on
    any host OS. `DefaultPath()` wraps it for the real process
    environment.
  - `validate.go`: a `keyDefs` registry (22 entries, one per settable
    key) driving `comrade config set`'s validation — enum membership for
    `general.mode`/`general.language`/`llm.provider`, positive-int
    parsing for the five int keys, bool parsing, comma-separated
    string-slice parsing for `llm.fallback`/`safety.denylist_extra`.
    Unknown keys produce an error that lists every valid key.
  - `loader.go`: `Loader` — no global state; constructed with an
    explicit path (or `""` to resolve the platform default) and passed
    down by the caller. `Load()` creates the file with defaults on first
    run (reporting that back via a `created bool`), then layers
    defaults → file → `COMRADE_` environment variables through viper
    (`MergeConfig` for defaults, `MergeInConfig` for the file,
    `AutomaticEnv` + three explicit `BindEnv` aliases —
    `COMRADE_MODE`/`COMRADE_PROVIDER`/`COMRADE_MODEL` — for the
    generic `COMRADE_GENERAL_MODE`-style mapping). `Get`/`Source`
    resolve one key's effective value and report whether it came from
    `env`/`file`/`default`. `SetAndSave` validates the key is known,
    then rewrites the whole file (defaults + file merged, deliberately
    excluding any env override) with the one key changed.
- `internal/cli/config.go`: replaces the FAZ 0 stub with a real command
  tree: `get <key>`, `set <key> <value>`, `list`, `edit`, `path`. `set`
  disables cobra's flag parsing (its two args are fixed positionals, and
  values like `-5` must reach `config.Validate`'s "must be > 0" check
  instead of being swallowed as an unrecognized shorthand flag by
  pflag). `list` prints an aligned `KEY / VALUE / SOURCE` table via
  `text/tabwriter`. `edit` opens `$EDITOR` (falling back to `vi` on
  Unix, `notepad` on Windows) on the config file. The root command's
  loader is a closure (`func() (*config.Loader, error)`) built once in
  `NewRootCmd`, resolved fresh on every subcommand invocation — this is
  the dependency-injection seam CLAUDE.md asks for: no viper/config
  state lives at package scope anywhere in this tree.
- First run: any config subcommand invocation, when no config file
  exists yet, creates one (defaults only) and prints one hardcoded
  English line — `Created default config at <path>` — to **stderr**
  (not stdout), so `x=$(comrade config get key)`-style output capture
  stays limited to the command's actual value, before doing anything
  else. `path` is the one exception: it does not create the file,
  since printing the resolved path is a pure query.
- Dependency: `github.com/spf13/viper v1.21.0` (pinned exact version, as
  instructed), pulling in its usual transitive set (`go-toml/v2`,
  `mapstructure/v2`, `afero`, `fsnotify`, `cast`, `pflag` upgraded to
  v1.0.10, etc.) — recorded in `go.sum`.

## Decisions & deviations

- **Derive-not-duplicate schema.** `defaultConfigTOML` (the plan's exact
  TOML block) is the only place the 22 keys and their defaults are
  written out. `Default()`, the runtime `Loader`, and the file written on
  first run all parse this one constant instead of re-stating the same
  defaults as Go `SetDefault` calls or struct literals — avoiding the
  two-source drift the project's house standards call out.
- **Bidirectional drift guards, not just "they happen to match".** Three
  things must stay in lockstep: the `Config` struct's `mapstructure`
  tags, the `keyDefs` validation registry, and `defaultConfigTOML`'s own
  keys. `internal/config/schema_test.go` has two reflection/parse-based
  tests (`TestKeyDefsMatchConfigStruct`,
  `TestKeyDefsMatchDefaultConfigTOML`) that fail if any one of the three
  gains or loses a key the others don't have — a key added to the struct
  without a matching `keyDefs` entry (or vice versa) is a red test, not
  a silent gap discovered later by a user's `config set`.
- **`comrade config set`'s write-back does not preserve comments or key
  order.** Persisting a single key goes through viper's
  `WriteConfigAs`, which serializes `AllSettings()` — a
  `map[string]any` — so the plan's inline `# auto | ask | info`-style
  comments and the section ordering shown in UYGULAMA_PLANI.md are lost
  after the first `set`. The *first-run* file (written directly from
  `defaultConfigTOML`'s literal bytes) does preserve them; only a
  subsequent `set` rewrites the file through viper. This was accepted
  as the simplest correct implementation for FAZ 1; a
  comment-preserving TOML editor (if ever wanted) is a candidate for a
  later phase, not a defect.
- **`llm.fallback`/`safety.denylist_extra` accept a comma-separated
  string on the command line.** UYGULAMA_PLANI.md's `set <key> <value>`
  signature takes one value argument; for the two list-typed keys, that
  value is split on `,` (entries trimmed, empty value → empty slice).
  Not spelled out in the plan; documented here as the interpretation
  taken.
- **`comrade config set`'s flag parsing is disabled
  (`DisableFlagParsing: true`).** Cobra/pflag otherwise treats a value
  like `-5` as an unrecognized shorthand flag before `RunE` ever sees
  it, which would make it impossible to reject a negative
  `timeout_seconds` with the intended validation error. `set` has no
  flags of its own and exactly two fixed positional arguments, so
  disabling flag parsing is safe; `get`/`list`/`edit`/`path` are
  unaffected.
- **`formatValue` handles both `[]string` and `[]any`.** A slice value
  `config.Validate` just parsed (what `set` echoes) is a concrete
  `[]string`, but the same key read back through `Loader.Get` off the
  merged viper config comes back as `[]any` (`[]interface{}`) —
  viper/go-toml never rebuilds a decoded array into a typed
  `[]string`. Handling only `[]string` meant `config get llm.fallback`
  rendered Go's `[a b]` bracket syntax instead of the `a,b` comma
  format `set` had just echoed for the identical value. Fixed by
  matching both types (and, for an empty list, rendering `""` rather
  than `"[]"`).
- **`SilenceErrors`/`SilenceUsage` on the root command.** Without them,
  a failing subcommand produced the error three times over: cobra's
  own `Error: ...` line, cobra's full `Usage:` block, and
  `cmd/comrade/main.go`'s own `fmt.Fprintln(os.Stderr, err)`. Cobra
  checks these two flags on either the command that actually ran or
  root, so setting them once on root suppresses cobra's own output for
  every subcommand, leaving `main.go` as the single place that ever
  prints an error — one clean line, no usage dump.
- **`context.Context`-first-param rule.** Per CLAUDE.md this applies
  "where it fits naturally". `internal/config`'s file I/O
  (`os.Stat`/`os.WriteFile`/viper's `ReadInConfig`/`WriteConfigAs`) has
  no context-aware variant in either the stdlib or viper's API, so no
  function in this package takes a `context.Context` — there is nothing
  a context could cancel or carry here. `comrade config edit` does use
  `exec.CommandContext(cmd.Context(), ...)` since `os/exec` does support
  cancellation.
- **API keys are out of scope**, per the task: `[llm]` has no credential
  fields; FAZ 8 adds keychain-backed auth separately.

## Acceptance evidence

- `go vet ./...` — clean.
- `$(go env GOPATH)/bin/golangci-lint run` — `0 issues.`
- `go test ./...` — all packages pass (58 `--- PASS` lines across
  `internal/config` + `internal/cli` combined, several table-driven with
  multiple subtests): first-run creation, partial-file default fill-in,
  env-beats-file-beats-default precedence (both generic
  `COMRADE_GENERAL_MODE`-style and the three named aliases), invalid
  `set` values (bad enum, non-numeric/zero/negative int, bad bool,
  unknown key) rejected without persisting, path resolution for both
  the Windows and XDG branches (including the XDG-unset fallback and the
  "neither set" error), the `Config`↔`keyDefs`↔`defaultConfigTOML`
  drift guards, the `llm.fallback`/`safety.denylist_extra` comma-format
  round trip through `set`→`get`→`list` (including the empty-list case),
  and the single-clean-error-line/no-`Usage:` regression guard.
- `make build` → `./comrade`. Manual run against an isolated
  `$HOME`/`$XDG_CONFIG_HOME` (not the real user config):
  - `./comrade config list` on a fresh directory prints the first-run
    notice, then a 22-row aligned table with every key at its default
    value and `SOURCE=file` (the just-created file always contains
    every key explicitly, since `defaultConfigTOML` is not sparse —
    `SOURCE=default` is reachable only against a hand-edited, partial
    file, which is what `TestLoaderSourceReportsDefaultThenFileThenEnv`
    exercises).
  - `./comrade config set general.mode auto` → `general.mode = auto`;
    `./comrade config get general.mode` → `auto`; the file on disk
    subsequently shows `mode = 'auto'` under `[general]`.
  - `./comrade config set general.mode hizli` → exit 1,
    `Error: invalid value "hizli" for general.mode; must be one of: auto, ask, info`.
  - `./comrade config set general.bogus x` → exit 1,
    `Error: unknown config key "general.bogus"; valid keys are: audit.enabled, ...` (all 22 keys listed).
