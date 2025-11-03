# Changelog

All notable changes to the Unraid Management Agent will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [2025.11.0] - 2025-11-03

### Added

#### Enhanced System Information Collector
- **CPU Model Detection**: Automatic detection of CPU model, cores, threads, and frequency from `/proc/cpuinfo`
- **BIOS Information**: Server model, BIOS version, and BIOS release date via `dmidecode`
- **Per-Core CPU Usage**: Individual CPU core usage monitoring with `cpu_per_core_usage` field
- **Server Model Identification**: Hardware model detection for better system identification

#### Detailed Disk Metrics
- **I/O Statistics**: Read/write operations and bytes per disk from `/sys/block/{device}/stat`
  - `read_ops` - Total read operations
  - `read_bytes` - Total bytes read
  - `write_ops` - Total write operations
  - `write_bytes` - Total bytes written
  - `io_utilization_percent` - Disk I/O utilization percentage
- **Disk Spin State Detection**: Enhanced spin state detection (active, standby, unknown)
- **Per-Disk Performance Metrics**: Real-time performance monitoring for each disk

### Changed

#### Documentation Updates
- **README.md Roadmap Reorganization**:
  - Added "Recently Implemented ✅" section to highlight completed features
  - Moved Enhanced System Info Collector and Detailed Disk Metrics from planned to implemented
  - Updated "Planned Enhancements" to only include outstanding features
  - Added detailed sub-items for each implemented feature
- **Third-Party Plugin Notice**: Added prominent disclaimer distinguishing this plugin from official Unraid API
- **System Compatibility Section**: Added hardware compatibility notice and tested configuration details
- **Contributing Guidelines**: Expanded contribution workflow for hardware compatibility fixes
- **Version References**: Updated from Unraid 6.x to 7.x throughout documentation

#### Configuration Management
- **Log Rotation**: Implemented automatic log rotation with 5 MB max file size (using lumberjack.v2)
- **Log Level Support**: Added configurable log levels (DEBUG, INFO, WARNING, ERROR) with `--log-level` CLI flag
- **Default Log Level**: Set to WARNING for production to minimize disk usage
- **Configuration File Management**: Improved config file creation and synchronization
- **Auto-Start Behavior**: Service now always starts automatically when Unraid array starts (removed toggle option)

### Fixed
- **Configuration Synchronization**: Fixed LOG_LEVEL not being read from config file
- **Start Script**: Now properly creates config directory and default config file
- **Deployment Script**: Updated to use start script instead of bypassing configuration

### Testing
- **Test Suite**: All 66 tests passing across 3 packages (100% pass rate)
- **Deployment Verification**: Successfully deployed and verified on Unraid 7.x server

---

## [2025.10.03] - Initial Release

### Added

#### Phase 1 & 2 API Enhancements

**Phase 1.1: Array Control Operations**
- `POST /api/v1/array/start` - Start the Unraid array
- `POST /api/v1/array/stop` - Stop the Unraid array
- `POST /api/v1/array/parity-check/start` - Start parity check (with correcting option)
- `POST /api/v1/array/parity-check/stop` - Stop parity check
- `POST /api/v1/array/parity-check/pause` - Pause parity check
- `POST /api/v1/array/parity-check/resume` - Resume parity check
- `GET /api/v1/array/parity-check/history` - Get parity check history from log file
- New `ParityCollector` to parse `/boot/config/parity-checks.log`
- New `ParityCheckRecord` and `ParityCheckHistory` DTOs

**Phase 1.2: Single Resource Endpoints**
- `GET /api/v1/disks/{id}` - Get single disk by ID, device, or name
- `GET /api/v1/docker/{id}` - Get single container by ID or name
- `GET /api/v1/vm/{id}` - Get single VM by ID or name
- Support for multiple identifier types (ID, name, device)
- 404 error handling for missing resources

**Phase 1.3: Enhanced Disk Details**
- Added `serial_number` field to DiskInfo DTO
- Added `model` field to DiskInfo DTO
- Added `role` field to DiskInfo DTO (parity, parity2, data, cache, pool)
- Added `spin_state` field to DiskInfo DTO (active, standby, unknown)
- Automatic role detection based on disk name/ID
- Spin state detection based on temperature
- SMART data extraction for serial number and model

**Phase 2.1: Read-Only Configuration Endpoints**
- `GET /api/v1/shares/{name}/config` - Get share configuration
- `GET /api/v1/network/{interface}/config` - Get network interface configuration
- `GET /api/v1/settings/system` - Get system settings
- `GET /api/v1/settings/docker` - Get Docker settings
- `GET /api/v1/settings/vm` - Get VM Manager settings
- New `ShareConfig`, `NetworkConfig`, `SystemSettings`, `DockerSettings`, `VMSettings` DTOs
- New `ConfigCollector` with parsers for all config files

**Phase 2.2: Configuration Write Endpoints**
- `POST /api/v1/shares/{name}/config` - Update share configuration
- `POST /api/v1/settings/system` - Update system settings
- Automatic backup creation before config updates (.bak files)
- JSON request body validation
- Error handling for write operations

#### Disk Settings Feature

- `GET /api/v1/settings/disks` - Get global disk settings
- New `DiskSettings` DTO with fields:
  - `spindown_delay_minutes` - Default spin down delay (critical for Home Assistant)
  - `start_array` - Auto start array on boot
  - `spinup_groups` - Enable spinup groups
  - `shutdown_timeout_seconds` - Shutdown timeout
  - `default_filesystem` - Default filesystem type
- Reads from `/boot/config/disk.cfg`
- Enables Home Assistant to avoid waking spun-down disks

