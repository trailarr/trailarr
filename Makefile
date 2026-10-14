APP_NAME=trailarr
BIN_DIR=bin
SRC_DIR=cmd/trailarr

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Derive version from git if available; can be overridden by passing VERSION on the make command line.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

ifeq ($(GOOS),windows)
	BIN_EXT=.exe
else
	BIN_EXT=
endif

.PHONY: build clean run

.PHONY: test test-fast


build:
	go mod download
	go mod vendor
	# Inject AppVersion into the binary via ldflags so getModuleVersion can return a real app version.
	GOOS=$(GOOS) GOARCH=$(GOARCH) GOFLAGS=-mod=vendor go build -ldflags "-X trailarr/internal.AppVersion=$(VERSION)" -o $(BIN_DIR)/$(APP_NAME)$(BIN_EXT) $(SRC_DIR)/main.go
	ls -l $(BIN_DIR)

run: build
ifeq ($(GOOS),windows)
	$(BIN_DIR)\\$(APP_NAME)$(BIN_EXT)
else
	./$(BIN_DIR)/$(APP_NAME)$(BIN_EXT)
endif

clean:
ifeq ($(GOOS),windows)
	del /Q $(BIN_DIR)\\*
else
	rm -rf $(BIN_DIR)/*
endif

# Run the full test suite in a CI-friendly way (skip embedded store startup)
test:
	@echo "Running full test suite (embedded store not required; using bbolt storage)"
	go test ./... -v

# Faster, package-scoped tests for quick cycles
test-fast:
	@echo "Running internal package tests (embedded store not required; using bbolt storage)"
	go test ./internal -v

# Generate coverage report (coverage/coverage.out)
coverage:
	@echo "Generating coverage report (coverage/coverage.out)"
	@mkdir -p coverage
	# Ensure covdata tool (used by Go toolchain for coverage handling) is installed.
	# Installing is a no-op if already present in module cache/bin.
	@echo "Ensuring covdata tool is available..."
	@which covdata >/dev/null 2>&1 || (echo "Attempting to install covdata (non-fatal)..." && \
		go install golang.org/x/tools/cmd/covdata@latest >/dev/null 2>&1 || \
		echo "covdata install failed or package not available; continuing without it.")
	go test ./... -coverprofile=coverage/coverage.out

# Generate HTML coverage (coverage/coverage.html)
coverage-html: coverage
	@echo "Generating HTML coverage report (coverage/coverage.html)"
	go tool cover -html=coverage/coverage.out -o coverage/coverage.html
