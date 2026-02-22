---
description: Step-by-step guide for adding a new WebSocket broadcast event
tools: ["editor", "terminal"]
---

# Add a New WebSocket Event

Follow these steps to add a new real-time WebSocket broadcast event.

## Step 1: Define the Event Topic

Add a topic constant in `daemon/constants/const.go`:

```go
const MyFeatureUpdateTopic = "my_feature_update"
```

## Step 2: Ensure Collector Publishes

The collector must publish data to the event bus:

```go
c.ctx.Hub.Pub(data, constants.MyFeatureUpdateTopic)
```

If no collector exists yet, follow the "Add Collector" prompt.

## Step 3: Subscribe in API Server

In `daemon/services/api/server.go` `subscribeToEvents()`:

1. Add the topic to the `Hub.Sub()` call:

```go
sub := s.ctx.Hub.Sub(
    "system_update",
    "my_feature_update",  // Add here
    // ... other topics
)
```

2. Add a case in the switch statement:

```go
case dto.MyFeatureInfo:
    s.cacheMutex.Lock()
    s.myFeatureCache = v
    s.cacheMutex.Unlock()
    s.broadcastToClients("my_feature_update", v)
```

## Step 4: WebSocket Event Format

Events are broadcast to clients as `dto.WSEvent`:

```json
{
    "event": "my_feature_update",
    "timestamp": "2025-01-01T00:00:00Z",
    "data": { /* DTO fields */ }
}
```

## Step 5: Test

- Verify the event is broadcast by connecting a WebSocket client
- Add tests if there's specific transformation logic

## Step 6: Document

- Update WebSocket events documentation in `docs/api/websocket-events.md`
- Update `CHANGELOG.md`
