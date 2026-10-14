# Diagnostics API Reference

The Unraid Management Agent provides diagnostics endpoints to verify data source health across Unraid OS versions and export sanitized troubleshooting bundles.

For troubleshooting command-line guides and service inspection, see the [Diagnostic Commands](../troubleshooting/diagnostics.md) documentation.

---

## Endpoints

### 1. Agent Self-Test

Checks the health of all platform data sources and capabilities.

- **Method**: `GET`
- **Path**: `/api/v1/diagnostics/self-test`
- **Authentication**: Not required (or Bearer token if configured)
- **Response Format**: `application/json`

#### Response (`200 OK`)

```json
{
  "unraid_version": "6.12.10",
  "overall_state": "healthy",
  "capabilities": {
    "docker": true,
    "vms": true,
    "zfs": true,
    "ups": true,
    "gpu": true
  },
  "subsystems": [
    {
      "name": "system",
      "state": "healthy",
      "last_healthy": "2026-10-10T05:00:00Z"
    },
    {
      "name": "array",
      "state": "healthy",
      "last_healthy": "2026-10-10T05:00:00Z"
    }
  ],
  "timestamp": "2026-10-10T05:00:00Z"
}
```

---

### 2. Download Diagnostics Bundle

Generates and downloads a ZIP archive containing host metrics, disk status, container states, VM states, recent system logs, and configuration with known sensitive values redacted. Review the archive for sensitive data before sharing it publicly.

- **Method**: `GET`
- **Path**: `/api/v1/diagnostics/bundle`
- **Authentication**: Bearer token (if configured)
- **Response Format**: `application/zip`

#### Response Headers (`200 OK`)

| Header                   | Example Value                                                         | Description                                             |
| ------------------------ | --------------------------------------------------------------------- | ------------------------------------------------------- |
| `Content-Type`           | `application/zip`                                                     | Binary ZIP archive                                      |
| `Content-Disposition`    | `attachment; filename="unraid-diagnostics-tower-20261010-050000.zip"` | Suggested download filename                             |
| `Cache-Control`          | `no-store`                                                            | Prevents intermediate caching of sensitive host metrics |
| `X-Content-Type-Options` | `nosniff`                                                             | MIME type sniffing protection                           |

#### Error Response (`500 Internal Server Error`)

```json
{
  "success": false,
  "message": "Failed to collect diagnostics — check the agent log",
  "timestamp": "2026-10-10T05:00:00Z"
}
```

### 3. Detected Capabilities

Returns the capability snapshot: the OS paths and binaries the agent depends
on, plus the detected hardware classes that gate the hardware-dependent
collectors ([#261](https://github.com/tinkerologie/unraid-management-agent/issues/261)).
See [Hardware Auto-Detection](../guides/configuration.md#hardware-auto-detection).

- **Method**: `GET`
- **Path**: `/api/v1/capabilities`
- **Query**: `refresh=true` re-checks now; within 30 seconds of the last check
  the cached snapshot is returned. Capabilities are also re-checked every
  15 minutes.
- **Authentication**: Bearer token (if configured)
- **Response Format**: `application/json`

#### Response (`200 OK`)

```json
{
  "unraid_version": "7.3.3",
  "checked_at": "2026-10-14T09:00:00Z",
  "items": [
    { "name": "docker", "available": true, "state": "present", "target": "/usr/bin/docker" },
    {
      "name": "hardware.nic_optics",
      "available": false,
      "state": "absent",
      "detail": "no NIC with optics or temperature sensors"
    },
    {
      "name": "hardware.storage_topology",
      "available": true,
      "state": "present",
      "detail": "SAS controller at 0000:01:00.0; storcli found, sg_ses found"
    }
  ]
}
```

`state` is `present`, `absent` or `unknown`; `available` is `true` only when
`present`. The self-test's `capabilities` object has the same shape, so it also
gains the additive `state` and `checked_at` fields.

#### Error Responses

- `400 Bad Request` — `refresh` is not a boolean.
- `503 Service Unavailable` — capability detection is not initialised.

---

## See Also

- [Troubleshooting & Diagnostic Commands](../troubleshooting/diagnostics.md) — Comprehensive guide to debugging, shell inspection, and unredacted host logs.
- [REST API Reference](rest-api.md) — Full REST API endpoint reference.
- [WebSocket Events](websocket-events.md) — Real-time event streaming.
