# SGCSF Makefile

.PHONY: all build test clean run-server run-examples lint fmt deps check

# Build variables
APP_NAME := sgcsf
VERSION := 1.0.0
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD)

# Go build flags
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"
GOFLAGS := -mod=readonly

# Default target
all: deps fmt lint test build

# Install dependencies
deps:
	@echo "📦 Installing dependencies..."
	go mod download
	go mod tidy

# Format code
fmt:
	@echo "🎨 Formatting code..."
	go fmt ./...

# Lint code
lint:
	@echo "🔍 Linting code..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "⚠️  golangci-lint not installed, skipping..."; \
		go vet ./...; \
	fi

# Run tests
test:
	@echo "🧪 Running tests..."
	go test ./... -v

# Run tests with coverage
test-coverage:
	@echo "🧪 Running tests with coverage..."
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "📊 Coverage report generated: coverage.html"

# Run benchmarks
benchmark:
	@echo "⚡ Running benchmarks..."
	go test ./test -bench=. -benchmem

# Build all binaries
build: build-server build-gateways build-examples

# Build server
build-server:
	@echo "🔨 Building SGCSF server..."
	go build $(GOFLAGS) $(LDFLAGS) -o bin/sgcsf-server cmd/server/main.go

# Build gateways
build-gateways:
	@echo "🔨 Building SGCSF gateways..."
	go build $(GOFLAGS) $(LDFLAGS) -o bin/ground-gateway cmd/ground-gateway/main.go
	go build $(GOFLAGS) $(LDFLAGS) -o bin/satellite-gateway cmd/satellite-gateway/main.go

# Build examples
build-examples:
	@echo "🔨 Building examples..."
	go build $(GOFLAGS) -o bin/sensor-publisher cmd/examples/sensor-publisher/main.go
	go build $(GOFLAGS) -o bin/ground-subscriber cmd/examples/ground-subscriber/main.go
	go build $(GOFLAGS) -o bin/sync-communication cmd/examples/sync-communication/main.go
	go build $(GOFLAGS) -o bin/file-transfer cmd/examples/file-transfer/main.go
	go build $(GOFLAGS) -o bin/http-adapter-demo cmd/examples/http-adapter-demo/main.go

# Run server
run-server:
	@echo "🚀 Starting SGCSF server..."
	go run cmd/server/main.go

# Run examples
run-sensor-publisher:
	@echo "📡 Starting sensor publisher..."
	go run cmd/examples/sensor-publisher/main.go

run-ground-subscriber:
	@echo "🌍 Starting ground subscriber..."
	go run cmd/examples/ground-subscriber/main.go

run-sync-demo:
	@echo "🔄 Starting sync communication demo..."
	go run cmd/examples/sync-communication/main.go

run-file-transfer:
	@echo "📁 Starting file transfer demo..."
	go run cmd/examples/file-transfer/main.go

run-http-demo:
	@echo "🌐 Starting HTTP adapter demo..."
	go run cmd/examples/http-adapter-demo/main.go

# Development setup
dev-setup: deps
	@echo "🛠️  Setting up development environment..."
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "📥 Installing golangci-lint..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.54.2; \
	fi

# Demo: Run complete example scenario
demo: demo-clean
	@echo "🎬 Starting SGCSF complete demo..."
	@echo "Starting server in background..."
	@go run cmd/server/main.go > logs/server.log 2>&1 & echo $$! > .server.pid
	@sleep 3
	@echo "Starting sensor publisher in background..."
	@go run cmd/examples/sensor-publisher/main.go > logs/publisher.log 2>&1 & echo $$! > .publisher.pid
	@sleep 2
	@echo "Starting ground subscriber in background..."
	@go run cmd/examples/ground-subscriber/main.go > logs/subscriber.log 2>&1 & echo $$! > .subscriber.pid
	@sleep 2
	@echo "Running sync communication demo..."
	@go run cmd/examples/sync-communication/main.go > logs/sync.log 2>&1
	@sleep 5
	@echo "Running file transfer demo..."
	@go run cmd/examples/file-transfer/main.go > logs/transfer.log 2>&1
	@sleep 5
	@echo "Demo completed! Check logs/ directory for output."
	@echo "Run 'make demo-stop' to stop background processes."

