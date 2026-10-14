package collectors

import (
	"errors"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/digitalocean/go-libvirt"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestNewVMCollector(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{
		Hub: hub,
	}

	collector := NewVMCollector(ctx)

	if collector == nil {
		t.Fatal("Expected collector to be created")
	}

	if collector.appCtx != ctx {
		t.Error("Expected appCtx to be set")
	}

	if collector.previousStats == nil {
		t.Error("Expected previousStats map to be initialized")
	}
}

func TestVMCollector_Collect_NoLibvirt(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{
		Hub: hub,
	}

	collector := NewVMCollector(ctx)

	// Subscribe to events
	sub := hub.Sub("vm_list_update")

	// Collect should not panic even without libvirt
	collector.Collect()

	// Should receive an event (empty list or actual VMs)
	select {
	case msg := <-sub:
		if msg == nil {
			t.Error("Expected non-nil message")
		}
		// Message should be a slice of VMInfo
		vms, ok := msg.([]*dto.VMInfo)
		if !ok {
			t.Errorf("Expected []*dto.VMInfo, got %T", msg)
		}
		// In test environment without libvirt, should be empty
		t.Logf("Received %d VMs", len(vms))
	case <-time.After(2 * time.Second):
		t.Error("Expected to receive vm_list_update event")
	}
}

func TestVMCollector_FormatMemoryDisplay(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{
		Hub: hub,
	}

	collector := NewVMCollector(ctx)

	tests := []struct {
		name      string
		used      uint64
		allocated uint64
		expected  string
	}{
		{
			name:      "zero allocated",
			used:      0,
			allocated: 0,
			expected:  "0 B / 0 B",
		},
		{
			name:      "megabytes",
			used:      512 * 1024 * 1024,   // 512 MB
			allocated: 1024 * 1024 * 1024,  // 1 GB
			expected:  "0.50 GB / 1.00 GB", // GB format since allocated >= 1GB
		},
		{
			name:      "gigabytes",
			used:      4 * 1024 * 1024 * 1024, // 4 GB
			allocated: 8 * 1024 * 1024 * 1024, // 8 GB
			expected:  "4.00 GB / 8.00 GB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.formatMemoryDisplay(tt.used, tt.allocated)
			if result != tt.expected {
				t.Errorf("formatMemoryDisplay(%d, %d) = %s, expected %s", tt.used, tt.allocated, result, tt.expected)
			}
		})
	}
}

func TestExtractDiskTargets(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		expected []string
	}{
		{
			name:     "empty xml",
			xml:      "",
			expected: []string{},
		},
		{
			name:     "single disk",
			xml:      `<disk type='file'><target dev="vda"/></disk>`,
			expected: []string{"vda"},
		},
		{
			name:     "multiple disks",
			xml:      `<disk><target dev="vda"/></disk><disk><target dev="vdb"/></disk>`,
			expected: []string{"vda", "vdb"},
		},
		{
			name:     "sata disks",
			xml:      `<disk><target dev="sda"/></disk><disk><target dev="sdb"/></disk>`,
			expected: []string{"sda", "sdb"},
		},
		{
			// libvirt's own XML uses single quotes
			name: "libvirt single quotes",
			xml: `<disk type='file' device='disk'><driver name='qemu' type='raw'/>` +
				`<source file='/mnt/user/domains/vm/vdisk1.img'/><target dev='hdc' bus='sata'/></disk>`,
			expected: []string{"hdc"},
		},
		{
			name: "interface targets are not disks",
			xml: `<disk type='block' device='disk'><target dev='vda' bus='virtio'/></disk>` +
				`<interface type='bridge'><target dev='vnet0'/></interface>`,
			expected: []string{"vda"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDiskTargets(tt.xml)
			if len(result) != len(tt.expected) {
				t.Errorf("extractDiskTargets() returned %d items, expected %d", len(result), len(tt.expected))
				return
			}
			for i, target := range result {
				if target != tt.expected[i] {
					t.Errorf("extractDiskTargets()[%d] = %s, expected %s", i, target, tt.expected[i])
				}
			}
		})
	}
}

