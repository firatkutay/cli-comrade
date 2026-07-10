# FAZ 00 — Project Skeleton

## What was delivered

- Go module `github.com/firatkutay/cli-comrade`, `go 1.23.0` (language-version
  floor, matching CLAUDE.md's "Go 1.23+") plus `toolchain go1.26.4` (pins the
  exact compiler CI and developers use). Verified both direct deps require a
  lower floor than 1.23 (cobra v1.10.2 → `go 1.15`; testify v1.11.1 →
  `go 1.17`), so 1.23.0 is a safe, non-inflated floor.
- Directory layout exactly matching CLAUDE.md's "Dizin Yapısı": `cmd/comrade/`
  plus `internal/{cli,config,llm,context,redact,engine,executor,safety,audit,
  i18n,tui}/`, `scripts/`, `docs/`. Every `internal/*` package (other than
  `cli`, which has real FAZ 0 logic) carries a `doc.go` placeholder comment
  describing what it will hold and which phase fills it in.
- A cobra-based `comrade` root command (`internal/cli`):
  - Bare invocation prints the version banner followed by the standard help
    output.
  - `--version`/`-v` prints `comrade version <version>` via cobra's built-in
    version flag.
  - Six stub subcommands (`fix`, `explain`, `chat`, `config`, `init`,
    `history`), each routed through one shared helper
    (`internal/cli/stub.go`) so the FAZ 9 i18n migration only has to touch
    one place.
  - `version` is injected at build time via `-ldflags -X main.version=...`
    and defaults to `"dev"`.
- `Makefile` with `build`, `test`, `lint`, `vet`, `cross` (five platforms:
  linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64), and
  `tools` (installs golangci-lint v2.12.2 into `$(go env GOPATH)/bin`,
  invoked by full path so it works regardless of `PATH`).
- `.golangci.yml` in the v2 schema (`version: "2"`), using the `standard`
  preset plus `misspell`, `unconvert`; `gofmt`/`goimports` formatters.
- `.github/workflows/ci.yml`: a build+test matrix across
  ubuntu-latest/macos-latest/windows-latest, plus a separate lint job. All
  actions pinned by full commit SHA with a version comment.
- `.goreleaser.yaml` (schema `version: 2`): builds for the five target
  platforms, tar.gz/zip archives, checksums. Brew/scoop/winget are
  deliberately deferred to FAZ 10 per the plan.
- `README.md` with the project vision, the three-mode behavior table, and an
  "under development" warning, in one Turkish and one English section.
- `.gitignore`, `.gitattributes` (LF by default, CRLF for `*.ps1`),
  `LICENSE` (MIT), `CHANGELOG.md` (Keep a Changelog format).

## Decisions taken

- **`internal/context` package name.** The directory is named `context` per
  CLAUDE.md's tree, and CLAUDE.md pins the *directory* name, not the Go
  package identifier. In FAZ 0 the package is a placeholder (`package
  context`, `doc.go` only) with no imports, so there is no stdlib
  `context.Context` shadowing conflict yet. This is flagged here so that
  whoever implements FAZ 3 is aware: if `internal/context` ever needs to
  import the stdlib `context` package in the same file, that file will need
  an import alias (e.g. `stdctx "context"`) — the package's own name does not
  need to change.
- **golangci-lint is not vendored or embedded.** It is GPL-3.0-licensed and
  is invoked strictly as a separate CI/developer-tooling process via
  `make lint`, never imported into the module. This is also recorded in
  `docs/PROGRESS.md`.
- **`.goreleaser.yaml` validated without a git remote configured.**
  `goreleaser check` fails on a repo with no `origin` remote
  ("no remote configured to list refs from") — this is expected for a
  freshly `git init`'d repo and is unrelated to the config's own validity;
  the config was independently confirmed valid by temporarily adding a
  placeholder `origin` remote, running the check, and removing it again.

## Acceptance evidence

- `make build` → `./comrade`; `./comrade --version` → `comrade version dev`;
  `./comrade --help` lists all six subcommands; each stub prints
  `comrade <name>: this feature is not ready yet.`
- `make cross` → five binaries in `dist/`
  (`comrade-linux-amd64`, `comrade-linux-arm64`, `comrade-darwin-amd64`,
  `comrade-darwin-arm64`, `comrade-windows-amd64.exe`).
- `go vet ./...` — clean.
- `$(go env GOPATH)/bin/golangci-lint run` — `0 issues.`
- `go test ./...` — all packages pass; `internal/cli` has real assertions
  covering the version banner, `--version` output, and every stub's exact
  message text.
