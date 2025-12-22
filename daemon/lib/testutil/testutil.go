// Package testutil provides test utilities and mocks for unit testing.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TempDir creates a temporary directory and returns its path and a cleanup function.
func TempDir(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "unraid-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	return dir, func() {
		//nolint:gosec,errcheck // G104: Cleanup in tests - errors are acceptable
		_ = os.RemoveAll(dir)
	}
}

// WriteFile writes content to a file in the given directory.
func WriteFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	//nolint:gosec // G301: Test directory permissions - 0755 is acceptable for tests
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	//nolint:gosec // G306: Test file permissions - 0644 is acceptable for tests
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file %s: %v", path, err)
	}
	return path
}

// ReadFileContent reads file content or fails the test.
func ReadFileContent(t *testing.T, path string) string {
	t.Helper()
	//nolint:gosec // G304: Test utility - path comes from test code, not user input
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", path, err)
	}
	return string(data)
}

// SampleProcMeminfo returns sample /proc/meminfo content.
func SampleProcMeminfo() string {
	return `MemTotal:       32653968 kB
MemFree:        15234568 kB
MemAvailable:   20123456 kB
Buffers:          512000 kB
Cached:          4876900 kB
SwapCached:            0 kB
Active:          8765432 kB
Inactive:        5432100 kB
`
}

// SampleProcStat returns sample /proc/stat content.
func SampleProcStat() string {
	return `cpu  10132153 290696 3084719 46828483 16683 0 25195 0 0 0
cpu0 1292830 36410 386526 5765120 3479 0 11149 0 0 0
cpu1 1291881 36252 385618 5764888 2500 0 3146 0 0 0
cpu2 1291758 36194 385598 5764817 2413 0 2674 0 0 0
cpu3 1291737 36194 385572 5764808 2396 0 2339 0 0 0
intr 2063079 0 9 0 0 0 0 4 0 1 0 0 0 156 0 0 0 0 0 0 0 0 0 0 0 0
ctxt 123456789
btime 1609459200
processes 123456
procs_running 2
procs_blocked 0
`
}

// SampleProcUptime returns sample /proc/uptime content.
func SampleProcUptime() string {
	return `12345.67 98765.43`
}

// SampleProcCPUInfo returns sample /proc/cpuinfo content.
func SampleProcCPUInfo() string {
	return `processor	: 0
vendor_id	: GenuineIntel
cpu family	: 6
model		: 158
model name	: Intel(R) Core(TM) i7-8700K CPU @ 3.70GHz
stepping	: 10
microcode	: 0xea
cpu MHz		: 3700.000
cache size	: 12288 KB
physical id	: 0
siblings	: 12
core id		: 0
cpu cores	: 6
apicid		: 0
initial apicid	: 0
fpu		: yes
fpu_exception	: yes
cpuid level	: 22
wp		: yes
flags		: fpu vme de pse tsc msr pae mce cx8 apic sep mtrr pge mca cmov pat pse36 clflush dts acpi mmx fxsr sse sse2 ss ht tm pbe syscall nx pdpe1gb rdtscp lm constant_tsc art arch_perfmon pebs bts rep_good nopl xtopology nonstop_tsc cpuid aperfmperf pni pclmulqdq dtes64 monitor ds_cpl vmx smx est tm2 ssse3 sdbg fma cx16 xtpr pdcm pcid sse4_1 sse4_2 x2apic movbe popcnt tsc_deadline_timer aes xsave avx f16c rdrand lahf_lm abm 3dnowprefetch cpuid_fault epb invpcid_single pti ssbd ibrs ibpb stibp tpr_shadow vnmi flexpriority ept vpid ept_ad fsgsbase tsc_adjust bmi1 hle avx2 smep bmi2 erms invpcid rtm mpx rdseed adx smap clflushopt intel_pt xsaveopt xsavec xgetbv1 xsaves dtherm ida arat pln pts hwp hwp_notify hwp_act_window hwp_epp md_clear flush_l1d
vmx flags	: vnmi preemption_timer invvpid ept_x_only ept_ad ept_1gb flexpriority tsc_offset vtpr mtf vapic ept vpid unrestricted_guest ple shadow_vmcs pml ept_mode_based_exec
bugs		: cpu_meltdown spectre_v1 spectre_v2 spec_store_bypass l1tf mds swapgs itlb_multihit
bogomips	: 7399.70
clflush size	: 64
cache_alignment	: 64
address sizes	: 39 bits physical, 48 bits virtual
power management:

processor	: 1
vendor_id	: GenuineIntel
cpu family	: 6
model		: 158
model name	: Intel(R) Core(TM) i7-8700K CPU @ 3.70GHz
stepping	: 10
cpu MHz		: 3700.000
physical id	: 0
siblings	: 12
core id		: 1
cpu cores	: 6
flags		: fpu vme de pse tsc msr pae mce cx8 apic sep mtrr pge mca cmov pat pse36 clflush dts acpi mmx fxsr sse sse2 ss ht tm pbe syscall nx pdpe1gb rdtscp lm constant_tsc art arch_perfmon pebs bts rep_good nopl xtopology nonstop_tsc cpuid aperfmperf pni pclmulqdq dtes64 monitor ds_cpl vmx smx est tm2 ssse3 sdbg fma cx16 xtpr pdcm pcid sse4_1 sse4_2 x2apic movbe popcnt tsc_deadline_timer aes xsave avx f16c rdrand lahf_lm abm 3dnowprefetch cpuid_fault epb invpcid_single pti ssbd ibrs ibpb stibp
bogomips	: 7399.70
`
}