func TestExtractInterfaceTargets(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		expected []string
	}{
		{
			name:     "empty xml",
			xml:      "",
			expected: []string{},
		},
		{
			name:     "single interface double quote",
			xml:      `<interface type='network'><target dev="vnet0"/></interface>`,
			expected: []string{"vnet0"},
		},
		{
			name:     "single interface single quote",
			xml:      `<interface type='network'><target dev='vnet0'/></interface>`,
			expected: []string{"vnet0"},
		},
		{
			name:     "multiple interfaces",
			xml:      `<interface type='network'><target dev="vnet0"/></interface><interface type='bridge'><target dev="vnet1"/></interface>`,
			expected: []string{"vnet0", "vnet1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractInterfaceTargets(tt.xml)
			if len(result) != len(tt.expected) {
				t.Errorf("extractInterfaceTargets() returned %d items, expected %d", len(result), len(tt.expected))
				return
			}
			for i, target := range result {
				if target != tt.expected[i] {
					t.Errorf("extractInterfaceTargets()[%d] = %s, expected %s", i, target, tt.expected[i])
				}
			}
		})
	}
}

func TestVMCollector_ClearCPUStats(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{
		Hub: hub,
	}

	collector := NewVMCollector(ctx)

	// Add some stats
	collector.previousStats["test-vm"] = &vmCPUStats{
		guestCPUTime: 1000,
		timestamp:    time.Now(),
	}

	if len(collector.previousStats) != 1 {
		t.Error("Expected 1 entry in previousStats")
	}

	// Clear stats
	collector.clearCPUStats("test-vm")

	if len(collector.previousStats) != 0 {
		t.Error("Expected previousStats to be empty after clear")
	}

	// Clear non-existent should not panic
	collector.clearCPUStats("non-existent")
}

// TestResolveHostCPUCount covers the host CPU count fallbacks (#175).
func TestResolveHostCPUCount(t *testing.T) {
	if got := resolveHostCPUCount(128, nil); got != 128 {
		t.Errorf("libvirt count: got %d, want 128", got)
	}
	want := max(runtime.NumCPU(), 1)
	if got := resolveHostCPUCount(0, nil); got != want {
		t.Errorf("zero libvirt count: got %d, want runtime.NumCPU() %d", got, want)
	}
	if got := resolveHostCPUCount(64, errors.New("rpc failed")); got != want {
		t.Errorf("libvirt error: got %d, want runtime.NumCPU() %d", got, want)
	}
}

// TestCPUPercent covers the shared percentage calculation and its guards.
func TestCPUPercent(t *testing.T) {
	tests := []struct {
		name      string
		prev, cur uint64
		elapsed   float64
		divisor   int
		want      float64
		valid     bool
	}{
		{"4 vCPUs fully busy", 0, 4 * 30e9, 30, 4, 100, true},
		{"4 busy vCPUs on a 128-thread host", 0, 4 * 30e9, 30, 128, 3.125, true},
		{"half busy", 10e9, 25e9, 30, 1, 50, true},
		{"idle", 5e9, 5e9, 30, 2, 0, true},
		{"clamped to 100", 0, 90e9, 30, 1, 100, true},
		{"counter reset", 50e9, 10e9, 30, 2, 0, false},
		{"no elapsed time", 0, 10e9, 0, 2, 0, false},
		{"negative elapsed time", 0, 10e9, -1, 2, 0, false},
		{"no CPUs", 0, 10e9, 30, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, valid := cpuPercent(tt.prev, tt.cur, tt.elapsed, tt.divisor)
			if valid != tt.valid || math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("cpuPercent = %v, %v; want %v, %v", got, valid, tt.want, tt.valid)
			}
		})
	}
}

// TestVMCollectorRecordCPUSample covers the per-VM history: baseline on the
// first sample, guest and host shares on the next, and a counter reset.
func TestVMCollectorRecordCPUSample(t *testing.T) {
	c := NewVMCollector(&domain.Context{Hub: domain.NewEventBus(10)})
	start := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

	first := &dto.VMInfo{CPUCount: 4}
	c.recordCPUSample("win11", 100e9, start, 128, first)
	if first.GuestCPUPercent != 0 || first.HostCPUPercent != 0 {
		t.Fatalf("first sample must only store a baseline: %+v", first)
	}

	second := &dto.VMInfo{CPUCount: 4}
	c.recordCPUSample("win11", 100e9+4*30e9, start.Add(30*time.Second), 128, second)
	if math.Abs(second.GuestCPUPercent-100) > 1e-9 || math.Abs(second.HostCPUPercent-3.125) > 1e-9 {
		t.Fatalf("guest/host = %v/%v, want 100/3.125", second.GuestCPUPercent, second.HostCPUPercent)
	}

	reset := &dto.VMInfo{CPUCount: 4}
	c.recordCPUSample("win11", 5e9, start.Add(60*time.Second), 128, reset)
	if reset.GuestCPUPercent != 0 || reset.HostCPUPercent != 0 {
		t.Fatalf("counter reset must not report usage: %+v", reset)
	}
	after := &dto.VMInfo{CPUCount: 4}
	c.recordCPUSample("win11", 5e9+2*30e9, start.Add(90*time.Second), 128, after)
	if math.Abs(after.GuestCPUPercent-50) > 1e-9 {
		t.Fatalf("after reset the new baseline must be used: guest = %v, want 50", after.GuestCPUPercent)
	}

	// Unknown vCPU count: no guest share, but the host share is still reported.
	c.recordCPUSample("novcpu", 0, start, 8, &dto.VMInfo{})
	noVCPU := &dto.VMInfo{}
	c.recordCPUSample("novcpu", 8*30e9, start.Add(30*time.Second), 8, noVCPU)
	if noVCPU.GuestCPUPercent != 0 || math.Abs(noVCPU.HostCPUPercent-100) > 1e-9 {
		t.Fatalf("CPUCount 0: guest/host = %v/%v, want 0/100", noVCPU.GuestCPUPercent, noVCPU.HostCPUPercent)
	}
}

