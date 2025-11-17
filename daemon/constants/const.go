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
	// NvidiaSMIBin is the path to the nvidia-smi binary.
	NvidiaSMIBin = "/usr/bin/nvidia-smi"

	// NutPidFile is the path to the NUT UPS monitor PID file.
	NutPidFile = "/var/run/nut/upsmon.pid"
	// ApcPidFile is the path to the APC UPS daemon PID file.
	ApcPidFile = "/var/run/apcupsd.pid"

	// IntervalSystem is the collection interval for system metrics in seconds.
	IntervalSystem = 5
	// IntervalArray is the collection interval for array metrics in seconds.
	IntervalArray = 10
	// IntervalDisk is the collection interval for disk metrics in seconds.
	IntervalDisk = 30
	// IntervalDocker is the collection interval for Docker metrics in seconds.
	IntervalDocker = 10
	// IntervalVM is the collection interval for VM metrics in seconds.
	IntervalVM = 10
	// IntervalUPS is the collection interval for UPS metrics in seconds.
	IntervalUPS = 10
	// IntervalGPU is the collection interval for GPU metrics in seconds.
	IntervalGPU = 10
	// IntervalShares is the collection interval for share metrics in seconds.
	IntervalShares = 60
	// IntervalNetwork is the collection interval for network metrics in seconds.
	IntervalNetwork = 15
	// IntervalHardware is the collection interval for hardware metrics in seconds.
	IntervalHardware = 300

	// WSPingInterval is the WebSocket ping interval in seconds.
	WSPingInterval = 30
	// WSMaxClients is the maximum number of concurrent WebSocket clients.
	WSMaxClients = 10
	// WSBufferSize is the WebSocket message buffer size.
	WSBufferSize = 256
)
