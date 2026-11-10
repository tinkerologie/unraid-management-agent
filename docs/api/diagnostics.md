# Diagnostics API Reference

Use the diagnostics endpoints to check source health and download a redacted diagnostics bundle.

See [Diagnostic Commands](../troubleshooting/diagnostics.md) for checks you can run on the server.

---

## Endpoints

### 1. Agent Self-Test

Checks the health of all platform data sources and capabilities.

- **Method**: `GET`
- **Path**: `/api/v1/diagnostics/self-test`
- **Authentication**: Bearer token if the agent has an API token
- **Response Format**: `application/json`

#### Response (`200 OK`)

```json
{
  "unraid_version": "7.3.4",
  "overall_state": "healthy",
  "capabilities": {
    "unraid_version": "7.3.4",
    "items": [
      {
        "name": "docker",
        "available": true,
        "state": "present",
        "target": "/usr/bin/docker"
      }
    ],
    "checked_at": "2026-10-10T05:00:00Z"
  },
  "subsystems": [
    {
      "subsystem": "system",
      "state": "healthy",
      "last_checked": "2026-10-10T05:00:00Z",
      "last_healthy": "2026-10-10T05:00:00Z"
    }
  ],
  "timestamp": "2026-10-10T05:00:00Z"
}
```

---

### 2. Download Diagnostics Bundle

The ZIP contains host metrics, component status, recent system logs, and configuration.
The agent removes known sensitive values.

> [!WARNING]
> Examine the ZIP before you share it.
> It can contain private information about your server.

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

The snapshot contains OS paths, executable files, and detected hardware classes.
The agent uses the hardware classes to decide which automatic collectors can run.
See [Hardware Auto-Detection](../guides/configuration.md#hardware-auto-detection).

- **Method**: `GET`
- **Path**: `/api/v1/capabilities`
- **Query**: `refresh=true` requests a new check. Within 30 seconds of the last check, the agent returns the cached snapshot.
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

`state` is `present`, `absent`, or `unknown`. `available` is `true` only when the state is `present`.
`checked_at` gives the time of the check.

#### Error Responses

- `400 Bad Request` — `refresh` is not a boolean.
- `503 Service Unavailable` — capability detection is not initialised.

---

## See Also

- [Troubleshooting & Diagnostic Commands](../troubleshooting/diagnostics.md) — Comprehensive guide to debugging, shell inspection, and unredacted host logs.
- [REST API Reference](rest-api.md) — Full REST API endpoint reference.
- [WebSocket Events](websocket-events.md) — Real-time event streaming.
