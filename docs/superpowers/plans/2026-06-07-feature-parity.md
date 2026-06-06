# Feature Parity (Sub-project C) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the _safe_ control operations the official Unraid GraphQL API has that this agent lacks — VM reset, Docker container remove/autostart/port-conflicts, and an array safe-subset (clear-stats, mount/unmount) — exposed via REST + MCP, confirm-gated and capability-gated, with no breaking changes.

**Architecture:** Extend existing controllers (`daemon/services/controllers/{vm,docker,array}.go`), REST handlers (`daemon/services/api/`), and MCP tools (`daemon/services/mcp/server.go`). Lifecycle verbs extend the existing `vm_action`/`container_action` enums; distinct operations get their own endpoint + tool. Destructive ops require `confirm=true`; `mdcmd`-backed ops reuse sub-project B's `requireBinary` gate.

**Tech Stack:** Go 1.26, moby SDK (Docker), digitalocean/go-libvirt (VMs), `/proc/mdcmd`/`mdcmd`, gorilla/mux, the project's `lib`/`logger`/`platform` packages, and the Ansible deploy/verify role.

**Spec:** `docs/superpowers/specs/2026-06-07-feature-parity-design.md`

---

## MANDATORY verification gate (every task)

No task is "done" until, in order: (1) `go test ./...` + `go vet` + `gofmt`/golangci-lint; (2) `make local`; (3) `ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify` → `failed=0`; (4) `coderabbit review --agent -t uncommitted`, fix valid findings; (5) CHANGELOG only in the final task, after hardware verification. Unraid host has no Python (Ansible ad-hoc uses `raw`). CodeRabbit may rate-limit (~8 min) and is nondeterministic — verify findings against real code. **Never disrupt the live production VMs/containers** when verifying.

## File structure

- Modify `daemon/services/controllers/vm.go` — add `Reset`.
- Modify `daemon/services/controllers/docker.go` — add `Remove`, `SetAutostart`, `PortConflicts`.
- Modify `daemon/services/controllers/array.go` — add safe-subset (verify-or-drop).
- Modify `daemon/services/api/handlers.go` (+ `server.go` routes) — endpoints + extend action handlers.
- Modify `daemon/services/mcp/server.go` — extend action tools + new tools.
- Add `daemon/dto/` fields/types as needed (port conflicts result).
- Tests beside each (`*_test.go`).
- Docs: `docs/integrations/mcp.md`, skill catalog, swagger regen, `AGENTS.md`, `CHANGELOG.md`.

---

### Task 0: VM reset

**Goal:** `vm_action` gains a `reset` action that hard-resets a running VM via libvirt.

**Files:**

- Modify: `daemon/services/controllers/vm.go` (add `Reset`)
- Modify: `daemon/services/api/handlers.go` (vm action handler), `daemon/services/mcp/server.go` (`vm_action` enum + dispatch)
- Test: `daemon/services/controllers/vm_test.go`

**Acceptance Criteria:**

- [ ] `VMController.Reset(vmName)` calls `libvirt DomainReset`, returns a clear error if the VM is absent or not running.
- [ ] REST `POST /vm/{name}/reset` and `vm_action` action `reset` both invoke it; `reset` requires `confirm=true`.
- [ ] Existing vm actions unchanged.

**Verify:** `go test ./daemon/services/controllers/ -run TestVMReset -v` → PASS

**Steps:**

- [ ] **Step 1: Add `Reset` to `vm.go`** (mirror `Restart` which uses `vc.connect` + `DomainReboot`):

