// Package constants provides shared constants and configuration values for the Unraid Management Agent.
package constants

const (
	// VarIni is the path to the Unraid var.ini configuration file.
	VarIni = "/var/local/emhttp/var.ini"
	// DisksIni is the path to the Unraid disks.ini configuration file.
	DisksIni = "/var/local/emhttp/disks.ini"
	// SharesIni is the path to the Unraid shares.ini configuration file.
	SharesIni = "/var/local/emhttp/shares.ini"
	// NetworkIni is the path to the Unraid network.ini configuration file.
	NetworkIni = "/var/local/emhttp/network.ini"
	// NginxIni is the path to the Unraid nginx.ini configuration file.
	NginxIni = "/var/local/emhttp/nginx.ini"

	// DiskCfg is the path to the disk configuration file (boot config).
	DiskCfg = "/boot/config/disk.cfg"
	// DockerCfg is the path to the Docker configuration file.
	DockerCfg = "/boot/config/docker.cfg"
	// DomainCfg is the path to the VM Manager configuration file.
	DomainCfg = "/boot/config/domain.cfg"
	// ShareCfg is the path to the global share configuration file.
	ShareCfg = "/boot/config/share.cfg"
	// IdentCfg is the path to the server identity configuration file.
	IdentCfg = "/boot/config/ident.cfg"
	// SharesConfigDir is the directory containing per-share configuration files.
	SharesConfigDir = "/boot/config/shares"
	// PluginsConfigDir is the directory containing plugin files.
	PluginsConfigDir = "/boot/config/plugins"
	// PluginsTempDir is the directory containing downloaded plugin updates.
	PluginsTempDir = "/tmp/plugins"

	// DynamixCfg is the path to the dynamix plugin configuration (contains temp thresholds).
	DynamixCfg = "/boot/config/plugins/dynamix/dynamix.cfg"
	// ParityCheckCron is the path to the parity check schedule cron file.
	ParityCheckCron = "/boot/config/plugins/dynamix/parity-check.cron"
	// ParityChecksLog is the path to the parity check history log.
	ParityChecksLog = "/boot/config/parity-checks.log"

	// ProcCPUInfo is the path to the /proc/cpuinfo file.
	ProcCPUInfo = "/proc/cpuinfo"
	// ProcMemInfo is the path to the /proc/meminfo file.
	ProcMemInfo = "/proc/meminfo"
	// ProcUptime is the path to the /proc/uptime file.
	ProcUptime = "/proc/uptime"
	// ProcStat is the path to the /proc/stat file.
	ProcStat = "/proc/stat"
	// SysHwmon is the path to the /sys/class/hwmon directory.
	SysHwmon = "/sys/class/hwmon"

	// SensorsBin is the path to the sensors binary.
	SensorsBin = "/usr/bin/sensors"
	// SmartctlBin is the path to the smartctl binary.
	SmartctlBin = "/usr/sbin/smartctl"
	// DockerBin is the path to the docker binary.
	DockerBin = "/usr/bin/docker"
	// VirshBin is the path to the virsh binary.
	VirshBin = "/usr/bin/virsh"
	// MdcmdBin is the path to the mdcmd binary.
	MdcmdBin = "/usr/local/sbin/mdcmd"
	// ApcaccessBin is the path to the apcaccess binary.
	ApcaccessBin = "/sbin/apcaccess"
	// UpscBin is the path to the upsc binary.
	UpscBin = "/usr/bin/upsc"
	// UpscmdBin is the path to the upscmd binary (NUT commands).
	UpscmdBin = "/usr/bin/upscmd"
	// NvidiaSMIBin is the path to the nvidia-smi binary.
	NvidiaSMIBin = "/usr/bin/nvidia-smi"
	// ZpoolBin is the path to the zpool binary.
	ZpoolBin = "/usr/sbin/zpool"
	// ZfsBin is the path to the zfs binary.
	ZfsBin = "/usr/sbin/zfs"

	// ProcSPLARCStats is the path to the ZFS ARC statistics file.
	ProcSPLARCStats = "/proc/spl/kstat/zfs/arcstats"

	// NutPidFile is the path to the NUT UPS monitor PID file.
	NutPidFile = "/var/run/nut/upsmon.pid"
	// NutConfigDir is the path to the NUT configuration directory.
	NutConfigDir = "/etc/nut"
	// NutPluginCfg is the path to the NUT plugin configuration file.
	NutPluginCfg = "/boot/config/plugins/nut-dw/nut-dw.cfg"
	// NutPluginDir is the path to the NUT plugin directory.
	NutPluginDir = "/usr/local/emhttp/plugins/nut-dw"
	// ApcPidFile is the path to the APC UPS daemon PID file.
	ApcPidFile = "/var/run/apcupsd.pid"

	// Collection intervals optimized for power efficiency (Issue #8)
	// Higher intervals reduce CPU wake-ups and allow deeper C-states

	// IntervalSystem is the collection interval for system metrics in seconds.
	// Increased from 5s to 15s - sensors command is CPU intensive
	IntervalSystem = 15
	// IntervalArray is the collection interval for array metrics in seconds.
	// Increased from 10s to 30s - array status rarely changes
	IntervalArray = 30
	// IntervalDisk is the collection interval for disk metrics in seconds.
	IntervalDisk = 30
	// IntervalDocker is the collection interval for Docker metrics in seconds.
	// Increased from 10s to 30s - docker stats is very CPU intensive with many containers
	IntervalDocker = 30
	// IntervalVM is the collection interval for VM metrics in seconds.
	// Increased from 10s to 30s - virsh commands spawn multiple processes
	IntervalVM = 30
	// IntervalUPS is the collection interval for UPS metrics in seconds.
	// Increased from 10s to 60s - UPS status rarely changes
	IntervalUPS = 60
	// IntervalGPU is the collection interval for GPU metrics in seconds.
	// Increased from 10s to 60s - intel_gpu_top is extremely CPU intensive
	IntervalGPU = 60
	// IntervalShares is the collection interval for share metrics in seconds.
	IntervalShares = 60
	// IntervalNetwork is the collection interval for network metrics in seconds.
	// Increased from 15s to 30s - network status rarely changes
	IntervalNetwork = 30
	// IntervalHardware is the collection interval for hardware metrics in seconds.
	IntervalHardware = 300
	// IntervalZFS is the collection interval for ZFS metrics in seconds.
	IntervalZFS = 30
	// IntervalNotification is the collection interval for notification metrics in seconds.
	IntervalNotification = 30
	// IntervalRegistration is the collection interval for registration metrics in seconds.
	IntervalRegistration = 300
	// IntervalUnassigned is the collection interval for unassigned devices in seconds.
	IntervalUnassigned = 60

	// WSPingInterval is the WebSocket ping interval in seconds.
	WSPingInterval = 30
	// WSMaxClients is the maximum number of concurrent WebSocket clients.
	WSMaxClients = 10
	// WSBufferSize is the WebSocket message buffer size.
	WSBufferSize = 256
)
