---
applyTo: "daemon/dto/**/*.go"
---

# DTO Instructions

Reference: [`AGENTS.md`](../../AGENTS.md) for full project context.

## Purpose

Data Transfer Objects (DTOs) define the shared data structures used between collectors, API handlers, WebSocket broadcasts, and MCP tools.

## Conventions

- Use `json:"field_name"` tags on all exported fields
- Use `json:"field_name,omitempty"` for optional fields
- Keep struct names descriptive: `SystemInfo`, `DiskInfo`, `ContainerInfo`
- Group related fields logically within structs
- Add comments for fields that aren't self-explanatory

## Common DTOs

- `SystemInfo` — CPU, RAM, uptime, temperatures
- `ArrayStatus` — Array state, disk assignments
- `DiskInfo` — Per-disk info including SMART data
- `ContainerInfo` — Docker container state
- `VMInfo` — Virtual machine state
- `UPSStatus` — UPS monitoring data
- `GPUMetrics` — GPU utilization/temperature
- `WebSocketMessage` / `WSEvent` — WebSocket event wrapper
- `Response` — Standard API response for control operations

## Thread Safety

DTOs are published to the PubSub event bus and read from the API cache. The API server handles thread safety via `sync.RWMutex` — DTOs themselves don't need synchronization, but they should be safe to serialize to JSON concurrently (avoid maps without synchronization in DTOs).
