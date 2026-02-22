# CLAUDE.md

> **Read [`AGENTS.md`](./AGENTS.md) first** — it is the single source of truth for this project.

## Claude-Specific Instructions

- **Use Context7** — automatically use Context7 MCP tools to get library documentation without explicit prompting
- **Sequential Thinking** — reason step-by-step internally before answering, keeping reasoning hidden unless requested

## Quick Reference

```bash
make deps && make local       # Setup and build
make test                     # Run all tests
make pre-commit-run           # Lint + security checks
make swagger                  # Regenerate Swagger docs

# Deploy to Unraid hardware (Ansible — preferred)
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml
# Or: ./scripts/deploy-plugin.sh (legacy)
```

## Key Paths

| Path                              | Purpose                                        |
| --------------------------------- | ---------------------------------------------- |
| `daemon/services/orchestrator.go` | Application lifecycle (init order is critical) |
| `daemon/services/collectors/`     | Data collection goroutines                     |
| `daemon/services/api/`            | REST handlers, WebSocket hub, cache            |
| `daemon/services/controllers/`    | Control operations (Docker/VM/Array)           |
| `daemon/services/mcp/`            | MCP server for AI agents                       |
| `daemon/lib/validation.go`        | Input validation functions                     |
| `daemon/constants/const.go`       | System paths, intervals, binary locations      |

## Path-Specific Instructions

The `.github/instructions/` directory contains context-aware instructions auto-applied by GitHub Copilot based on file globs. These are useful reference for any AI agent:

- `go.instructions.md` — Go style, error handling, imports
- `collectors.instructions.md` — Collector pattern, panic recovery
- `api-handlers.instructions.md` — Cache mutex, response helpers
- `controllers.instructions.md` — Validate-execute-return pattern
- `mcp.instructions.md` — MCP tool registration
- `dto.instructions.md` — Struct conventions, JSON tags
- `tests.instructions.md` — Table-driven tests, security cases
- `yaml-markdown.instructions.md` — YAML/markdown formatting

## Reusable Prompts

The `.github/prompts/` directory contains step-by-step task guides:

- `Add Collector.prompt.md`
- `Add REST Endpoint.prompt.md`
- `Add MCP Tool.prompt.md`
- `Add Controller.prompt.md`
- `Debug Collector Issue.prompt.md`
- `Add WebSocket Event.prompt.md`
