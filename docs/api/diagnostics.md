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

| Header | Example Value | Description |
| --- | --- | --- |
| `Content-Type` | `application/zip` | Binary ZIP archive |
| `Content-Disposition` | `attachment; filename="unraid-diagnostics-tower-20261010-050000.zip"` | Suggested download filename |
| `Cache-Control` | `no-store` | Prevents intermediate caching of sensitive host metrics |
| `X-Content-Type-Options` | `nosniff` | MIME type sniffing protection |

#### Error Response (`500 Internal Server Error`)

```json
{
  "success": false,
  "message": "Failed to collect diagnostics — check the agent log",
  "timestamp": "2026-10-10T05:00:00Z"
}
```

---

## See Also

- [Troubleshooting & Diagnostic Commands](../troubleshooting/diagnostics.md) — Comprehensive guide to debugging, shell inspection, and unredacted host logs.
- [REST API Reference](rest-api.md) — Full REST API endpoint reference.
- [WebSocket Events](websocket-events.md) — Real-time event streaming.
