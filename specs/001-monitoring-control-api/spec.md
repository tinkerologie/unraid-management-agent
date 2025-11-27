# Feature Specification: Unraid Monitoring and Control Interface

**Feature Branch**: `001-monitoring-control-api`
**Created**: 2025-11-27
**Status**: Draft
**Input**: User description: "Monitoring and control interface for Unraid servers with REST API and WebSocket support"

## Clarifications

### Session 2025-11-27

- Q: What operational metrics should be exposed for monitoring the agent itself? → A: Basic health metrics only (uptime, request count, error rate, cache hit rate) exposed via /health endpoint
- Q: How should API versions be managed when breaking changes become unavoidable? → A: Version in URL path (e.g., /api/v1/system, /api/v2/system)
- Q: Which specific collection interval should be used for each collector type? → A: Use intervals from existing daemon/constants/const.go (System: 5s, Array: 10s, Disk: 30s, Docker: 10s, VM: 10s, Network: 15s, GPU: 10s, UPS: 10s, Shares: 60s, Hardware: 300s, ZFS: 30s)
- Q: What specific timeout and interval values should be used for WebSocket ping/pong health monitoring? → A: 30-second ping interval with 60-second timeout
- Q: What is the maximum number of simultaneous WebSocket clients the system should support? → A: 10 simultaneous clients
- Q: When optional hardware (GPU, UPS) is unavailable, what specific HTTP response should be returned? → A: HTTP 200 with JSON indicating unavailable status (e.g., {"available": false, "reason": "Hardware not detected"})
- Q: What timeout should be used for shell command execution (docker, virsh, mdcmd)? → A: 60-second timeout

## User Scenarios & Testing

### User Story 1 - Real-Time System Monitoring (Priority: P1)

As a home lab enthusiast, I want to monitor my Unraid server's vital statistics in real-time so that I can quickly identify and respond to system issues before they cause data loss or service interruption.

**Why this priority**: This is the foundational capability that all other features depend on. Without reliable monitoring data, users cannot make informed decisions about their systems.

**Independent Test**: Can be fully tested by connecting to monitoring endpoints and verifying that current system metrics (CPU, RAM, temperature, disk status) are returned within acceptable timeframes. Delivers immediate value by providing visibility into server health.

**Acceptance Scenarios**:

1. **Given** the monitoring interface is running, **When** a user requests system information via REST API, **Then** current CPU usage, RAM usage, temperature, and uptime are returned within 50ms
2. **Given** a user connects via WebSocket, **When** system metrics change, **Then** updated metrics are broadcast to the connected client within 1 second of the change
3. **Given** multiple monitoring endpoints exist, **When** a user queries any endpoint, **Then** data is returned from in-memory cache without querying the system directly
4. **Given** the server has no GPU or UPS hardware, **When** a user requests GPU or UPS metrics, **Then** the system returns HTTP 200 with JSON indicating unavailable status (e.g., {"available": false, "reason": "Hardware not detected"})

---

### User Story 2 - Remote Service Control (Priority: P2)

As a system administrator, I want to start, stop, and restart Docker containers and virtual machines remotely so that I can manage my services without needing direct access to the Unraid web interface.

**Why this priority**: Control operations enable automation and remote management, but depend on monitoring capabilities being in place first. Users need to see what's running before they can control it.

**Independent Test**: Can be tested by issuing control commands (start/stop/restart) to existing Docker containers or VMs and verifying the operations complete successfully. Delivers value by enabling remote administration.

**Acceptance Scenarios**:

1. **Given** a Docker container is running, **When** a user sends a stop command, **Then** the container stops within 5 seconds and the operation is logged
2. **Given** a virtual machine is stopped, **When** a user sends a start command, **Then** the VM starts and the user receives confirmation of the state change
3. **Given** an invalid container or VM name is provided, **When** a user attempts a control operation, **Then** a clear error message is returned with HTTP 400 status
4. **Given** a control operation fails, **When** the system attempts to execute it, **Then** the error is logged and a detailed error response is returned to the user
5. **Given** multiple control operations are requested simultaneously, **When** the operations execute, **Then** each operation is handled independently without blocking others

