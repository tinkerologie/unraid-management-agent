VERSION := $(shell cat VERSION)
BINARY := unraid-management-agent
BUILD_DIR := build
PLUGIN_DIR := meta/plugin
DATE := $(shell date '+%Y.%m.%d')
HASH := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")

.PHONY: all local release package clean test test-coverage deps

all: test local

deps:
	@echo "Installing Go dependencies..."
	go mod download
	go mod tidy

local: deps
	@echo "Building for local architecture..."
	go build -ldflags "-X main.Version=$(VERSION)-$(DATE)-$(HASH)" -o $(BINARY)

release: deps
	@echo "Building for Linux/amd64..."
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY)

package: release
	@echo "Creating plugin package..."
	mkdir -p $(BUILD_DIR)/usr/local/emhttp/plugins/$(BINARY)
	cp $(BUILD_DIR)/$(BINARY) $(BUILD_DIR)/usr/local/emhttp/plugins/$(BINARY)/
	cp VERSION $(BUILD_DIR)/usr/local/emhttp/plugins/$(BINARY)/
	cp -r $(PLUGIN_DIR)/* $(BUILD_DIR)/usr/local/emhttp/plugins/$(BINARY)/
	cd $(BUILD_DIR) && tar --no-xattrs -czf $(BINARY)-$(VERSION).tgz usr/
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
