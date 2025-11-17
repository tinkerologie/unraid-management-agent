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

## [2025.11.23] - 2025-11-17

### Changed

- **Dependency Updates**:
  - Updated `github.com/alecthomas/kong` from v0.9.0 to v1.13.0
  - Updated `golang.org/x/sys` from v0.13.0 to v0.38.0
  - Upgraded Go language version from 1.23 to 1.24.0 (required by golang.org/x/sys v0.38.0)
  - Migrated from deprecated `github.com/vaughan0/go-ini` (last updated 2013) to actively maintained `gopkg.in/ini.v1` v1.67.0
  - All dependencies now use modern, actively maintained libraries

- **Code Quality Improvements**:
  - Fixed all pre-existing linting issues (now 0 errors, 0 warnings)
  - Converted if-else chains to switch statements for better readability (gocritic)
  - Reduced cyclomatic complexity by extracting helper functions (gocyclo)
  - Renamed `daemon/common` package to `daemon/constants` for better clarity (revive)
  - Improved code maintainability and adherence to Go best practices

### Technical Details

- **Linting Fixes**:
  - `daemon/lib/dmidecode.go`: Converted cache level parsing to switch statement
  - `daemon/lib/ethtool.go`: Extracted `parseEthtoolKeyValue()` helper (complexity 34 → 18)
  - `daemon/services/collectors/disk.go`: Extracted `parseDisksINI()`, `parseDiskKeyValue()`, `enrichDisks()` helpers (complexity 32 → 12)
  - `daemon/services/collectors/disk.go`: Converted disk role determination to switch statement
  - `daemon/services/collectors/gpu.go`: Converted Intel GPU error handling to switch statement

- **INI Library Migration**:
  - Updated `daemon/lib/parser.go`: Refactored `ParseINIFile()` to use new API
  - Updated `daemon/services/collectors/array.go`: Migrated `collectArrayStatus()` and `countParityDisks()`
  - Updated `daemon/services/collectors/registration.go`: Migrated `collectRegistration()`
  - API changes: `ini.LoadFile()` → `ini.Load()`, `file.Get()` → `section.HasKey()` + `section.Key().String()`

All tests pass successfully. Builds verified for local and release targets.

---

## [2025.11.22] - 2025-11-17

### Added

