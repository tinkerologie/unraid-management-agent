# Keep demand-aware and manually scheduled collection modes

Existing decision, recorded 2026-11-07.

Dynamic mode lets the Management Agent choose collector timing from resource policies, consumer demand,
event sources, and alert requirements instead of polling every enabled subsystem at a fixed rate.
Demand includes subscribed WebSocket clients, connected MQTT consumers for their published collectors,
and recent REST or MCP reads; alert floors keep required monitoring active independently of client demand.
Manual mode retains configured intervals, and changing those intervals while Dynamic mode manages them is rejected.

## Consequences

An unfiltered WebSocket client counts as a consumer of all collector topics, so a client that wants reduced
collection work must filter its subscriptions. Dynamic mode reduces unnecessary collection work, but an idle
collector is not the same as a disabled collector, and hardware gating remains a separate eligibility decision.

## Evidence

- [Collection modes](../../daemon/domain/context.go)
- [Demand accounting](../../daemon/services/scheduler/demand.go)
- [Dynamic collector lifecycle](../../daemon/services/collector_manager_dynamic.go)
- [Alert floors](../../daemon/services/orchestrator.go)
