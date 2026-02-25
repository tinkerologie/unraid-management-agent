---
description: Step-by-step guide for adding a new data collector
tools: ["editor", "terminal"]
---

# Add a New Collector

Follow these steps to add a new data collector to the Unraid Management Agent.

## Step 1: Define the DTO

Create or update a struct in `daemon/dto/` for the data this collector will produce.

- Use `json:"field_name"` tags on all exported fields
- Use `json:"field_name,omitempty"` for optional fields

## Step 2: Create the Collector

Create a new file in `daemon/services/collectors/` (e.g., `myfeature.go`).

Follow the pattern from `system.go`:

```go
package collectors

import (
    "context"
    "time"

    "unraid-management-agent/daemon/domain"
    "unraid-management-agent/daemon/dto"
    "unraid-management-agent/daemon/logger"
)

type MyFeatureCollector struct {
    ctx *domain.Context
}

func NewMyFeatureCollector(ctx *domain.Context) *MyFeatureCollector {
    return &MyFeatureCollector{ctx: ctx}
}

func (c *MyFeatureCollector) Collect() {
    // Collect data from system sources
    data := dto.MyFeatureInfo{
        // populate fields
    }
    c.ctx.Hub.Pub(data, "my_feature_update")
}

func (c *MyFeatureCollector) Start(ctx context.Context, interval time.Duration) {
    func() {
        defer func() {
            if r := recover(); r != nil {
                logger.Error("MyFeature collector PANIC on startup: %v", r)
            }
        }()
        c.Collect()
    }()

    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            func() {
                defer func() {
                    if r := recover(); r != nil {
                        logger.Error("MyFeature collector PANIC in loop: %v", r)
                    }
                }()
                c.Collect()
            }()
        }
    }
}
```

## Step 3: Define the Interval

Add a constant in `daemon/constants/const.go` for the collection interval. Choose an appropriate interval considering CPU/power impact.

## Step 4: Register the Collector

In `daemon/services/collector_manager.go` `RegisterAllCollectors()`, register the new collector.

## Step 5: Add API Subscription

In `daemon/services/api/server.go` `subscribeToEvents()`:

1. Add the topic string to the `Hub.Sub()` call
2. Add a case in the switch statement to update the cache
3. Broadcast to WebSocket clients

## Step 6: Add Cache and Handler

In `daemon/services/api/`:

1. Add a cache field to the `Server` struct in `server.go`
2. Add a handler in `handlers.go`:

```go
func (s *Server) handleMyFeature(w http.ResponseWriter, _ *http.Request) {
    s.cacheMutex.RLock()
    data := s.myFeatureCache
    s.cacheMutex.RUnlock()
    respondJSON(w, http.StatusOK, data)
}
```

1. Register the route in `setupRoutes()`

## Step 7: Test

- Add table-driven tests in a `*_test.go` file alongside the collector
- Include security test cases if the collector accepts any user input
- Run `make test` to verify

## Step 8: Document

- Update `CHANGELOG.md`
- Update Swagger annotations and run `make swagger`
