#!/usr/bin/env bash
# Dev container one-time setup — runs once when the container is first created.
# Lifecycle: onCreateCommand → updateContentCommand → postCreateCommand
set -euo pipefail

echo "==> Downloading Go modules..."
go mod download

echo "==> Installing Go development tools..."
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
go install github.com/swaggo/swag/cmd/swag@latest
go install golang.org/x/tools/cmd/goimports@latest

echo "==> Installing Python tools..."
pip install --no-cache-dir pre-commit ansible ansible-lint

echo "==> Development environment ready!"
