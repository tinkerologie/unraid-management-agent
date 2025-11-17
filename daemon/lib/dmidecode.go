package lib

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// ParseDmidecodeType parses dmidecode output for a specific type
// Returns a map of sections, where each section is a map of key-value pairs
func ParseDmidecodeType(typeNum string) ([]map[string]string, error) {
	output, err := ExecCommandOutput("dmidecode", "-t", typeNum)
	if err != nil {
		return nil, fmt.Errorf("failed to execute dmidecode: %w", err)
	}

	return parseDmidecodeOutput(output), nil
}

// parseDmidecodeOutput parses dmidecode output into sections
func parseDmidecodeOutput(output string) []map[string]string {
	var sections []map[string]string
	var currentSection map[string]string
	var currentKey string

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Skip empty lines and header lines
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "SMBIOS") || strings.HasPrefix(line, "Getting") {
			continue
		}

		// New section starts with non-indented line
		if !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, " ") {
			if len(currentSection) > 0 {
				sections = append(sections, currentSection)
			}
			currentSection = make(map[string]string)
			currentKey = ""
			continue
		}

		// Parse key-value pairs
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				currentSection[key] = value
				currentKey = key
			}
		} else if currentKey != "" {
			// Continuation of previous value (multi-line)
			if existing, ok := currentSection[currentKey]; ok {
				currentSection[currentKey] = existing + " " + trimmed
			}
		}
	}

	// Add last section
	if len(currentSection) > 0 {
		sections = append(sections, currentSection)
	}

	return sections
}

// ParseBIOSInfo parses BIOS information from dmidecode type 0
func ParseBIOSInfo() (*dto.BIOSInfo, error) {
	sections, err := ParseDmidecodeType("0")
	if err != nil {
		return nil, err
	}

	if len(sections) == 0 {
		return nil, fmt.Errorf("no BIOS information found")
	}

	section := sections[0]
	bios := &dto.BIOSInfo{
		Vendor:      section["Vendor"],
		Version:     section["Version"],
		ReleaseDate: section["Release Date"],
		Address:     section["Address"],
		RuntimeSize: section["Runtime Size"],
		ROMSize:     section["ROM Size"],
		Revision:    section["BIOS Revision"],
	}

	// Parse characteristics
	if chars, ok := section["Characteristics"]; ok {
		bios.Characteristics = strings.Split(chars, ",")
		for i := range bios.Characteristics {
			bios.Characteristics[i] = strings.TrimSpace(bios.Characteristics[i])
		}
	}

	return bios, nil
}

// ParseBaseboardInfo parses baseboard information from dmidecode type 2
func ParseBaseboardInfo() (*dto.BaseboardInfo, error) {
	sections, err := ParseDmidecodeType("2")
	if err != nil {
		return nil, err
	}

	if len(sections) == 0 {
		return nil, fmt.Errorf("no baseboard information found")
	}

	section := sections[0]
	baseboard := &dto.BaseboardInfo{
		Manufacturer:      section["Manufacturer"],
		ProductName:       section["Product Name"],
		Version:           section["Version"],
		SerialNumber:      section["Serial Number"],
		AssetTag:          section["Asset Tag"],
		LocationInChassis: section["Location In Chassis"],
		Type:              section["Type"],
	}

	// Parse features
	if features, ok := section["Features"]; ok {
		baseboard.Features = strings.Split(features, ",")
		for i := range baseboard.Features {
			baseboard.Features[i] = strings.TrimSpace(baseboard.Features[i])
		}
	}

	return baseboard, nil
}

// ParseCPUInfo parses CPU information from dmidecode type 4
func ParseCPUInfo() (*dto.CPUHardwareInfo, error) {
	sections, err := ParseDmidecodeType("4")
	if err != nil {
		return nil, err
	}

	if len(sections) == 0 {
		return nil, fmt.Errorf("no CPU information found")
	}

	section := sections[0]
	cpu := &dto.CPUHardwareInfo{
		SocketDesignation: section["Socket Designation"],
		Family:            section["Family"],
		Manufacturer:      section["Manufacturer"],
		Signature:         section["Signature"],
		Voltage:           section["Voltage"],
		Status:            section["Status"],
		Upgrade:           section["Upgrade"],
		SerialNumber:      section["Serial Number"],
		AssetTag:          section["Asset Tag"],
		PartNumber:        section["Part Number"],
	}

	// Parse integer fields
	if val, ok := section["External Clock"]; ok {
		if mhz, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(val), " MHz")); err == nil {
			cpu.ExternalClock = mhz
		}
	}
	if val, ok := section["Max Speed"]; ok {
		if mhz, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(val), " MHz")); err == nil {
			cpu.MaxSpeed = mhz
		}
	}
	if val, ok := section["Current Speed"]; ok {
		if mhz, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(val), " MHz")); err == nil {
			cpu.CurrentSpeed = mhz
		}
	}
	if val, ok := section["Core Enabled"]; ok {
		if cores, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			cpu.CoreEnabled = cores
		}
	}
	if val, ok := section["Thread Count"]; ok {
		if threads, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			cpu.ThreadCount = threads
		}
	}

	// Parse flags
	if flags, ok := section["Flags"]; ok {
		cpu.Flags = strings.Fields(flags)
	}

	// Parse characteristics
	if chars, ok := section["Characteristics"]; ok {
		cpu.Characteristics = strings.Split(chars, ",")
		for i := range cpu.Characteristics {
			cpu.Characteristics[i] = strings.TrimSpace(cpu.Characteristics[i])
		}
	}

	return cpu, nil
}

