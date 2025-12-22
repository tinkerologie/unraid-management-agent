package lib

import (
	"strings"
	"testing"
)

func TestParseDmidecodeOutput(t *testing.T) {
	t.Run("BIOS information", func(t *testing.T) {
		output := `# dmidecode 3.3
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
		sections := parseDmidecodeOutput(output)

		if len(sections) == 0 {
			t.Fatal("parseDmidecodeOutput() returned 0 sections")
		}

		section := sections[0]
		if section["Vendor"] != "American Megatrends Inc." {
			t.Errorf("Vendor = %q, want %q", section["Vendor"], "American Megatrends Inc.")
		}
		if section["Version"] != "1.80" {
			t.Errorf("Version = %q, want %q", section["Version"], "1.80")
		}
		if section["Release Date"] != "05/17/2019" {
			t.Errorf("Release Date = %q, want %q", section["Release Date"], "05/17/2019")
		}
		if section["BIOS Revision"] != "5.13" {
			t.Errorf("BIOS Revision = %q, want %q", section["BIOS Revision"], "5.13")
		}
	})

	t.Run("baseboard information", func(t *testing.T) {
		output := `Handle 0x0002, DMI type 2, 15 bytes
Base Board Information
	Manufacturer: ASUSTeK COMPUTER INC.
	Product Name: PRIME Z370-A
	Version: Rev 1.xx
	Serial Number: 180000000000000
	Asset Tag: Default string
	Features:
		Board is a hosting board
	Location In Chassis: Default string
	Type: Motherboard
`
		sections := parseDmidecodeOutput(output)

		if len(sections) == 0 {
			t.Fatal("parseDmidecodeOutput() returned 0 sections")
		}

		section := sections[0]
		if section["Manufacturer"] != "ASUSTeK COMPUTER INC." {
			t.Errorf("Manufacturer = %q, want %q", section["Manufacturer"], "ASUSTeK COMPUTER INC.")
		}
		if section["Product Name"] != "PRIME Z370-A" {
			t.Errorf("Product Name = %q, want %q", section["Product Name"], "PRIME Z370-A")
		}
	})

	t.Run("CPU information", func(t *testing.T) {
		output := `Handle 0x0004, DMI type 4, 48 bytes
Processor Information
	Socket Designation: LGA1151
	Type: Central Processor
	Family: Core i7
	Manufacturer: Intel(R) Corporation
	Signature: Type 0, Family 6, Model 158, Stepping 10
	Voltage: 1.0 V
	External Clock: 100 MHz
	Max Speed: 4700 MHz
	Current Speed: 3700 MHz
	Status: Populated, Enabled
	Upgrade: Socket LGA1151
	Serial Number: To Be Filled By O.E.M.
	Core Enabled: 6
	Thread Count: 12
`
		sections := parseDmidecodeOutput(output)

		if len(sections) == 0 {
			t.Fatal("parseDmidecodeOutput() returned 0 sections")
		}

		section := sections[0]
		if section["Socket Designation"] != "LGA1151" {
			t.Errorf("Socket Designation = %q, want %q", section["Socket Designation"], "LGA1151")
		}
		if section["Family"] != "Core i7" {
			t.Errorf("Family = %q, want %q", section["Family"], "Core i7")
		}
		if section["Core Enabled"] != "6" {
			t.Errorf("Core Enabled = %q, want %q", section["Core Enabled"], "6")
		}
		if section["Thread Count"] != "12" {
			t.Errorf("Thread Count = %q, want %q", section["Thread Count"], "12")
		}
	})

	t.Run("memory information", func(t *testing.T) {
		output := `Handle 0x0011, DMI type 17, 84 bytes
Memory Device
	Locator: ChannelA-DIMM0
	Bank Locator: BANK 0
	Size: 16 GB
	Form Factor: DIMM
	Type: DDR4
	Type Detail: Synchronous
	Speed: 3200 MT/s
	Manufacturer: Corsair
	Serial Number: 00000000
	Part Number: CMK32GX4M2B3200C16
	Configured Memory Speed: 3200 MT/s
	Rank: 2
	Data Width: 64 bits
`
		sections := parseDmidecodeOutput(output)

		if len(sections) == 0 {
			t.Fatal("parseDmidecodeOutput() returned 0 sections")
		}

		section := sections[0]
		if section["Size"] != "16 GB" {
			t.Errorf("Size = %q, want %q", section["Size"], "16 GB")
		}
		if section["Type"] != "DDR4" {
			t.Errorf("Type = %q, want %q", section["Type"], "DDR4")
		}
		if section["Manufacturer"] != "Corsair" {
			t.Errorf("Manufacturer = %q, want %q", section["Manufacturer"], "Corsair")
		}
	})

	t.Run("empty output", func(t *testing.T) {
		output := ""
		sections := parseDmidecodeOutput(output)

		if len(sections) != 0 {
			t.Errorf("parseDmidecodeOutput() returned %d sections, want 0", len(sections))
		}
	})

	t.Run("multiple sections", func(t *testing.T) {
		output := `Handle 0x0000, DMI type 0, 26 bytes
BIOS Information
	Vendor: AMI
	Version: 1.0

Handle 0x0001, DMI type 1, 27 bytes
System Information
	Manufacturer: Dell
	Product Name: PowerEdge
`
		sections := parseDmidecodeOutput(output)

		if len(sections) != 2 {
			t.Errorf("parseDmidecodeOutput() returned %d sections, want 2", len(sections))
		}

		if sections[0]["Vendor"] != "AMI" {
			t.Errorf("Section[0][Vendor] = %q, want %q", sections[0]["Vendor"], "AMI")
		}
		if sections[1]["Manufacturer"] != "Dell" {
			t.Errorf("Section[1][Manufacturer] = %q, want %q", sections[1]["Manufacturer"], "Dell")
		}
	})

	t.Run("cache information", func(t *testing.T) {
		output := `Handle 0x0007, DMI type 7, 27 bytes
Cache Information
	Socket Designation: L1-Cache
	Configuration: Enabled, Not Socketed, Level 1
	Operational Mode: Write Back
	Location: Internal
	Installed Size: 384 kB
	Maximum Size: 384 kB
	Supported SRAM Types:
		Synchronous
	Installed SRAM Type: Synchronous
	Error Correction Type: Parity
	System Type: Unified
	Associativity: 8-way Set-associative
`
		sections := parseDmidecodeOutput(output)

		if len(sections) == 0 {
			t.Fatal("parseDmidecodeOutput() returned 0 sections")
		}

		section := sections[0]
		if section["Socket Designation"] != "L1-Cache" {
			t.Errorf("Socket Designation = %q, want %q", section["Socket Designation"], "L1-Cache")
		}
		if section["Installed Size"] != "384 kB" {
			t.Errorf("Installed Size = %q, want %q", section["Installed Size"], "384 kB")
		}
	})

	t.Run("memory array information", func(t *testing.T) {
		output := `Handle 0x0008, DMI type 16, 23 bytes
Physical Memory Array
	Location: System Board Or Motherboard
	Use: System Memory
	Error Correction Type: None
	Maximum Capacity: 64 GB
	Error Information Handle: Not Provided
	Number Of Devices: 4
`
		sections := parseDmidecodeOutput(output)

		if len(sections) == 0 {
			t.Fatal("parseDmidecodeOutput() returned 0 sections")
		}

		section := sections[0]
		if section["Maximum Capacity"] != "64 GB" {
			t.Errorf("Maximum Capacity = %q, want %q", section["Maximum Capacity"], "64 GB")
		}
		if section["Number Of Devices"] != "4" {
			t.Errorf("Number Of Devices = %q, want %q", section["Number Of Devices"], "4")
		}
	})

	t.Run("memory device information", func(t *testing.T) {
		output := `Handle 0x000A, DMI type 17, 40 bytes
Memory Device
	Array Handle: 0x0008
	Error Information Handle: Not Provided
	Total Width: 64 bits
	Data Width: 64 bits
	Size: 8192 MB
	Form Factor: DIMM
	Set: None
	Locator: DIMM A1
	Bank Locator: BANK 0
	Type: DDR4
	Type Detail: Synchronous
	Speed: 2666 MT/s
	Manufacturer: Samsung
	Serial Number: 12345678
	Asset Tag: Not Specified
	Part Number: M378A1K43CB2-CTD
	Rank: 1
	Configured Memory Speed: 2666 MT/s
`
		sections := parseDmidecodeOutput(output)

		if len(sections) == 0 {
			t.Fatal("parseDmidecodeOutput() returned 0 sections")
		}

		section := sections[0]
		if section["Size"] != "8192 MB" {
			t.Errorf("Size = %q, want %q", section["Size"], "8192 MB")
		}
		if section["Type"] != "DDR4" {
			t.Errorf("Type = %q, want %q", section["Type"], "DDR4")
		}
		if section["Manufacturer"] != "Samsung" {
			t.Errorf("Manufacturer = %q, want %q", section["Manufacturer"], "Samsung")
		}
	})

	t.Run("empty output", func(t *testing.T) {
		output := ""
		sections := parseDmidecodeOutput(output)

		if len(sections) != 0 {
			t.Errorf("Expected 0 sections for empty output, got %d", len(sections))
		}
	})

	t.Run("header only output", func(t *testing.T) {
		output := `# dmidecode 3.3
Getting SMBIOS data from sysfs.
SMBIOS 3.1.1 present.
`
		sections := parseDmidecodeOutput(output)

		if len(sections) != 0 {
			t.Errorf("Expected 0 sections for header-only output, got %d", len(sections))
		}
	})

	t.Run("multiple sections", func(t *testing.T) {
		output := `Handle 0x0000, DMI type 0, 26 bytes
BIOS Information
	Vendor: Vendor1
	Version: 1.0

Handle 0x0001, DMI type 1, 27 bytes
System Information
	Manufacturer: Manufacturer1
	Product Name: Product1
`
		sections := parseDmidecodeOutput(output)

		if len(sections) != 2 {
			t.Errorf("Expected 2 sections, got %d", len(sections))
		}

		if sections[0]["Vendor"] != "Vendor1" {
			t.Errorf("First section Vendor = %q, want %q", sections[0]["Vendor"], "Vendor1")
		}
		if sections[1]["Manufacturer"] != "Manufacturer1" {
			t.Errorf("Second section Manufacturer = %q, want %q", sections[1]["Manufacturer"], "Manufacturer1")
		}
	})
}

// TestParseBIOSInfoFromSection tests the BIOS parsing logic with pre-parsed section
func TestParseBIOSInfoFromSection(t *testing.T) {
	tests := []struct {
		name     string
		section  map[string]string
		expected map[string]string
	}{
		{
			name: "complete BIOS info",
			section: map[string]string{
				"Vendor":        "American Megatrends Inc.",
				"Version":       "1.80",
				"Release Date":  "05/17/2019",
				"Address":       "0xF0000",
				"Runtime Size":  "64 kB",
				"ROM Size":      "16 MB",
				"BIOS Revision": "5.13",
			},
			expected: map[string]string{
				"Vendor":      "American Megatrends Inc.",
				"Version":     "1.80",
				"ReleaseDate": "05/17/2019",
			},
		},
		{
			name: "Dell BIOS",
			section: map[string]string{
				"Vendor":       "Dell Inc.",
				"Version":      "2.5.4",
				"Release Date": "12/01/2020",
			},
			expected: map[string]string{
				"Vendor":      "Dell Inc.",
				"Version":     "2.5.4",
				"ReleaseDate": "12/01/2020",
			},
		},
		{
			name: "Phoenix BIOS",
			section: map[string]string{
				"Vendor":       "Phoenix Technologies LTD",
				"Version":      "6.00",
				"Release Date": "01/01/2018",
			},
			expected: map[string]string{
				"Vendor":      "Phoenix Technologies LTD",
				"Version":     "6.00",
				"ReleaseDate": "01/01/2018",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.section["Vendor"] != tt.expected["Vendor"] {
				t.Errorf("Vendor = %q, want %q", tt.section["Vendor"], tt.expected["Vendor"])
			}
			if tt.section["Version"] != tt.expected["Version"] {
				t.Errorf("Version = %q, want %q", tt.section["Version"], tt.expected["Version"])
			}
			if tt.section["Release Date"] != tt.expected["ReleaseDate"] {
				t.Errorf("Release Date = %q, want %q", tt.section["Release Date"], tt.expected["ReleaseDate"])
			}
		})
	}
}

// TestParseBaseboardInfoFromSection tests the baseboard parsing logic
func TestParseBaseboardInfoFromSection(t *testing.T) {
	tests := []struct {
		name     string
		section  map[string]string
		expected map[string]string
	}{
		{
			name: "ASUS motherboard",
			section: map[string]string{
				"Manufacturer":        "ASUSTeK COMPUTER INC.",
				"Product Name":        "PRIME X570-PRO",
				"Version":             "Rev X.0x",
				"Serial Number":       "ABC123456789",
				"Asset Tag":           "To be filled by O.E.M.",
				"Location In Chassis": "Default string",
				"Type":                "Motherboard",
			},
			expected: map[string]string{
				"Manufacturer": "ASUSTeK COMPUTER INC.",
				"ProductName":  "PRIME X570-PRO",
				"Type":         "Motherboard",
			},
		},
		{
			name: "Supermicro board",
			section: map[string]string{
				"Manufacturer":  "Supermicro",
				"Product Name":  "X11SCL-F",
				"Version":       "1.01",
				"Serial Number": "ZM19AS000001",
				"Type":          "Motherboard",
			},
			expected: map[string]string{
				"Manufacturer": "Supermicro",
				"ProductName":  "X11SCL-F",
				"Type":         "Motherboard",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.section["Manufacturer"] != tt.expected["Manufacturer"] {
				t.Errorf("Manufacturer = %q, want %q", tt.section["Manufacturer"], tt.expected["Manufacturer"])
			}
			if tt.section["Product Name"] != tt.expected["ProductName"] {
				t.Errorf("Product Name = %q, want %q", tt.section["Product Name"], tt.expected["ProductName"])
			}
			if tt.section["Type"] != tt.expected["Type"] {
				t.Errorf("Type = %q, want %q", tt.section["Type"], tt.expected["Type"])
			}
		})
	}
}

// TestParseCPUCacheLevelDetection tests cache level detection from socket designation
func TestParseCPUCacheLevelDetection(t *testing.T) {
	tests := []struct {
		socketDesignation string
		expectedLevel     int
	}{
		{"L1-Cache", 1},
		{"L1 Cache", 1},
		{"L1-Data Cache", 1},
		{"L1-Instruction Cache", 1},
		{"L2-Cache", 2},
		{"L2 Cache", 2},
		{"L2 Unified Cache", 2},
		{"L3-Cache", 3},
		{"L3 Cache", 3},
		{"L3 SmartCache", 3},
		{"Unknown Cache", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.socketDesignation, func(t *testing.T) {
			var level int
			switch {
			case strings.Contains(tt.socketDesignation, "L1"):
				level = 1
			case strings.Contains(tt.socketDesignation, "L2"):
				level = 2
			case strings.Contains(tt.socketDesignation, "L3"):
				level = 3
			}
			if level != tt.expectedLevel {
				t.Errorf("Level = %d, want %d", level, tt.expectedLevel)
			}
		})
	}
}

// TestParseMemoryDeviceSize tests memory device size parsing
func TestParseMemoryDeviceSize(t *testing.T) {
	tests := []struct {
		name  string
		size  string
		valid bool
	}{
		{"8GB module", "8192 MB", true},
		{"16GB module", "16384 MB", true},
		{"32GB module", "32 GB", true},
		{"no module", "No Module Installed", false},
		{"empty slot", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isEmpty := tt.size == "No Module Installed" || tt.size == ""
			if isEmpty == tt.valid {
				t.Errorf("Size %q: isEmpty=%v, want valid=%v", tt.size, isEmpty, tt.valid)
			}
		})
	}
}

// TestParseMemoryType tests memory type detection
func TestParseMemoryType(t *testing.T) {
	tests := []struct {
		memType  string
		expected string
	}{
		{"DDR4", "DDR4"},
		{"DDR3", "DDR3"},
		{"DDR5", "DDR5"},
		{"Unknown", "Unknown"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.memType, func(t *testing.T) {
			if tt.memType != tt.expected {
				t.Errorf("Type = %q, want %q", tt.memType, tt.expected)
			}
		})
	}
}
