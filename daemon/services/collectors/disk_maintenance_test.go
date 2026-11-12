package collectors

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestDiskCollectionUsesNativeTelemetryWithoutProbingDuringMaintenance(t *testing.T) {
	hub := domain.NewEventBus(8)
	sub := hub.Sub(constants.TopicDiskListUpdate.Name)
	t.Cleanup(func() { hub.Unsub(sub) })
	c := NewDiskCollector(&domain.Context{Hub: hub})
	dir := t.TempDir()
	c.varIni = filepath.Join(dir, "var.ini")
	c.disksIni = filepath.Join(dir, "disks.ini")
	if err := os.WriteFile(c.disksIni, []byte("[\"disk1\"]\nname=disk1\nid=fixture-disk\ndevice=fixture\nsize=40\ntemp=33\nnumErrors=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	smartCalls, statCalls, dockerCalls := 0, 0, 0
	c.probeSMART = func(disk *dto.DiskInfo) {
		smartCalls++
		disk.SMARTStatus = "PASSED"
		disk.SMARTExitStatus = dto.SMARTExitErrorLog
		if smartCalls == 2 {
			disk.SMARTStatus = "UNKNOWN"
			disk.SMARTExitStatus = dto.SMARTExitDeviceOpen
		}
	}
	dockerSample := time.Now()
	c.probeDockerDisk = func() *dto.DiskInfo {
		dockerCalls++
		return &dto.DiskInfo{ID: "docker_vdisk", Timestamp: dockerSample}
	}
	mockDiskMounts(t, "/dev/md1p1 /mnt/disk1 xfs rw 0 0\n", func(_ string, stat *syscall.Statfs_t) error {
		statCalls++
		stat.Blocks = 10
		stat.Bfree = 3
		stat.Bsize = 4096
		return nil
	})

	for _, step := range []struct {
		native, smart string
		probes        int
		exit          int
	}{
		{"mdResync=1\nmdResyncPos=0\n", "UNKNOWN", 0, 0},
		{"mdResync=0\nmdResyncPos=3\n", "UNKNOWN", 0, 0},
		{"mdResync=invalid\n", "UNKNOWN", 0, 0},
		{"mdResync=0\nmdResyncPos=0\n", "PASSED", 1, dto.SMARTExitErrorLog},
		{"mdResync=0\nmdResyncPos=0\n", "UNKNOWN", 2, dto.SMARTExitDeviceOpen},
		{"mdResync=1\nmdResyncPos=1\n", "PASSED", 2, 0},
	} {
		if err := os.WriteFile(c.varIni, []byte(step.native), 0o600); err != nil {
			t.Fatal(err)
		}
		c.Collect()
		select {
		case event := <-sub:
			disks := event.([]dto.DiskInfo)
			disk := disks[0]
			if disk.Name != "disk1" || disk.MountPoint != "/mnt/disk1" || disk.Temperature == nil ||
				*disk.Temperature != 33 || disk.SMARTErrors != 2 || disk.SMARTStatus != step.smart ||
				disk.SMARTExitStatus != step.exit || smartCalls != step.probes || statCalls != step.probes {
				t.Fatalf("state=%q: disk=%+v SMART probes=%d capacity probes=%d", step.native, disk, smartCalls, statCalls)
			}
			if dockerCalls != step.probes {
				t.Fatalf("state=%q: Docker vDisk probes=%d, want %d", step.native, dockerCalls, step.probes)
			}
			if step.probes > 0 && (len(disks) < 2 || disks[1].ID != "docker_vdisk" || !disks[1].Timestamp.Equal(dockerSample)) {
				t.Fatalf("state=%q: last Docker vDisk sample missing", step.native)
			}
		case <-time.After(time.Second):
			t.Fatal("Disk Health did not publish")
		}
	}
}