// SampleDmidecodeOutput returns sample dmidecode output for BIOS (type 0).
func SampleDmidecodeOutput() string {
	return `# dmidecode 3.3
Getting SMBIOS data from sysfs.
SMBIOS 3.1.1 present.

Handle 0x0000, DMI type 0, 26 bytes
BIOS Information
	Vendor: American Megatrends Inc.
	Version: 1.80
	Release Date: 05/17/2019
	Address: 0xF0000
	Runtime Size: 64 kB
	ROM Size: 16 MB
	Characteristics:
		PCI is supported
		BIOS is upgradeable
	BIOS Revision: 5.13
`
}

// SampleEthtoolOutput returns sample ethtool output.
func SampleEthtoolOutput() string {
	return `Settings for eth0:
	Supported ports: [ TP ]
	Supported link modes:   10baseT/Half 10baseT/Full
	                        100baseT/Half 100baseT/Full
	                        1000baseT/Full
	Supported pause frame use: Symmetric
	Supports auto-negotiation: Yes
	Supported FEC modes: Not reported
	Advertised link modes:  10baseT/Half 10baseT/Full
	                        100baseT/Half 100baseT/Full
	                        1000baseT/Full
	Advertised pause frame use: Symmetric
	Advertised auto-negotiation: Yes
	Advertised FEC modes: Not reported
	Speed: 1000Mb/s
	Duplex: Full
	Auto-negotiation: on
	Port: Twisted Pair
	PHYAD: 1
	Transceiver: internal
	MDI-X: off (auto)
	Supports Wake-on: pumbg
	Wake-on: g
	Current message level: 0x00000007 (7)
	                       drv probe link
	Link detected: yes
`
}

// SampleINIFile returns sample INI file content.
func SampleINIFile() string {
	return `version="7.2.0"
name="Tower"
timeZone="America/Los_Angeles"
port=80
localMaster=yes
flashGUID=1234-5678-9ABC-DEF0
`
}

// SampleSensorsOutput returns sample sensors -u output.
func SampleSensorsOutput() string {
	return `coretemp-isa-0000
Adapter: ISA adapter
Core 0:
  temp2_input: 45.000
  temp2_max: 100.000
  temp2_crit: 100.000
Core 1:
  temp3_input: 46.000
  temp3_max: 100.000
  temp3_crit: 100.000
MB Temp:
  temp1_input: 38.000

nct6792-isa-0a20
Adapter: ISA adapter
fan1:
  fan1_input: 1200.000
fan2:
  fan2_input: 800.000
`
}

// SampleDockerPSOutput returns sample docker ps --format json output.
func SampleDockerPSOutput() string {
	return `{"ID":"abc123","Names":"nginx","Image":"nginx:latest","State":"running","Status":"Up 2 hours"}
{"ID":"def456","Names":"redis","Image":"redis:alpine","State":"running","Status":"Up 1 hour"}`
}

// SampleVirshListOutput returns sample virsh list --all output.
func SampleVirshListOutput() string {
	return ` Id   Name        State
-----------------------------
 1    ubuntu20    running
 -    windows10   shut off
 -    debian11    shut off
`
}

// SampleArrayINI returns sample array configuration.
func SampleArrayINI() string {
	return `mdState=STARTED
mdNumDisks=4
mdNumParity=1
sbSynced="Mon Jan  1 00:00:01 2024 18645 MB/s + 38044 MB/s"
sbSynced2=0
`
}

