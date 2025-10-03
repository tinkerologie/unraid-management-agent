package common

const (
	// Unraid configuration files
	VarIni      = "/var/local/emhttp/var.ini"
	DisksIni    = "/var/local/emhttp/disks.ini"
	SharesIni   = "/var/local/emhttp/shares.ini"
	NetworkIni  = "/var/local/emhttp/network.ini"
	NginxIni    = "/var/local/emhttp/nginx.ini"

	// System paths
	ProcCPUInfo  = "/proc/cpuinfo"
	ProcMemInfo  = "/proc/meminfo"
	ProcUptime   = "/proc/uptime"
	ProcStat     = "/proc/stat"
	SysHwmon     = "/sys/class/hwmon"

	// Command binaries
	SensorsBin   = "/usr/bin/sensors"
	SmartctlBin  = "/usr/sbin/smartctl"
	DockerBin    = "/usr/bin/docker"
	VirshBin     = "/usr/bin/virsh"
	MdcmdBin     = "/usr/local/sbin/mdcmd"
	ApcaccessBin = "/sbin/apcaccess"
	UpscBin      = "/usr/bin/upsc"
	NvidiaSMIBin = "/usr/bin/nvidia-smi"

	// UPS detection
	NutPidFile = "/var/run/nut/upsmon.pid"
	ApcPidFile = "/var/run/apcupsd.pid"

	// Collection intervals (seconds)
	IntervalSystem  = 5
	IntervalArray   = 10
	IntervalDisk    = 30
	IntervalDocker  = 10
	IntervalVM      = 10
	IntervalUPS     = 10
	IntervalGPU     = 10
	IntervalShares  = 60
	IntervalNetwork = 15

	// WebSocket settings
	WSPingInterval = 30
	WSMaxClients   = 10
	WSBufferSize   = 256
)