- **Management Agent Version Field** (Issue #26):
  - Added `agent_version` field to `/api/v1/system` endpoint
  - Returns the Unraid Management Agent plugin version (e.g., "2025.11.22")
  - Complements the `version` field which returns the Unraid OS version
  - Enables API clients to:
    - Implement agent-specific compatibility checks
    - Detect features based on agent capabilities
    - Display complete version information in diagnostics
    - Track agent updates independently from OS updates
  - Improves Home Assistant integration and other API clients
  - Version is automatically populated from the VERSION file during build

---

## [2025.11.21] - 2025-11-17

### Fixed

- **Motherboard Temperature API** (Issue #24):
  - Fixed motherboard temperature returning 0 instead of actual value
  - Improved sensor parser to capture sensor labels (e.g., "MB Temp") from `sensors -u` output
  - Updated temperature matching logic to correctly identify motherboard temperature sensor
  - Motherboard temperature now displays actual readings (e.g., 45°C) instead of 0°C
  - Affects Home Assistant integration and other API clients relying on temperature data

- **Unraid OS Version Field** (Issue #25):
  - Fixed system API returning empty string for `version` field
  - Added `getUnraidVersion()` function to read Unraid OS version from `/etc/unraid-version`
  - Fallback to `/var/local/emhttp/var.ini` if primary version file is unavailable
  - Version field now correctly displays Unraid OS version (e.g., "7.2.0")
  - Improves device information display in Home Assistant and other integrations
  - Enables version-specific feature detection and compatibility checks

---

## [2025.11.20] - 2025-11-16

### Fixed

- **VM Endpoint ID Field** (Issue #22):
  - Fixed VM endpoint returning empty string for `id` field
  - Changed from using `virsh domid` (runtime ID) to `virsh domuuid` (persistent UUID)
  - VM IDs are now stable, unique identifiers that work for all VM states (running, shut off, paused)
  - Provides consistent identification for API clients and automation systems
  - Falls back to VM name if UUID is not available

- **Array Parity Status** (Issue #21):
  - Fixed incorrect parity status reporting (`parity_valid: false` when parity is actually valid)
  - Fixed incorrect parity disk count (`num_parity_disks: 0` when parity disks exist)
  - Now correctly counts parity disks from `disks.ini` (field `type="Parity"`)
  - Improved parity validity logic to check `sbSynced` timestamp and `sbSyncErrs` count
  - Parity is marked valid only when:
    - At least one parity disk exists
    - Parity has been synced (sbSynced is non-zero timestamp)
    - No parity errors exist (sbSyncErrs is 0)

### Technical Details

- **VM Collector**: Updated `getVMID()` to use `virsh domuuid` for stable UUID-based identification
- **Array Collector**: Added `countParityDisks()` function to parse disks.ini and count parity disks
- **Parity Logic**: Improved validation to check timestamp-based sync status instead of yes/no flag
- **Compatibility**: Both fixes maintain backward compatibility with existing API clients

---

## [2025.11.19] - 2025-11-16

### Fixed

- **Code Quality Improvements**:
  - Resolved all critical linting warnings across the entire codebase (93% reduction from 302 to 22 warnings)
  - Added comprehensive godoc comments to all exported types, functions, and methods
  - Fixed all gosec security warnings with proper justifications
  - Fixed revive warnings (indent-error-flow, empty-block)
  - Fixed staticcheck warnings (unnecessary nil checks)
  - Fixed unconvert warnings (unnecessary type conversions)
  - Fixed ineffassign warnings (ineffectual assignments)
  - Achieved zero tolerance policy compliance for all critical linting issues

### Technical Details

- **Files Modified**: 21 files across collectors, controllers, API server, and main package
- **Linting Compliance**: Zero critical errors, zero critical warnings
- **Documentation**: All exported symbols now have proper godoc comments
- **Security**: All gosec warnings properly justified with #nosec comments
- **Code Style**: Removed unnecessary else blocks, nil checks, and type conversions

---

## [2025.11.18] - 2025-11-16

### Added

- **Unassigned Devices Plugin Support** (Issue #7):
  - Complete support for Unassigned Devices plugin integration
  - New `/api/v1/unassigned` endpoint to list all unassigned devices and remote shares
  - New `/api/v1/unassigned/devices` endpoint for unassigned devices only
  - New `/api/v1/unassigned/remote-shares` endpoint for remote shares only
  - Automatic detection of unassigned disk devices (USB drives, eSATA, internal disks not in array)
  - Support for remote SMB/NFS shares and ISO file mounts
  - Per-device information: serial number, model, partitions, mount status, spin state
  - Per-partition information: label, filesystem, mount point, size, usage, SMB/NFS share status
  - Automatic filtering of array disks, loop devices, md devices, and zram
  - Real-time monitoring with 30-second collection interval
  - WebSocket real-time updates when unassigned devices change

### Technical Details

- **New DTOs**: `daemon/dto/unassigned.go` - UnassignedDevice, UnassignedPartition, UnassignedRemoteShare, UnassignedDeviceList structures
- **New Collector**: `daemon/services/collectors/unassigned.go` - Unassigned devices collector with lsblk integration
- **Device Discovery**: Uses lsblk to enumerate all block devices and filters out array disks
- **Array Disk Detection**: Parses `/var/local/emhttp/disks.ini` to identify array disks
- **Partition Information**: Collects filesystem type, mount point, size, usage for each partition
- **Remote Share Support**: Detects mounted ISO files from `/proc/mounts`
- **API Integration**: Added 3 new monitoring endpoints for unassigned devices
- **WebSocket Events**: Real-time `unassigned_devices_update` events for connected clients
- **Collection Interval**: 30 seconds for device discovery and status updates

---

## [2025.11.17] - 2025-11-16

### Added

- **Unraid Notifications System** (Issue #10):
  - Complete notification management system with file monitoring
  - New `/api/v1/notifications` endpoint to list all notifications with overview counts
  - New `/api/v1/notifications/unread` endpoint for unread notifications only
  - New `/api/v1/notifications/archive` endpoint for archived notifications only
  - New `/api/v1/notifications/overview` endpoint for notification counts by importance
  - New `/api/v1/notifications/{id}` endpoint to get specific notification
  - Create custom notifications via POST `/api/v1/notifications`
  - Archive notifications via POST `/api/v1/notifications/{id}/archive`
  - Unarchive notifications via POST `/api/v1/notifications/{id}/unarchive`
  - Delete notifications via DELETE `/api/v1/notifications/{id}`
  - Archive all unread notifications via POST `/api/v1/notifications/archive/all`
  - Real-time file monitoring with fsnotify for instant notification updates
  - Automatic notification discovery from `/usr/local/emhttp/state/notifications/`
  - Support for all importance levels: alert, warning, info
  - Notification counts by type (unread/archive) and importance level
  - WebSocket real-time updates when notifications change
  - Filter notifications by importance level via query parameter

### Technical Details

- **New DTOs**: `daemon/dto/notification.go` - Notification, NotificationOverview, NotificationCounts, NotificationList structures
- **New Collector**: `daemon/services/collectors/notification.go` - File-based notification collector with fsnotify monitoring
- **New Controller**: `daemon/services/controllers/notification.go` - Notification CRUD operations (create, archive, unarchive, delete)
- **File Monitoring**: Uses fsnotify to watch notification directories for real-time updates
- **Collection Interval**: 15 seconds (with instant updates via file watcher)
- **Notification Format**: Parses Unraid .notify files with key-value pairs
- **Archive Support**: Moves notifications between active and archive directories
- **Bulk Operations**: Archive all unread notifications at once
- **Security**: Proper file permissions (0644 for files, 0755 for directories)
- **Error Handling**: Graceful handling of missing directories, parse errors, and file operations

---

## [2025.11.16] - 2025-11-16

### Added

- **System Log File Access API** (Issue #15):
  - New `/api/v1/logs` endpoint to list all available log files
  - Log content retrieval with pagination support via query parameters
  - Tail behavior: `?path=/var/log/syslog&lines=100` returns last 100 lines
  - Range retrieval: `?path=/var/log/syslog&start=500&lines=100` returns lines 500-600
  - Automatic discovery of common Unraid log files (syslog, docker.log, libvirtd.log, agent log)
  - Plugin log file discovery from `/boot/config/plugins/*/logs/*.log`
  - Directory traversal protection for secure log file access
  - Returns log metadata: name, path, size, modified timestamp
  - Returns log content: full content string, line array, total lines, start/end line numbers

### Technical Details

- **New DTOs**: `daemon/dto/logs.go` - LogFile and LogFileContent structures
- **New API Module**: `daemon/services/api/logs.go` - Log listing and content retrieval logic
- **Security**: Path validation prevents directory traversal attacks
- **Pagination**: Supports both tail behavior (last N lines) and range retrieval (start + lines)
- **Common Logs**: Automatically discovers syslog, Docker, libvirt, and plugin logs
- **Error Handling**: Graceful handling of missing files, permission errors, and invalid paths

---

## [2025.11.15] - 2025-11-16

### Added

- **Registration/License Information API** (Issue #18):
  - New `/api/v1/registration` endpoint to retrieve Unraid license and registration information
  - Returns license type (trial, basic, plus, pro, lifetime), state (valid, expired, invalid, trial), expiration dates, server name, and registration GUID
  - Automatically determines license state based on expiration date and license type
  - Supports all Unraid license types including trial, basic, plus, pro, and lifetime/unleashed licenses
  - Real-time updates via WebSocket when registration information changes
  - Graceful handling of missing or invalid registration data

### Technical Details

- **New DTO**: `daemon/dto/registration.go` - Registration data structure with license type, state, expiration, server name, and GUID
- **New Collector**: `daemon/services/collectors/registration.go` - Parses `/var/local/emhttp/var.ini` for registration data
- **API Integration**: Added registration cache, handler, route, and event subscription to API server
- **Event Bus**: Registration collector publishes `registration_update` events to the PubSub hub
- **Collection Interval**: Uses hardware collection interval (60 seconds) for registration data updates

---

## [2025.11.14] - 2025-11-16

### Added

- **Enhanced GPU Monitoring and Multi-GPU Support** (Issue #8 - High Priority Items):
  - **GPU Identification Fields**: Added `Index`, `PCIID`, `Vendor`, and `UUID` fields to GPUMetrics DTO
    - `Index`: GPU index for multi-GPU systems (0-based)
    - `PCIID`: PCI bus ID (e.g., "0000:01:00.0")
    - `Vendor`: GPU vendor ("nvidia", "intel", "amd")
    - `UUID`: Device UUID (NVIDIA only)
  - **Fan Speed Monitoring**:
    - NVIDIA: `FanSpeed` field (percentage, 0-100)
    - AMD: `FanRPM` and `FanMaxRPM` fields (discrete GPUs only)
  - **AMD GPU Detection Improvements**:
    - Switched from `rocm-smi` to `radeontop` for broader AMD GPU compatibility
    - Now supports consumer Radeon GPUs (RX 5000/6000/7000 series)
    - Fallback to `rocm-smi` for datacenter GPUs (Instinct series)
    - AMD GPU temperature and fan speed read from sysfs
  - **Multiple Intel GPU Detection**:
    - Fixed bug where only first Intel GPU was detected
    - Now detects all Intel GPUs (iGPU + discrete Arc GPUs)
    - Each Intel GPU gets unique index and PCI ID
  - **NVIDIA Enhancements**:
    - Added UUID field for unique GPU identification
    - Added fan speed monitoring (percentage)
    - Added PCI bus ID extraction

### Changed

- **GPU Collector**: Complete refactor to support multi-GPU systems
  - Intel GPU collector now detects ALL Intel GPUs (removed `break` statement)
  - AMD GPU collector uses `radeontop` by default, `rocm-smi` as fallback
  - NVIDIA GPU collector queries additional fields (UUID, PCI ID, fan speed)
  - All GPU collectors now populate vendor, index, and PCI ID fields
- **GPU API Response**: Already returns array of GPUMetrics (no breaking change)
- **GPU Detection Order**: Intel → NVIDIA → AMD (unchanged)

### Technical Details

- **Intel Multi-GPU**: Collects metrics for each detected Intel GPU separately
  - Note: `intel_gpu_top` limitation - reports only first GPU's metrics
  - Multiple GPUs detected via lspci, but metrics may be from primary GPU
- **AMD radeontop**: Parses dump mode output for GPU/VRAM utilization
  - Temperature read from `/sys/class/drm/card*/device/hwmon/hwmon*/temp1_input`
  - Fan speed read from `/sys/class/drm/card*/device/hwmon/hwmon*/fan1_input`
  - Fan max RPM read from `/sys/class/drm/card*/device/hwmon/hwmon*/fan1_max`
- **NVIDIA nvidia-smi**: Extended query to include `pci.bus_id`, `uuid`, `fan.speed`
- **Driver Versions**: Extracted from `modinfo` (Intel/AMD) or `nvidia-smi` (NVIDIA)

### Compatibility

- **Backward Compatible**: All new fields use `omitempty` JSON tags
- **AMD GPU Requirements**:
  - Consumer GPUs: Requires `radeontop` (install via Nerd Tools or manual)
  - Datacenter GPUs: Requires `rocm-smi` (ROCm packages)
- **Intel GPU Requirements**: `intel_gpu_top` from `igt-gpu-tools` (Unraid 6.12+)
- **NVIDIA GPU Requirements**: `nvidia-smi` (included with NVIDIA driver)

### Known Limitations

- **Intel Multi-GPU**: `intel_gpu_top` doesn't support per-device metrics
  - Multiple Intel GPUs detected, but metrics may be from primary GPU only
  - This is a limitation of `intel_gpu_top` tool, not the agent
- **AMD Fan Speed**: Only available on discrete GPUs with fan sensors
  - Integrated AMD GPUs (APUs) don't have fan sensors
- **AMD radeontop**: May require manual installation on some systems

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
