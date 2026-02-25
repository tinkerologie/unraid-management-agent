package api

import (
	"sync/atomic"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/alerting"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/collectors"
)

// networkServicesCacheTTL controls how often network services status is refreshed.
const networkServicesCacheTTL = 30 * time.Second

// Compile-time assertion: CacheStore satisfies alerting.DataProvider.
var _ alerting.DataProvider = (*CacheStore)(nil)

// CacheStore holds all collector cache fields using lock-free atomic pointers.
// It is embedded in Server and provides the pure cache getter methods required
// by the mcp.CacheProvider interface.
type CacheStore struct {
	systemCache          atomic.Pointer[dto.SystemInfo]
	arrayCache           atomic.Pointer[dto.ArrayStatus]
	disksCache           atomic.Pointer[[]dto.DiskInfo]
	sharesCache          atomic.Pointer[[]dto.ShareInfo]
	dockerCache          atomic.Pointer[[]dto.ContainerInfo]
	vmsCache             atomic.Pointer[[]dto.VMInfo]
	upsCache             atomic.Pointer[dto.UPSStatus]
	gpuCache             atomic.Pointer[[]*dto.GPUMetrics]
	networkCache         atomic.Pointer[[]dto.NetworkInfo]
	hardwareCache        atomic.Pointer[dto.HardwareInfo]
	registrationCache    atomic.Pointer[dto.Registration]
	notificationsCache   atomic.Pointer[dto.NotificationList]
	unassignedCache      atomic.Pointer[dto.UnassignedDeviceList]
	zfsPoolsCache        atomic.Pointer[[]dto.ZFSPool]
	zfsDatasetsCache     atomic.Pointer[[]dto.ZFSDataset]
	zfsSnapshotsCache    atomic.Pointer[[]dto.ZFSSnapshot]
	zfsARCStatsCache     atomic.Pointer[dto.ZFSARCStats]
	nutCache             atomic.Pointer[dto.NUTResponse]
	networkServicesCache atomic.Pointer[dto.NetworkServicesStatus]
}

// ---------- Pointer-type getters (direct Load) ----------

// GetSystemCache returns cached system information.
func (c *CacheStore) GetSystemCache() *dto.SystemInfo {
	return c.systemCache.Load()
}

// GetArrayCache returns cached array status.
func (c *CacheStore) GetArrayCache() *dto.ArrayStatus {
	return c.arrayCache.Load()
}

// GetUPSCache returns cached UPS status.
func (c *CacheStore) GetUPSCache() *dto.UPSStatus {
	return c.upsCache.Load()
}

// GetHardwareCache returns cached hardware information.
func (c *CacheStore) GetHardwareCache() *dto.HardwareInfo {
	return c.hardwareCache.Load()
}

// GetRegistrationCache returns cached registration information.
func (c *CacheStore) GetRegistrationCache() *dto.Registration {
	return c.registrationCache.Load()
}

// GetNotificationsCache returns cached notifications.
func (c *CacheStore) GetNotificationsCache() *dto.NotificationList {
	return c.notificationsCache.Load()
}

// GetZFSARCStatsCache returns cached ZFS ARC statistics.
func (c *CacheStore) GetZFSARCStatsCache() *dto.ZFSARCStats {
	return c.zfsARCStatsCache.Load()
}

// GetUnassignedCache returns cached unassigned devices information.
func (c *CacheStore) GetUnassignedCache() *dto.UnassignedDeviceList {
	return c.unassignedCache.Load()
}

// GetNUTCache returns cached NUT (Network UPS Tools) information.
func (c *CacheStore) GetNUTCache() *dto.NUTResponse {
	return c.nutCache.Load()
}

// ---------- Slice-type getters (dereference atomic pointer) ----------

// GetDisksCache returns cached disk information.
func (c *CacheStore) GetDisksCache() []dto.DiskInfo {
	if v := c.disksCache.Load(); v != nil {
		return *v
	}
	return nil
}

// GetSharesCache returns cached share information.
func (c *CacheStore) GetSharesCache() []dto.ShareInfo {
	if v := c.sharesCache.Load(); v != nil {
		return *v
	}
	return nil
}

// GetDockerCache returns cached Docker container information.
func (c *CacheStore) GetDockerCache() []dto.ContainerInfo {
	if v := c.dockerCache.Load(); v != nil {
		return *v
	}
	return nil
}

// GetVMsCache returns cached VM information.
func (c *CacheStore) GetVMsCache() []dto.VMInfo {
	if v := c.vmsCache.Load(); v != nil {
		return *v
	}
	return nil
}

// GetGPUCache returns cached GPU metrics.
func (c *CacheStore) GetGPUCache() []*dto.GPUMetrics {
	if v := c.gpuCache.Load(); v != nil {
		return *v
	}
	return nil
}

// GetNetworkCache returns cached network information.
func (c *CacheStore) GetNetworkCache() []dto.NetworkInfo {
	if v := c.networkCache.Load(); v != nil {
		return *v
	}
	return nil
}

// GetZFSPoolsCache returns cached ZFS pool information.
func (c *CacheStore) GetZFSPoolsCache() []dto.ZFSPool {
	if v := c.zfsPoolsCache.Load(); v != nil {
		return *v
	}
	return nil
}

// GetZFSDatasetsCache returns cached ZFS dataset information.
func (c *CacheStore) GetZFSDatasetsCache() []dto.ZFSDataset {
	if v := c.zfsDatasetsCache.Load(); v != nil {
		return *v
	}
	return nil
}

// GetZFSSnapshotsCache returns cached ZFS snapshot information.
func (c *CacheStore) GetZFSSnapshotsCache() []dto.ZFSSnapshot {
	if v := c.zfsSnapshotsCache.Load(); v != nil {
		return *v
	}
	return nil
}

// ---------- Non-collector caches ----------

// GetParityHistoryCache returns cached parity check history.
// Note: This is dynamically loaded, not cached by a collector.
// Returns an empty sentinel so callers never receive nil.
func (c *CacheStore) GetParityHistoryCache() *dto.ParityCheckHistory {
	return &dto.ParityCheckHistory{}
}

// GetNetworkServicesCache returns cached network services status,
// refreshing from disk if the cache is stale (older than networkServicesCacheTTL).
func (c *CacheStore) GetNetworkServicesCache() *dto.NetworkServicesStatus {
	if cached := c.networkServicesCache.Load(); cached != nil {
		if time.Since(cached.Timestamp) < networkServicesCacheTTL {
			return cached
		}
	}
	sc := collectors.NewSettingsCollector()
	status, err := sc.GetNetworkServicesStatus()
	if err != nil {
		logger.Warning("CacheStore: failed to refresh network services: %v", err)
		return c.networkServicesCache.Load() // return stale data if available
	}
	c.networkServicesCache.Store(status)
	return status
}
