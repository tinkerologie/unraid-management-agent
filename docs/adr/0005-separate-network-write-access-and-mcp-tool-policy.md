# Separate network write access from MCP tool policy

Existing decision, recorded 2026-11-07.

Without an API token, network monitoring remains available but network control operations are locked unless
unauthenticated writes are explicitly allowed. MCP read-only mode and per-tool policies are additional
restrictions on MCP tool use, rather than a substitute for network authentication or a REST-wide read-only
setting. Local MCP over standard input/output does not acquire the HTTP network write lock, while the
global MCP read-only setting still applies to its tools.

## Consequences

Disabling MCP writes does not establish a read-only REST deployment. A client must evaluate its transport's
authentication and write-access rules as well as the effective policy of the tool it wants to use;
the local and network transports deliberately have different trust boundaries.

## Evidence

- [Configuration and network write-lock calculation](../../daemon/domain/config.go)
- [HTTP write-lock enforcement](../../daemon/services/api/middleware.go)
- [MCP global and network policy](../../daemon/services/mcp/server.go)
- [Transport initialization](../../daemon/services/orchestrator.go)
