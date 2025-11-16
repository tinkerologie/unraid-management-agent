# Changelog

All notable changes to the Unraid Management Agent will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added

### Changed

### Fixed

### Removed

---

## [2025.11.13] - 2025-11-16

### Added

- **Enhanced Share List API** (Issue #6): Eliminated N+1 query problem for share information
  - Share list endpoint `/api/v1/shares` now includes configuration details directly
  - New fields in ShareInfo DTO:
    - `comment` - Share comment/description from config
    - `smb_export` - Boolean indicating if share is exported via SMB
    - `nfs_export` - Boolean indicating if share is exported via NFS
    - `storage` - Storage location: "cache", "array", "cache+array", or "unknown"
    - `use_cache` - Cache usage setting: "yes", "no", "only", "prefer"
    - `security` - Security setting: "public", "private", "secure"
  - Share collector automatically enriches shares with config data
  - SMB export detection based on security settings and export field
  - NFS export detection based on export field
  - Storage location determined from UseCache setting
  - All new fields use `omitempty` for backward compatibility
  - Single API call now provides complete share information matching Unraid UI
  - Graceful error handling: shares without config files return basic info only

### Changed

- Share collector now reads individual share config files during collection
- Share enrichment happens automatically for all shares in the list

### Performance

- Share collection remains at 60-second interval
- Config file reads are lightweight and fast
- No performance impact from enrichment process

---

## [2025.11.12] - 2025-11-16

### Added

- **Hardware Information API** (Issue #5): Comprehensive hardware details via dmidecode and ethtool
  - New `/api/v1/hardware/*` endpoints exposing detailed hardware information
  - `/api/v1/hardware/full` - Complete hardware information
  - `/api/v1/hardware/bios` - BIOS information (vendor, version, release date, characteristics)
  - `/api/v1/hardware/baseboard` - Motherboard information (manufacturer, product name, version, serial number)
  - `/api/v1/hardware/cpu` - CPU hardware details (socket, manufacturer, family, max speed, core/thread count, voltage)
  - `/api/v1/hardware/cache` - CPU cache information (L1/L2/L3 cache levels, size, type, associativity)
  - `/api/v1/hardware/memory-array` - Memory array information (location, max capacity, error correction, number of devices)
  - `/api/v1/hardware/memory-devices` - Individual memory module details (size, speed, manufacturer, part number, type)
  - Hardware collector runs every 5 minutes (hardware information is static)
  - All hardware data is cached and broadcast via WebSocket for real-time updates

- **Enhanced System Information**:
  - `HVMEnabled` - Hardware virtualization support (Intel VT-x/AMD-V detection via /proc/cpuinfo)
  - `IOMMUEnabled` - IOMMU support detection (kernel command line and /sys/class/iommu/)
  - `OpenSSLVersion` - OpenSSL version information
  - `KernelVersion` - Linux kernel version
  - `ParityCheckSpeed` - Current parity check speed from var.ini

- **Enhanced Network Information** via ethtool:
  - `SupportedPorts` - Supported port types (TP, AUI, MII, Fibre, etc.)
  - `SupportedLinkModes` - Supported link speeds and modes
  - `SupportedPauseFrame` - Pause frame support
  - `SupportsAutoNeg` - Auto-negotiation support
  - `SupportedFECModes` - Forward Error Correction modes
  - `AdvertisedLinkModes` - Advertised link speeds and modes
  - `AdvertisedPauseFrame` - Advertised pause frame use
  - `AdvertisedAutoNeg` - Advertised auto-negotiation
  - `AdvertisedFECModes` - Advertised FEC modes
  - `Duplex` - Duplex mode (Full/Half)
  - `AutoNegotiation` - Auto-negotiation status (on/off)
  - `Port` - Port type (Twisted Pair, Fibre, etc.)
  - `PHYAD` - PHY address
  - `Transceiver` - Transceiver type (internal/external)
  - `MDIX` - MDI-X status (on/off/Unknown)
  - `SupportsWakeOn` - Supported Wake-on-LAN modes
  - `WakeOn` - Current Wake-on-LAN setting
  - `MessageLevel` - Driver message level
  - `LinkDetected` - Link detection status
  - `MTU` - Maximum Transmission Unit

- **New Libraries**:
  - `daemon/lib/dmidecode.go` - Parser for dmidecode output (SMBIOS/DMI types 0, 2, 4, 7, 16, 17)
  - `daemon/lib/ethtool.go` - Parser for ethtool output with comprehensive network interface details

- **New DTOs**:
  - `HardwareInfo` - Container for all hardware information
  - `BIOSInfo` - BIOS/UEFI information
  - `BaseboardInfo` - Motherboard/baseboard information
  - `CPUHardwareInfo` - CPU hardware specifications
  - `CPUCacheInfo` - CPU cache level information
  - `MemoryArrayInfo` - Memory array/controller information
  - `MemoryDeviceInfo` - Individual memory module information

### Changed

- **System Collector**: Enhanced with virtualization and additional system information
  - Added `isHVMEnabled()` method to detect hardware virtualization support
  - Added `isIOMMUEnabled()` method to detect IOMMU support
  - Added `getOpenSSLVersion()` method to retrieve OpenSSL version
  - Added `getKernelVersion()` method to retrieve kernel version
  - Added `getParityCheckSpeed()` method to parse parity check speed from var.ini

- **Network Collector**: Enhanced with ethtool integration
  - Added `enrichWithEthtool()` method to populate network interface details
  - Network information now includes comprehensive ethtool data when available
  - Gracefully handles cases where ethtool is not available or fails

- **Orchestrator**: Updated to manage hardware collector
  - Increased collector count from 9 to 10
  - Hardware collector initialized and started with 5-minute interval

- **API Server**: Updated to cache and serve hardware information
  - Added `hardwareCache` field to Server struct
  - Subscribed to `hardware_update` events
  - Hardware events broadcast to WebSocket clients

---

## [2025.11.11] - 2025-11-08

### Fixed

- **VM CPU Percentage Tracking**: Implemented proper CPU percentage calculation for VMs
  - Added historical tracking to VM collector using `cpuStats` struct with mutex protection
  - Guest CPU % now calculated from `virsh domstats` CPU time deltas over time intervals
  - Host CPU % now calculated from QEMU process CPU usage via `/proc/[pid]/stat`
  - CPU percentages are calculated as: `(current_time - previous_time) / time_interval / num_vcpus * 100`
  - Percentages are clamped to valid range [0, 100] to handle edge cases
  - CPU stats history is automatically cleared when VMs are shut off
  - First measurement after VM start returns 0% (requires two measurements for delta calculation)
  - Subsequent measurements return accurate real-time CPU percentages
  - Host CPU % matches the percentage shown in `ps`/`top` for the QEMU process
  - Guest CPU % represents the percentage of allocated vCPUs being used inside the guest OS

### Changed

- **VM Collector**: Enhanced with CPU tracking infrastructure
  - Added `cpuStats` struct to store guest CPU time, host CPU time, and timestamp
  - Added `previousStats` map with mutex for thread-safe historical tracking
  - Added `getGuestCPUTime()` method using `virsh domstats --cpu-total`
  - Added `getHostCPUTime()` method reading `/proc/[pid]/stat`
  - Added `getQEMUProcessPID()` method using `pgrep` to find QEMU process
  - Added `clearCPUStats()` method to reset tracking when VMs are shut off
  - Updated `getVMCPUUsage()` to accept `numVCPUs` parameter and calculate real percentages

### Removed

- **Placeholder CPU Values**: Removed the "needs historical data" limitation
  - CPU percentage fields now return real values instead of always returning 0
  - Removed placeholder comments about needing historical data implementation

---

## [2025.11.10] - 2025-11-08

### Added

- **Enhanced VM Statistics**: Added comprehensive VM monitoring metrics
  - Guest CPU usage percentage (placeholder for future implementation with historical data)
  - Host CPU usage percentage (placeholder for future implementation with historical data)
  - Memory display in human-readable format (e.g., "1.17 GB / 4.00 GB")
  - Disk I/O statistics: total read and write bytes across all VM disks
  - Network I/O statistics: total RX and TX bytes across all VM network interfaces
  - New DTO fields: `guest_cpu_percent`, `host_cpu_percent`, `memory_display`, `disk_read_bytes`, `disk_write_bytes`, `network_rx_bytes`, `network_tx_bytes`

- **Enhanced Docker Container Statistics**: Added comprehensive container monitoring metrics
  - Container version extracted from image tag
  - Network mode (e.g., "bridge", "host", "none")
  - Container IP address
  - Port mappings in "host_port:container_port" format
  - Volume mappings with host path, container path, and mode (rw/ro)
  - Restart policy (e.g., "always", "unless-stopped", "on-failure", "no")
  - Container uptime in human-readable format (e.g., "2d 5h 30m", "3h 45m", "15m")
  - Memory display in human-readable format (e.g., "512.00 MB / 2.00 GB")
  - New DTO fields: `version`, `network_mode`, `ip_address`, `port_mappings`, `volume_mappings`, `restart_policy`, `uptime`, `memory_display`

### Changed

- **VM Collector**: Enhanced data collection using additional virsh commands
  - Added `getVMCPUUsage()` method using `virsh cpu-stats` (returns 0 pending historical data implementation)
  - Added `getVMDiskIO()` method using `virsh domblklist` and `virsh domblkstat`
  - Added `getVMNetworkIO()` method using `virsh domiflist` and `virsh domifstat`
  - Added `formatMemoryDisplay()` helper for human-readable memory formatting

- **Docker Collector**: Enhanced data collection using docker inspect
  - Added `getContainerDetails()` method using `docker inspect` for comprehensive container metadata
  - Added `formatUptime()` helper for human-readable uptime formatting
  - Added `formatMemoryDisplay()` helper for human-readable memory formatting
  - Container details now include network configuration, volume mappings, and restart policies

---

## [2025.11.9] - 2025-11-08

### Fixed

- **VM Collector**: Fixed parsing of VM names containing spaces
  - Changed from parsing `virsh list --all` column-based output to using `virsh list --all --name`
  - Added `getVMState()` helper method to get VM state using `virsh domstate <name>`
  - Added `getVMID()` helper method to get VM ID using `virsh domid <name>`
  - Now correctly handles VM names with spaces, hyphens, underscores, and special characters
  - Example: "Windows Server 2016" is now correctly parsed instead of being split into "Windows" and "Server 2016 running"

- **VM Control API**: Fixed VM control endpoints to work with VM names containing spaces
  - Updated VM name validation regex to allow spaces: `^[a-zA-Z0-9 _.-]{1,253}$`
  - Fixed route parameter mismatch: changed VM control routes from `{id}` to `{name}`
  - VM control endpoints now correctly accept URL-encoded spaces (e.g., `Windows%20Server%202016`)
  - All VM operations (start, stop, restart, pause, resume, hibernate, force-stop) now work with spaces in VM names

---

## [2025.11.8] - 2025-11-08

### Added

- **User Scripts API**: New REST API endpoints for discovering and executing Unraid User Scripts
  - GET `/api/v1/user-scripts` - List all available user scripts with metadata
  - POST `/api/v1/user-scripts/{name}/execute` - Execute a user script with background/wait options
  - Supports reading script descriptions from the `description` file
  - Includes path traversal protection and input validation
  - Returns script metadata: name, description, path, executable status, last modified timestamp
  - Execution options: `background` (default: true), `wait` (default: false)
  - Enables automation tools like Home Assistant to remotely execute Unraid maintenance scripts

---

## [2025.11.7] - 2025-11-08

### Changed

- **README.md Simplified**: Reduced README.md to essential information only
  - Removed detailed feature lists, API endpoints, and support links
  - Kept only the plugin name heading and brief description
  - Maintains proper display name format for Plugin Manager
  - Reduces file size while preserving functionality

---

## [2025.11.6] - 2025-11-08

### Fixed

- **Plugin Display Name**: Plugin now displays as "Unraid Management Agent" in the Unraid Plugin Manager instead of "unraid-management-agent"
  - Added README.md file to plugin directory with proper display name formatting
  - Follows Unraid plugin naming conventions used by Community Applications and other established plugins
  - Settings menu display name remains unchanged (was already correct)
  - Improves user experience and plugin discoverability in the Plugin Manager

---

## [2025.11.5] - 2025-11-08

### Added

- **USB Flash Drive Detection**: Plugin now detects USB flash drives (including the Unraid boot drive) and skips SMART data collection
  - Checks device sysfs path to identify USB transport
  - Detects Unraid boot drive by checking if device is mounted at `/boot`
  - Avoids unnecessary SMART commands on devices that don't support SMART monitoring
  - SMART status remains "UNKNOWN" for USB flash drives (consistent with previous behavior)
  - Adds debug logging to indicate when USB flash drive detection occurs

### Changed

- **NVMe-Specific SMART Collection**: Optimized SMART data collection for NVMe drives
  - NVMe drives are now detected by checking device name pattern (e.g., `nvme0n1`)
  - NVMe drives skip the `-n standby` flag since they don't support standby mode
  - Uses `smartctl -H /dev/{device}` for NVMe drives (without `-n standby`)
  - SATA/SAS drives continue to use `smartctl -n standby -H /dev/{device}` (existing behavior)
  - Adds debug logging to indicate device type detection (NVMe vs SATA/SAS)
  - Improves efficiency by avoiding unnecessary standby checks on NVMe drives

---

## [2025.11.4] - 2025-11-08

### Fixed

- **CRITICAL FIX**: Disk spin-down compatibility - Plugin now respects Unraid's disk spin-down settings
  - Changed SMART data collection to use `smartctl -n standby` flag
  - Disks in standby mode are no longer woken up for SMART health checks
  - Previous implementation was preventing disks from spinning down by accessing them every 30 seconds
  - SMART status is now only collected when disks are already active
  - Preserves power savings and reduces disk wear for users with spin-down configured
  - Fixes critical issue where plugin prevented Unraid's disk spin-down functionality from working

---

## [2025.11.3] - 2025-11-08

### Changed

- Improved plugin UI/UX in Unraid interface
  - Added clickable settings link to plugin icon
  - Updated settings page title to "Unraid Management Agent"
  - Added server icon to plugin listing

### Fixed

- Settings page URL now uses correct lowercase-with-hyphens format
  - Changed from `/Settings/UnraidManagementAgent` to `/Settings/unraid-management-agent`
  - Plugin icon now navigates to the correct settings page URL
- **CRITICAL FIX**: SMART health status now correctly retrieved by running `smartctl -H` directly
  - Previous implementation tried to read from Unraid's cached files which use disk names instead of device names
  - Cached files also don't include the health status line
  - Now executes `smartctl -H /dev/{device}` to get actual health status
  - Fixes issue where all disks showed `smart_status: "UNKNOWN"` (Issue #4)

---

## [2025.11.2] - 2025-11-07

### Added

- Automated CI/CD workflow for releases using GitHub Actions
  - Automatically builds release package when Git tag is pushed
  - Calculates MD5 checksum and includes in release notes
  - Creates GitHub release with .tgz file attached
  - Extracts release notes from CHANGELOG.md
  - Supports pre-release detection (alpha, beta, rc versions)

### Fixed

- SMART health status now correctly reported in `/api/v1/disks` endpoint (#4)
  - Parses actual SMART health status from Unraid's cached smartctl output
  - Returns "PASSED" for healthy disks (SATA/SAS drives)
  - Returns "PASSED" for healthy NVMe drives (normalizes "OK" to "PASSED")
  - Returns actual status values like "FAILED" when SMART tests fail
  - No longer returns "UNKNOWN" for all disks when SMART data is available

---

## [2025.11.1] - 2025-11-07

### Added

- Docker vDisk usage monitoring in `/api/v1/disks` endpoint (#2)
  - Automatically detects Docker vDisk at `/var/lib/docker` mount point
  - Reports size, used, free bytes, and usage percentage
  - Identifies vDisk file path (e.g., `/mnt/user/system/docker/docker.vdisk`)
  - Includes filesystem type detection
  - Assigned role `docker_vdisk` for easy filtering
  - Enables monitoring of Docker storage capacity for alerts

- Log filesystem usage monitoring in `/api/v1/disks` endpoint (#3)
  - Automatically detects log filesystem at `/var/log` mount point
  - Reports size, used, free bytes, and usage percentage
  - Identifies device name (e.g., `tmpfs` for RAM-based log storage)
  - Includes filesystem type detection (tmpfs, ext4, xfs, etc.)
  - Assigned role `log` for easy filtering
  - Enables monitoring of log storage capacity to prevent system failures
  - Critical for tmpfs-based log filesystems that can fill up and cause issues

### Fixed

- UPS API endpoint now returns actual UPS model name instead of hostname (#1)

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

- **GitHub Repository**: <https://github.com/ruaan-deysel/unraid-management-agent>
- **Documentation**: [docs/README.md](docs/README.md)
- **API Reference**: [docs/api/API_REFERENCE.md](docs/api/API_REFERENCE.md)
- **Issues**: <https://github.com/ruaan-deysel/unraid-management-agent/issues>

---

**Last Updated**: 2025-11-03
**Current Version**: 2025.11.0
