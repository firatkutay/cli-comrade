GOLANGCI_LINT_VERSION := v2.12.2
GORELEASER_VERSION     := v2.16.0
GOPATH_BIN             := $(shell go env GOPATH)/bin
GOLANGCI_LINT          := $(GOPATH_BIN)/golangci-lint
GORELEASER             := $(GOPATH_BIN)/goreleaser

BINARY   := comrade
VERSION  ?= dev
LDFLAGS  := -X main.version=$(VERSION)
DIST_DIR := dist

CROSS_TARGETS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64

.PHONY: build test lint vet cross tools clean release-check release-snapshot

build:
	go build -ldflags "$(LDFLAGS)" -o ./$(BINARY) ./cmd/comrade

test:
	go test ./...

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
