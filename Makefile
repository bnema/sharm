# Sharm Makefile
# Variables can be overridden with environment variables or in .env

# Include .env file if it exists (for local development)
ifneq (,$(wildcard ./.env))
	include .env
	export
endif

# =====================================================
# Project Variables
# =====================================================
PROJECT_NAME := sharm
BINARY_NAME := sharm
GO_MODULE := github.com/bnema/sharm

# Version from git tags (falls back to "dev" if no tags)
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD)
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Docker Registry (can be set in .env or as env var)
REGISTRY ?= ghcr.io/bnema
IMAGE_NAME := $(PROJECT_NAME)

# Go variables
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := $(GOCMD) fmt
GOVET := $(GOCMD) vet

# Build variables
LDFLAGS := -ldflags="-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"
GOFLAGS := -buildvcs=false

# Directories
CMD_DIR := ./cmd/$(BINARY_NAME)
DIST_DIR := ./dist
DATA_DIR := ./data

# Docker variables
DOCKER := docker
DOCKER_COMPOSE := docker compose
PLATFORMS := linux/amd64

# =====================================================
# Development Targets
# =====================================================

.PHONY: all
all: deps generate build

## deps: Download Go module dependencies
.PHONY: deps
deps:
	$(info Downloading dependencies...)
	$(GOMOD) download
	$(GOMOD) tidy

## generate: Generate code (sqlc, templ, mocks)
.PHONY: generate
generate:
	$(info Generating sqlc code...)
	@command -v sqlc >/dev/null 2>&1 || { echo "sqlc not found. Install with: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest"; exit 1; }
	sqlc generate
	$(info Generating templ code...)
	@command -v templ >/dev/null 2>&1 || { echo "templ not found. Install with: go install github.com/a-h/templ/cmd/templ@latest"; exit 1; }
	templ generate
	$(info Generating mocks...)
	@command -v mockery >/dev/null 2>&1 || { echo "mockery not found. Install with: go install github.com/vektra/mockery/v3@latest"; exit 1; }
	mockery

## fmt: Format Go code
.PHONY: fmt
fmt:
	$(info Formatting code...)
	$(GOFMT) ./...

## lint: Run linters
.PHONY: lint
lint:
	$(info Running linters...)
	$(GOVET) ./...
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed (optional)"
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed (optional)"

## vet: Run go vet
.PHONY: vet
vet:
	$(GOVET) ./...

# =====================================================
# Build Targets
# =====================================================

## build: Build the Go binary for current platform
.PHONY: build
build:
	$(info Building $(BINARY_NAME)...)
	@mkdir -p $(DIST_DIR)
	$(GOBUILD) $(GOFLAGS) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME) $(CMD_DIR)

## build-linux-amd64: Build for Linux AMD64
.PHONY: build-linux-amd64
build-linux-amd64:
	$(info Building $(BINARY_NAME) for linux/amd64...)
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(GOFLAGS) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)

## build-linux-arm64: Build for Linux ARM64
.PHONY: build-linux-arm64
build-linux-arm64:
	$(info Building $(BINARY_NAME) for linux/arm64...)
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) $(GOFLAGS) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_DIR)

## build-all: Build for all platforms
.PHONY: build-all
build-all: build-linux-amd64 build-linux-arm64

## clean: Remove build artifacts
.PHONY: clean
clean:
	$(info Cleaning build artifacts...)
	@rm -rf $(DIST_DIR)
	@rm -rf $(DATA_DIR)
	@find . -name "*.tmp" -delete

# =====================================================
# Test Targets
# =====================================================

## test: Run all tests
.PHONY: test
test:
	$(info Running tests...)
	$(GOTEST) -v ./...

## test-short: Run short tests only
.PHONY: test-short
test-short:
	$(info Running short tests...)
	$(GOTEST) -short -v ./...

## test-race: Run tests with race detector
.PHONY: test-race
test-race:
	$(info Running tests with race detector...)
	$(GOTEST) -race -v ./...

## test-coverage: Run tests with coverage
.PHONY: test-coverage
test-coverage:
	$(info Running tests with coverage...)
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo Coverage report generated: coverage.html

## benchmark: Run benchmarks
.PHONY: benchmark
benchmark:
	$(info Running benchmarks...)
	$(GOTEST) -bench=. -benchmem ./...

# =====================================================
# Docker Targets
# =====================================================

## docker-build: Build Docker image for current platform
.PHONY: docker-build
docker-build:
	$(info Building Docker image...)
	$(DOCKER) build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) .
	$(DOCKER) tag $(REGISTRY)/$(IMAGE_NAME):$(VERSION) $(REGISTRY)/$(IMAGE_NAME):latest

## docker-buildx: Build Docker image for current platform using buildx
.PHONY: docker-buildx
docker-buildx:
	$(info Building Docker image with buildx...)
	$(DOCKER) buildx build \
		--platform linux/amd64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) \
		-t $(REGISTRY)/$(IMAGE_NAME):latest \
		--load \
		.

## docker-buildx-multi: Build Docker image for multiple platforms
.PHONY: docker-buildx-multi
docker-buildx-multi:
	$(info Building Docker image for multiple platforms...)
	$(DOCKER) buildx build \
		--platform $(PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) \
		-t $(REGISTRY)/$(IMAGE_NAME):latest \
		--load \
		.

