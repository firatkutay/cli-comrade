# FAZ 10 — Paketleme ve Dağıtım

## What was built

### `.goreleaser.yaml` — complete packaging pipeline

Extended FAZ 0's archives/checksums-only config with every package
manager UYGULAMA_PLANI.md FAZ 10 item 1 asks for:

- **`homebrew_casks:`** (macOS/Linux) — NOT `brews:`. Researching the
  current v2 schema (Context7 `/websites/goreleaser`) surfaced that
  `brews:` (Homebrew Formula) is itself deprecated as of goreleaser
  v2.16 in favor of `homebrew_casks:` (Homebrew Cask) — `goreleaser
  check` fails outright on a deprecated key, not just a warning. Also
  applied within that block: `repository:` (not the older `tap:`) and
  `directory: Casks` (not `folder:`). Target repo:
  `firatkutay/homebrew-tap`.
- **`scoops:`** (Windows) — `repository:` with `owner`/`name`. Target
  repo: `firatkutay/scoop-bucket`.
- **`winget:`** — `publisher`, `short_description`, `license`,
  `package_identifier: FiratKutay.comrade`, `repository:` with a
  version-scoped `branch` and `pull_request.enabled: true` (winget
  publishes via PR, not a direct push). Target repo:
  `firatkutay/winget-pkgs` (a staging fork; production winget publishing
  is a PR to `microsoft/winget-pkgs`, out of this phase's scope).
- **`nfpms:`** — `.deb` + `.rpm`, `maintainer: "Fırat Kutay
  <firat.kutay@gmail.com>"`, `license: MIT`, `section: utils`,
  `homepage`.
- **`release: github: {owner, name}`** — this repo has no git remote
  configured (confirmed via `git remote -v`), so the GitHub repository
  is named explicitly rather than left for goreleaser to infer from
  `git remote get-url origin`.
- **`signs:`** — a commented-out cosign block (not wired to run in CI):
  it requires a cosign key pair or keyless/Fulcio+Rekor OIDC signing
  provisioned as CI secrets, which this repo does not have. The comment
  documents exactly what to provision (`COSIGN_PASSWORD` +
  `COSIGN_PRIVATE_KEY`/OIDC) to enable it later — CLAUDE.md rule #2's
  spirit ("API keys never in plaintext config") extended to "never wire
  a signing step that would either silently no-op or fail CI without the
  secret it needs."

None of `homebrew-tap`/`scoop-bucket`/`winget-pkgs` need to exist for
`goreleaser release --snapshot --clean` (snapshot mode never publishes);
they DO need to exist, empty, before a real `goreleaser release`.
`GITHUB_TOKEN` is the default publish token; cross-repo pushes to those
three repos need a separate PAT only if they're not already covered by
`GITHUB_TOKEN`'s own permissions — documented as
`HOMEBREW_TAP_GITHUB_TOKEN`/`SCOOP_BUCKET_GITHUB_TOKEN`/
`WINGET_PKGS_GITHUB_TOKEN` secrets, with the matching commented-out
`repository.token:` lines, rather than hardcoded.

### goreleaser + supporting-Action version pins (currency verified, not assumed)

Applying the ≥15-day version-selection rule (silently — see below for
the one place it's worth stating explicitly since it overrides what the
task text itself suggested):

| Tool | Chosen | Published | Why not newer |
|---|---|---|---|
| goreleaser CLI | `v2.16.0` | 2026-05-24 | `v2.17.0` (2026-07-04) is only ~5 days old |
| `goreleaser/goreleaser-action` | `v7.2.2` @ `5daf1e9...` | 2026-05-19 | `v7.2.3` (2026-06-29) is only ~10 days old |
| `anchore/sbom-action` | `v0.24.0` @ `e22c389...` | 2026-03-20 | already the newest available; well past 15 days |

Every SHA above was fetched live from each repo's GitHub API
(`git/refs/tags/<tag>`), not copied from memory or a doc.

### Self-update: hand-rolled, not a library — and why

Researched `minio/selfupdate` (Apache-2.0, MIT-compatible in spirit,
binary-diff-capable) and `creativeprojects/go-selfupdate` (MIT) as the
two maintained, reputable candidates. Chose to **hand-roll** instead,
because:

1. **Scope mismatch.** Both libraries solve a broader problem (binary
   patching, multi-provider release detection, GitHub Enterprise, etc.)
   than what's actually needed here: one GitHub repo, one release asset
   per platform, one checksums.txt already produced for install.sh to
   verify. Adopting either pulls in its own release-detection
   abstraction that would have to be threaded through anyway to satisfy
   the "injectable, mockable, never hits real GitHub in tests"
   requirement — at which point the library buys little over ~250 lines
   of stdlib.
2. **House convention.** CLAUDE.md's own architecture already avoids
   provider SDKs even for the LLM connectors ("ham REST istemcileri
   yazılır, bağımlılığı azaltmak için") — a hand-rolled `net/http` +
   `crypto/sha256` self-updater is the same philosophy applied one more
   time, not a new one.
3. **Zero new dependency.** `internal/update` imports only the standard
   library (`net/http`, `archive/tar`, `archive/zip`, `compress/gzip`,
   `crypto/sha256`, `encoding/json`, `os`, `path/filepath`, `time`) —
   satisfying this phase's "Otherwise zero new deps" constraint outright
   rather than needing a license/maintenance write-up for a third-party
   package.

**Security requirement enforced in code, not just policy:**
`update.Updater.Apply` downloads the platform archive AND
`checksums.txt` from the release, then calls `VerifyChecksum` — which
recomputes the archive's SHA-256 and compares it against the
`checksums.txt` entry — BEFORE `ExtractBinary` ever runs. A mismatch
returns an error and `ReplaceBinary` is never reached (asserted directly
in tests — `TestUpdaterApplyChecksumMismatchErrors`,
`TestUpgradeApplyChecksumMismatchNeverCallsReplace` — both check the
replace function was never invoked, not just that an error was
returned). The Windows "can't overwrite a running exe" case is handled
by `ReplaceBinary`'s rename-to-`.old` dance (`replace.go`), with
`CleanupOldBinary` removing the leftover on a later run — wired into the
passive version-notice hook (`updatenotice.go`) so it fires on every
subsequent command's startup on Windows, not only inside `comrade
upgrade` itself.

### `comrade upgrade` (`internal/cli/upgrade.go`)

`--check` only calls `Updater.Check` and prints either the "already on
latest" or "newer available: vX (you have vY) — URL" line; without
`--check`, `Updater.Apply` downloads/verifies/extracts, then the command
resolves its own running executable path (`os.Executable`, injected as
`upgradeDeps.executable` for tests) and calls `ReplaceBinary`. Both
paths refuse outright on a `dev` build (`update.IsDevBuild`) with a
translated error. Every network/OS touchpoint
(`fetcher`/`downloader`/`executable`/`replace`) is a field on
`upgradeDeps`, mirroring `initDeps`/`defaultInitDeps`'s existing
pattern exactly — `defaultUpgradeDeps(version)` wires the real ones in
`NewRootCmd`; tests construct their own with fakes.

### Passive version notice (`internal/cli/updatenotice.go`)

Wired as `root.PersistentPostRunE` (fires only after a subcommand's
`RunE` returns `nil` — confirmed by reading `spf13/cobra`'s own
`Command.execute`, not assumed). Skips: a true bare `comrade`
invocation (`cmd == root && len(args) == 0` — a free-text `comrade
<request>` do-dispatch is `cmd == root` too but WITH args, and is NOT
skipped), `comrade upgrade` itself, a `dev` build, and
`general.update_check = false`. When due (≥7 days since the last
attempt, `update.ShouldCheck`), fetches the latest release with a 3s
timeout and persists the attempt's timestamp REGARDLESS of success —
an offline machine is throttled to one attempt per week, not one
attempt per command, for the whole time it stays offline. Every failure
mode is silent (no stderr output, no command-failure) except the one
success case that actually found a newer version, which prints one line
to stderr.

`NewRootCmd(version string) *cobra.Command`'s own public signature is
untouched; it now delegates to an unexported `newRootCmd(version string,
updateFetcher update.ReleaseFetcher) *cobra.Command`, wiring in the real
`&update.GitHubClient{}`. Tests that need to exercise a full
successful/failed background check call `newRootCmd` directly with a
fake `ReleaseFetcher` — `internal/cli`'s test files are `package cli`,
so this unexported constructor is directly reachable without widening
`NewRootCmd`'s own public API.

### `general.update_check` config key

Added to `internal/config/schema.go` (`GeneralConfig.UpdateCheck`,
default `true`) and `internal/config/validate.go`'s `keyDefs` registry
(`KindBool`) — both bidirectional drift-guard tests
(`TestKeyDefsMatchConfigStruct`, `TestKeyDefsMatchDefaultConfigTOML`)
stay green because both sides were updated together, exactly as FAZ 1's
own convention requires. No env-alias needed: the generic
`COMRADE_GENERAL_UPDATE_CHECK` mapping `bindEnvAliases`/`envCandidates`
already derive from the dotted key covers it.

### Install scripts (`scripts/install.sh`/`install.ps1`)

`install.sh` gained the two FAZ 4-deferred reviewer findings:

1. **`wget` fallback.** `fetch_url`/`fetch_url_to_file` dispatch on
   whichever of `curl`/`wget` `require_downloader` finds (curl
   preferred), so every download in the script (version lookup, archive,
   checksums.txt) goes through ONE code path instead of two
   independently-maintained curl-only/wget-only copies. Neither present
   → a clear, actionable error, not a cryptic "command not found."
2. **The install-script/goreleaser archive-name mirror is now guarded**
   (see below) instead of being an unguarded hand-copy.

Also added (beyond the two explicit findings, since the task text also
asked for a "sudo prompt" fallback): if neither `~/.local/bin` nor
`/usr/local/bin` is writable, the script now falls back to `sudo`
(prompting for a password) instead of failing outright, when `sudo` is
on PATH.

`install.ps1` was already checksum-verifying and PATH-aware from FAZ 4;
no functional change was needed there this phase.

### Release-name drift guard (Derive-or-Guard)

`internal/cli/release_names_test.go`'s
`TestReleaseArchiveNamingIsConsistentAcrossGoreleaserInstallScriptsAndUpdatePackage`
is the bidirectional guard UYGULAMA_PLANI.md FAZ 10 item 2 requires. It:

1. Reads `.goreleaser.yaml` and extracts `project_name`, the archives
   `name_template` (parsed as a real `text/template`, then EXECUTED for
   all 5 platform builds — not string-matched), the default/windows
   archive formats, and `checksum.name_template`.
2. Reads `scripts/install.sh`'s `BIN_NAME` and its
   `archive="${BIN_NAME}_${version_number}_${os}_${arch}.tar.gz"` line,
   parsing the interpolated variable sequence + extension.
3. Reads `scripts/install.ps1`'s equivalent `$archive = "comrade_...` line
   the same way.
4. Cross-checks all of the above against
   `internal/update.ArchiveName`/`BinaryName`/`ChecksumsFileName`'s own
   literal output.

A drift in ANY of the four — goreleaser's template, either install
script, or `internal/update` — fails this one test. It is a Go test, so
it already runs via `go test ./...` in `.github/workflows/ci.yml`'s
existing `build-test` job; no separate CI step was added (a separate
step would itself be an unguarded, hand-maintained duplicate of "run the
Go test suite").

### Docs (`docs/INSTALL.md`, `CONFIGURATION.md`, `SECURITY.md`,
`TROUBLESHOOTING.md`)

Each bilingual (Türkçe section, then English section — matching
`README.md`'s own existing convention, not two separate files per
language). Content was read out of the actual implemented code (config
schema, denylist rule names, redact pattern families, secrets
precedence, audit entry shape) rather than invented — e.g.
`CONFIGURATION.md`'s key table was built directly from
`internal/config/schema.go`'s `defaultConfigTOML` and
`internal/config/validate.go`'s `keyDefs`, and `SECURITY.md`'s denylist
list is the literal 8 rule names in `internal/safety/denylist.go`.
`README.md` gained one-line per-OS install commands (TR and EN sections)
and links to all four new docs.

## Decisions

- **`homebrew_casks:` over `brews:`** — see above; this is a hard
  requirement (`goreleaser check` fails on the deprecated key), not a
  style preference.
- **goreleaser `v2.16.0`, not `v2.17.0`** — the ≥15-day rule applied
  silently; `v2.17.0` was only ~5 days old at the time of this work.
- **Hand-rolled self-update over `minio/selfupdate`/
  `creativeprojects/go-selfupdate`** — see the dedicated section above.
- **`newRootCmd(version, updateFetcher)` as an unexported second
  constructor** instead of widening `NewRootCmd`'s public signature —
  keeps `cmd/comrade/main.go` and every pre-existing test unchanged,
  while still making the passive-notice's network dependency fully
  injectable for the tests that need it.
- **Update-notice state lives under the state dir
  (`update_check.json`), never inside `config.toml`** — it's
  machine-local bookkeeping (a timestamp), not user-facing
  configuration, matching `audit.jsonl`/`last_command.json`'s existing
  placement (both already under the same directory).
- **Synchronous-with-a-short-timeout, not a detached goroutine, for the
  background check** — a goroutine outliving `Execute()` has no
  guaranteed chance to print before the process exits (Go does not wait
  for orphaned goroutines on `main` return), which would make the notice
  silently flaky. A bounded 3s worst case, once per 7 days, is the
  documented tradeoff instead.
- **A source SBOM in `release.yml`, not asked for verbatim in the task
  list but added anyway** — this repo ships no container image, so
  there is no separate "container SBOM" to also generate; a source-only
  CycloneDX SBOM via `anchore/sbom-action` is the complete, minimal scope
  for a binary-only Go CLI release, and is a standing devops-role
  requirement for every release, not scope creep.
- **No separate CI step for the release-name drift guard** — it is a Go
  test, already covered by `ci.yml`'s existing `go test ./...` step; a
  dedicated step would itself become a second, parallel thing to keep in
  sync.

## Tests

- `internal/update`: `github_test.go` (httptest-mocked `GitHubClient`,
  never real GitHub), `downloader_test.go`, `asset_test.go` (exact
  archive-name/binary-name/version-stripping table cases),
  `checksum_test.go` (accept/reject/missing-entry/binary-mode-asterisk),
  `extract_test.go` (real in-memory tar.gz/zip fixtures, both
  found/not-found paths), `version_test.go` (`IsNewer`/`IsDevBuild` table
  cases incl. snapshot-suffix and missing-trailing-component edge cases),
  `state_test.go` (path resolution incl. Windows branch, read/write
  round-trip, corrupt-JSON tolerance, `ShouldCheck` boundary at exactly 7
  days), `replace_test.go` (both the Unix rename path and the Windows
  rename-to-`.old` dance, incl. a pre-existing stale `.old` file),
  `updater_test.go` (`Check`/`Apply` against fakes, incl. the
  checksum-mismatch-never-installs assertion).
- `internal/cli`: `upgrade_test.go` (dev-build refusal for both `--check`
  and apply, up-to-date/newer-available reporting, fetch-error
  propagation, the full download→verify→replace path asserting the
  EXACT bytes handed to `replace`, and the checksum-mismatch-never-calls-
  replace assertion), `updatenotice_test.go` (every skip condition,
  the full successful-check path via a fake fetcher through `newRootCmd`
  asserting the exact printed version strings AND the persisted state's
  `LatestKnownVersion`, the silent-on-failure path asserting empty
  stderr, and the weekly-throttle path asserting the fetcher is never
  even called), `release_names_test.go` (the drift guard above).
- `internal/config`: `TestDefaultMatchesPlanExactly` extended for
  `general.update_check`; both pre-existing bidirectional drift-guard
  tests (`TestKeyDefsMatchConfigStruct`,
  `TestKeyDefsMatchDefaultConfigTOML`) pass unchanged, proving the new
  key was added symmetrically.
- `internal/i18n`: `TestCatalogsCoverIdenticalKeys` /
  `TestCatalogsHaveNoEmptyValues` pass unchanged with the new
  `comrade upgrade`/update-notice message IDs added to both catalogs.

## Manual/local verification

- `go vet ./...` — clean.
- `/home/firfir/go/bin/golangci-lint run ./...` — `0 issues.`
- `go test ./... -count=1` — all packages pass, including
  `internal/update` (new) and every extended `internal/cli`/
  `internal/config`/`internal/i18n` test.
- `make build` and `make cross` (5 platform binaries) — both succeed.
- `/home/firfir/go/bin/goreleaser check` (v2.16.0, installed to
  `$(go env GOPATH)/bin` per the pinned-tool convention, invoked by full
  path exactly like `golangci-lint`) — `1 configuration file(s)
  validated`.
- `/home/firfir/go/bin/goreleaser release --snapshot --clean` —
  succeeded; full `dist/` listing:

```
dist/artifacts.json
dist/checksums.txt
dist/comrade-0.0.0~SNAPSHOT_none-1.aarch64.rpm
dist/comrade-0.0.0~SNAPSHOT_none-1.x86_64.rpm
dist/comrade_0.0.0~SNAPSHOT-none_amd64.deb
dist/comrade_0.0.0~SNAPSHOT-none_arm64.deb
dist/comrade_0.0.0-SNAPSHOT-none_darwin_amd64.tar.gz
dist/comrade_0.0.0-SNAPSHOT-none_darwin_arm64.tar.gz
dist/comrade_0.0.0-SNAPSHOT-none_linux_amd64.tar.gz
dist/comrade_0.0.0-SNAPSHOT-none_linux_arm64.tar.gz
dist/comrade_0.0.0-SNAPSHOT-none_windows_amd64.zip
dist/comrade_darwin_amd64_v1/comrade
dist/comrade_darwin_arm64_v8.0/comrade
dist/comrade_linux_amd64_v1/comrade
dist/comrade_linux_arm64_v8.0/comrade
dist/comrade_windows_amd64_v1/comrade.exe
dist/config.yaml
dist/homebrew/Casks/comrade.rb
dist/metadata.json
dist/scoop/comrade.json
dist/winget/manifests/f/FiratKutay/comrade/0.0.0-SNAPSHOT-none/FiratKutay.comrade.installer.yaml
dist/winget/manifests/f/FiratKutay/comrade/0.0.0-SNAPSHOT-none/FiratKutay.comrade.locale.en-US.yaml
dist/winget/manifests/f/FiratKutay/comrade/0.0.0-SNAPSHOT-none/FiratKutay.comrade.yaml
```

  Spot-checked `dist/homebrew/Casks/comrade.rb`, `dist/scoop/comrade.json`,
  and the winget manifest's contents directly (not just their presence) —
  correct URLs, correct sha256 per-platform entries, correct `name`/
  `homepage`/`license`/`description`.
- `sh -n scripts/install.sh` and `bash -n scripts/install.sh` — both
  clean. `shellcheck` and `pwsh`/`powershell` are NOT available in this
  sandbox (confirmed via `command -v`) — `internal/cli/scripts_test.go`'s
  pre-existing `TestInstallShIsValidPOSIXShell`/
  `TestInstallPs1IsSyntacticallyValidPowerShell` already `t.Skip` the
  PowerShell half for exactly this reason (a pre-existing, documented
  gap from FAZ 4, not new to this phase); ran the sh-only checks
  successfully.

## Known limitations / deferred work

- **No live end-to-end run of `install.sh`/`install.ps1` against a real
  published release** — this repo has never actually cut a `vX.Y.Z` tag,
  so there is no real GitHub release for the scripts' hardcoded
  `github.com/firatkutay/cli-comrade` URLs to hit. Verified instead via:
  static syntax checks, the release-name drift guard (proves the
  archive-name construction is CORRECT, even if never exercised against
  a live download), and the actual `goreleaser --snapshot` artifacts'
  content. The user should do one real, live `curl | sh` /
  `irm | iex` run after the first real tag is cut.
- **`comrade upgrade`'s full download→replace path has no live-network
  test either**, for the same reason (no real release exists yet) — only
  exercised against fakes (which is exactly what the task's own
  constraint required: "do NOT hit real GitHub in tests"). Once a real
  `v0.1.0` (or later) tag exists, running `comrade upgrade --check` (safe,
  read-only) and then a real `comrade upgrade` from an older installed
  build is the recommended one-time manual verification.
- **The `homebrew-tap`/`scoop-bucket`/`winget-pkgs` target repos do not
  exist yet** — `goreleaser release --snapshot --clean` does not need
  them (snapshot mode skips publish entirely), but a REAL `goreleaser
  release` will fail at the brew/scoop/winget publish steps until they
  are created (empty is fine; goreleaser creates the actual
  files/commits). This is expected and was explicitly scoped as
  acceptable by the phase's own acceptance criterion.
- **cosign signing is documented, not wired** — see the commented `signs:`
  block; enabling it needs a key-provisioning decision (a committed
  keypair with a CI secret, vs. keyless OIDC) that is a real, standalone
  decision for the user to make, not something to default silently.
- **`README.md`'s "Durum"/"Status" sections still say "FAZ 0"** — this
  is pre-existing staleness from before this phase (not touched, since
  updating overall project-status prose was outside this phase's scope
  and risked scope creep); flagging it here so it isn't silently missed
  by a later phase.
