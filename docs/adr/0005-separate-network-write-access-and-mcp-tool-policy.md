# Separate network write access from MCP tool policy

Existing decision, recorded 2026-11-07.

Without an API token, clients can monitor the agent. The agent blocks network control unless you explicitly permit unauthenticated writes.
MCP read-only mode and per-tool policies add restrictions to MCP tool use. REST uses the separate network access rules.
Local MCP over standard input/output operates outside the HTTP write lock. The global MCP read-only setting still applies to its tools.

## Consequences

REST still uses its network access rules when you disable MCP writes.
Check the authentication and write-access rules for the transport. Check the policy of the tool before you call it.

## Evidence

- [Configuration and network write-lock calculation](../../daemon/domain/config.go)
- [HTTP write-lock enforcement](../../daemon/services/api/middleware.go)
- [MCP global and network policy](../../daemon/services/mcp/server.go)
- [Transport initialization](../../daemon/services/orchestrator.go)
