package api

import (
	"reflect"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// eventBinding connects a topic to its cache update function.
// The binding table replaces manual type switches in subscribeToEvents,
// ensuring a single source of truth for event-to-cache mappings.
type eventBinding struct {
	topicName string
	msgType   reflect.Type
	update    func(*CacheStore, any)
}

// bind creates a type-safe eventBinding using generics. The Topic[T] parameter
// enforces at compile time that the handler function accepts the correct type.
func bind[T any](topic domain.Topic[T], fn func(*CacheStore, T)) eventBinding {
	return eventBinding{
		topicName: topic.Name,
		msgType:   reflect.TypeFor[T](),
		update:    func(c *CacheStore, v any) { fn(c, v.(T)) },
	}
}

// cacheBindings returns every event-to-cache mapping.
// Adding a new collector requires only one new entry here.
func cacheBindings() []eventBinding {
	return []eventBinding{
		bind(constants.TopicSystemUpdate, func(c *CacheStore, v *dto.SystemInfo) {
			c.systemCache.Store(v)
		}),
		bind(constants.TopicArrayStatusUpdate, func(c *CacheStore, v *dto.ArrayStatus) {
			c.arrayCache.Store(v)
		}),
		bind(constants.TopicDiskListUpdate, func(c *CacheStore, v []dto.DiskInfo) {
			c.disksCache.Store(&v)
		}),
		bind(constants.TopicShareListUpdate, func(c *CacheStore, v []dto.ShareInfo) {
			c.sharesCache.Store(&v)
		}),
		bind(constants.TopicContainerListUpdate, func(c *CacheStore, v []*dto.ContainerInfo) {
			containers := make([]dto.ContainerInfo, len(v))
			for i, ct := range v {
				containers[i] = *ct
			}
			c.dockerCache.Store(&containers)
		}),
		bind(constants.TopicVMListUpdate, func(c *CacheStore, v []*dto.VMInfo) {
			vms := make([]dto.VMInfo, len(v))
			for i, vm := range v {
				vms[i] = *vm
			}
			c.vmsCache.Store(&vms)
		}),
		bind(constants.TopicUPSStatusUpdate, func(c *CacheStore, v *dto.UPSStatus) {
			c.upsCache.Store(v)
		}),
		bind(constants.TopicNUTStatusUpdate, func(c *CacheStore, v *dto.NUTResponse) {
			c.nutCache.Store(v)
		}),
		bind(constants.TopicGPUMetricsUpdate, func(c *CacheStore, v []*dto.GPUMetrics) {
			c.gpuCache.Store(&v)
		}),
		bind(constants.TopicNetworkListUpdate, func(c *CacheStore, v []dto.NetworkInfo) {
			c.networkCache.Store(&v)
		}),
		bind(constants.TopicHardwareUpdate, func(c *CacheStore, v *dto.HardwareInfo) {
			c.hardwareCache.Store(v)
		}),
		bind(constants.TopicRegistrationUpdate, func(c *CacheStore, v *dto.Registration) {
			c.registrationCache.Store(v)
		}),
		bind(constants.TopicNotificationsUpdate, func(c *CacheStore, v *dto.NotificationList) {
			c.notificationsCache.Store(v)
		}),
		bind(constants.TopicUnassignedDevicesUpdate, func(c *CacheStore, v *dto.UnassignedDeviceList) {
			c.unassignedCache.Store(v)
		}),
		bind(constants.TopicZFSPoolsUpdate, func(c *CacheStore, v []dto.ZFSPool) {
			c.zfsPoolsCache.Store(&v)
		}),
		bind(constants.TopicZFSDatasetsUpdate, func(c *CacheStore, v []dto.ZFSDataset) {
			c.zfsDatasetsCache.Store(&v)
		}),
		bind(constants.TopicZFSSnapshotsUpdate, func(c *CacheStore, v []dto.ZFSSnapshot) {
			c.zfsSnapshotsCache.Store(&v)
		}),
		bind(constants.TopicZFSARCStatsUpdate, func(c *CacheStore, v dto.ZFSARCStats) {
			c.zfsARCStatsCache.Store(&v)
		}),
	}
}

// buildCacheDispatch creates a type-to-handler map for O(1) event dispatch.
func buildCacheDispatch(bindings []eventBinding) map[reflect.Type]func(*CacheStore, any) {
	m := make(map[reflect.Type]func(*CacheStore, any), len(bindings))
	for _, b := range bindings {
		m[b.msgType] = b.update
	}
	return m
}

// broadcastTopicNames derives the full list of topics forwarded to WebSocket
// clients from cacheBindings() plus any non-cache topics that clients need.
// This ensures adding a new cache binding automatically enables its broadcast.
func broadcastTopicNames() []string {
	bindings := cacheBindings()
	names := make([]string, 0, len(bindings)+1)
	for _, b := range bindings {
		names = append(names, b.topicName)
	}
	// CollectorStateChange is broadcast but not cached.
	names = append(names, constants.TopicCollectorStateChange.Name)
	return names
}

// buildTypeToTopicMap returns a reflect.Type → topic name map
// for resolving the topic name of a broadcast message.
func buildTypeToTopicMap() map[reflect.Type]string {
	bindings := cacheBindings()
	m := make(map[reflect.Type]string, len(bindings))
	for _, b := range bindings {
		m[b.msgType] = b.topicName
	}
	return m
}
