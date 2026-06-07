# Feature Parity (Sub-project C) — Design

**Date:** 2026-06-07
**Sub-project:** C (of a 3-part effort: A = Security/Access Control [descoped — LAN-only], B = OS-Resilience [done, merged], C = Feature Parity)
**Status:** Approved design — ready for implementation planning

## Problem

The grounded comparison against the official Unraid GraphQL API (`unraid/api`)
found a small set of control operations the official API has that this agent
does not: **VM reset**, Docker **container remove / autostart / port-conflict
detection**, and array **disk-membership** operations. Closing the _safe_ gaps
makes the agent's control surface match or exceed the official API for everyday
operations, while deliberately NOT shipping the genuinely dangerous array
reconfiguration operations on a third-party plugin.

## Decisions (locked during brainstorming)

| Decision                    | Choice                                                                                                                                                                                                                                 |
| --------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Array disk-membership scope | **Safe subset only** — clear-statistics + disk mount/unmount. **Defer** add/remove-disk-to-slot (array must be stopped; data-loss / parity-rebuild risk).                                                                              |
| Docker scope                | **All three** — container remove, autostart config, port-conflict detection.                                                                                                                                                           |
| VM scope                    | **reset** (hard reset via libvirt).                                                                                                                                                                                                    |
| API exposure                | **Approach 1** — extend existing `*_action` enums for lifecycle verbs (`vm_action`+`reset`, `container_action`+`remove`); dedicated REST endpoint + MCP tool for distinct ops (autostart, port-conflicts, clear-stats, mount/unmount). |
| Confirmation                | Destructive ops (`container_action remove`, disk `unmount`, `vm_action reset`) require `confirm=true`. Read-only port-conflicts and non-destructive clear-stats / autostart do not.                                                    |
| Capability gating           | Reuse sub-project B's `requireBinary` for `mdcmd`-backed array ops; Docker SDK / libvirt API paths already surface clear errors.                                                                                                       |

## Non-goals

- No add/remove disk to/from array slots (the data-loss-risky reconfiguration).
- No new auth (sub-project A, descoped).
- No new packages — work lives in existing controllers/api/mcp.
- No breaking changes — all additive; extended action enums stay backward-compatible.

## Architecture & surfaces

All operations follow the existing validate-execute-return controller pattern,
get a REST endpoint, and an MCP exposure.

