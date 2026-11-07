# Share collected telemetry through a cache and best-effort events

Existing decision, recorded 2026-11-07.

Collectors publish monitoring data to the internal event bus, and the API server installs the latest values in a
shared cache consumed by REST, MCP, and other monitoring facilities. The event bus drops deliveries to full
subscriber buffers rather than blocking publishers, so it is a live monitoring feed rather than a durable event
log. Startup waits for the API subscriptions' readiness signal before starting collectors, preventing initial
publications from being lost merely because subscribers have not been installed.

## Consequences

Consumers must tolerate missed events and refresh state when necessary. Cache values use atomic pointer
loads and stores; the old per-server cache mutex and fixed startup sleep are not the current design.
Cache-only startup also supports local MCP operation without starting the HTTP server.

## Evidence

- [Event bus and its slow-subscriber policy](../../daemon/domain/eventbus.go)
- [Shared cache](../../daemon/services/api/cache_store.go)
- [HTTP and cache-only startup ordering](../../daemon/services/orchestrator.go)
