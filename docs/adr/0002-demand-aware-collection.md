# Keep demand-aware and manually scheduled collection modes

Existing decision, recorded 2026-11-07.

In Dynamic mode, the agent uses resource policies, consumer demand, change events, and alert requirements to set collector timing.
Subscribed WebSocket clients and recent REST or MCP reads add demand. Connected MQTT consumers add demand for the collectors that publish their data.
Alert floors keep the necessary monitoring active without client demand.
Manual mode uses the configured intervals. The agent rejects interval changes in Dynamic mode.

## Consequences

An unfiltered WebSocket client counts as a consumer of all collector topics, so a client that wants reduced
collection work must filter its subscriptions. Dynamic mode reduces unnecessary collection work, but an idle
collector is not the same as a disabled collector, and hardware gating remains a separate eligibility decision.

## Evidence

- [Collection modes](../../daemon/domain/context.go)
- [Demand accounting](../../daemon/services/scheduler/demand.go)
- [Collector lifecycle](../../daemon/services/collector_manager.go)
- [Alert floors](../../daemon/services/orchestrator.go)