| Operation           | Controller method                                                              | REST                                                 | MCP                              | Confirm      | Gate                          |
| ------------------- | ------------------------------------------------------------------------------ | ---------------------------------------------------- | -------------------------------- | ------------ | ----------------------------- |
| VM reset            | `VMController.Reset(name)` → `libvirt DomainReset`                             | `vm_action` action `reset`                           | `vm_action` enum `reset`         | yes          | libvirt (clear error)         |
| Container remove    | `DockerController.Remove(id, removeImage)` → SDK `ContainerRemove(Force:true)` | `container_action` action `remove` (+`remove_image`) | `container_action` enum `remove` | yes          | SDK (clear error)             |
| Container autostart | `DockerController.SetAutostart(id, enabled)`                                   | `POST /docker/{id}/autostart` `{enabled}`            | `set_container_autostart`        | no           | SDK + Unraid dockerMan config |
| Port conflicts      | `DockerController.PortConflicts()` (read-only)                                 | `GET /docker/port-conflicts`                         | `get_port_conflicts`             | no           | SDK                           |
| Disk clear-stats    | `ArrayController.ClearDiskStats(disk)`                                         | `POST /disks/{id}/clear-stats`                       | `clear_disk_stats`               | no           | `mdcmd` (B's gate)            |
| Disk mount/unmount  | `ArrayController.MountDisk`/`UnmountDisk(disk)`                                | `POST /disks/{id}/mount` & `/unmount`                | `disk_mount_action`              | unmount: yes | `mdcmd`                       |

## Per-feature behavior & safety

- **VM reset** — `DomainReset(domain, 0)` (hard reset; no graceful shutdown). Look
  up domain (clear error if absent), verify running via `DomainGetState`
  (resetting a stopped VM → clear error). `confirm=true` required.
- **Container remove** — `ContainerRemove` with `Force: true` (stops+removes a
  running container). `removeImage` → follow with `ImageRemove` (best-effort; log
  if image in use). `confirm=true` required. Removed container drops from the next
  collector cycle.
- **Container autostart** — `SetAutostart(id, enabled)`. The exact Unraid
  persistence mechanism (`/boot/config/plugins/dockerMan/userprefs.cfg` vs the
  `unraid-autostart` file) **is verified during implementation**; the setter
  writes through the same mechanism the WebUI uses.
- **Port conflicts** — enumerate containers, collect host-port bindings, report
  any host port claimed by >1 container. Returns `[{port, containers[]}]`.
  Read-only.
- **Array safe subset (verify-or-drop)** — `ClearDiskStats` and
  `MountDisk`/`UnmountDisk` map to Unraid `mdcmd`/`emcmd` mechanisms that are less
  standardized than spin up/down. Implementation **first verifies a safe,
  supported mechanism exists** for each; if so it ships gated (+ confirm for
  unmount); **if no safe mechanism exists, that op is dropped and documented**
  rather than hacked in.

## Error handling

- Validate inputs (disk/container/VM identifiers) via `daemon/lib/validation.go`
  helpers; reject empty/invalid before executing.
- Destructive ops without `confirm=true` → `400` with a clear message.
- Capability-unavailable → typed `"<subsystem> control unavailable: …"` error
  (sub-project B's `requireBinary`).
- All exec paths pass arguments discretely (never via a shell).

## Testing

- **Controller unit tests:** arg validation, confirm enforcement, capability-gate
  errors (libvirt/docker/`mdcmd` absent), `PortConflicts` detection (table-driven
  overlapping vs non-overlapping fixtures).
- **API handler tests:** status codes; destructive endpoints reject without
  `confirm=true` (400) and accept with it; `port-conflicts` response shape.
- **MCP tests:** new tools registered, read/write annotations correct, tool-count
  assertion updated.
- **Live verification** (Ansible `verify` role): read-only `GET
/docker/port-conflicts` → 200; a reversible exercise where safe (toggle
  autostart on an idle/throwaway container and revert). **Never** disrupt the live
  production VMs/containers; skip-with-log if no safe target.

## Verification workflow (MANDATORY — every step)

No step is "done" until, in order: (1) **Test** `go test ./...` + vet/gofmt/lint;
(2) **Build** `make local`; (3) **Verify on Unraid** `ansible-playbook -i
ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify` (`failed=0`);
(4) **CodeRabbit** `coderabbit review --agent -t uncommitted`, fix valid findings;
(5) **CHANGELOG** updated only at the end, after hardware verification.

## Rollout (incremental — each step shippable, each through the gate)

1. VM reset (controller + `vm_action` enum + REST + MCP + tests).
2. Container remove (controller + `container_action` enum + REST + MCP + confirm + tests).
3. Container autostart (verify mechanism → controller + endpoint + tool + tests).
4. Port-conflict detection (controller + endpoint + tool + tests).
5. Array safe-subset (verify-or-drop: clear-stats, mount/unmount) + tests.
6. Docs (mcp.md count, skill catalog, swagger regen, AGENTS.md) + final Unraid verify + CodeRabbit → **then** CHANGELOG last.

## Compatibility

All additive. Extended `vm_action`/`container_action` enums accept new values
while existing values behave identically. New endpoints/tools don't alter
existing responses. No breaking changes.

## Success criteria

- VM reset, container remove (+optional image), autostart toggle, and port-conflict
  detection work via REST + MCP, verified on hardware (reversibly).
- Destructive ops refuse without `confirm=true`; capability-unavailable paths
  return clear errors.
- Array safe-subset ops either ship verified or are explicitly dropped+documented.
- No regression to existing endpoints/tools; healthy responses unchanged.
- Every step cleared the mandatory verification gate; CHANGELOG updated last.
