---
applyTo: "daemon/services/collectors/**/*.go"
---

# Collector Instructions

Reference: [`AGENTS.md`](../../AGENTS.md) for full project context.

## Pattern

Every collector is an independent goroutine that:

1. Runs at a fixed interval (defined in `daemon/constants/const.go`)
2. Collects data from system sources (native APIs or shell commands)
3. Publishes results to the PubSub event bus via `ctx.Hub.Pub(data, topic)`

## Panic Recovery (Required)

**All collector loops MUST wrap work in defer/recover:**

```go
func (c *Collector) Start(ctx context.Context, interval time.Duration) {
    func() {
        defer func() {
            if r := recover(); r != nil {
                logger.Error("Collector PANIC on startup: %v", r)
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
                        logger.Error("Collector PANIC in loop: %v", r)
                    }
                }()
                c.Collect()
            }()
        }
    }
}
```

## Registration

Register new collectors in `collector_manager.go` `RegisterAllCollectors()`.

## Intervals

Intervals are optimized for power efficiency. Do not lower intervals without considering CPU/power impact. See the collector table in `AGENTS.md` for current values.

## Native APIs Preferred

Use native Go libraries over shell commands where possible:

- Docker: `github.com/moby/moby/client`
- VMs: `github.com/digitalocean/go-libvirt`
- System: Direct `/proc`, `/sys` access

## Hardware Compatibility

Different hardware produces different output formats. Add fallback logic when parsing command output (GPU metrics, disk controllers, UPS tools, DMI data).
