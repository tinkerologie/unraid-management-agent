# Unraid Management Agent

The Management Agent exposes monitoring and control of one Unraid server to external consumers.
This vocabulary distinguishes server resources, collection policy, and access to control operations.

## Language

### Server and consumers

**Management Agent**:
The community Unraid plugin that exposes a server's monitoring data and supported control operations.
_Avoid_: Home Assistant integration, official Unraid API.

**Embedded AI agent**:
The optional assistant within the Management Agent that uses a language model and server tools.
_Avoid_: Management Agent when referring specifically to the assistant.

**VM guest telemetry**:
Measurements taken inside a configured virtual machine, distinct from the host's view of that VM.

### Collection and source health

**Collector**:
A Management Agent monitoring subsystem responsible for one area of server state, such as disks, containers, or UPS devices.

**Collection mode**:
The server-wide choice between Dynamic mode and Manual mode for scheduling collectors.

**Dynamic mode**:
The collection mode in which the Management Agent chooses collection timing according to demand,
resource policy, and monitoring requirements.
_Avoid_: Manual mode, automatic hardware detection.

**Manual mode**:
The collection mode in which enabled collectors run at their configured intervals.
_Avoid_: Dynamic mode, on-demand collection.

**Demand**:
Evidence that a consumer is using a collector's data, through a subscription or a recent read.
_Avoid_: Configured interval, collector enabled state.

**Hardware-gated collector**:
A collector whose eligibility to run depends on the presence of its required hardware.

**Capability**:
A detected server facility, such as a required binary, operating-system source, or hardware class.
Its presence can be confirmed, absent, or unknown.

**Source health**:
The condition of the underlying source supplying a monitoring subsystem's data.
A source may be healthy, degraded, unavailable, or intentionally disabled.
_Avoid_: Hardware presence, overall server health.

### Storage

**Array**:
Unraid's collection of assigned data disks and any parity disks, distinct from named storage pools.
_Avoid_: Pool, all server storage.

**Pool**:
A named Unraid storage resource separate from the array; a pool may use ZFS or another supported filesystem.
_Avoid_: Array, cache when no particular pool is meant.

**Parity disk**:
An array disk assigned to parity information used to reconstruct data after a supported disk failure.
_Avoid_: Data disk, backup.

**Parity check**:
An operation that checks array parity consistency and may correct parity when configured to do so.
_Avoid_: Backup, filesystem scrub.

**User share**:
A named Unraid view of files stored on its assigned pool or array storage.
_Avoid_: Remote share, storage pool.

**Share content usage**:
The measured space occupied by a user share's contents; an unavailable measurement is different from zero usage.
_Avoid_: Backing storage usage, share capacity.

**Backing storage usage**:
The used, free, and total capacity of the pool or array backing a user share.
Multiple shares on the same storage can report the same backing storage usage.
_Avoid_: Share content usage, per-share quota.

**Exclusive share**:
A user share whose access goes directly to its pool, bypassing Unraid's FUSE user-share layer.
_Avoid_: Private share, access-control setting.

**FUSE fallback**:
The condition in which an exclusive-eligible user share uses the FUSE user-share layer instead of direct pool access.

**Unassigned device**:
A locally attached storage device managed outside the array and named pools through Unassigned Devices.
_Avoid_: Array disk, remote share.

**Remote share**:
A network filesystem mounted on the Unraid server through Unassigned Devices.
_Avoid_: User share, locally attached unassigned device.

**Storage topology**:
The relationships among storage controllers, enclosures, slots, and attached drives.
_Avoid_: Filesystem layout, user-share layout.

**Enclosure**:
A storage chassis or expander that groups drive slots and associated hardware information.
_Avoid_: Storage pool, array.

### Control and automation

**MCP read-only mode**:
The Management Agent setting that prevents state-changing MCP tools while retaining monitoring tools.
_Avoid_: REST read-only mode, Home Assistant read-only mode.

**Network write lock**:
The refusal of network control operations when no API token is configured and unauthenticated writes are not allowed.
_Avoid_: MCP read-only mode.

**Tool policy**:
The access policy for an individual MCP tool, distinct from the server's network authentication policy.

**Alert rule**:
A condition evaluated against server monitoring data that can produce an alert and configured notifications.
_Avoid_: Home Assistant repair issue.

**Watchdog**:
The optional server supervision facility that checks services and can perform configured recovery actions.
_Avoid_: Alert rule, collector.