---

### User Story 3 - Custom Dashboard Integration (Priority: P2)

As a developer building monitoring tools, I want to consume real-time server data via REST API and WebSocket so that I can create custom dashboards and integrations tailored to my specific needs.

**Why this priority**: This is a key differentiator from the official Unraid API and enables the community-driven innovation that makes this plugin valuable. It's prioritized equally with control operations because it serves a different user persona.

**Independent Test**: Can be tested by building a simple dashboard client that consumes API endpoints and WebSocket events. Delivers value by enabling third-party tool development.

**Acceptance Scenarios**:

1. **Given** a developer wants to build a dashboard, **When** they connect to the WebSocket endpoint, **Then** they receive real-time events for all collector updates without polling
2. **Given** a REST API client, **When** it requests data from any monitoring endpoint, **Then** well-formatted JSON responses are returned with consistent structure
3. **Given** multiple clients are connected via WebSocket, **When** a system event occurs, **Then** all connected clients receive the event broadcast simultaneously
4. **Given** a WebSocket client disconnects unexpectedly, **When** the connection is lost, **Then** the server cleans up resources and continues serving other clients
5. **Given** CORS restrictions, **When** a web-based dashboard makes API requests, **Then** appropriate CORS headers are returned to allow cross-origin access

---

### User Story 4 - Multi-Server Fleet Management (Priority: P3)

As a system integrator managing multiple Unraid servers, I want to query the same API endpoints across all my servers so that I can build unified management platforms that work consistently across my entire infrastructure.

**Why this priority**: This serves power users and commercial use cases but isn't essential for basic monitoring. The API's consistent design makes this possible without additional features.

**Independent Test**: Can be tested by deploying the agent to multiple Unraid servers and verifying API responses have identical structure and behavior. Delivers value for enterprise and multi-server scenarios.

**Acceptance Scenarios**:

1. **Given** the same API version is deployed on multiple servers, **When** clients query the same endpoints, **Then** response formats are identical across all servers
2. **Given** different hardware configurations on each server, **When** unavailable features are queried, **Then** graceful degradation occurs consistently
3. **Given** a management platform polling multiple servers, **When** requests are made simultaneously, **Then** each server responds independently without interference
4. **Given** API endpoints on multiple servers, **When** versions differ, **Then** the version information is clearly exposed in health check endpoints

---

### User Story 5 - Automated Response to System Events (Priority: P3)

As an automation engineer, I want to receive immediate notifications when system thresholds are exceeded so that I can trigger automated responses like restarting services or sending alerts.

**Why this priority**: This represents advanced automation capabilities that build on monitoring and control. It's valuable but not essential for basic use cases.

**Independent Test**: Can be tested by monitoring WebSocket events and triggering actions when specific conditions are met (e.g., high temperature, disk failure). Delivers value by enabling proactive system management.

**Acceptance Scenarios**:

1. **Given** a WebSocket client monitoring system events, **When** CPU temperature exceeds a threshold, **Then** the client receives the temperature update within 1 second
2. **Given** an automation script monitoring disk status, **When** a disk failure is detected, **Then** the script receives the disk status update immediately
3. **Given** multiple event types are being monitored, **When** events occur, **Then** each event type is clearly identified in the WebSocket message format
4. **Given** a long-running WebSocket connection, **When** no events occur for extended periods, **Then** the connection remains stable with heartbeat/ping-pong messages

---

### Edge Cases

- What happens when the Unraid server is under extremely high load (>95% CPU, >95% RAM)?
  - Monitoring endpoints should continue to respond (from cache) even if data collection intervals slow down
- What happens when a collector fails or encounters an error?
  - The system should log the error, continue running other collectors, and return partial data rather than failing completely