# Stop demo processes
demo-stop:
	@echo "🛑 Stopping demo processes..."
	@if [ -f .server.pid ]; then kill `cat .server.pid` 2>/dev/null || true; rm .server.pid; fi
	@if [ -f .publisher.pid ]; then kill `cat .publisher.pid` 2>/dev/null || true; rm .publisher.pid; fi
	@if [ -f .subscriber.pid ]; then kill `cat .subscriber.pid` 2>/dev/null || true; rm .subscriber.pid; fi
	@echo "✅ Demo processes stopped."

# Clean demo logs and processes
demo-clean: demo-stop
	@echo "🧹 Cleaning demo artifacts..."
	@mkdir -p logs
	@rm -f logs/*.log
	@rm -f .*.pid

# Quality checks
check: fmt lint test
	@echo "✅ All quality checks passed!"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -f .*.pid

# Docker build
docker-build:
	@echo "🐳 Building Docker image..."
	docker build -t sgcsf:$(VERSION) .
	docker tag sgcsf:$(VERSION) sgcsf:latest

# Docker run
docker-run:
	@echo "🐳 Running SGCSF in Docker..."
	docker run -p 7000:7000 sgcsf:latest

# Release build
release: clean
	@echo "📦 Building release binaries..."
	@mkdir -p release
	# Linux
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o release/sgcsf-server-linux-amd64 cmd/server/main.go
	GOOS=linux GOARCH=arm64 go build $(GOFLAGS) $(LDFLAGS) -o release/sgcsf-server-linux-arm64 cmd/server/main.go
	# Windows
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o release/sgcsf-server-windows-amd64.exe cmd/server/main.go
	# macOS
	GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o release/sgcsf-server-darwin-amd64 cmd/server/main.go
	GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) $(LDFLAGS) -o release/sgcsf-server-darwin-arm64 cmd/server/main.go
	@echo "📦 Release binaries built in release/ directory"

# Help
help:
	@echo "SGCSF Makefile Commands:"
	@echo ""
	@echo "Development:"
	@echo "  deps            Install dependencies"
	@echo "  dev-setup       Setup development environment"
	@echo "  fmt             Format code"
	@echo "  lint            Lint code"
	@echo "  test            Run tests"
	@echo "  test-coverage   Run tests with coverage"
	@echo "  benchmark       Run benchmarks"
	@echo "  check           Run all quality checks"
	@echo ""
	@echo "Building:"
	@echo "  build           Build all binaries"
	@echo "  build-server    Build server binary"
	@echo "  build-examples  Build example binaries"
	@echo "  release         Build release binaries for all platforms"
	@echo ""
	@echo "Running:"
	@echo "  run-server              Run SGCSF server (legacy)"
	@echo "  run-ground-gateway      Run Ground Gateway"
	@echo "  run-satellite-gateway   Run Satellite Gateway"
	@echo "  run-sensor-publisher    Run sensor publisher example"
	@echo "  run-ground-subscriber   Run ground subscriber example"
	@echo "  run-sync-demo          Run sync communication demo"
	@echo "  run-file-transfer      Run file transfer demo"
	@echo "  run-http-demo          Run HTTP adapter demo"
	@echo ""
	@echo "Demo:"
	@echo "  demo            Run complete demo scenario"
	@echo "  demo-stop       Stop demo processes"
	@echo "  demo-clean      Clean demo artifacts"
	@echo ""
	@echo "Docker:"
	@echo "  docker-build    Build Docker image"
	@echo "  docker-run      Run in Docker"
	@echo ""
	@echo "Cleanup:"
	@echo "  clean           Clean build artifacts"