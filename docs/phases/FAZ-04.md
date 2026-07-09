# FAZ 04 — Shell Entegrasyonu (`comrade init`)

## What was delivered

### `internal/shellinit` — hook snippets + idempotent install logic

- **Four `go:embed`'d hook snippets** under `internal/shellinit/snippets/`:
  `bash.sh`, `zsh.sh`, `fish.fish`, `powershell.ps1`. Every snippet's only
  side effect on each prompt is: guard on `comrade` being on `PATH`,
  compare the current command text against a per-shell last-recorded
  guard variable to skip empty/duplicate entries, and — only if it's new
  — exec `comrade hook record --shell <name> --exit <code> --command
  <text>`. All output is swallowed (`>/dev/null 2>&1 || true` /
  `2>$null`); a hook failure can never make a shell prompt noisy or
  break it.
- **`Shell`** (`Bash`/`Zsh`/`Fish`/`PowerShell`) + `ParseShell(name)`:
  the only way to obtain a `Shell` value from user input, so an
  unsupported name (`tcsh`) is rejected once, centrally, with a message
  naming every supported shell.
- **`Snippet(shell)`** returns the raw embedded body; **`Block(shell)`**
  wraps it in the exact marker pair `# >>> cli-comrade init >>>` / `#
  <<< cli-comrade init <<<` — the same two lines for every shell,
  including PowerShell (`#` is a valid PowerShell comment too), per
  UYGULAMA_PLANI.md FAZ 4 item 1.
- **`ApplyBlock(original, shell)`** / **`RemoveBlock(original)`**: pure
  string-in/string-out functions (no file I/O) implementing the
  idempotency contract — `ApplyBlock` appends when no marker is found,
  leaves the file byte-identical when an existing block already matches
  (`StatusAlreadyInstalled`), and replaces an existing-but-different
  block in place (`StatusUpgraded`, the "older snippet version" upgrade
  path) — so two `comrade init` runs on the same shell always leave
  exactly one block. `RemoveBlock` deletes the marker-delimited region
  plus the one separator blank line `ApplyBlock`'s append path adds, so
  an install-then-remove round trip restores the original file exactly;
  a file with no markers (or a malformed half-marker pair) is left
  completely untouched.
- **`RCPath(ctx, shell, goos, getenv, lookPath, run)`** resolves the
  target rc/profile file, with `goos`/`getenv`/`lookPath`/`run` all
  injected (the same pattern as `config.ResolvePath` and
  `context.LastCommandPath`) so every branch — including
  Windows-specific ones — is testable from Linux CI:
  - bash: `$HOME/.bashrc`
  - zsh: `$ZDOTDIR/.zshrc` if `ZDOTDIR` is set, else `$HOME/.zshrc`
  - fish: `$XDG_CONFIG_HOME/fish/config.fish` if set, else
    `$HOME/.config/fish/config.fish`
  - powershell: actually invokes a PowerShell binary
    (`powershell` on windows, `pwsh` elsewhere — Windows PowerShell
    isn't expected off-Windows) with `-NoProfile -Command '$PROFILE'`
    and uses its answer. If that binary isn't on `PATH`, or the
    invocation fails, `RCPath` returns `ok=false` with an explanatory
    `note` — **it never guesses a path**. `comrade init` then falls
    back to printing the block with manual-install instructions, per
    FAZ 4's "keep honest" requirement (mirrors FAZ 3's honest
    `IsAdmin` windows handling).

### `internal/context` — `WriteLastCommand` (the format's only writer)

- `WriteLastCommand(path, cmd)`: marshals `cmd` to JSON and writes it
  **atomically** — a temp file in `path`'s own directory (so the rename
  stays on one filesystem), then `os.Rename` into place — creating the
  parent directory first if needed. `ReadLastCommand` (FAZ 3) never
  observes a partially-written file. This is the **only** place
  `last_command.json` is ever written; shell hooks never touch it
  directly (see the Decisions section below for why).

### `internal/cli` — `comrade init` (replacing the FAZ 0 stub) + hidden `comrade hook record`