- What happens when WebSocket clients disconnect without proper cleanup?
  - The server should detect stale connections and clean them up automatically to prevent resource leaks
- What happens when control operations are issued for non-existent containers or VMs?
  - Clear error messages with HTTP 400/404 status codes should be returned with actionable information
- What happens when the system reads data from Unraid files that don't exist or have unexpected formats?
  - Graceful degradation with logging - return default/empty values rather than crashing
- What happens when multiple REST API clients query the same endpoint simultaneously?
  - Thread-safe cache access should serve all clients without data corruption or blocking
- What happens when hardware features (GPU, UPS, specific disk controllers) vary across deployments?
  - The system should detect hardware capabilities and gracefully indicate when features are unavailable

## Requirements

### Functional Requirements

#### Monitoring Capabilities

- **FR-001**: System MUST expose REST API endpoints that return current system metrics including CPU usage, RAM usage, system temperature, and uptime
- **FR-002**: System MUST expose REST API endpoints that return array status including array state, parity status, and disk counts
- **FR-003**: System MUST expose REST API endpoints that return per-disk information including SMART data, temperatures, and space usage
- **FR-004**: System MUST expose REST API endpoints that return network interface statistics including bandwidth, IP addresses, and connection status
- **FR-005**: System MUST expose REST API endpoints that return Docker container information including status, resource usage, and configuration
- **FR-006**: System MUST expose REST API endpoints that return virtual machine information including state and resource allocation
- **FR-007**: System MUST support optional hardware features (UPS, GPU) and return HTTP 200 with JSON indicating unavailable status when hardware is not detected (e.g., {"available": false, "reason": "Hardware not detected"})
- **FR-008**: System MUST return cached monitoring data from REST endpoints within 50ms
- **FR-009**: System MUST collect system metrics at specific intervals: System (5s), Array (10s), Disk (30s), Docker (10s), VM (10s), Network (15s), GPU (10s), UPS (10s), Shares (60s), Hardware (300s), ZFS (30s)

#### Real-Time Event Streaming

- **FR-010**: System MUST provide WebSocket endpoint that accepts client connections for real-time event streaming
- **FR-011**: System MUST broadcast collector updates to all connected WebSocket clients when data changes
- **FR-012**: System MUST implement WebSocket connection health monitoring using ping/pong messages with 30-second ping interval and 60-second timeout
- **FR-013**: System MUST gracefully handle WebSocket client disconnections and clean up resources
- **FR-014**: System MUST support up to 10 simultaneous WebSocket connections without performance degradation
- **FR-015**: System MUST use consistent JSON message format for all WebSocket events with event type identification

#### Control Operations

- **FR-016**: System MUST provide REST API endpoints to start, stop, restart, pause, and unpause Docker containers
- **FR-017**: System MUST provide REST API endpoints to start, stop, restart, pause, resume, and hibernate virtual machines
- **FR-018**: System MUST provide REST API endpoints to start and stop the Unraid array
- **FR-019**: System MUST validate all control operation inputs to prevent command injection and unauthorized access, and use 60-second timeout for shell command execution
- **FR-020**: System MUST log all control operations with timestamps and outcomes
- **FR-021**: System MUST return appropriate HTTP status codes (200 for success, 400 for client errors, 500 for server errors)
- **FR-022**: System MUST complete control operations within 5 seconds or provide appropriate timeout messaging

#### API Design and Integration

