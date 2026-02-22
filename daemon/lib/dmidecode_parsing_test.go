package lib

import (
	"testing"
)

// TestParseBIOSInfoFromSectionFunc tests the actual parseBIOSInfoFromSection function.
func TestParseBIOSInfoFromSectionFunc(t *testing.T) {
	t.Run("complete BIOS", func(t *testing.T) {
		section := map[string]string{
			"Vendor":          "American Megatrends Inc.",
			"Version":         "1.80",
			"Release Date":    "05/17/2019",
			"Address":         "0xF0000",
			"Runtime Size":    "64 kB",
			"ROM Size":        "16 MB",
			"BIOS Revision":   "5.13",
			"Characteristics": "PCI is supported, BIOS is upgradeable, Boot from CD is supported",
		}
		bios := parseBIOSInfoFromSection(section)
		if bios == nil {
			t.Fatal("expected non-nil BIOS info")
		}
		if bios.Vendor != "American Megatrends Inc." {
			t.Errorf("Vendor = %q", bios.Vendor)
		}
		if bios.Version != "1.80" {
			t.Errorf("Version = %q", bios.Version)
		}
		if bios.ReleaseDate != "05/17/2019" {
			t.Errorf("ReleaseDate = %q", bios.ReleaseDate)
		}
		if bios.Address != "0xF0000" {
			t.Errorf("Address = %q", bios.Address)
		}
		if bios.RuntimeSize != "64 kB" {
			t.Errorf("RuntimeSize = %q", bios.RuntimeSize)
		}
		if bios.ROMSize != "16 MB" {
			t.Errorf("ROMSize = %q", bios.ROMSize)
		}
		if bios.Revision != "5.13" {
			t.Errorf("Revision = %q", bios.Revision)
		}
		if len(bios.Characteristics) != 3 {
			t.Fatalf("expected 3 characteristics, got %d", len(bios.Characteristics))
		}
		if bios.Characteristics[0] != "PCI is supported" {
			t.Errorf("first characteristic = %q", bios.Characteristics[0])
		}
	})

	t.Run("no characteristics", func(t *testing.T) {
		section := map[string]string{"Vendor": "Test"}
		bios := parseBIOSInfoFromSection(section)
		if bios.Characteristics != nil {
			t.Errorf("expected nil characteristics, got %v", bios.Characteristics)
		}
	})

	t.Run("empty section", func(t *testing.T) {
		bios := parseBIOSInfoFromSection(map[string]string{})
		if bios == nil {
			t.Fatal("expected non-nil even for empty section")
		}
		if bios.Vendor != "" {
			t.Errorf("expected empty vendor, got %q", bios.Vendor)
		}
	})
}

// TestParseBaseboardInfoFromSectionFunc tests the actual parseBaseboardInfoFromSection function.
func TestParseBaseboardInfoFromSectionFunc(t *testing.T) {
	t.Run("complete baseboard - ASUS", func(t *testing.T) {
		section := map[string]string{
			"Manufacturer":        "ASUSTeK COMPUTER INC.",
			"Product Name":        "PRIME Z370-A",
			"Version":             "Rev 1.xx",
			"Serial Number":       "180000000000000",
			"Asset Tag":           "Default string",
			"Location In Chassis": "Default string",
			"Type":                "Motherboard",
			"Features":            "Board is a hosting board, Board requires at least one daughter board",
		}
		baseboard := parseBaseboardInfoFromSection(section)
		if baseboard.Manufacturer != "ASUSTeK COMPUTER INC." {
			t.Errorf("Manufacturer = %q", baseboard.Manufacturer)
		}
		if baseboard.ProductName != "PRIME Z370-A" {
			t.Errorf("ProductName = %q", baseboard.ProductName)
		}
		if baseboard.Version != "Rev 1.xx" {
			t.Errorf("Version = %q", baseboard.Version)
		}
		if baseboard.SerialNumber != "180000000000000" {
			t.Errorf("SerialNumber = %q", baseboard.SerialNumber)
		}
		if baseboard.Type != "Motherboard" {
			t.Errorf("Type = %q", baseboard.Type)
		}
		if len(baseboard.Features) != 2 {
			t.Fatalf("expected 2 features, got %d", len(baseboard.Features))
		}
	})

	t.Run("Supermicro board without features", func(t *testing.T) {
		section := map[string]string{
			"Manufacturer": "Supermicro",
			"Product Name": "X11SCL-F",
			"Type":         "Motherboard",
		}
		baseboard := parseBaseboardInfoFromSection(section)
		if baseboard.Manufacturer != "Supermicro" {
			t.Errorf("Manufacturer = %q", baseboard.Manufacturer)
		}
		if baseboard.Features != nil {
			t.Errorf("expected nil features, got %v", baseboard.Features)
		}
	})

	t.Run("empty section", func(t *testing.T) {
		baseboard := parseBaseboardInfoFromSection(map[string]string{})
		if baseboard == nil {
			t.Fatal("expected non-nil")
		}
	})
}

