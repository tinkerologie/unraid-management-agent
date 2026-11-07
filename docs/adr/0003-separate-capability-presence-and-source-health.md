# Separate capability presence from source health

Existing decision, recorded 2026-11-07.

The Management Agent reports capability presence separately from the health of the sources supplying
monitoring data, rather than treating every empty response as missing hardware or a broken server.
Capability probes distinguish present, absent, and unknown; hardware policy leaves an existing decision
unchanged on an unknown result and requires repeated absence before stopping a previously seen gated collector.
Source health separately distinguishes healthy, degraded, unavailable, and intentionally disabled sources,
with an intentionally disabled service excluded from degraded-source counts.

## Consequences

A failed probe must not be converted into confirmed hardware absence. Consumers can distinguish an
intentionally disabled service from an unavailable source and can receive source-state transitions without
repeated logs for an unchanged condition.

## Evidence

- [Capability and source-state definitions](../../daemon/dto/source_status.go)
- [Source-health registry](../../daemon/platform/registry.go)
- [Hardware eligibility policy](../../daemon/services/collector_auto_policy.go)
