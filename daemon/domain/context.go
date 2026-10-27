package domain

import (
	"errors"

	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"github.com/tinkerologie/unraid-management-agent/daemon/platform"
)

// Intervals holds collection interval settings in seconds.
type Intervals struct {
	System         int
	Array          int
	Disk           int
	Docker         int
	VM             int
	UPS            int
	NUT            int
	GPU            int
	Shares         int
	Network        int
	Hardware       int
	ZFS            int
	Notification   int
	Registration   int
	Unassigned     int
	FanControl     int
	Tuning         int
	DockerUpdate   int
	DockerNetworks int
	PluginUpdate   int
	OSUpdate       int
	Mover          int

	// StorageTopology is the storage_topology collector interval in seconds.
	StorageTopology int
	// NIC is the nic collector interval in seconds.
	NIC int
	PSU int
}

// Collection modes (#280). In Dynamic mode the agent decides when each
// collector runs; in Manual mode every collector runs at its configured interval.
const (
	CollectionModeDynamic = "dynamic"
	CollectionModeManual  = "manual"
)

// ErrDynamicCollectionMode is returned for interval changes in Dynamic
// collection mode, where the agent decides each collector's interval.
var ErrDynamicCollectionMode = errors.New("collector intervals are managed by the agent in dynamic collection mode; switch to manual mode to set them")

// Context holds the application runtime context including the event hub and configuration.
type Context struct {
	Hub                *EventBus
	Platform           *platform.Registry
	Intervals          Intervals
	MQTTConfig         MQTTConfig
	DiscoveryConfig    DiscoveryConfig
	DiagnosticLogger   *logger.DiagnosticLogger
	LogsDir            string
	DockerUpdateNotify bool
	// GatedCollectors are the hardware-gated collectors the capability policy
	// controls: off while their hardware is absent, whatever the configured
	// interval (#261).
	GatedCollectors map[string]bool
	// AutoCollectors are the gated collectors in automatic mode: they run at
	// the auto-enable interval while their hardware is present.
	AutoCollectors map[string]bool
	// DisabledCollectors are the collectors turned off by UNRAID_DISABLE_COLLECTORS.
	DisabledCollectors map[string]bool
	// CollectionMode is CollectionModeDynamic or CollectionModeManual.
	CollectionMode string
	// LowPowerMode multiplies collector intervals by 4 (also auto-enabled ones).
	LowPowerMode bool
	Config
}

// DynamicCollection reports whether the agent runs in Dynamic collection mode.
func (c *Context) DynamicCollection() bool {
	return c.CollectionMode == CollectionModeDynamic
}