// TestParseCPUInfoFromSectionFunc tests the actual parseCPUInfoFromSection function.
func TestParseCPUInfoFromSectionFunc(t *testing.T) {
	t.Run("complete Intel CPU", func(t *testing.T) {
		section := map[string]string{
			"Socket Designation": "LGA1151",
			"Family":             "Core i7",
			"Manufacturer":       "Intel(R) Corporation",
			"Signature":          "Type 0, Family 6, Model 158, Stepping 10",
			"Voltage":            "1.0 V",
			"Status":             "Populated, Enabled",
			"Upgrade":            "Socket LGA1151",
			"Serial Number":      "To Be Filled By O.E.M.",
			"Asset Tag":          "To Be Filled By O.E.M.",
			"Part Number":        "To Be Filled By O.E.M.",
			"External Clock":     "100 MHz",
			"Max Speed":          "4700 MHz",
			"Current Speed":      "3700 MHz",
			"Core Enabled":       "6",
			"Thread Count":       "12",
			"Flags":              "fpu vme de sse sse2 ht",
			"Characteristics":    "Multi-Core, Hardware Thread, Execute Protection",
		}
		cpu := parseCPUInfoFromSection(section)
		if cpu.SocketDesignation != "LGA1151" {
			t.Errorf("Socket = %q", cpu.SocketDesignation)
		}
		if cpu.Family != "Core i7" {
			t.Errorf("Family = %q", cpu.Family)
		}
		if cpu.ExternalClock != 100 {
			t.Errorf("ExternalClock = %d, want 100", cpu.ExternalClock)
		}
		if cpu.MaxSpeed != 4700 {
			t.Errorf("MaxSpeed = %d, want 4700", cpu.MaxSpeed)
		}
		if cpu.CurrentSpeed != 3700 {
			t.Errorf("CurrentSpeed = %d, want 3700", cpu.CurrentSpeed)
		}
		if cpu.CoreEnabled != 6 {
			t.Errorf("CoreEnabled = %d, want 6", cpu.CoreEnabled)
		}
		if cpu.ThreadCount != 12 {
			t.Errorf("ThreadCount = %d, want 12", cpu.ThreadCount)
		}
		if len(cpu.Flags) != 6 {
			t.Errorf("expected 6 flags, got %d", len(cpu.Flags))
		}
		if len(cpu.Characteristics) != 3 {
			t.Errorf("expected 3 characteristics, got %d", len(cpu.Characteristics))
		}
	})

	t.Run("non-numeric speed", func(t *testing.T) {
		section := map[string]string{
			"Max Speed": "Unknown",
		}
		cpu := parseCPUInfoFromSection(section)
		if cpu.MaxSpeed != 0 {
			t.Errorf("MaxSpeed = %d, want 0 for non-numeric", cpu.MaxSpeed)
		}
	})

	t.Run("empty section", func(t *testing.T) {
		cpu := parseCPUInfoFromSection(map[string]string{})
		if cpu == nil {
			t.Fatal("expected non-nil")
		}
		if cpu.CoreEnabled != 0 {
			t.Errorf("expected 0 cores for empty section")
		}
	})
}

