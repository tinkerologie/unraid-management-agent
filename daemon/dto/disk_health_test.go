package dto

import "testing"

func TestDiskInfoHealthClass(t *testing.T) {
	tests := []struct {
		name string
		disk DiskInfo
		want DiskHealthClass
	}{
		{"ok passed 32C", DiskInfo{Status: "DISK_OK", SMARTStatus: "PASSED", Temperature: new(32.0), Role: "data"}, DiskHealthHealthy},
		{"ok passed at 50C threshold", DiskInfo{Status: "DISK_OK", SMARTStatus: "PASSED", Temperature: new(50.0), Role: "data"}, DiskHealthHealthy},
		{"ok passed 51C", DiskInfo{Status: "DISK_OK", SMARTStatus: "PASSED", Temperature: new(51.0), Role: "data"}, DiskHealthWarning},
		{"ok passed no temperature", DiskInfo{Status: "DISK_OK", SMARTStatus: "PASSED", Role: "parity"}, DiskHealthHealthy},
		{"standby smart unknown", DiskInfo{Status: "DISK_OK", SMARTStatus: "UNKNOWN", SpinState: "standby", Role: "data"}, DiskHealthUnknown},
		{"smart status empty", DiskInfo{Status: "DISK_OK", Role: "data"}, DiskHealthUnknown},
		{"native status empty", DiskInfo{SMARTStatus: "PASSED", Role: "data"}, DiskHealthUnknown},
		{"hot standby smart unknown", DiskInfo{Status: "DISK_OK", SMARTStatus: "UNKNOWN", Temperature: new(55.0), Role: "data"}, DiskHealthWarning},
		{"ok smart failed", DiskInfo{Status: "DISK_OK", SMARTStatus: "FAILED", Role: "data"}, DiskHealthWarning},
		{"disabled smart passed", DiskInfo{Status: "DISK_DSBL", SMARTStatus: "PASSED", Role: "data"}, DiskHealthWarning},
		{"vacant parity2 slot", DiskInfo{Name: "parity2", Status: "DISK_NP_DSBL", Role: "parity2"}, DiskHealthExcluded},
		{"missing assigned disk keeps id", DiskInfo{ID: "WDC_WD140EDGZ_7LGH9KAK", Status: "DISK_NP_DSBL", Role: "data"}, DiskHealthWarning},
		{"missing assigned disk keeps device", DiskInfo{Device: "sdf", Status: "DISK_NP_MISSING", Role: "data"}, DiskHealthWarning},
		{"hot failing flash", DiskInfo{Status: "DISK_DSBL", SMARTStatus: "FAILED", Temperature: new(70.0), Role: "flash"}, DiskHealthExcluded},
		{"hot failing docker vdisk", DiskInfo{Status: "DISK_INVALID", SMARTStatus: "FAILED", Temperature: new(70.0), Role: "docker_vdisk"}, DiskHealthExcluded},
		{"hot failing log filesystem", DiskInfo{Status: "DISK_INVALID", SMARTStatus: "FAILED", Temperature: new(70.0), Role: "log"}, DiskHealthExcluded},
		{"unknown role pool disk", DiskInfo{Device: "nvme1n1", Status: "DISK_OK", SMARTStatus: "PASSED", Temperature: new(38.0), Role: "unknown"}, DiskHealthHealthy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.disk.HealthClass(); got != tt.want {
				t.Errorf("HealthClass() = %q, want %q", got, tt.want)
			}
		})
	}
	var missing *DiskInfo
	if got := missing.HealthClass(); got != DiskHealthExcluded {
		t.Errorf("HealthClass() on nil disk = %q, want %q", got, DiskHealthExcluded)
	}
}