## docker-buildx-push: Build and push multi-platform image to registry
.PHONY: docker-buildx-push
docker-buildx-push:
	$(info Building and pushing multi-platform Docker image to $(REGISTRY)...)
	$(DOCKER) buildx build \
		--platform $(PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) \
		-t $(REGISTRY)/$(IMAGE_NAME):latest \
		--push \
		.

## docker-push: Push current image to registry
.PHONY: docker-push
docker-push:
	$(info Pushing Docker image to $(REGISTRY)...)
	$(DOCKER) push $(REGISTRY)/$(IMAGE_NAME):$(VERSION)
	$(DOCKER) push $(REGISTRY)/$(IMAGE_NAME):latest

## docker-tag-latest: Tag image as latest
.PHONY: docker-tag-latest
docker-tag-latest:
	$(DOCKER) tag $(REGISTRY)/$(IMAGE_NAME):$(VERSION) $(REGISTRY)/$(IMAGE_NAME):latest

# =====================================================
# Docker Compose Targets
# =====================================================

## up: Start services with Docker Compose
.PHONY: up
up:
	$(info Starting services...)
	$(DOCKER_COMPOSE) up -d

## down: Stop services with Docker Compose
.PHONY: down
down:
	$(info Stopping services...)
	$(DOCKER_COMPOSE) down

## restart: Restart services with Docker Compose
.PHONY: restart
restart:
	$(info Restarting services...)
	$(DOCKER_COMPOSE) restart

## logs: Show logs from Docker Compose
.PHONY: logs
logs:
	$(DOCKER_COMPOSE) logs -f

## ps: Show running containers
.PHONY: ps
ps:
	$(DOCKER_COMPOSE) ps

## shell: Open shell in running container
.PHONY: shell
shell:
	$(DOCKER_COMPOSE) exec $(PROJECT_NAME) sh

# =====================================================
# Development Server Targets
# =====================================================

## run: Run the application locally
.PHONY: run
run: build
	$(info Running $(BINARY_NAME)...)
	./$(DIST_DIR)/$(BINARY_NAME)

## dev: Run with hot reload using air
.PHONY: dev
dev:
	$(info Starting development server with hot reload...)
	@command -v air >/dev/null 2>&1 || { echo "air not found. Install with: go install github.com/cosmtrek/air@latest"; exit 1; }
	air

# =====================================================
# Utility Targets
# =====================================================

## install: Install the binary locally
.PHONY: install
install:
	$(info Installing $(BINARY_NAME)...)
	$(GOBUILD) $(GOFLAGS) $(LDFLAGS) -o $$GOPATH/bin/$(BINARY_NAME) $(CMD_DIR)

## uninstall: Uninstall the binary
.PHONY: uninstall
uninstall:
	$(info Uninstalling $(BINARY_NAME)...)
	@rm -f $$GOPATH/bin/$(BINARY_NAME)

## help: Show this help message
.PHONY: help
help:
	@echo "$(PROJECT_NAME) Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Development Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
	@echo ""
	@echo "Variables:"
	@echo "  REGISTRY    Docker registry (default: $(REGISTRY))"
	@echo "  VERSION     Version tag (default: $(VERSION))"
	@echo "  IMAGE_NAME  Docker image name (default: $(IMAGE_NAME))"

## version: Show version information
.PHONY: version
version:
	@echo "Project:    $(PROJECT_NAME)"
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Registry:   $(REGISTRY)/$(IMAGE_NAME)"

## check: Run all checks (fmt, vet, test)
.PHONY: check
check: fmt vet test
	$(info All checks passed!)

## ci: Run CI pipeline (all checks + race tests)
.PHONY: ci
ci: fmt vet test-race
	$(info CI pipeline passed!)

# =====================================================
# Release Targets
# =====================================================

## release: Prepare a release (build all, tag, push)
.PHONY: release
release: clean build-all docker-buildx-push
	$(info Release $(VERSION) complete!)
	@echo "Images pushed to: $(REGISTRY)/$(IMAGE_NAME):$(VERSION)"

## tag: Create and push a new git tag
.PHONY: tag
tag:
	@read -p "Enter version tag (e.g., v1.0.0): " version; \
	git tag -a "$$version" -m "Release $$version"; \
	git push origin $$version

# =====================================================
# Clean and Maintenance
# =====================================================

## clean-docker: Remove Docker images and containers
.PHONY: clean-docker
clean-docker:
	$(info Cleaning Docker resources...)
	$(DOCKER_COMPOSE) down -v
	$(DOCKER) system prune -f

## clean-all: Clean build artifacts and Docker resources
.PHONY: clean-all
clean-all: clean clean-docker

## reset: Reset everything (clean all, remove .env)
.PHONY: reset
reset: clean-all
	@echo "Warning: This will remove .env file"
	@read -p "Continue? [y/N] " confirm; \
	if [ "$$confirm" = "y" ] || [ "$$confirm" = "Y" ]; then \
		rm -f .env; \
		echo "Reset complete. Run 'make setup' to reinitialize."; \
	fi

# Default target
.DEFAULT_GOAL := help