```go
// Reset performs a hard reset of a running VM (equivalent to the reset button —
// no graceful shutdown). The VM must be running.
func (vc *VMController) Reset(vmName string) error {
 logger.Info("VM: Resetting %s...", vmName)
 l, domain, err := vc.connect(vmName)
 if err != nil {
  return err
 }
 defer l.Disconnect() //nolint:errcheck

 state, _, err := l.DomainGetState(domain, 0)
 if err != nil {
  return fmt.Errorf("failed to get VM state: %w", err)
 }
 if libvirt.DomainState(state) != libvirt.DomainRunning {
  return fmt.Errorf("cannot reset VM %q: not running", vmName)
 }
 if err := l.DomainReset(domain, 0); err != nil {
  return fmt.Errorf("failed to reset VM %q: %w", vmName, err)
 }
 logger.Info("VM: %s reset", vmName)
 return nil
}
```

- [ ] **Step 2: Wire REST + MCP.** Find how `Restart` is wired:

  - REST: `grep -n "vm/{name}/restart\|handleVM" daemon/services/api/server.go daemon/services/api/handlers.go` — add a `POST /vm/{name}/reset` route + handler calling `vc.Reset`, mirroring restart.
  - MCP: in `vm_action` tool (server.go ~line 1253), add `"reset"` to the action enum/switch and require `confirm` for it (mirror how `force-stop`/destructive actions check confirm).

- [ ] **Step 3: Test** `daemon/services/controllers/vm_test.go`:

```go
func TestVMReset(t *testing.T) {
 vc := NewVMController()
 // No libvirt in CI → connect fails with a clear error (not a panic).
 err := vc.Reset("nonexistent-vm")
 if err == nil {
  t.Skip("libvirt available; reset reached the daemon")
 }
}
```

- [ ] **Step 4: Gate + commit**

```bash
go test ./... && go vet ./... && make local
git add daemon/services/controllers/vm.go daemon/services/controllers/vm_test.go daemon/services/api/ daemon/services/mcp/server.go
git commit -m "feat(parity): VM reset (libvirt DomainReset) via vm_action + REST"
```

Then deploy/verify on Unraid (`failed=0`) + CodeRabbit.

---

### Task 1: Container remove

**Goal:** `container_action` gains a `remove` action (optional `remove_image`), confirm-gated.

**Files:**

- Modify: `daemon/services/controllers/docker.go` (add `Remove`)
- Modify: `daemon/services/api/handlers.go` (container action handler), `daemon/services/mcp/server.go` (`container_action` enum)
- Test: `daemon/services/controllers/docker_test.go`

**Acceptance Criteria:**

- [ ] `DockerController.Remove(id string, removeImage bool)` uses SDK `ContainerRemove` with `Force: true`; if `removeImage`, also `ImageRemove` (best-effort, log if in use).
- [ ] `container_action` action `remove` + REST require `confirm=true`.
- [ ] Existing container actions unchanged.

**Verify:** `go test ./daemon/services/controllers/ -run TestDockerRemove -v` → PASS

**Steps:**

- [ ] **Step 1: Add `Remove` to `docker.go`** (mirror `Stop` which uses `dc.initClient()` + `dc.client`):

```go
// Remove deletes a container (force-stopping it if running). When removeImage is
// true it also removes the container's image (best-effort).
func (dc *DockerController) Remove(containerID string, removeImage bool) error {
 if err := dc.initClient(); err != nil {
  return fmt.Errorf("docker unavailable: %w", err)
 }
 ctx := context.Background()
 var imageRef string
 if removeImage {
  if info, err := dc.client.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{}); err == nil {
   imageRef = info.Config.Image
  }
 }
 if _, err := dc.client.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true}); err != nil {
  return fmt.Errorf("failed to remove container %q: %w", containerID, err)
 }
 logger.Info("Docker: removed container %s", containerID)
 if removeImage && imageRef != "" {
  if _, err := dc.client.ImageRemove(ctx, imageRef, client.ImageRemoveOptions{}); err != nil {
   logger.Warning("Docker: container removed but image %s not removed (may be in use): %v", imageRef, err)
  }
 }
 return nil
}
```

