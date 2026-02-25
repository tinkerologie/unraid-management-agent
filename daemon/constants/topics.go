package constants

import (
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// Typed event bus topics. Each Topic[T] enforces at compile time that publishers
// send the correct Go type, eliminating a class of runtime type-assertion bugs.

var (
	// TopicSystemUpdate is published by the system collector with *dto.SystemInfo.
	TopicSystemUpdate = domain.NewTopic[*dto.SystemInfo]("system_update")
	// TopicArrayStatusUpdate is published by the array collector with *dto.ArrayStatus.
	TopicArrayStatusUpdate = domain.NewTopic[*dto.ArrayStatus]("array_status_update")
	// TopicDiskListUpdate is published by the disk collector with []dto.DiskInfo.
	TopicDiskListUpdate = domain.NewTopic[[]dto.DiskInfo]("disk_list_update")
	// TopicShareListUpdate is published by the share collector with []dto.ShareInfo.
	TopicShareListUpdate = domain.NewTopic[[]dto.ShareInfo]("share_list_update")
	// TopicContainerListUpdate is published by the docker collector with []*dto.ContainerInfo.
	TopicContainerListUpdate = domain.NewTopic[[]*dto.ContainerInfo]("container_list_update")
	// TopicVMListUpdate is published by the VM collector with []*dto.VMInfo.
	TopicVMListUpdate = domain.NewTopic[[]*dto.VMInfo]("vm_list_update")
	// TopicUPSStatusUpdate is published by the UPS collector with *dto.UPSStatus.
	TopicUPSStatusUpdate = domain.NewTopic[*dto.UPSStatus]("ups_status_update")
	// TopicNUTStatusUpdate is published by the NUT collector with *dto.NUTResponse.
	TopicNUTStatusUpdate = domain.NewTopic[*dto.NUTResponse]("nut_status_update")
	// TopicGPUMetricsUpdate is published by the GPU collector with []*dto.GPUMetrics.
	TopicGPUMetricsUpdate = domain.NewTopic[[]*dto.GPUMetrics]("gpu_metrics_update")
	// TopicNetworkListUpdate is published by the network collector with []dto.NetworkInfo.
	TopicNetworkListUpdate = domain.NewTopic[[]dto.NetworkInfo]("network_list_update")
	// TopicHardwareUpdate is published by the hardware collector with *dto.HardwareInfo.
	TopicHardwareUpdate = domain.NewTopic[*dto.HardwareInfo]("hardware_update")
	// TopicRegistrationUpdate is published by the registration collector with *dto.Registration.
	TopicRegistrationUpdate = domain.NewTopic[*dto.Registration]("registration_update")
	// TopicNotificationsUpdate is published by the notification collector with *dto.NotificationList.
	TopicNotificationsUpdate = domain.NewTopic[*dto.NotificationList]("notifications_update")
	// TopicUnassignedDevicesUpdate is published by the unassigned collector with *dto.UnassignedDeviceList.
	TopicUnassignedDevicesUpdate = domain.NewTopic[*dto.UnassignedDeviceList]("unassigned_devices_update")
	// TopicZFSPoolsUpdate is published by the ZFS collector with []dto.ZFSPool.
	TopicZFSPoolsUpdate = domain.NewTopic[[]dto.ZFSPool]("zfs_pools_update")
	// TopicZFSDatasetsUpdate is published by the ZFS collector with []dto.ZFSDataset.
	TopicZFSDatasetsUpdate = domain.NewTopic[[]dto.ZFSDataset]("zfs_datasets_update")
	// TopicZFSSnapshotsUpdate is published by the ZFS collector with []dto.ZFSSnapshot.
	TopicZFSSnapshotsUpdate = domain.NewTopic[[]dto.ZFSSnapshot]("zfs_snapshots_update")
	// TopicZFSARCStatsUpdate is published by the ZFS collector with dto.ZFSARCStats.
	TopicZFSARCStatsUpdate = domain.NewTopic[dto.ZFSARCStats]("zfs_arc_stats_update")
	// TopicCollectorStateChange is published by the collector manager with dto.CollectorStateEvent.
	TopicCollectorStateChange = domain.NewTopic[dto.CollectorStateEvent]("collector_state_change")
)
