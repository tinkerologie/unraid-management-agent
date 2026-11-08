# Share collected telemetry through a cache and best-effort events

Existing decision, recorded 2026-11-07.

Collectors publish monitoring data to the internal event bus. The API server saves the latest values in a shared cache.
REST, MCP, and other monitoring features read from that cache.
The event bus drops events for subscribers with full buffers. Publishers continue, and the event bus gives no durable event log.
At startup, the agent waits for the API subscriptions to become ready. It then starts the collectors.

## Consequences

Consumers must tolerate missed events and refresh state when necessary. The cache uses atomic pointer loads and stores.
Cache-only startup lets local MCP operate without the HTTP server.

## Evidence

- [Event bus and its slow-subscriber policy](../../daemon/domain/eventbus.go)
- [Shared cache](../../daemon/services/api/cache_store.go)
- [HTTP and cache-only startup ordering](../../daemon/services/orchestrator.go)