- **FR-023**: System MUST use RESTful principles with GET for queries and POST for control actions
- **FR-024**: System MUST return consistent JSON response formats across all endpoints
- **FR-025**: System MUST provide CORS headers to enable web-based dashboard access
- **FR-026**: System MUST expose a health check endpoint that returns service status, version information, and basic operational metrics (uptime, request count, error rate, cache hit rate)
- **FR-027**: System MUST maintain backward compatibility within a major version (add fields, don't remove existing ones); breaking changes require a new version in the URL path (e.g., /api/v1/ to /api/v2/)
- **FR-028**: System MUST support at least two concurrent API versions to allow gradual client migration when new versions are introduced
- **FR-029**: System MUST run on configurable port (default 8043)

#### Reliability and Error Handling

- **FR-030**: System MUST never crash due to individual collector failures
- **FR-031**: System MUST implement panic recovery in all concurrent operations
- **FR-032**: System MUST log errors with context (operation, parameters, error details)
- **FR-033**: System MUST gracefully degrade when optional hardware features are unavailable
- **FR-034**: System MUST handle high system load without becoming unresponsive
- **FR-035**: System MUST protect shared state with appropriate synchronization mechanisms
- **FR-036**: System MUST support graceful shutdown with resource cleanup

#### Security

- **FR-036**: System MUST validate all user-provided input before use in system operations
- **FR-037**: System MUST prevent path traversal attacks on file system operations
- **FR-038**: System MUST prevent command injection in control operations
- **FR-039**: System MUST log security-relevant events (control operations, validation failures)
- **FR-040**: System MUST NOT expose sensitive information in error messages

### Key Entities

- **System Metrics**: Represents current server health including CPU usage percentage, RAM usage percentage, temperature readings, uptime duration, and timestamp
- **Array Status**: Represents Unraid array state including operational state, number of disks, parity disk count, sync percentage, and parity validation status
- **Disk Information**: Represents individual disk details including identifier, device path, SMART health status, temperature, size, used space, and file system type
- **Network Interface**: Represents network adapter details including interface name, MAC address, IP address, link speed, operational state, bytes/packets sent/received, and error counts
- **Container Information**: Represents Docker container details including name/ID, image, current state, resource limits, and network configuration
- **Virtual Machine**: Represents VM details including name/ID, state, CPU allocation, RAM allocation, and disk configuration
- **UPS Status**: Represents uninterruptible power supply details including battery level, runtime estimate, power state, and load percentage
- **GPU Metrics**: Represents graphics processor details including utilization percentage, memory usage, temperature, and model information
- **User Share**: Represents Unraid share details including share name, path, used space, allocation method, and security settings
- **WebSocket Message**: Represents event notifications including event type, payload data, and timestamp
- **Control Operation Request**: Represents user command including target resource, action type, and parameters

## Success Criteria

### Measurable Outcomes

#### Performance and Reliability

- **SC-001**: REST API monitoring endpoints respond to queries in under 50ms for 99% of requests
- **SC-002**: System runs continuously for 30+ days without crashes or restarts
- **SC-003**: System handles at least 100 concurrent REST API clients without performance degradation
- **SC-004**: WebSocket events are delivered to clients within 1 second of data changes
- **SC-005**: Control operations complete within 5 seconds or provide clear progress indication
- **SC-006**: System gracefully handles collector failures with 100% availability of remaining features

#### User Experience

- **SC-007**: Developers can build a functional monitoring dashboard using API documentation without additional support
- **SC-008**: 95% of control operations complete successfully on first attempt
- **SC-009**: Error messages provide actionable information for troubleshooting (no cryptic errors)
- **SC-010**: API responses have consistent JSON structure across all endpoints (passes schema validation)
- **SC-011**: WebSocket clients can maintain connections for 24+ hours without disconnection

#### Integration and Compatibility

- **SC-012**: API works identically on different Unraid hardware configurations (different CPUs, disk controllers, network cards)
- **SC-013**: System coexists with official Unraid GraphQL API without conflicts or resource contention
- **SC-014**: Third-party tools can query the same server using both REST and WebSocket simultaneously
- **SC-015**: API version updates maintain backward compatibility (existing clients continue working)

#### Community and Adoption

- **SC-016**: At least 3 community-developed dashboard or monitoring tools successfully integrate with the API
- **SC-017**: Documentation enables self-service integration (measured by <5% of users requesting help)
- **SC-018**: Community contributors successfully fix hardware-specific compatibility issues
- **SC-019**: Plugin installation and initial setup takes under 10 minutes for typical users

#### Operational Excellence

- **SC-020**: System resource usage remains under 100MB RAM and <5% CPU under normal operation
- **SC-021**: Log files provide sufficient detail for troubleshooting without excessive verbosity
- **SC-022**: No security vulnerabilities discovered in input validation or command execution
- **SC-023**: Health check endpoint provides sufficient operational metrics to diagnose agent performance issues without external monitoring tools

## Assumptions

1. **Deployment Environment**: The system will run on Unraid 6.9+ servers with Linux/amd64 architecture
2. **Access Control**: Authentication and authorization are out of scope for this version; access control is managed at the network/firewall level
3. **Data Persistence**: Historical data storage is not included; users who want historical trends will use external time-series databases
4. **Network Reliability**: The system assumes reasonable network stability for WebSocket connections; reconnection logic is the client's responsibility
5. **Unraid File Structure**: The system assumes standard Unraid file locations and formats as documented in Unraid 6.9+
6. **Concurrent Users**: Typical usage will involve 1-10 concurrent clients per server; extreme scale (100+ clients) is not the primary use case
7. **Update Frequency**: System metrics changing every 5-30 seconds is acceptable for most monitoring use cases
8. **Client Capabilities**: API clients are assumed to be modern tools that understand JSON, REST, and WebSocket protocols
9. **Error Recovery**: Clients are responsible for implementing their own retry logic for failed requests
10. **Hardware Diversity**: The system will encounter varied hardware (different GPUs, UPS models, disk controllers) and must handle differences gracefully

## Out of Scope

The following capabilities are explicitly NOT included in this feature:

- **User Interface**: No web dashboard or graphical interface is provided; this is a backend API only
- **Authentication/Authorization**: No built-in user management, API keys, or access control mechanisms
- **Historical Data Storage**: No time-series database or data retention capabilities
- **Advanced Analytics**: No data aggregation, trending analysis, or predictive capabilities
- **Alerting System**: No built-in alerting, notification, or threshold monitoring features
- **Plugin Management**: No API endpoints for installing/removing Unraid plugins
- **Configuration Management**: Limited configuration endpoints; full Unraid configuration is out of scope
- **Data Export**: No bulk export or backup capabilities for monitoring data
- **Multi-Server Orchestration**: No built-in capabilities for coordinating actions across multiple Unraid servers
- **Non-Unraid Support**: No compatibility with other Linux distributions or NAS systems

## Dependencies

- **Unraid Operating System**: Must be running on Unraid 6.9 or later
- **Unraid System Files**: Depends on standard Unraid file locations (/var/local/emhttp/, /proc/, /sys/)
- **System Binaries**: Requires standard Linux utilities (docker, virsh, smartctl, etc.)
- **Network Port Availability**: Requires configurable port (default 8043) to be available
- **Go Runtime**: Built with Go 1.24 for Linux/amd64 architecture

## Notes

### Key Design Principles

1. **Reliability Over Features**: The system must never crash production servers, even when individual components fail
2. **Security First**: All user inputs must be validated; no command injection or path traversal vulnerabilities
3. **Event-Driven Architecture**: PubSub pattern ensures decoupled components with real-time data flow
4. **Thread Safety**: All shared state is properly synchronized; no data races
5. **Simplicity**: Clear separation of concerns, consistent patterns, no premature optimization

### Implementation Constraints

- Must run as a single binary with no external dependencies
- Must not interfere with Unraid core operations
- Must be lightweight (<100MB RAM, <5% CPU)
- Must support graceful shutdown with proper cleanup
- Must log to rotating log files with size limits

### Community Considerations

- This is a third-party plugin, NOT an official Unraid product
- Hardware variations will cause compatibility issues that need community fixes
- Documentation must enable self-service integration
- Contributions are welcomed and encouraged
