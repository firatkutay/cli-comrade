GOLANGCI_LINT_VERSION := v2.12.2
GOPATH_BIN             := $(shell go env GOPATH)/bin
GOLANGCI_LINT          := $(GOPATH_BIN)/golangci-lint

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

.PHONY: build test lint vet cross tools clean

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
