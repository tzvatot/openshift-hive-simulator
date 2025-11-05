.PHONY: help build run clean deps test setup-envtest install-tools all restart generate-provision-shard-config

# Default target
.DEFAULT_GOAL := help

# Binary and version info
BINARY_NAME := hive-simulator
BIN_DIR := bin
CMD_DIR := cmd
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X github.com/tzvatot/openshift-hive-simulator/pkg/info.Version=$(VERSION)

# Go settings
GO := go
GOBIN := $(shell pwd)/$(BIN_DIR)
GOCMD := CGO_ENABLED=1 $(GO)

# Envtest settings
ENVTEST_K8S_VERSION := 1.28.0
ENVTEST_BIN_DIR := $(BIN_DIR)/k8s

# Configuration
CONFIG_FILE := config/hive-simulator.yaml
LOG_LEVEL := info
API_PORT := 8080

help: ## Display this help message
	@echo "OpenShift Hive Simulator - Makefile targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2}'
	@echo ""

all: deps setup-envtest build ## Download dependencies, setup envtest, and build

deps: ## Download Go module dependencies
	@echo "==> Downloading dependencies..."
	$(GO) mod download
	@echo "==> Tidying dependencies..."
	$(GO) mod tidy

install-tools: ## Install required tools (setup-envtest)
	@echo "==> Installing setup-envtest..."
	GOBIN=$(GOBIN) $(GO) install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

setup-envtest: install-tools ## Setup envtest Kubernetes binaries
	@echo "==> Setting up envtest binaries..."
	@mkdir -p $(ENVTEST_BIN_DIR)
	$(GOBIN)/setup-envtest use $(ENVTEST_K8S_VERSION) --bin-dir $(ENVTEST_BIN_DIR)
	@echo "==> Envtest binaries installed to $(ENVTEST_BIN_DIR)"

build: ## Build the hive-simulator binary
	@echo "==> Building $(BINARY_NAME)..."
	@mkdir -p $(BIN_DIR)
	$(GOCMD) build \
		-ldflags="$(LDFLAGS)" \
		-o $(BIN_DIR)/$(BINARY_NAME) \
		./$(CMD_DIR)
	@echo "==> Build complete: $(BIN_DIR)/$(BINARY_NAME)"

run: build setup-envtest ## Build and run the simulator
	@echo "==> Starting Hive Simulator..."
	@echo "  Config file: $(CONFIG_FILE)"
	@echo "  API port: $(API_PORT)"
	@echo "  Log level: $(LOG_LEVEL)"
	KUBEBUILDER_ASSETS=$$($(GOBIN)/setup-envtest use $(ENVTEST_K8S_VERSION) -p path) \
		./$(BIN_DIR)/$(BINARY_NAME) --config $(CONFIG_FILE) --log-level $(LOG_LEVEL)

test: ## Run unit tests
	@echo "==> Running tests..."
	$(GO) test -v -race -coverprofile=coverage.out ./...
	@echo "==> Coverage report:"
	$(GO) tool cover -func=coverage.out

test-coverage: test ## Run tests and generate HTML coverage report
	@echo "==> Generating HTML coverage report..."
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "==> Coverage report: coverage.html"

fmt: ## Format Go code
	@echo "==> Formatting code..."
	$(GO) fmt ./...

vet: ## Run go vet
	@echo "==> Running go vet..."
	$(GO) vet ./...

lint: fmt vet ## Run formatters and linters

clean: ## Clean build artifacts
	@echo "==> Cleaning build artifacts..."
	rm -rf $(BIN_DIR)
	rm -f coverage.out coverage.html
	rm -f /tmp/hive-simulator-kubeconfig.yaml
	rm -f provision_shards_simulator.yaml
	@echo "==> Clean complete"

clean-all: clean ## Clean everything including dependencies
	@echo "==> Cleaning module cache..."
	$(GO) clean -modcache

restart: ## Stop, rebuild, and restart the simulator with config regeneration
	@echo "==> Restarting Hive Simulator..."
	@./$(CMD_DIR)/restart-simulator.sh

generate-provision-shard-config: ## Generate provision shard configuration
	@echo "==> Generating provision shard configuration..."
	@./$(CMD_DIR)/generate-provision-shard-config.sh

stop: ## Stop any running simulator processes
	@echo "==> Stopping simulator processes..."
	@pkill -9 -f "bin/hive-simulator" || true
	@pkill -9 -f "kube-apiserver.*envtest" || true
	@pkill -9 -f "etcd.*k8s_test_framework" || true
	@echo "==> Stopped"

dev: ## Run in development mode with debug logging
	@$(MAKE) run LOG_LEVEL=debug

check: lint test ## Run all checks (lint + test)

.PHONY: version
version: ## Display version information
	@echo "Version: $(VERSION)"
	@echo "Binary: $(BIN_DIR)/$(BINARY_NAME)"
	@echo "Go version: $$($(GO) version)"
