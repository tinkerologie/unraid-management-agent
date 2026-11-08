---
applyTo: "daemon/services/collectors/**/*.go"
---

# Collector Instructions

Reference: [`AGENTS.md`](../../AGENTS.md) for full project context.

## Pattern

Manual mode uses fixed intervals and existing change triggers. Dynamic mode
uses the shared scheduler policy for demand, floors, and event triggers.
Both publish results on the event bus.

## Resource lifetime

The collector manager runs one generation per collector. Resource-owning
collectors implement `Open(ctx)`, `Refresh(ctx)`, and `Close()`. Keep
construction free of resources that require cleanup. `Open` starts
context-bound workers. `Refresh` collects and publishes once. `Close` releases
resources and joins workers after cancellation, including after a failed or
panicking `Open`. The manager serializes refreshes, recovers and logs panics,
and joins event sources before calling `Close`. Use a function adapter for a
collector with no resource setup or teardown.

Disabling or restarting a collector requests cancellation. A replacement
waits until the previous generation closes, and shutdown reports generations
still stopping at its deadline. Preserve the existing status values and
cached snapshots during a restart.

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
