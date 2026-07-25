GOLANGCI_LINT_VERSION := v2.12.2
GORELEASER_VERSION     := v2.16.0
GOPATH_BIN             := $(shell go env GOPATH)/bin
GOLANGCI_LINT          := $(GOPATH_BIN)/golangci-lint
GORELEASER             := $(GOPATH_BIN)/goreleaser

BINARY      := comrade
VERSION     ?= dev
LDFLAGS     := -X main.version=$(VERSION)
DIST_DIR    := dist
NPM_OUT_DIR := npm/packages

# NPM_VERSION deliberately does NOT default to $(VERSION) ("dev" is not a
# valid semver and scripts/build-npm-packages.sh rejects it on purpose —
# npm packages must never be silently assembled under a guessed version).
# Pass it explicitly: `make npm-package NPM_VERSION=0.4.2`.
NPM_VERSION ?=

CROSS_TARGETS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64

.PHONY: build test lint vet cross tools clean release-check release-snapshot coverage-check \
	npm-test npm-package npm-dry-run npm-smoke

build:
	go build -ldflags "$(LDFLAGS)" -o ./$(BINARY) ./cmd/comrade

test:
	go test ./...

# coverage-check is the per-package coverage ratchet (GitHub issue #21):
# fails when a package's go test coverage drops below the floor recorded
# in coverage-floors.txt, or when that file has drifted out of sync with
# go list ./... in either direction. See coverage-floors.txt's own header
# for the re-baselining procedure after intentionally changing coverage.
coverage-check:
	bash scripts/check-coverage-floors.sh

vet:
	go vet ./...

lint: tools
	$(GOLANGCI_LINT) run

tools:
	@if [ ! -x "$(GOLANGCI_LINT)" ]; then \
		echo "installing golangci-lint $(GOLANGCI_LINT_VERSION)..."; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi
	@if [ ! -x "$(GORELEASER)" ]; then \
		echo "installing goreleaser $(GORELEASER_VERSION)..."; \
		go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION); \
	fi

# release-check validates .goreleaser.yaml (schema + deprecations) without
# building anything — the fast, no-side-effects half of the FAZ 10
# acceptance check.
release-check: tools
	$(GORELEASER) check

# release-snapshot performs a full local dry-run build of every release
# artifact (archives, checksums, .deb/.rpm, brew/scoop/winget manifests)
# with --clean --snapshot, so it never publishes or requires a real tag —
# docs/history/UYGULAMA_PLANI.md FAZ 10's acceptance check, runnable with no GITHUB_TOKEN.
release-snapshot: tools
	$(GORELEASER) release --snapshot --clean

cross:
	@mkdir -p $(DIST_DIR)
	@for target in $(CROSS_TARGETS); do \
		os=$${target%/*}; \
		arch=$${target#*/}; \
		ext=""; \
		[ "$$os" = "windows" ] && ext=".exe"; \
		echo "building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" \
			-o $(DIST_DIR)/$(BINARY)-$$os-$$arch$$ext ./cmd/comrade || exit 1; \
	done

clean:
	rm -f ./$(BINARY) ./$(BINARY).exe
	rm -rf $(DIST_DIR)

# --- npm distribution (stage 1: packaging + dispatcher + assembly only —
# see docs/PACKAGING.md; nothing here ever runs `npm publish` for real) ---

# npm-test runs the dispatcher/platform-map Node assertion scripts plus the
# assembly script's own failure-mode tests (bad version, missing binary).
# No npm dependencies are installed for this — see npm/main/package.json's
# "engines" floor; the tests use only Node built-ins.
npm-test:
	bash npm/test/run-node-tests.sh
	bash npm/test/test-assemble.sh

# npm-package assembles the 6 ready-to-publish package directories from an
# existing goreleaser dist/ (run `make release-snapshot` first) into
# $(NPM_OUT_DIR). NPM_VERSION must be the git tag WITHOUT its leading "v".
npm-package:
	bash scripts/build-npm-packages.sh "$(NPM_VERSION)" "$(DIST_DIR)" "$(NPM_OUT_DIR)"

# npm-dry-run assembles the packages, then runs `npm publish --dry-run`
# against every one of them — shows the exact tarball file lists/sizes
# without ever contacting the registry to actually publish.
npm-dry-run: npm-package
	@for pkg in $(NPM_OUT_DIR)/*/; do \
		echo "--- npm publish --dry-run: $$pkg ---"; \
		npm publish --dry-run "$$pkg" || exit 1; \
	done

# npm-smoke assembles a fresh copy of the packages, `npm pack`s the
# linux-x64 platform package + the main package, installs both tarballs
# together into a throwaway prefix, and runs the resulting `comrade`
# binary — Linux only (see npm/test/test-smoke.sh). NPM_VERSION may be
# left unset here: test-smoke.sh falls back to the nearest git tag.
npm-smoke:
	bash npm/test/test-smoke.sh "$(DIST_DIR)" "$(NPM_VERSION)"