// TestParseCPUCacheInfoFromSectionsFunc tests the actual parseCPUCacheInfoFromSections function.
func TestParseCPUCacheInfoFromSectionsFunc(t *testing.T) {
	t.Run("L1 L2 L3 caches", func(t *testing.T) {
		sections := []map[string]string{
			{
				"Socket Designation":    "L1-Cache",
				"Configuration":         "Enabled, Not Socketed, Level 1",
				"Operational Mode":      "Write Back",
				"Location":              "Internal",
				"Installed Size":        "384 kB",
				"Maximum Size":          "384 kB",
				"Installed SRAM Type":   "Synchronous",
				"Error Correction Type": "Parity",
				"System Type":           "Unified",
				"Associativity":         "8-way Set-associative",
				"Supported SRAM Types":  "Synchronous, Pipeline Burst",
			},
			{
				"Socket Designation": "L2-Cache",
				"Installed Size":     "1536 kB",
			},
			{
				"Socket Designation": "L3-Cache",
				"Installed Size":     "12288 kB",
			},
		}
		caches := parseCPUCacheInfoFromSections(sections)
		if len(caches) != 3 {
			t.Fatalf("expected 3 caches, got %d", len(caches))
		}
		if caches[0].Level != 1 {
			t.Errorf("L1 level = %d", caches[0].Level)
		}
		if caches[1].Level != 2 {
			t.Errorf("L2 level = %d", caches[1].Level)
		}
		if caches[2].Level != 3 {
			t.Errorf("L3 level = %d", caches[2].Level)
		}
		if caches[0].InstalledSize != "384 kB" {
			t.Errorf("InstalledSize = %q", caches[0].InstalledSize)
		}
		if len(caches[0].SupportedSRAMTypes) != 2 {
			t.Errorf("expected 2 SRAM types, got %d", len(caches[0].SupportedSRAMTypes))
		}
	})

	t.Run("unknown level", func(t *testing.T) {
		sections := []map[string]string{
			{"Socket Designation": "Custom Cache"},
		}
		caches := parseCPUCacheInfoFromSections(sections)
		if len(caches) != 1 {
			t.Fatalf("expected 1 cache")
		}
		if caches[0].Level != 0 {
			t.Errorf("expected level 0 for unknown, got %d", caches[0].Level)
		}
	})

	t.Run("empty sections", func(t *testing.T) {
		caches := parseCPUCacheInfoFromSections(nil)
		if caches != nil {
			t.Errorf("expected nil for nil sections")
		}
	})
}

// TestParseMemoryArrayInfoFromSectionFunc tests the actual parseMemoryArrayInfoFromSection function.
func TestParseMemoryArrayInfoFromSectionFunc(t *testing.T) {
	t.Run("standard array", func(t *testing.T) {
		section := map[string]string{
			"Location":              "System Board Or Motherboard",
			"Use":                   "System Memory",
			"Error Correction Type": "None",
			"Maximum Capacity":      "64 GB",
			"Number Of Devices":     "4",
		}
		memArray := parseMemoryArrayInfoFromSection(section)
		if memArray.Location != "System Board Or Motherboard" {
			t.Errorf("Location = %q", memArray.Location)
		}
		if memArray.MaximumCapacity != "64 GB" {
			t.Errorf("MaximumCapacity = %q", memArray.MaximumCapacity)
		}
		if memArray.NumberOfDevices != 4 {
			t.Errorf("NumberOfDevices = %d, want 4", memArray.NumberOfDevices)
		}
	})

	t.Run("ECC memory", func(t *testing.T) {
		section := map[string]string{
			"Error Correction Type": "Multi-bit ECC",
			"Maximum Capacity":      "256 GB",
			"Number Of Devices":     "8",
		}
		memArray := parseMemoryArrayInfoFromSection(section)
		if memArray.ErrorCorrectionType != "Multi-bit ECC" {
			t.Errorf("ErrorCorrectionType = %q", memArray.ErrorCorrectionType)
		}
		if memArray.NumberOfDevices != 8 {
			t.Errorf("NumberOfDevices = %d", memArray.NumberOfDevices)
		}
	})

	t.Run("non-numeric devices", func(t *testing.T) {
		section := map[string]string{
			"Number Of Devices": "Unknown",
		}
		memArray := parseMemoryArrayInfoFromSection(section)
		if memArray.NumberOfDevices != 0 {
			t.Errorf("expected 0, got %d", memArray.NumberOfDevices)
		}
	})
}

