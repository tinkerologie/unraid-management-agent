package domain

import (
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/platform"
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
}

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
	Config
}