#### Documentation

- Complete API reference guide (`docs/api/API_REFERENCE.md`)
- API coverage analysis (`docs/api/API_COVERAGE_ANALYSIS.md`)
- WebSocket events documentation (`docs/WEBSOCKET_EVENTS_DOCUMENTATION.md`)
- WebSocket event structure guide (`docs/WEBSOCKET_EVENT_STRUCTURE.md`)
- Phase 1 & 2 implementation report (`docs/implementation/PHASE_1_2_IMPLEMENTATION_REPORT.md`)
- Disk settings implementation report (`docs/implementation/DISK_SETTINGS_IMPLEMENTATION.md`)
- Deployment guides (`docs/deployment/`)
- Documentation index (`docs/README.md`)

#### Core Features

- Comprehensive system monitoring (CPU, memory, uptime, temperature)
- Array status monitoring (state, parity, disk counts)
- Per-disk metrics (SMART data, temperature, space usage, spin state)
- Network interface monitoring (bandwidth, IP addresses, MAC addresses)
- Docker container monitoring and control
- Virtual machine monitoring and control
- UPS status monitoring
- GPU metrics monitoring
- User share monitoring
- REST API with 46 endpoints
- WebSocket support with 9 event types
- Event-driven architecture with pubsub
- Graceful shutdown and panic recovery
- Automatic log rotation

### Changed

- Updated deployment script to include `--port 8043` flag
- Improved error handling in collectors
- Enhanced logging with structured messages
- Optimized data collection intervals

### Fixed

- Fixed logger method calls (changed `logger.Warn` to `logger.Debug`)
- Fixed API server port configuration
- Fixed icon display in Unraid Plugins page
- Fixed backup creation in deployment script

### Deployment

- Deployed to live Unraid server (192.168.20.21)
- All endpoints tested and verified
- Service running on port 8043
- Icon fix verified in Unraid UI

---

## API Endpoint Summary

### Total Endpoints: 46

**Monitoring** (13):
- GET /api/v1/health
- GET /api/v1/system
- GET /api/v1/array
- GET /api/v1/disks
- GET /api/v1/disks/{id}
- GET /api/v1/shares
- GET /api/v1/docker
- GET /api/v1/docker/{id}
- GET /api/v1/vm
- GET /api/v1/vm/{id}
- GET /api/v1/ups
- GET /api/v1/gpu
- GET /api/v1/network

**Control** (19):
- POST /api/v1/docker/{id}/start
- POST /api/v1/docker/{id}/stop
- POST /api/v1/docker/{id}/restart
- POST /api/v1/docker/{id}/pause
- POST /api/v1/docker/{id}/unpause
- POST /api/v1/vm/{id}/start
- POST /api/v1/vm/{id}/stop
- POST /api/v1/vm/{id}/restart
- POST /api/v1/vm/{id}/pause
- POST /api/v1/vm/{id}/resume
- POST /api/v1/vm/{id}/hibernate
- POST /api/v1/vm/{id}/force-stop
- POST /api/v1/array/start
- POST /api/v1/array/stop
- POST /api/v1/array/parity-check/start
- POST /api/v1/array/parity-check/stop
- POST /api/v1/array/parity-check/pause
- POST /api/v1/array/parity-check/resume
- GET /api/v1/array/parity-check/history

**Configuration** (13):
- GET /api/v1/shares/{name}/config
- POST /api/v1/shares/{name}/config
- GET /api/v1/network/{interface}/config
- GET /api/v1/settings/system
- POST /api/v1/settings/system
- GET /api/v1/settings/docker
- GET /api/v1/settings/vm
- GET /api/v1/settings/disks

**WebSocket** (1):
- GET /api/v1/ws

---

## API Coverage

| Category | Coverage | Status |
|----------|----------|--------|
| **Overall** | **60%** | 🟡 Partial |
| Monitoring | 85% | ✅ Good |
| Control Operations | 75% | ✅ Good |
| Configuration | 40% | 🟡 Partial |
| Administration | 0% | 🔴 None |

---

## WebSocket Events

**Total Event Types**: 9

- `system` - System metrics updates
- `array` - Array status changes
- `disk` - Disk status changes
- `docker` - Docker container events
- `vm` - VM state changes
- `ups` - UPS status updates
- `gpu` - GPU metrics updates
- `network` - Network statistics updates
- `share` - Share information updates

---

## Known Issues

None at this time.

---

## Planned Features

### Phase 3: Advanced Configuration
- Network configuration write endpoint
- Docker settings write endpoint
- VM settings write endpoint
- Per-disk spindown override settings

### Phase 4: User Management
- User list endpoint
- User permissions endpoint
- User creation/modification endpoints

### Phase 5: Plugin Management
- Plugin list endpoint
- Plugin install/update/remove endpoints
- Plugin settings endpoints

### Future Enhancements
- Historical data storage
- Alerting and notification system
- Network statistics trending
- Enhanced SMART attribute monitoring

---

## Migration Guide

### From Pre-1.0.0 Versions

This is the initial release. No migration required.

---

## Contributors

- Ruaan Deysel (@ruaan-deysel)

---

## Links

- **GitHub Repository**: https://github.com/ruaan-deysel/unraid-management-agent
- **Documentation**: [docs/README.md](docs/README.md)
- **API Reference**: [docs/api/API_REFERENCE.md](docs/api/API_REFERENCE.md)
- **Issues**: https://github.com/ruaan-deysel/unraid-management-agent/issues

---

**Last Updated**: 2025-11-03
**Current Version**: 2025.11.0