// SampleDisksINI returns sample disks configuration.
func SampleDisksINI() string {
	return `[disk1]
name=disk1
device=sda
id=WDC_WD40EFAX-68JH4N1_WD-WX11D80D1234
size=4000787030016
status=DISK_OK
temp=35

[disk2]
name=disk2
device=sdb
id=WDC_WD40EFAX-68JH4N1_WD-WX11D80D5678
size=4000787030016
status=DISK_OK
temp=36
`
}

// SampleNetworkINI returns sample network configuration.
func SampleNetworkINI() string {
	return `[eth0]
NAME=eth0
IPADDR=192.168.1.100
NETMASK=255.255.255.0
GATEWAY=192.168.1.1
DNS_SERVER1=8.8.8.8
DNS_SERVER2=8.8.4.4
`
}

// SampleSharesINI returns sample shares configuration.
func SampleSharesINI() string {
	return `[appdata]
name=appdata
comment=Application Data
allocator=highwater
splitLevel=
include=disk1,disk2
exclude=
useCache=yes

[media]
name=media
comment=Media Files
allocator=highwater
splitLevel=
include=
exclude=
useCache=no
`
}

// SampleNvidiaSMIOutput returns sample nvidia-smi output.
func SampleNvidiaSMIOutput() string {
	return `==============NVSMI LOG==============

Timestamp                                 : Thu Jan  1 00:00:00 2024
Driver Version                            : 535.154.05
CUDA Version                              : 12.2

Attached GPUs                             : 1
GPU 00000000:01:00.0
    Product Name                          : NVIDIA GeForce RTX 3080
    Product Brand                         : GeForce
    GPU UUID                              : GPU-12345678-1234-1234-1234-123456789abc
    Fan Speed                             : 45 %
    Temperature
        GPU Current Temp                  : 55 C
        GPU Shutdown Temp                 : 98 C
        GPU Max Operating Temp            : 93 C
    Power Readings
        Power Draw                        : 120.50 W
        Power Limit                       : 320.00 W
    Memory Usage
        Total                             : 10240 MiB
        Used                              : 2048 MiB
        Free                              : 8192 MiB
    Utilization
        Gpu                               : 25 %
        Memory                            : 20 %
`
}

// SampleUPSOutput returns sample apcaccess output.
func SampleUPSOutput() string {
	return `APC      : 001,034,0856
DATE     : 2024-01-01 00:00:00 +0000
HOSTNAME : tower
VERSION  : 3.14.14
UPSNAME  : Back-UPS RS 1500
CABLE    : USB Cable
DRIVER   : USB UPS Driver
UPSMODE  : Stand Alone
STARTTIME: 2024-01-01 00:00:00 +0000
MODEL    : Back-UPS RS 1500MS
STATUS   : ONLINE
LINEV    : 120.0 Volts
LOADPCT  : 25.0 Percent
BCHARGE  : 100.0 Percent
TIMELEFT : 45.0 Minutes
MBATTCHG : 5 Percent
MINTIMEL : 3 Minutes
MAXTIME  : 0 Seconds
OUTPUTV  : 120.0 Volts
SENSE    : Medium
DWAKE    : -1 Seconds
DSHUTD   : 0 Seconds
LOTRANS  : 88.0 Volts
HITRANS  : 139.0 Volts
ALARMDEL : 30 Seconds
BATTV    : 27.1 Volts
LASTXFER : Automatic or explicit self test
NUMXFERS : 0
TONBATT  : 0 Seconds
CUMONBATT: 0 Seconds
XOFFBATT : N/A
SELFTEST : NO
STATFLAG : 0x05000008
SERIALNO : 1B2345C67890
BATTDATE : 2023-01-15
NOMINV   : 120 Volts
NOMBATTV : 24.0 Volts
NOMPOWER : 900 Watts
FIRMWARE : 928.a9 .D USB FW:a9
END APC  : 2024-01-01 00:00:00 +0000
`
}

// SampleZFSPoolOutput returns sample zpool list output.
func SampleZFSPoolOutput() string {
	return `NAME    SIZE  ALLOC   FREE  CKPOINT  EXPANDSZ   FRAG    CAP  DEDUP    HEALTH  ALTROOT
pool1  3.62T  1.21T  2.41T        -         -     5%    33%  1.00x    ONLINE  -
pool2  7.27T  3.50T  3.77T        -         -    10%    48%  1.00x    ONLINE  -
`
}

// SampleZFSDatasetOutput returns sample zfs list output.
func SampleZFSDatasetOutput() string {
	return `NAME                   USED  AVAIL     REFER  MOUNTPOINT
pool1                 1.21T  2.30T       96K  /mnt/pool1
pool1/data            500G  2.30T      500G  /mnt/pool1/data
pool1/backup          720G  2.30T      720G  /mnt/pool1/backup
`
}