// TestParseMemoryDevicesFromSectionsFunc tests the actual parseMemoryDevicesFromSections function.
func TestParseMemoryDevicesFromSectionsFunc(t *testing.T) {
	t.Run("mixed installed and empty slots", func(t *testing.T) {
		sections := []map[string]string{
			{
				"Locator":                 "ChannelA-DIMM0",
				"Bank Locator":            "BANK 0",
				"Size":                    "16 GB",
				"Form Factor":             "DIMM",
				"Type":                    "DDR4",
				"Type Detail":             "Synchronous",
				"Speed":                   "3200 MT/s",
				"Manufacturer":            "Corsair",
				"Serial Number":           "00000000",
				"Part Number":             "CMK32GX4M2B3200C16",
				"Configured Memory Speed": "3200 MT/s",
				"Rank":                    "2",
				"Data Width":              "64 bits",
				"Total Width":             "64 bits",
				"Minimum Voltage":         "1.2 V",
				"Maximum Voltage":         "1.35 V",
				"Configured Voltage":      "1.2 V",
			},
			{
				"Size": "No Module Installed",
			},
			{
				"Size": "",
			},
			{
				"Locator":    "ChannelB-DIMM0",
				"Size":       "16 GB",
				"Type":       "DDR4",
				"Data Width": "64 bits",
				"Rank":       "2",
			},
		}
		devices := parseMemoryDevicesFromSections(sections)
		if len(devices) != 2 {
			t.Fatalf("expected 2 devices (skipping empty), got %d", len(devices))
		}
		if devices[0].Locator != "ChannelA-DIMM0" {
			t.Errorf("Locator = %q", devices[0].Locator)
		}
		if devices[0].Type != "DDR4" {
			t.Errorf("Type = %q", devices[0].Type)
		}
		if devices[0].Rank != 2 {
			t.Errorf("Rank = %d, want 2", devices[0].Rank)
		}
		if devices[0].DataWidth != 64 {
			t.Errorf("DataWidth = %d, want 64", devices[0].DataWidth)
		}
		if devices[0].TotalWidth != 64 {
			t.Errorf("TotalWidth = %d, want 64", devices[0].TotalWidth)
		}
		if devices[0].ConfiguredSpeed != "3200 MT/s" {
			t.Errorf("ConfiguredSpeed = %q", devices[0].ConfiguredSpeed)
		}
	})

	t.Run("all empty slots", func(t *testing.T) {
		sections := []map[string]string{
			{"Size": "No Module Installed"},
			{"Size": ""},
		}
		devices := parseMemoryDevicesFromSections(sections)
		if devices != nil {
			t.Errorf("expected nil for all empty slots, got %d devices", len(devices))
		}
	})

	t.Run("nil sections", func(t *testing.T) {
		devices := parseMemoryDevicesFromSections(nil)
		if devices != nil {
			t.Errorf("expected nil for nil sections")
		}
	})

	t.Run("non-numeric width", func(t *testing.T) {
		sections := []map[string]string{
			{
				"Size":       "8 GB",
				"Data Width": "Unknown",
				"Rank":       "bad",
			},
		}
		devices := parseMemoryDevicesFromSections(sections)
		if len(devices) != 1 {
			t.Fatalf("expected 1 device")
		}
		if devices[0].DataWidth != 0 {
			t.Errorf("expected 0 for non-numeric width, got %d", devices[0].DataWidth)
		}
		if devices[0].Rank != 0 {
			t.Errorf("expected 0 for non-numeric rank, got %d", devices[0].Rank)
		}
	})
}

// TestParseEthtoolKeyValueWakeOn tests the Wake-on parsing through the actual ethtool parsing flow.
func TestParseEthtoolKeyValueWakeOn(t *testing.T) {
	info := &EthtoolInfo{}
	parseEthtoolKeyValue(info, "Supports Wake-on", "pumbg", false, false)
	if len(info.SupportsWakeOn) != 5 {
		t.Errorf("expected 5 wake-on flags, got %d: %v", len(info.SupportsWakeOn), info.SupportsWakeOn)
	}
	parseEthtoolKeyValue(info, "Wake-on", "g", false, false)
	if info.WakeOn != "g" {
		t.Errorf("WakeOn = %q, want g", info.WakeOn)
	}
}
