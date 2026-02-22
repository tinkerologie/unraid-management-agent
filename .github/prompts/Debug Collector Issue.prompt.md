---
description: Step-by-step guide for debugging hardware-specific collector failures
tools: ["editor", "terminal"]
---

# Debug a Collector Issue

Follow these steps to debug a hardware-specific collector failure.

## Step 1: Identify the Failing Collector

Check the log file at `/var/log/unraid-management-agent.log` for errors. Look for:

- `PANIC` messages (collector crashed)
- Parse errors (unexpected output format)
- Command execution failures

## Step 2: Reproduce the Issue

If possible, get the raw command output that the collector is trying to parse. Common commands:

| Collector | Command / Source |
|-----------|-----------------|
| System | `/proc/cpuinfo`, `/proc/meminfo`, `sensors` |
| Disk | `smartctl` output |
| GPU | `nvidia-smi`, `intel_gpu_top` |
| UPS | `apcaccess`, `upsc` |
| Hardware | `dmidecode` |
| Network | `ethtool`, `/sys/class/net/` |
| VM | `virsh` via go-libvirt |
| Docker | Docker Engine SDK |

## Step 3: Compare Expected vs Actual Output

The collector's parsing logic expects a specific format. Compare:

1. What format does the parsing code expect?
2. What format does this hardware actually produce?

Check the relevant parsing code in:

- `daemon/lib/parser.go` — INI file parsing
- `daemon/lib/dmidecode.go` — DMI/SMBIOS parsing
- `daemon/lib/ethtool.go` — Network interface parsing
- `daemon/services/collectors/*.go` — Collector-specific parsing

## Step 4: Add Fallback Logic

When different hardware produces different output:

1. Don't remove support for the original format
2. Add detection for the new format
3. Use defensive parsing (check array bounds, handle nil)
4. Log warnings for unexpected formats (not errors)

## Step 5: Test

- Add test cases for both the original and new hardware formats
- Use actual output samples as test fixtures
- Run `make test` to verify nothing broke

## Step 6: Document

- Document the hardware details in the PR description
- Update `CHANGELOG.md` with the fix
- Consider updating `docs/guides/system-requirements.md` if relevant