// ParseCPUCacheInfo parses CPU cache information from dmidecode type 7
func ParseCPUCacheInfo() ([]dto.CPUCacheInfo, error) {
	sections, err := ParseDmidecodeType("7")
	if err != nil {
		return nil, err
	}

	var caches []dto.CPUCacheInfo
	for _, section := range sections {
		cache := dto.CPUCacheInfo{
			SocketDesignation:   section["Socket Designation"],
			Configuration:       section["Configuration"],
			OperationalMode:     section["Operational Mode"],
			Location:            section["Location"],
			InstalledSize:       section["Installed Size"],
			MaximumSize:         section["Maximum Size"],
			InstalledSRAMType:   section["Installed SRAM Type"],
			ErrorCorrectionType: section["Error Correction Type"],
			SystemType:          section["System Type"],
			Associativity:       section["Associativity"],
		}

		// Parse level from socket designation (e.g., "L1-Cache", "L2-Cache")
		switch {
		case strings.Contains(cache.SocketDesignation, "L1"):
			cache.Level = 1
		case strings.Contains(cache.SocketDesignation, "L2"):
			cache.Level = 2
		case strings.Contains(cache.SocketDesignation, "L3"):
			cache.Level = 3
		}

		// Parse supported SRAM types
		if types, ok := section["Supported SRAM Types"]; ok {
			cache.SupportedSRAMTypes = strings.Split(types, ",")
			for i := range cache.SupportedSRAMTypes {
				cache.SupportedSRAMTypes[i] = strings.TrimSpace(cache.SupportedSRAMTypes[i])
			}
		}

		caches = append(caches, cache)
	}

	return caches, nil
}

// ParseMemoryArrayInfo parses memory array information from dmidecode type 16
func ParseMemoryArrayInfo() (*dto.MemoryArrayInfo, error) {
	sections, err := ParseDmidecodeType("16")
	if err != nil {
		return nil, err
	}

	if len(sections) == 0 {
		return nil, fmt.Errorf("no memory array information found")
	}

	section := sections[0]
	memArray := &dto.MemoryArrayInfo{
		Location:            section["Location"],
		Use:                 section["Use"],
		ErrorCorrectionType: section["Error Correction Type"],
		MaximumCapacity:     section["Maximum Capacity"],
	}

	// Parse number of devices
	if val, ok := section["Number Of Devices"]; ok {
		if num, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			memArray.NumberOfDevices = num
		}
	}

	return memArray, nil
}

// ParseMemoryDevices parses memory device information from dmidecode type 17
func ParseMemoryDevices() ([]dto.MemoryDeviceInfo, error) {
	sections, err := ParseDmidecodeType("17")
	if err != nil {
		return nil, err
	}

	var devices []dto.MemoryDeviceInfo
	for _, section := range sections {
		// Skip empty slots
		if section["Size"] == "No Module Installed" || section["Size"] == "" {
			continue
		}

		device := dto.MemoryDeviceInfo{
			Locator:           section["Locator"],
			BankLocator:       section["Bank Locator"],
			Size:              section["Size"],
			FormFactor:        section["Form Factor"],
			Type:              section["Type"],
			TypeDetail:        section["Type Detail"],
			Speed:             section["Speed"],
			Manufacturer:      section["Manufacturer"],
			SerialNumber:      section["Serial Number"],
			AssetTag:          section["Asset Tag"],
			PartNumber:        section["Part Number"],
			ConfiguredSpeed:   section["Configured Memory Speed"],
			MinimumVoltage:    section["Minimum Voltage"],
			MaximumVoltage:    section["Maximum Voltage"],
			ConfiguredVoltage: section["Configured Voltage"],
		}

		// Parse integer fields
		if val, ok := section["Rank"]; ok {
			if rank, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
				device.Rank = rank
			}
		}
		if val, ok := section["Data Width"]; ok {
			if width, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(val), " bits")); err == nil {
				device.DataWidth = width
			}
		}
		if val, ok := section["Total Width"]; ok {
			if width, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(val), " bits")); err == nil {
				device.TotalWidth = width
			}
		}

		devices = append(devices, device)
	}

	return devices, nil
}
