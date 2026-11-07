---
applyTo: "daemon/services/api/**/*.go"
---

# API Server Instructions

Reference: [`AGENTS.md`](../../AGENTS.md) for full project context.

## Cache Access

Read collected monitoring values through the `CacheStore` getters:

```go
data := s.GetSystemCache()
respondJSON(w, http.StatusOK, data)
```

- Collector cache values use atomic pointer loads and stores, not a server-wide cache mutex.
- Install replacement values through the existing subscription and cache-update paths.
- Keep separate synchronization for other shared mutable state.
- See [the shared-telemetry decision](../../docs/adr/0001-shared-telemetry-cache-and-best-effort-events.md).

## Response Helpers

- Use `respondJSON()` for all JSON responses
- Control endpoints return `dto.Response` with status and message

## Route Registration

Register new routes in `server.go` `setupRoutes()`. Follow existing patterns for URL structure (`/api/v1/{resource}`).

## Subscriptions

Event subscriptions in `subscribeToEvents()`:

1. Add topic to `Hub.Sub()` call
2. Add case in the switch statement to update the appropriate cache field
3. Broadcast to WebSocket clients

**Critical:** Subscriptions must be created BEFORE collectors start publishing.

## WebSocket

- `websocket.go` manages the WebSocket hub with client registration
- Ping/pong health checks maintain connection liveness
- All events received from PubSub are broadcast to connected clients

## Swagger

Run `make swagger` after modifying endpoints or adding annotations.
