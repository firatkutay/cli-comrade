# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- FAZ 0: project skeleton — Go module, `internal/`/`cmd/` directory layout,
  cobra root command (`comrade --version` / `--help`), stub subcommands
  (`fix`, `explain`, `chat`, `config`, `init`, `history`), Makefile
  (`build`/`test`/`lint`/`vet`/`cross`/`tools`), `.golangci.yml`, GitHub
  Actions CI (build/test/lint across ubuntu/macos/windows), base
  `.goreleaser.yaml`, README, LICENSE.
