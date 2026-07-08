# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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
