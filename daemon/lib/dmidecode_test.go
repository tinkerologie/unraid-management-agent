package lib

import (
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
}
