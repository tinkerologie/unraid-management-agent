---
name: Bug Report
about: Report a bug or issue with the Unraid Management Agent
title: "[BUG] "
labels: bug
assignees: ""
---

## Bug Description

A clear and concise description of what the bug is.

## Environment

**Unraid Version:** (e.g., 7.2)
**Plugin Version:** (e.g., 2025.11.1)
**Architecture:** (e.g., x86_64)

## Hardware Configuration

Please provide details about your hardware, especially if the issue relates to monitoring or control:

- **CPU:** (e.g., Intel Core i7-12700K, AMD Ryzen 9 5950X)
- **Disk Controller:** (e.g., LSI 9300-8i HBA, Dell PERC H310, onboard SATA)
- **GPU:** (e.g., NVIDIA RTX 3080, AMD Radeon RX 6800 XT, N/A)
- **UPS:** (e.g., APC Back-UPS Pro 1500, CyberPower CP1500PFCLCD, N/A)
- **Network:** (e.g., Intel I350, Realtek RTL8125, bonded interfaces)
- **Other Relevant Hardware:**

## Steps to Reproduce

1. Go to '...'
2. Click on '...'
3. Execute command '...'
4. See error

## Expected Behavior

A clear and concise description of what you expected to happen.

## Actual Behavior

A clear and concise description of what actually happened.

## API Endpoint (if applicable)

- **Endpoint:** (e.g., GET /api/v1/gpu)
- **Request:** (include request details if applicable)
- **Response:** (include response or error message)

```json
{
  "error": "example error response"
}
```

## Logs

Please provide relevant logs from `/var/log/unraid-management-agent.log`:

```
[Paste relevant log entries here]
```

To get debug logs, run: `./unraid-management-agent boot --debug`

## Screenshots

If applicable, add screenshots to help explain your problem.

## Additional Context

Add any other context about the problem here. For example:

- Did this work in a previous version?
- Does this only happen with specific configurations?
- Are there any workarounds?

## Possible Solution

If you have ideas about what might be causing this or how to fix it, please share!
