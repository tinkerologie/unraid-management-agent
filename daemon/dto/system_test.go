package dto

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSystemInfoJSON(t *testing.T) {
	// Create sample system info
	cpuPower := 65.5
	dramPower := 5.2
	info := SystemInfo{
		Hostname:        "test-server",
		Version:         "1.0.0",
		Uptime:          3600,
		CPUUsage:        45.5,
		RAMUsage:        62.3,
		RAMTotal:        16 * 1024 * 1024 * 1024, // 16 GB
		RAMUsed:         10 * 1024 * 1024 * 1024, // 10 GB
		RAMFree:         6 * 1024 * 1024 * 1024,  // 6 GB
		CPUTemp:         65.0,
		CPUPowerWatts:   &cpuPower,
		DRAMPowerWatts:  &dramPower,
		MotherboardTemp: 45.0,
		Fans: []FanInfo{
			{Name: "CPU Fan", RPM: 1200},
			{Name: "Case Fan", RPM: 800},
		},
		Timestamp: time.Now(),
	}

	// Marshal to JSON
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal SystemInfo: %v", err)
	}

	// Unmarshal back
	var decoded SystemInfo
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal SystemInfo: %v", err)
	}

	// Verify key fields
	if decoded.Hostname != info.Hostname {
		t.Errorf("Hostname mismatch: got %s, want %s", decoded.Hostname, info.Hostname)
	}
	if decoded.CPUUsage != info.CPUUsage {
		t.Errorf("CPUUsage mismatch: got %f, want %f", decoded.CPUUsage, info.CPUUsage)
	}
	if decoded.RAMTotal != info.RAMTotal {
		t.Errorf("RAMTotal mismatch: got %d, want %d", decoded.RAMTotal, info.RAMTotal)
	}
	if len(decoded.Fans) != len(info.Fans) {
		t.Errorf("Fans count mismatch: got %d, want %d", len(decoded.Fans), len(info.Fans))
	}

	// Verify power fields
	if decoded.CPUPowerWatts == nil {
		t.Error("CPUPowerWatts should not be nil")
	} else if *decoded.CPUPowerWatts != cpuPower {
		t.Errorf("CPUPowerWatts mismatch: got %f, want %f", *decoded.CPUPowerWatts, cpuPower)
	}
	if decoded.DRAMPowerWatts == nil {
		t.Error("DRAMPowerWatts should not be nil")
	} else if *decoded.DRAMPowerWatts != dramPower {
		t.Errorf("DRAMPowerWatts mismatch: got %f, want %f", *decoded.DRAMPowerWatts, dramPower)
	}
}

func TestFanInfoJSON(t *testing.T) {
	fan := FanInfo{
		Name: "CPU Fan",
		RPM:  1500,
	}

	data, err := json.Marshal(fan)
	if err != nil {
		t.Fatalf("Failed to marshal FanInfo: %v", err)
	}

	var decoded FanInfo
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal FanInfo: %v", err)
	}

	if decoded.Name != fan.Name {
		t.Errorf("Name mismatch: got %s, want %s", decoded.Name, fan.Name)
	}
	if decoded.RPM != fan.RPM {
		t.Errorf("RPM mismatch: got %d, want %d", decoded.RPM, fan.RPM)
	}
}

func TestSystemInfoPowerFieldsOmitEmpty(t *testing.T) {
	// When power fields are nil, they should be omitted from JSON
	info := SystemInfo{
		Hostname: "test-server",
		CPUUsage: 10.0,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal SystemInfo: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal SystemInfo JSON: %v", err)
	}
	if _, ok := m["cpu_power_watts"]; ok {
		t.Error("cpu_power_watts should be omitted when nil")
	}
	if _, ok := m["dram_power_watts"]; ok {
		t.Error("dram_power_watts should be omitted when nil")
	}
}