- **`comrade init [bash|zsh|fish|powershell]`**: no positional arg ⇒
  detect the current shell via `context.DetectShell`; an empty or
  unsupported detection result (e.g. Windows `cmd`) is an error asking
  the user to name one explicitly rather than guessing.
  - Default (no flags): prints the block that would be added and the
    target file, then asks `[y/N]` on stdin before writing — unless
    `--yes`/`-y` is given (needed for install scripts and tests; a
    small, documented extension beyond the plan's literal flag list —
    see Decisions).
  - `--print`: prints the block only; makes no file changes at all.
  - `--remove`: deletes the installed block; a friendly "nothing to do"
    message when no block is present, never an error.
  - `--print` and `--remove` together is a hard error (mutually
    exclusive).
  - Every dependency on the real OS (`goos`, `getenv`, `lookPath`, the
    PowerShell-profile `run` function) is bundled into an `initDeps`
    struct injected into `newInitCmd`; `defaultInitDeps()` wires the
    real `runtime.GOOS`/`os.Getenv`/`exec.LookPath`/
    `context.RunCommand` in `NewRootCmd`, tests construct their own.
- **`comrade hook record`** (hidden, under a hidden `comrade hook`
  group): flags `--shell`, `--exit`, `--command`. Resolves
  `last_command.json`'s path and calls `WriteLastCommand`. **Always
  returns `nil`** from `RunE` — any failure is swallowed silently and
  only surfaced on stderr when `COMRADE_DEBUG` is set — because this
  command is invoked from inside a user's shell prompt on every command,
  and a broken write must never make a terminal session fail or get
  noisy.

### `scripts/install.sh` / `scripts/install.ps1`

Basic-but-functional installers (FAZ 10 finalizes packaging further):
resolve a version (`COMRADE_VERSION` or the latest GitHub release),
detect OS/arch, download the matching `goreleaser` archive +
`checksums.txt`, verify the checksum (`sha256sum -c` / `Get-FileHash`),
extract, and install — `~/.local/bin` falling back to `/usr/local/bin`
on POSIX; `%LOCALAPPDATA%\Programs\cli-comrade` + a user-`PATH` update
on Windows. Both end by suggesting `comrade init`.

## Decisions / deviations

- **Hooks call the compiled binary; they never hand-assemble JSON.**
  This is the load-bearing architectural decision for this phase. Shell
  quoting/escaping rules differ across bash/zsh/fish/PowerShell, and
  none of them make it *safe* to embed arbitrary command text (nested
  quotes, embedded newlines, unicode, backticks) into a JSON string
  literal from inside shell script — a single mis-escaped character
  either corrupts `last_command.json` or, worse, opens a command/code
  injection path in the hook itself. Delegating the encoding to
  `encoding/json` inside the one Go binary means there is exactly one
  code path to get this right, tested once
  (`TestHookRecordWritesLastCommandJSONRoundTrip` round-trips a command
  string containing quotes, an accented character, and an embedded
  newline), and identical across every shell. The cost is one extra
  process spawn per prompt; negligible next to an interactive user's
  typing speed, and the snippet already no-ops instantly via `command -v
  comrade` when the binary isn't installed.
- **No stderr/stdout capture in the hooks — `StderrTail`/`StdoutTail`
  stay empty.** UYGULAMA_PLANI.md FAZ 4 explicitly calls this out:
  "bash/zsh'de güvenilir global stderr tee riskli" — there is no safe,
  low-overhead way to durably tee every command's stderr from a
  PROMPT_COMMAND/precmd/fish_postexec hook without either an `exec 2>
  >(tee ...)` redirect (which changes the shell's own fd table for the
  rest of the session and interacts badly with subshells/pipes) or a
  wrapper around every single command (which the plan also rejects as
  too invasive). The hook therefore records only `command`, `exit_code`,
  `timestamp`, and `shell`. FAZ 7's `comrade fix --rerun` is the primary
  strategy for getting stderr: it controllably re-runs the last command
  through `internal/executor` and captures its output directly, which is
  both simpler and more reliable than any prompt-hook-based tee.
- **`--yes`/`-y` flag**: not in the plan's literal item list, but
  required for install scripts to call `comrade init` non-interactively
  and for every install-path test in this phase to run without a stdin
  fixture. A small, explicitly-flagged extension, not a scope
  expansion — no other behavior changed.
