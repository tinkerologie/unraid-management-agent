# Homelab Hardening Design

Date: 2026-03-08

## Goal

Make the plugin defensibly GA-ready for LAN-only homelab users by fixing the confirmed WebSocket concurrency bug and reaching a clean state for `golangci-lint`, `gosec`, and `govulncheck` without breaking Home Assistant integration.

## Context

This design assumes the plugin is used on a trusted local network and that the Home Assistant Unraid Management Agent integration depends on the current REST, WebSocket, and MQTT surfaces. The goal is not to add internet-facing auth or redesign the product. The goal is to harden the current implementation just enough to remove real defects and satisfy the existing quality gates.

## Constraints

- Preserve the current REST API shape and control endpoints.
- Preserve the current WebSocket event schema.
- Preserve MQTT topic structure and Home Assistant compatibility.
- Favor targeted fixes over broad refactors.
- End state must include zero findings from `golangci-lint`, `gosec`, and `govulncheck`.

## Options Considered

### Option 1: Targeted hardening

Fix the WebSocket defect directly, replace avoidable unsafe patterns with tighter implementations, and use narrow documented suppressions only where a generic abstraction is safe but difficult for static analysis to prove.

Pros:

- Smallest behavior risk
- Preserves Home Assistant compatibility
- Clears current findings with limited churn

Cons:

- Leaves the generic shell abstraction in place
- Requires careful justification for any remaining analyzer suppressions

### Option 2: Broad command-execution refactor

Replace the shared shell wrapper with operation-specific allowlisted helpers and refactor multiple subsystems around that model.

Pros:

- Cleaner long-term security story
- Fewer static-analysis false positives

Cons:

- Much larger surface area
- Higher regression risk
- Not needed for this release target

### Option 3: Tooling-first suppression cleanup

Silence most findings through config or broad `nosec` usage with minimal code changes.

Pros:

- Fastest path to green checks

Cons:

- Weakens confidence in the gates
- Does not materially improve code quality
- Rejected

## Chosen Approach

Use targeted hardening. Fix real defects and tighten high-signal code paths. Only use narrow inline suppression when the code is safe by design and a broader refactor would be disproportionate to the problem.

## Design

### 1. WebSocket hub

The current broadcast path mutates `h.clients` and closes client channels while holding `RLock`. This is a real concurrency bug.

The fix will ensure all client-map mutation happens under exclusive ownership. The implementation may snapshot eligible clients under read lock, attempt sends outside the map mutation path, collect stale clients, then remove them under `Lock`. Alternatively, stale-client removal can be routed through the unregister path if that keeps ownership clearer. The key invariant is that the client map is never mutated while read-locked.

No wire-format changes are allowed.

### 2. Log-file access

The current log reader opens a raw path string after minimal validation. The API handler already resolves log files from an allowlisted inventory, so the lower-level reader should be tightened to match that model.

The fix will introduce an explicit resolver or membership check so only paths produced by the known log inventory can be opened. This removes the taint-analysis finding by construction and preserves existing API behavior.

### 3. Unassigned collector filesystem stats

The unassigned collector currently shells out to `df` to compute mounted-partition and remote-share sizes. Those subprocess calls are avoidable.

The fix will replace them with native filesystem stats from Go, using `statfs`-based logic for size, free, used, and usage percentage. This reduces process spawn overhead and clears the subprocess findings without changing returned DTO fields.

### 4. MQTT QoS normalization

The current MQTT code converts configured QoS from `int` to `byte` directly. Static analysis flags that conversion.

The fix will add one small normalization helper that ensures QoS is always in the valid MQTT range `0..2` before conversion. Invalid values will be clamped or defaulted in a way that preserves operational compatibility. For this project, defaulting invalid values to `0` is the safest behavior because it is already the default configuration and does not break Home Assistant command or discovery flows.

### 5. Collector cancel lifecycle

The collector manager stores cancel functions per collector. Static analysis currently flags one path as if the cancellation function were leaked.

The fix will make the lifecycle explicit enough for the analyzer and future readers. The manager will continue to own cancellation and will clear or invoke stored cancel functions in the appropriate state transitions.

### 6. Alert dispatcher cleanup

The alert dispatcher has low-risk formatting findings from `staticcheck`.

The fix will replace `WriteString(fmt.Sprintf(...))` patterns with `fmt.Fprintf`.

### 7. Shared shell wrapper

The shared shell wrapper exists to centralize timeouts and process execution. `gosec` flags it because it accepts variable command paths and args, but that is the point of the abstraction.

The design keeps the wrapper. The safety contract will be made explicit in code comments, and narrow inline `#nosec` annotations will be added with justification. This is the one accepted case where the analyzer is not capable of proving the safety model without a much larger redesign.

## Testing Strategy

- Add a regression test around the WebSocket broadcast path that exercises stale-client eviction and run it with the race detector.
- Add tests for log-path resolution so disallowed paths cannot be opened and allowlisted paths still can.
- Add tests for the native filesystem-stat helper used by the unassigned collector.
- Add tests for MQTT QoS normalization edge cases.
- Keep existing API and integration tests green.

## Verification

The implementation is only complete when all of the following succeed on a fresh run:

- `go test ./...`
- `go test -race ./daemon/services/api`
- `go test -race ./daemon/services`
- `golangci-lint run --config .golangci.yml --max-issues-per-linter 0 --max-same-issues 0 ./...`
- `gosec ./...`
- `govulncheck ./...`
- `ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build`
- `ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags deploy`
- `ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags verify`

For final release confidence on real Unraid hardware, prefer the full lifecycle command:

- `ansible-playbook -i ansible/inventory.yml ansible/deploy.yml`

## Out of Scope

- Authentication or authorization changes
- Reverse proxy guidance
- API redesign
- Home Assistant topic or payload redesign
- Broader refactors beyond the current findings and the confirmed WebSocket bug