(Verify the exact moby option type names — `ContainerRemoveOptions`, `ImageRemoveOptions`, `ContainerInspect` field for image — against the codebase's moby version; adjust to match the existing `Stop`/`ContainerInspect` usage in this file.)

- [ ] **Step 2: Wire REST + MCP.** Add `"remove"` to the `container_action` enum (server.go ~line 1216) with a `remove_image` bool arg, requiring `confirm=true`; add a REST `POST /docker/{id}/remove` handler (mirror `handleDockerStop`) that reads `confirm` + `remove_image`.

- [ ] **Step 3: Test** (`docker_test.go`): `Remove` on a bogus id with no docker daemon returns a clear "docker unavailable" error (skip if daemon present).

- [ ] **Step 4: Gate + commit** (`feat(parity): container remove (+optional image) via container_action + REST`), then deploy/verify + CodeRabbit.

---

### Task 2: Container autostart (verify mechanism)

**Goal:** Set a container's auto-start on/off through the same mechanism the Unraid WebUI uses.

**Files:**

- Modify: `daemon/services/controllers/docker.go` (add `SetAutostart`)
- Modify: `daemon/services/api/handlers.go` + `server.go` (`POST /docker/{id}/autostart`), `daemon/services/mcp/server.go` (`set_container_autostart`)
- Test: `daemon/services/controllers/docker_test.go`

**Acceptance Criteria:**

- [ ] The Unraid autostart persistence mechanism is identified (grep the running system / docs).
- [ ] `SetAutostart(id, enabled)` writes through it; the WebUI reflects the change.
- [ ] `POST /docker/{id}/autostart {enabled}` + `set_container_autostart` tool.

**Verify:** `go test ./daemon/services/collectors/ ./daemon/services/controllers/ -run Autostart -v` → PASS

**Steps:**

- [ ] **Step 1: Verify the mechanism.** On the live server:

```bash
ansible unraid -i ansible/inventory.yml -m raw -a "ls -la /var/lib/docker/unraid-autostart /boot/config/plugins/dockerMan/userprefs.cfg 2>/dev/null; head -20 /var/lib/docker/unraid-autostart 2>/dev/null"
```

Determine the canonical file + format (typically `/var/lib/docker/unraid-autostart`: one container name per line that auto-starts, optionally with a wait). Document the finding in a code comment.

- [ ] **Step 2: Implement `SetAutostart`** to add/remove the container's _name_ in that file (read → modify set → write atomically), matching the discovered format exactly. Resolve id→name via `ContainerInspect` if needed.

- [ ] **Step 3: Wire REST + MCP** (`POST /docker/{id}/autostart` body `{"enabled": true}`; tool `set_container_autostart` write, no confirm).

- [ ] **Step 4: Test** the file read/modify/write logic with a temp-file seam (don't touch the real path in tests).

- [ ] **Step 5: Gate + commit** (`feat(parity): set container autostart`), deploy/verify (reversibly toggle an idle container, revert) + CodeRabbit.

---

### Task 3: Port-conflict detection

**Goal:** Read-only detection of host ports claimed by more than one container.

**Files:**

- Modify: `daemon/services/controllers/docker.go` (add `PortConflicts`)
- Add: `dto` result type (`PortConflict`)
- Modify: `daemon/services/api/` (`GET /docker/port-conflicts`), `daemon/services/mcp/server.go` (`get_port_conflicts`)
- Test: `daemon/services/controllers/docker_test.go`

**Acceptance Criteria:**

- [ ] `PortConflicts()` returns `[]dto.PortConflict{Port, Protocol, Containers[]}` for any host port bound by ≥2 containers.
- [ ] Pure detection logic is unit-tested table-driven (no live daemon).
- [ ] `GET /docker/port-conflicts` + `get_port_conflicts` (read-only).

**Verify:** `go test ./daemon/services/controllers/ -run TestPortConflicts -v` → PASS

**Steps:**

- [ ] **Step 1: DTO** in `daemon/dto/docker.go`:

```go
// PortConflict reports a host port bound by more than one container.
type PortConflict struct {
 HostPort   int      `json:"host_port"`
 Protocol   string   `json:"protocol"`
 Containers []string `json:"containers"`
}
```

- [ ] **Step 2: Pure detection helper** (testable without a daemon) in `docker.go`:

```go
// detectPortConflicts groups container names by (host port, protocol) and
// returns entries where more than one container binds the same host port.
func detectPortConflicts(bindings map[string][]string) []dto.PortConflict { /* see test */ }
```

Implement: input is map keyed `"port/proto"` → container names; output conflicts where len>1, sorted by port.

- [ ] **Step 3: Failing test** `docker_test.go`:

```go
func TestPortConflicts(t *testing.T) {
 in := map[string][]string{"8080/tcp": {"a", "b"}, "9000/tcp": {"c"}}
 got := detectPortConflicts(in)
 if len(got) != 1 || got[0].HostPort != 8080 || len(got[0].Containers) != 2 {
  t.Fatalf("unexpected conflicts: %+v", got)
 }
}
```

- [ ] **Step 4: `PortConflicts()`** wires the live data: build the bindings map from container port info (reuse the docker collector's container/port data or `ContainerList` + inspect), then call `detectPortConflicts`.

- [ ] **Step 5: Wire REST + MCP** (`GET /docker/port-conflicts`; tool `get_port_conflicts` read-only).

- [ ] **Step 6: Gate + commit** (`feat(parity): docker port-conflict detection`), deploy/verify (GET returns 200) + CodeRabbit.

---

### Task 4: Array safe-subset (verify-or-drop) · USER-GATE

**Goal:** Ship clear-disk-statistics and disk mount/unmount **only if** a safe, supported Unraid mechanism is verified; otherwise drop+document.

**USER-ORDERED GATE — NON-SKIPPABLE.** Requested by the user (verify on Unraid; verify-or-drop). Close only after each op is either verified working on hardware (reversibly) or explicitly dropped with the reason documented — with output captured.

**Files:**

- Modify: `daemon/services/controllers/array.go`
- Modify: `daemon/services/api/` + `daemon/services/mcp/server.go`
- Test: `daemon/services/controllers/array_test.go`

**Acceptance Criteria:**

- [ ] The Unraid mechanism for clear-stats and array-disk mount/unmount is verified on the live server (or determined unsafe/absent).
- [ ] For each op with a safe mechanism: controller method + REST endpoint + MCP tool, capability-gated (`mdcmd`), unmount requires `confirm=true`; verified reversibly on hardware.
- [ ] For each op WITHOUT a safe mechanism: dropped, with a one-line rationale in the spec/CHANGELOG and no half-working code.

**Verify:** `go test ./daemon/services/controllers/ -run TestArrayDisk -v` → PASS, AND live confirmation per implemented op.

**Steps:**

- [ ] **Step 1: Verify mechanisms** on the live server (non-destructive probing):

```bash
ansible unraid -i ansible/inventory.yml -m raw -a "cat /usr/local/sbin/mdcmd 2>/dev/null | grep -iE 'stat|mount|umount' ; mdcmd 2>&1 | head -20"
```

Determine whether `mdcmd` (or `emcmd`) exposes clear-stats / mount / unmount for array disks. Record findings.

- [ ] **Step 2: Implement only the verified ops** in `array.go` via `mdcmdExec(...)` (reusing the gate from sub-project B), e.g. `ClearDiskStats(disk)`, `MountDisk(disk)`, `UnmountDisk(disk)`. Validate `disk` via `lib` validation. Unmount requires `confirm` at the handler layer.

- [ ] **Step 3: Wire REST + MCP** for each implemented op (`POST /disks/{id}/clear-stats`, `/mount`, `/unmount`; tools `clear_disk_stats`, `disk_mount_action`).

- [ ] **Step 4: Tests** — capability-gate behavior (no `mdcmd` → clear error), arg validation, confirm enforcement on unmount.

- [ ] **Step 5: Gate + deploy + reversible live check** (e.g. clear-stats on a disk and confirm counters reset; only mount/unmount a disk if safely reversible — else skip-with-log). Document any dropped op.

- [ ] **Step 6: CodeRabbit**; fix findings.

```json:metadata
{"files": ["daemon/services/controllers/array.go","daemon/services/api/handlers.go","daemon/services/mcp/server.go"], "verifyCommand": "ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify", "acceptanceCriteria": ["Unraid mechanism for each array op verified or determined absent","verified ops ship gated + (unmount) confirm-required + reversibly checked on hardware","unsafe/absent ops dropped + documented, no half-working code"], "userGate": true, "tags": ["user-gate"], "gateScope": "this-step"}
```

---

### Task 5: Docs + final verify + CHANGELOG · USER-GATE

**Goal:** Update docs/swagger/tool-counts, do the final full hardware verify + CodeRabbit, then CHANGELOG last.

**USER-ORDERED GATE — NON-SKIPPABLE.** Requested by the user ("once done and tested and everything is working update the CHANGELOG.md"). CHANGELOG is the LAST action, only after full hardware verification + clean CodeRabbit, with output captured.

**Files:**

- Modify: `docs/integrations/mcp.md` (tool count + new tools), `skills/unraid-management-agent/references/mcp-tools.md`, `skills/unraid-management-agent/SKILL.md`, `AGENTS.md`, regenerate swagger, `CHANGELOG.md` (LAST)

**Acceptance Criteria:**

- [ ] MCP tool count updated to the actual new total; new tools + extended actions documented.
- [ ] Swagger regenerated (new endpoints present).
- [ ] Final `--tags build,deploy,verify` passes (`failed=0`); CodeRabbit clean; THEN CHANGELOG updated.

**Verify:** Final `ansible-playbook ... --tags build,deploy,verify` → `failed=0`.

**Steps:**

- [ ] **Step 1: Count tools** (`grep -c 'mcp.AddTool' daemon/services/mcp/server.go`) and update mcp.md / skill catalog / SKILL.md to the real number; add the new tools (`set_container_autostart`, `get_port_conflicts`, verified array tools) + extended `vm_action`/`container_action` actions.
- [ ] **Step 2: `make swagger`** and commit the regenerated `swagger.json`/`yaml`/`docs.go`.
- [ ] **Step 3: AGENTS.md** — bump the MCP count.
- [ ] **Step 4: FINAL gate** — `go test ./...`, `make local`, `ansible-playbook ... --tags build,deploy,verify` (`failed=0`), `coderabbit review --agent -t uncommitted` (clean).
- [ ] **Step 5: CHANGELOG (LAST)** — add a feature-parity entry under the current version, then commit.

```json:metadata
{"files": ["docs/integrations/mcp.md","skills/unraid-management-agent/references/mcp-tools.md","AGENTS.md","CHANGELOG.md"], "verifyCommand": "ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify", "acceptanceCriteria": ["docs + tool count + swagger updated","final Unraid verify failed=0 + CodeRabbit clean","CHANGELOG updated last after verification"], "userGate": true, "tags": ["user-gate"], "gateScope": "whole-plan"}
```

---

## Self-review notes

- **Spec coverage:** VM reset (T0) ✓; container remove (T1) ✓; autostart (T2, verify-mechanism) ✓; port-conflicts (T3) ✓; array safe-subset verify-or-drop (T4) ✓; docs+verify+CHANGELOG (T5) ✓; mandatory gate on every task ✓; deferred add/remove-disk-to-slot honored (non-goal) ✓; confirm + capability-gate model ✓.
- **Placeholder scan:** the "verify mechanism" steps (T2/T4) are deliberate verify-or-drop with concrete probe commands, not vague TODOs. moby option type names flagged to match the codebase's version.
- **Type consistency:** `Reset`/`Remove`/`SetAutostart`/`PortConflicts`/`detectPortConflicts`/`dto.PortConflict` used consistently; extends existing `vm_action`/`container_action`.
