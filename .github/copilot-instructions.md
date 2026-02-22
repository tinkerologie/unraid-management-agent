# Copilot Instructions

> **Read [`../AGENTS.md`](../AGENTS.md) first** — it is the single source of truth for this project.

Go-based Unraid plugin exposing system monitoring/control via REST API, WebSockets, and MCP. **Language:** Go 1.26, **Target:** Linux/amd64 (Unraid OS). Third-party community plugin.

## Copilot Workflow

- Follow Go best practices: idiomatic style, `fmt.Errorf("context: %w", err)`, context propagation
- Code must pass `golangci-lint` and `go vet`
- Run `make pre-commit-run` before committing
- Run `make swagger` after modifying API endpoints
- Follow **Conventional Commits**: `feat(scope):`, `fix(scope):`, `docs(scope):`

## Path-Specific Instructions

These files in `.github/instructions/` are auto-applied via `applyTo` globs:

| File                            | Applies To                            |
| ------------------------------- | ------------------------------------- |
| `go.instructions.md`            | `**/*.go`                             |
| `collectors.instructions.md`    | `daemon/services/collectors/**/*.go`  |
| `api-handlers.instructions.md`  | `daemon/services/api/**/*.go`         |
| `controllers.instructions.md`   | `daemon/services/controllers/**/*.go` |
| `mcp.instructions.md`           | `daemon/services/mcp/**/*.go`         |
| `dto.instructions.md`           | `daemon/dto/**/*.go`                  |
| `tests.instructions.md`         | `**/*_test.go`                        |
| `yaml-markdown.instructions.md` | `**/*.{yaml,yml,md}`                  |

## Reusable Prompts

Task-oriented step-by-step guides in `.github/prompts/`:

- `Add Collector.prompt.md` — Adding a new data collector
- `Add REST Endpoint.prompt.md` — Adding a REST API endpoint
- `Add MCP Tool.prompt.md` — Adding an MCP tool for AI agents
- `Add Controller.prompt.md` — Adding a control operation
- `Debug Collector Issue.prompt.md` — Debugging collector failures
- `Add WebSocket Event.prompt.md` — Adding a WebSocket broadcast event

## Quick Commands

```bash
make deps && make local       # Setup and build
make test                     # Run all tests
make pre-commit-run           # Lint + security checks
make swagger                  # Regenerate Swagger docs

# Deploy to Unraid hardware (Ansible — preferred)
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml
# Or: ./scripts/deploy-plugin.sh (legacy)
```
