# ZFS Investigation Findings - Unraid Management Agent

**Date**: 2025-11-18  
**Server**: 192.168.20.21  
**Unraid Version**: 7.2.0  
**ZFS Version**: 2.3.4-1

## Summary

ZFS is **available and functional** on the Unraid server. A test pool named "garbage" (222GB, single disk) is configured and ONLINE. The agent currently has **zero ZFS support** - no endpoints, collectors, or DTOs exist.

---

## Current API State

### Tested Endpoints
- `/api/v1/disks` - Returns 11 disks (no ZFS pool info)
- `/api/v1/array` - Returns array status (no ZFS info)
- `/api/v1/shares` - Returns 10 shares (no ZFS datasets)
- `/api/v1/zfs` - **404 Not Found** (endpoint doesn't exist)

### Conclusion
**No ZFS data is currently exposed through any API endpoint.**

---

## ZFS Availability on Server

### Module Status
```
ZFS Module: Available (version 2.3.4-1)
Location: /lib/modules/6.12.54-Unraid/extra/zfs.ko.xz
Status: Successfully loaded with `modprobe zfs`
Dependencies: spl (Solaris Porting Layer)
```

### Binaries
```
/usr/sbin/zpool - Pool management (276KB)
/usr/sbin/zfs   - Dataset management
```

### Kernel Interface
```
/proc/spl/kstat/zfs/arcstats - ARC cache statistics
/proc/spl/kstat/zfs/         - Additional ZFS kstats
```

---

## Test Pool Configuration

### Pool: "garbage"

**Basic Info:**
```
Name:          garbage
Size:          238,370,684,928 bytes (222 GB)
Allocated:     540,672 bytes (528 KB)
Free:          238,370,144,256 bytes (222 GB)
Fragmentation: 0%
Capacity:      0%
Dedup Ratio:   1.00x
Health:        ONLINE
State:         ONLINE
GUID:          12919208826490684836
```

**VDEVs:**
```
Single disk configuration:
  - sdg1 (ONLINE, 0 read/write/checksum errors)
```

**Properties:**
```
autoexpand:    on
autotrim:      on
ashift:        12
readonly:      off
delegation:    on
```

---

## Data Collection Methods

### 1. Pool List (Parseable Format)

**Command:**
```bash
zpool list -Hp -o name,size,allocated,free,fragmentation,capacity,dedupratio,health,altroot
```

**Output Format:** Tab-separated values
```
garbage	238370684928	540672	238370144256	0	0	1.00	ONLINE	-
```

**Fields:**
1. name (string)
2. size (uint64 bytes)
3. allocated (uint64 bytes)
4. free (uint64 bytes)
5. fragmentation (uint64 percent)
6. capacity (uint64 percent)
7. dedupratio (float64 ratio)
8. health (string: ONLINE/DEGRADED/FAULTED/OFFLINE/UNAVAIL/REMOVED)
9. altroot (string, "-" if none)

### 2. Pool Status (Tree Structure)

**Command:**
```bash
zpool status -v <poolname>
```

**Output:**
```
  pool: garbage
 state: ONLINE
config:

	NAME        STATE     READ WRITE CKSUM
	garbage     ONLINE       0     0     0
	  sdg1      ONLINE       0     0     0

errors: No known data errors
```

**Parsing Strategy:**
- Line starting with "pool:" → pool name
- Line starting with "state:" → pool state
- Lines under "config:" → vdev tree (indentation-based hierarchy)
- Lines under "errors:" → error summary

### 3. Pool Properties

**Command:**
```bash
zpool get all <poolname>
```

**Output Format:** Space-separated columns
```
NAME     PROPERTY      VALUE    SOURCE
garbage  size          222G     -
garbage  capacity      0%       -
garbage  health        ONLINE   -
```

### 4. Dataset List (Parseable Format)

**Command:**
```bash
zfs list -Hp -r <poolname> -o name,type,used,available,referenced,compressratio,mountpoint,quota,reservation,compression,readonly
```

**Output Format:** Tab-separated values
```
garbage	filesystem	540672	230988169216	98304	1.00	/mnt/garbage	0	0	off	off
```

**Fields:**
1. name (string)
2. type (string: filesystem/volume/snapshot)
3. used (uint64 bytes)
4. available (uint64 bytes)
5. referenced (uint64 bytes)
6. compressratio (float64 ratio, e.g., "1.00")
7. mountpoint (string)
8. quota (uint64 bytes, 0 = none)
9. reservation (uint64 bytes, 0 = none)
10. compression (string: off/lz4/gzip/zstd/etc.)
11. readonly (string: on/off)

### 5. Snapshots

**Command:**
```bash
zfs list -t snapshot -Hp -r <poolname> -o name,used,referenced,creation
```

**Output:** Tab-separated (currently no snapshots on test pool)

### 6. ARC Statistics

**File:** `/proc/spl/kstat/zfs/arcstats`

**Format:** Space-separated: `name type data`
```
hits                            4    29994
misses                          4    58
c                               4    1041510400
c_min                           4    1041510400
c_max                           4    4166041600
size                            4    3460568
```

**Key Stats:**
- `hits` - Cache hits
- `misses` - Cache misses
- `c` - Current ARC size (bytes)
- `c_min` - Minimum ARC size (bytes)
- `c_max` - Maximum ARC size (bytes)
- `size` - Actual data size in ARC (bytes)

### 7. I/O Statistics

**Command:**
```bash
zpool iostat -v <poolname> 1 2
```

**Output:**
```
              capacity     operations     bandwidth 
pool        alloc   free   read  write   read  write
----------  -----  -----  -----  -----  -----  -----
garbage      528K   222G      0      0  1.20K  4.55K
  sdg1       528K   222G      0      0  1.20K  4.55K
```

---

## Implementation Plan

### Phase 1: Core DTOs and Collector (v2025.11.24)

**Create:**
1. `daemon/dto/zfs.go` - ZFSPool, ZFSVdev, ZFSDevice, ZFSDataset, ZFSSnapshot, ZFSARCStats DTOs
2. `daemon/services/collectors/zfs.go` - ZFS collector with 30s interval
3. `daemon/constants/const.go` - Add ZFS binary paths and intervals
4. `daemon/services/api/handlers.go` - Add ZFS endpoint handlers
5. `daemon/services/api/server.go` - Subscribe to ZFS events

**Endpoints:**
- `GET /api/v1/zfs/pools` - List all pools
- `GET /api/v1/zfs/pools/{name}` - Get specific pool details
- `GET /api/v1/zfs/arc` - Get ARC statistics

### Phase 2: Advanced Features (Future)

- Dataset management endpoints
- Snapshot creation/deletion
- Pool scrub control
- I/O statistics endpoint
- WebSocket events for pool state changes

---

## Testing Strategy

1. **Build and deploy** to 192.168.20.21
2. **Test endpoints** with "garbage" pool
3. **Verify data accuracy** against `zpool` commands
4. **Check logs** for collection errors
5. **Validate WebSocket** events (if implemented)

---

## Notes

- ZFS module must be loaded (`modprobe zfs`) before collection
- Handle case where ZFS is not available gracefully
- Parse tab-separated output carefully (fields can be "-" for null)
- ARC stats file format is consistent across ZFS versions
- Pool status tree parsing requires indentation-aware logic

---

**Status**: Investigation complete, ready for implementation.

