VERSION := $(shell cat VERSION)
BINARY := unraid-management-agent
BUILD_DIR := build
PLUGIN_DIR := meta/plugin
DATE := $(shell date '+%Y.%m.%d')
HASH := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")

.PHONY: all local release package clean test test-coverage deps swagger pre-commit-install pre-commit-run lint security-check

all: test local

deps:
	@echo "Installing Go dependencies..."
	go mod download
	go mod tidy

# Generate Swagger documentation
swagger:
	@echo "Generating Swagger documentation..."
	@if command -v swag >/dev/null 2>&1; then \
		swag init -g daemon/docs/swagger.go -o daemon/docs --parseDependency --parseInternal; \
	else \
		echo "swag not found, installing..."; \
		go install github.com/swaggo/swag/cmd/swag@latest; \
		swag init -g daemon/docs/swagger.go -o daemon/docs --parseDependency --parseInternal; \
	fi

local: deps swagger
	@echo "Building for local architecture..."
	go build -ldflags "-X main.Version=$(VERSION)-$(DATE)-$(HASH)" -o $(BINARY)

release: deps swagger
	@echo "Building for Linux/amd64..."
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY)

package: release
	@echo "Creating plugin package..."
	mkdir -p $(BUILD_DIR)/usr/local/emhttp/plugins/$(BINARY)
	cp $(BUILD_DIR)/$(BINARY) $(BUILD_DIR)/usr/local/emhttp/plugins/$(BINARY)/
	cp VERSION $(BUILD_DIR)/usr/local/emhttp/plugins/$(BINARY)/
	COPYFILE_DISABLE=1 cp -r $(PLUGIN_DIR)/. $(BUILD_DIR)/usr/local/emhttp/plugins/$(BINARY)/
	find $(BUILD_DIR)/usr/local/emhttp/plugins/$(BINARY) -name '._*' -delete 2>/dev/null || true
	cd $(BUILD_DIR) && COPYFILE_DISABLE=1 tar -czf $(BINARY)-$(VERSION).tgz usr/
	@echo "Package created: $(BUILD_DIR)/$(BINARY)-$(VERSION).tgz"

test:
	@echo "Running tests..."
	go test -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR) $(BINARY) coverage.out coverage.html

install: local
	@echo "Installing binary to ~/bin..."
	mkdir -p ~/bin
	cp $(BINARY) ~/bin/

deploy: package
	@echo "Deploying to Unraid..."
	./meta/scripts/deploy
# Pre-commit hooks
pre-commit-install:
	@echo "Installing pre-commit hooks..."
	@if command -v pre-commit >/dev/null 2>&1; then \
		pre-commit install; \
		pre-commit install --hook-type commit-msg; \
		echo "✓ Pre-commit hooks installed successfully"; \
	else \
		echo "❌ pre-commit not found. Install with: pip install pre-commit"; \
		exit 1; \
	fi

pre-commit-run:
	@echo "Running pre-commit checks on all files..."
	@if command -v pre-commit >/dev/null 2>&1; then \
		pre-commit run --all-files; \
	else \
		echo "❌ pre-commit not found. Install with: pip install pre-commit"; \
		exit 1; \
	fi

# Linting
lint:
	@echo "Running golangci-lint..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --config .golangci.yml --max-issues-per-linter 0 --max-same-issues 0 ./...; \
	else \
		echo "❌ golangci-lint not found. Install from: https://golangci-lint.run/usage/install/"; \
		exit 1; \
	fi

# Security checks
security-check:
	@echo "Running security checks..."
	@echo "→ Running gosec..."
	@if command -v gosec >/dev/null 2>&1; then \
		gosec -fmt=text -exclude-dir=vendor -exclude-dir=tests -exclude=G115,G304,G301,G306,G703,G204,G117 -severity=medium -confidence=medium ./...; \
	else \
		echo "⚠️  gosec not found. Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"; \
	fi
	@echo "→ Running govulncheck..."
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "⚠️  govulncheck not found. Install with: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
	fi
	@echo "→ Running go mod verify..."
	@go mod verify
