# Unraid Management Agent

> **Read [`AGENTS.md`](./AGENTS.md) first** — it is the single source of truth for this project.

## Gemini-Specific Notes

- Follow Go best practices: idiomatic style, proper error handling with wrapped errors
- Code must pass `golangci-lint` and `go vet`
- Run `make pre-commit-run` before committing

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

## Documentation References

- **API Reference:** `docs/api/`
- **MCP Integration:** `docs/integrations/mcp.md`
- **WebSocket Events:** `docs/api/websocket-events.md`
- **Configuration:** `docs/guides/configuration.md`
