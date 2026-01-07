package collectors

import (
	"testing"
	"time"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestNewVMCollector(t *testing.T) {
	hub := pubsub.New(10)
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
	hub := pubsub.New(10)
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
	hub := pubsub.New(10)
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
	hub := pubsub.New(10)
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
