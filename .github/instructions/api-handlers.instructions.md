---
applyTo: "daemon/services/api/**/*.go"
---

# API Server Instructions

Reference: [`AGENTS.md`](../../AGENTS.md) for full project context.

## Cache Access

**Always use mutex locks for cache reads in handlers:**

```go
s.cacheMutex.RLock()
data := s.someCache
s.cacheMutex.RUnlock()
respondJSON(w, http.StatusOK, data)
```

- `RLock`/`RUnlock` for GET handlers (read-only)
- `Lock`/`Unlock` for cache writes (in subscription handler)

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
