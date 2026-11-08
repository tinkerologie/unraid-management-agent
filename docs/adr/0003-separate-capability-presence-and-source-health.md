# Separate capability presence from source health

Existing decision, recorded 2026-11-07.

The agent reports capability presence and source health separately. An empty response can have several causes.
Capability probes report present, absent, or unknown. An unknown result leaves the hardware eligibility decision in effect.
The policy stops a previously detected hardware-gated collector only after repeated absent results.
Source health distinguishes healthy, degraded, unavailable, and intentionally disabled sources.
Degraded-source counts exclude intentionally disabled services.

## Consequences

A failed probe must not be converted into confirmed hardware absence. Consumers can distinguish an
intentionally disabled service from an unavailable source and can receive source-state transitions without
repeated logs for an unchanged condition.

## Evidence

- [Capability and source-state definitions](../../daemon/dto/source_status.go)
- [Source-health registry](../../daemon/platform/registry.go)
- [Hardware eligibility policy](../../daemon/services/collector_auto_policy.go)
