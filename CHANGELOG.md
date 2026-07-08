# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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