- **PowerShell profile resolution "keeps honest"**, mirroring FAZ 3's
  `IsAdmin` windows handling: `RCPath` only returns a path it actually
  got back from invoking a real PowerShell binary's `$PROFILE`. If
  `pwsh`/`powershell` isn't found, or the invocation errors, `comrade
  init` prints the block with manual-install instructions instead of
  guessing a filesystem path that might not match the user's actual
  `$PROFILE` (which depends on PowerShell edition, host, and possibly a
  custom profile setup).
- **The `history -s`-seeding technique in the bash E2E test, and why it
  works.** bash's history *list* is only auto-populated by the
  interactive readline loop reading a user's typed line — commands a
  script executes (even with `set -o history` on) are not automatically
  appended to it (confirmed empirically while building this test: with
  `set -o history` on inside a sourced script, *every* executed script
  line got auto-appended, polluting `history 1`'s result with the test
  harness's own bookkeeping lines instead of the command under test —
  removing `set -o history` and seeding only the intended entry via the
  explicit `history -s` builtin fixed this). The test therefore, right
  after running the failing command: captures its exit code into `st`;
  calls `history -s "false"` to seed exactly the entry an interactive
  user's typed `false` would have produced; restores `$?` back to `st`
  via `( exit "$st" )` (a subshell's own exit becomes the calling
  shell's `$?`); and only then evaluates `$PROMPT_COMMAND` — reproducing
  exactly what an interactive shell does after a typed command, without
  needing a PTY. `TestBashE2EHookRecordsFailedCommand` also has a
  from-a-real-interactive-shell counterpart demonstrated manually below
  (`bash -i` under a real installed `.bashrc`, no seeding needed) as
  belt-and-braces proof the synthetic seeding matches real behavior.
- **Windows-side runtime verification is deferred**, per this task's own
  environment note: neither `pwsh` nor `powershell` is available in this
  WSL2/Linux sandbox. The PowerShell snippet and `RCPath`'s Windows
  branch are covered by golden/unit tests with injected `goos`/
  `lookPath`/`run`, and `scripts/install.ps1`'s syntax is checked via
  PowerShell's own AST parser when a PowerShell binary is present
  (`t.Skip` otherwise, exercised as a skip in this run — see Gate
  below). **Actually running `comrade init powershell` and the installed
  hook against a real Windows PowerShell/pwsh session is a manual item
  the coordinator should track in `docs/PROGRESS.md`.**

## Gate

- `go vet ./...` — clean.
- `/home/firfir/go/bin/golangci-lint run` — 0 issues.
- `go test ./... -count=1` — all packages pass, including two new
  packages: `internal/shellinit` (shell/snippet/block/rcpath unit +
  golden tests) and the FAZ 4 additions to `internal/cli` (`init_test.go`,
  `hook_test.go`, `e2e_bash_test.go`, `scripts_test.go`). The PowerShell
  syntax-check test (`TestInstallPs1IsSyntacticallyValidPowerShell`)
  reports `SKIP` in this environment (no `pwsh`/`powershell` on `PATH`) —
  expected, per the deferred-verification note above.
- `make build` — succeeds.
- `make cross` — succeeds for all five targets (linux/amd64,
  linux/arm64, darwin/amd64, darwin/arm64, windows/amd64), proving
  `internal/shellinit`'s embeds and every new `internal/cli` file
  compile clean everywhere, including Windows.

### Bash E2E evidence

Real interactive-shell demonstration — `comrade init bash --yes` was run
for real (writing an actual `~/.bashrc`), then a real interactive
`bash -i --rcfile ~/.bashrc` session (reading commands over stdin, so its
own `PROMPT_COMMAND` fires naturally — no manual history seeding needed
here, unlike the synthetic `bash -c` unit test) ran a nonexistent command
and exited:

```
$ HOME=<tmp> ./comrade init bash --yes
Installed cli-comrade shell integration in <tmp>/.bashrc

$ PATH="$(pwd):$PATH" HOME=<tmp> XDG_STATE_HOME=<tmp-state> \
    bash --rcfile <tmp>/.bashrc -i <<'EOF'
nosuchcmd-xyz-demo
exit
EOF

$ cat <tmp-state>/cli-comrade/last_command.json
{"command":"nosuchcmd-xyz-demo","exit_code":127,"stderr_tail":"","stdout_tail":"","timestamp":"2026-07-08T23:48:28.144603925Z","shell":"bash"}
```

The command text, the real "command not found" exit code (127), and the
shell name all round-trip correctly, and `stderr_tail`/`stdout_tail` are
empty exactly as designed. This is committed as a repeatable regression
test in `internal/cli/e2e_bash_test.go`
(`TestBashE2EHookRecordsFailedCommand`), using the seeding technique
described above so it runs deterministically under `go test` without a
PTY.
