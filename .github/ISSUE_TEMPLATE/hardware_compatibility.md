---
name: Hardware Compatibility Issue
about: Report that the plugin doesn't work correctly on your hardware
title: '[HARDWARE] '
labels: hardware-compatibility
assignees: ''
---

## Hardware Not Working

Which component/feature isn't working correctly?

- [ ] GPU monitoring (temperature, utilization)
- [ ] UPS monitoring (battery, status)
- [ ] Disk monitoring (SMART data, temperature)
- [ ] Network monitoring (interface stats, bandwidth)
- [ ] Array status
- [ ] Docker containers
- [ ] Virtual machines
- [ ] Other (describe below)

## Hardware Configuration

Please provide detailed information about your hardware:

**CPU:**
- Model: (e.g., Intel Core i7-12700K)
- Architecture: (e.g., x86_64)

**Disk Controller:**
- Type: (e.g., HBA, RAID controller, onboard SATA)
- Model: (e.g., LSI 9300-8i, Dell PERC H310)
- Driver: (if known)

**GPU:**
- Vendor: (NVIDIA / AMD / Intel)
- Model: (e.g., RTX 3080, RX 6800 XT)
- Driver Version: (if known)

**UPS:**
- Brand/Model: (e.g., APC Back-UPS Pro 1500)
- Monitoring Software: (apcupsd / NUT / other)
- Connection Type: (USB / Serial / Network)

**Network:**
- Card Model: (e.g., Intel I350)
- Configuration: (e.g., bonded interfaces, VLAN, bridge)

**Other Relevant Hardware:**

## Environment

**Unraid Version:** (e.g., 7.2)
**Plugin Version:** (e.g., 2025.11.1)

## Expected Behavior

What data or functionality should work with this hardware?

**Example:**
"GPU temperature should be reported in the /api/v1/gpu endpoint"

## Actual Behavior

What's actually happening?

**Example:**
"The /api/v1/gpu endpoint returns null for temperature field"

## Investigation

If you've investigated the issue, please share your findings:

### Command Output

If you've manually run the command that the plugin uses, share the output:

```bash
# Example for GPU issues:
$ /usr/bin/nvidia-smi --query-gpu=temperature.gpu --format=csv,noheader
[Paste output here]
```

### Log Output

Enable debug logging and share relevant entries:

```bash
$ ./unraid-management-agent boot --debug
```

```
[Paste relevant log entries here]
```

### Collector Affected

If you know which collector is failing:
- [ ] System Collector (`daemon/services/collectors/system.go`)
- [ ] Disk Collector (`daemon/services/collectors/disk.go`)
- [ ] GPU Collector (`daemon/services/collectors/gpu.go`)
- [ ] UPS Collector (`daemon/services/collectors/ups.go`)
- [ ] Network Collector (`daemon/services/collectors/network.go`)
- [ ] Other: _____________

## Possible Solution

If you've identified a potential fix:

**Root Cause:**
(e.g., "The nvidia-smi output format is different on RTX 4000 series")

**Proposed Fix:**
(e.g., "Add alternative parsing for newer nvidia-smi XML format")

**I can submit a PR:**
- [ ] Yes, I can fix this and submit a PR
- [ ] No, but I can test a fix on my hardware
- [ ] No, I just want to report the issue

## Tested Workarounds

Have you found any workarounds?

## Additional Context

Any other information that might help diagnose this issue:
- Does this affect all functionality or just specific metrics?
- Did this work in a previous plugin version?
- Are there any error messages in system logs?

---

**Note:** Hardware compatibility contributions are especially valuable! See [CONTRIBUTING.md](../../CONTRIBUTING.md#hardware-compatibility-contributions) for detailed guidance on fixing hardware-specific issues.
