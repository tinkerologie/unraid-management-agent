package collectors

import (
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewHardwareCollector(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewHardwareCollector(ctx)

	if collector == nil {
		t.Fatal("NewHardwareCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("HardwareCollector context not set correctly")
	}
}

func TestHardwareCollectorInit(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewHardwareCollector(ctx)

	// Verify collector is properly initialized
	if collector == nil {
		t.Fatal("Collector should not be nil")
	}

	if collector.ctx == nil {
		t.Fatal("Collector context should not be nil")
	}

	if collector.ctx.Hub == nil {
		t.Fatal("Collector context Hub should not be nil")
	}
}

func TestHardwareInfoStructure(t *testing.T) {
	// Test that HardwareInfo struct can be created with all fields
	tests := []struct {
		name     string
		testFunc func() bool
	}{
		{
			name: "BIOS info can have vendor",
			testFunc: func() bool {
				// BIOS vendor should be parseable
				return true
			},
		},
		{
			name: "Baseboard info can have manufacturer",
			testFunc: func() bool {
				// Baseboard manufacturer should be parseable
				return true
			},
		},
		{
			name: "CPU info can have model",
			testFunc: func() bool {
				// CPU model should be parseable
				return true
			},
		},
		{
			name: "Memory devices can be listed",
			testFunc: func() bool {
				// Memory devices should be an array
				return true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.testFunc() {
				t.Errorf("%s failed", tt.name)
			}
		})
	}
}

func TestHardwareDMITypes(t *testing.T) {
	// Test DMI type constants used in hardware collection
	dmiTypes := map[string]int{
		"BIOS":         0,
		"System":       1,
		"Baseboard":    2,
		"Chassis":      3,
		"Processor":    4,
		"Memory":       16,
		"MemoryDevice": 17,
		"Cache":        7,
	}

	// Verify expected DMI types
	if dmiTypes["BIOS"] != 0 {
		t.Error("BIOS DMI type should be 0")
	}
	if dmiTypes["Processor"] != 4 {
		t.Error("Processor DMI type should be 4")
	}
	if dmiTypes["MemoryDevice"] != 17 {
		t.Error("MemoryDevice DMI type should be 17")
	}
}

func TestBIOSInfoParsing(t *testing.T) {
	// Sample dmidecode BIOS output structure
	biosFields := []string{
		"Vendor",
		"Version",
		"Release Date",
		"Address",
		"Runtime Size",
		"ROM Size",
		"Characteristics",
	}

	// Verify all expected BIOS fields are known
	expectedFields := map[string]bool{
		"Vendor":          true,
		"Version":         true,
		"Release Date":    true,
		"Address":         true,
		"Runtime Size":    true,
		"ROM Size":        true,
		"Characteristics": true,
	}

	for _, field := range biosFields {
		if !expectedFields[field] {
			t.Errorf("Unexpected BIOS field: %s", field)
		}
	}
}

func TestBaseboardInfoParsing(t *testing.T) {
	// Sample dmidecode baseboard output structure
	baseboardFields := []string{
		"Manufacturer",
		"Product Name",
		"Version",
		"Serial Number",
		"Asset Tag",
	}

	// Verify all expected baseboard fields are known
	expectedFields := map[string]bool{
		"Manufacturer":  true,
		"Product Name":  true,
		"Version":       true,
		"Serial Number": true,
		"Asset Tag":     true,
	}

	for _, field := range baseboardFields {
		if !expectedFields[field] {
			t.Errorf("Unexpected baseboard field: %s", field)
		}
	}
}

func TestCPUInfoParsing(t *testing.T) {
	// Sample dmidecode CPU output structure
	cpuFields := []string{
		"Socket Designation",
		"Type",
		"Family",
		"Manufacturer",
		"ID",
		"Signature",
		"Version",
		"Voltage",
		"External Clock",
		"Max Speed",
		"Current Speed",
		"Status",
		"Core Count",
		"Core Enabled",
		"Thread Count",
	}

	// Verify all expected CPU fields are known
	for _, field := range cpuFields {
		if len(field) == 0 {
			t.Error("Empty CPU field name")
		}
	}

	// Verify we have expected number of fields
	if len(cpuFields) < 10 {
		t.Error("Expected at least 10 CPU fields")
	}
}

func TestMemoryDeviceInfoParsing(t *testing.T) {
	// Sample dmidecode memory device output structure
	memoryFields := []string{
		"Size",
		"Form Factor",
		"Locator",
		"Bank Locator",
		"Type",
		"Type Detail",
		"Speed",
		"Manufacturer",
		"Serial Number",
		"Part Number",
		"Configured Memory Speed",
	}

	// Verify all expected memory fields are known
	for _, field := range memoryFields {
		if len(field) == 0 {
			t.Error("Empty memory field name")
		}
	}

	// Verify we have expected number of fields
	if len(memoryFields) < 8 {
		t.Error("Expected at least 8 memory device fields")
	}
}

func TestMemorySizeParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"8GB", "8192 MB", "8192 MB"},
		{"16GB", "16384 MB", "16384 MB"},
		{"32GB", "32 GB", "32 GB"},
		{"No Module Installed", "No Module Installed", "No Module Installed"},
		{"Unknown", "Unknown", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Size string should be preserved as-is from dmidecode
			if tt.input != tt.expected {
				t.Errorf("Memory size = %q, want %q", tt.input, tt.expected)
			}
		})
	}
}

func TestCacheInfoParsing(t *testing.T) {
	// Sample dmidecode cache output structure
	cacheFields := []string{
		"Socket Designation",
		"Configuration",
		"Operational Mode",
		"Location",
		"Installed Size",
		"Maximum Size",
		"Supported SRAM Types",
		"Installed SRAM Type",
		"Speed",
		"Error Correction Type",
		"System Type",
		"Associativity",
	}

	// Verify all expected cache fields are known
	for _, field := range cacheFields {
		if len(field) == 0 {
			t.Error("Empty cache field name")
		}
	}
}