// fakeLibvirtCPU stands in for the libvirt calls of the CPU path.
type fakeLibvirtCPU struct {
	cpus    int32
	nodeErr error
	cpuTime uint64
	infoErr error
}

func (f *fakeLibvirtCPU) NodeGetInfo() (rModel [32]int8, rMemory uint64, rCpus int32, rMhz int32, rNodes int32, rSockets int32, rCores int32, rThreads int32, err error) {
	return rModel, 0, f.cpus, 0, 0, 0, 0, 0, f.nodeErr
}

func (f *fakeLibvirtCPU) DomainGetInfo(libvirt.Domain) (rState uint8, rMaxMem uint64, rMemory uint64, rNrVirtCPU uint16, rCPUTime uint64, err error) {
	return 1, 0, 0, 4, f.cpuTime, f.infoErr
}

// TestVMCollectorHostCPUCount checks the libvirt host CPU count and its fallbacks.
func TestVMCollectorHostCPUCount(t *testing.T) {
	c := NewVMCollector(&domain.Context{Hub: domain.NewEventBus(10)})
	if got := c.hostCPUCount(&fakeLibvirtCPU{cpus: 128}); got != 128 {
		t.Errorf("hostCPUCount = %d, want 128 from NodeGetInfo", got)
	}
	orig := numCPU
	t.Cleanup(func() { numCPU = orig })
	numCPU = func() int { return 12 }
	if got := c.hostCPUCount(&fakeLibvirtCPU{nodeErr: errors.New("rpc failed")}); got != 12 {
		t.Errorf("hostCPUCount on error = %d, want numCPU 12", got)
	}
	numCPU = func() int { return 0 }
	if got := resolveHostCPUCount(0, errors.New("rpc failed")); got != 1 {
		t.Errorf("last-resort count = %d, want 1", got)
	}
}

// TestVMCollectorGetCPUUsage drives getCPUUsage through the libvirt seam: a
// failed CPU query leaves the history alone, a successful one records a sample.
func TestVMCollectorGetCPUUsage(t *testing.T) {
	c := NewVMCollector(&domain.Context{Hub: domain.NewEventBus(10)})
	dom := libvirt.Domain{Name: "win11"}

	c.getCPUUsage(&fakeLibvirtCPU{infoErr: errors.New("domain gone")}, dom, &dto.VMInfo{CPUCount: 4}, 8)
	if _, ok := c.previousStats["win11"]; ok {
		t.Fatal("a failed CPU query must not store history")
	}

	c.getCPUUsage(&fakeLibvirtCPU{cpuTime: 1e9}, dom, &dto.VMInfo{CPUCount: 4}, 8)
	prev, ok := c.previousStats["win11"]
	if !ok || prev.guestCPUTime != 1e9 {
		t.Fatalf("baseline not stored: %+v", prev)
	}
	// Age the baseline so the next sample has elapsed time.
	prev.timestamp = prev.timestamp.Add(-10 * time.Second)
	vm := &dto.VMInfo{CPUCount: 4}
	c.getCPUUsage(&fakeLibvirtCPU{cpuTime: 1e9 + 8*10e9}, dom, vm, 8)
	// Exact values are checked with fixed times in TestVMCollectorRecordCPUSample;
	// here the wall clock runs, so only check that both shares were set.
	if vm.HostCPUPercent <= 0 || vm.GuestCPUPercent != 100 {
		t.Fatalf("guest/host = %v/%v, want 100/>0", vm.GuestCPUPercent, vm.HostCPUPercent)
	}
}
