package lib

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// createRAPLZone creates a simulated RAPL zone directory with the given name, energy, and max range.
func createRAPLZone(t *testing.T, dir, name string, energyUJ, maxRange uint64) string {
	t.Helper()

	zonePath := filepath.Join(dir, name)
	if err := os.MkdirAll(zonePath, 0755); err != nil {
		t.Fatalf("Failed to create zone dir %s: %v", name, err)
	}

	writeTestFile(t, filepath.Join(zonePath, "name"), zoneNameFromDir(name))
	writeTestFile(t, filepath.Join(zonePath, "energy_uj"), uintToStr(energyUJ))
	writeTestFile(t, filepath.Join(zonePath, "max_energy_range_uj"), uintToStr(maxRange))

	return zonePath
}

// zoneNameFromDir maps directory names to RAPL zone names.
func zoneNameFromDir(dir string) string {
	switch dir {
	case "intel-rapl:0":
		return "package-0"
	case "intel-rapl:1":
		return "package-1"
	case "intel-rapl:0:0":
		return "core"
	case "intel-rapl:0:1":
		return "uncore"
	case "intel-rapl:0:2":
		return "dram"
	case "intel-rapl:1:0":
		return "core"
	case "intel-rapl:1:1":
		return "dram"
	default:
		return "unknown"
	}
}

func uintToStr(v uint64) string {
	return strconv.FormatUint(v, 10) //nolint:perfsprint
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content+"\n"), 0644); err != nil {
		t.Fatalf("Failed to write %s: %v", path, err)
	}
}

func TestIsRAPLAvailable(t *testing.T) {
	// Test with non-existent path
	origPath := SysPowercapPath
	defer func() { SysPowercapPath = origPath }()

	SysPowercapPath = "/non/existent/path"
	if IsRAPLAvailable() {
		t.Error("Expected RAPL to be unavailable for non-existent path")
	}

	// Test with directory but no RAPL zones
	tmpDir := t.TempDir()
	SysPowercapPath = tmpDir
	if IsRAPLAvailable() {
		t.Error("Expected RAPL to be unavailable in empty directory")
	}

	// Test with RAPL zones present
	createRAPLZone(t, tmpDir, "intel-rapl:0", 1000, 262143328850)
	if !IsRAPLAvailable() {
		t.Error("Expected RAPL to be available with intel-rapl:0 zone")
	}
}

func TestReadRAPLEnergy(t *testing.T) {
	origPath := SysPowercapPath
	defer func() { SysPowercapPath = origPath }()

	tmpDir := t.TempDir()
	SysPowercapPath = tmpDir

	// Test with no RAPL data
	reading := ReadRAPLEnergy()
	if reading != nil {
		t.Error("Expected nil reading when no RAPL zones exist")
	}

	// Create a complete RAPL structure: package with core, uncore, dram
	createRAPLZone(t, tmpDir, "intel-rapl:0", 100_000_000, 262143328850)
	createRAPLZone(t, tmpDir, "intel-rapl:0:0", 80_000_000, 262143328850)
	createRAPLZone(t, tmpDir, "intel-rapl:0:1", 10_000_000, 262143328850)
	createRAPLZone(t, tmpDir, "intel-rapl:0:2", 20_000_000, 262143328850)

	reading = ReadRAPLEnergy()
	if reading == nil {
		t.Fatal("Expected non-nil reading with RAPL zones present")
	}

	// Verify package
	if len(reading.Packages) != 1 {
		t.Fatalf("Expected 1 package, got %d", len(reading.Packages))
	}
	if reading.Packages[0].Name != "package-0" {
		t.Errorf("Expected package name 'package-0', got '%s'", reading.Packages[0].Name)
	}
	if reading.Packages[0].EnergyUJ != 100_000_000 {
		t.Errorf("Expected package energy 100000000, got %d", reading.Packages[0].EnergyUJ)
	}
	if reading.Packages[0].MaxRange != 262143328850 {
		t.Errorf("Expected max range 262143328850, got %d", reading.Packages[0].MaxRange)
	}

	// Verify sub-zones
	if len(reading.Core) != 1 {
		t.Fatalf("Expected 1 core zone, got %d", len(reading.Core))
	}
	if reading.Core[0].EnergyUJ != 80_000_000 {
		t.Errorf("Expected core energy 80000000, got %d", reading.Core[0].EnergyUJ)
	}

	if len(reading.Uncore) != 1 {
		t.Fatalf("Expected 1 uncore zone, got %d", len(reading.Uncore))
	}

	if len(reading.DRAM) != 1 {
		t.Fatalf("Expected 1 DRAM zone, got %d", len(reading.DRAM))
	}
	if reading.DRAM[0].EnergyUJ != 20_000_000 {
		t.Errorf("Expected DRAM energy 20000000, got %d", reading.DRAM[0].EnergyUJ)
	}
}

func TestReadRAPLEnergyMultiSocket(t *testing.T) {
	origPath := SysPowercapPath
	defer func() { SysPowercapPath = origPath }()

	tmpDir := t.TempDir()
	SysPowercapPath = tmpDir

	// Create two sockets
	createRAPLZone(t, tmpDir, "intel-rapl:0", 50_000_000, 262143328850)
	createRAPLZone(t, tmpDir, "intel-rapl:0:0", 40_000_000, 262143328850)
	createRAPLZone(t, tmpDir, "intel-rapl:0:2", 10_000_000, 262143328850)
	createRAPLZone(t, tmpDir, "intel-rapl:1", 60_000_000, 262143328850)
	createRAPLZone(t, tmpDir, "intel-rapl:1:0", 45_000_000, 262143328850)
	createRAPLZone(t, tmpDir, "intel-rapl:1:1", 15_000_000, 262143328850)

	reading := ReadRAPLEnergy()
	if reading == nil {
		t.Fatal("Expected non-nil reading for multi-socket system")
	}

	if len(reading.Packages) != 2 {
		t.Errorf("Expected 2 packages, got %d", len(reading.Packages))
	}
}

func TestCalculateRAPLPower(t *testing.T) {
	// Test with nil inputs
	if power := CalculateRAPLPower(nil, nil); power != nil {
		t.Error("Expected nil power for nil inputs")
	}
	if power := CalculateRAPLPower(&RAPLReading{}, nil); power != nil {
		t.Error("Expected nil power when current is nil")
	}

	// Test normal power calculation
	now := time.Now()
	prev := &RAPLReading{
		Packages: []RAPLZone{{Name: "package-0", EnergyUJ: 100_000_000, MaxRange: 262143328850}},
		DRAM:     []RAPLZone{{Name: "dram", EnergyUJ: 10_000_000, MaxRange: 262143328850}},
		Time:     now,
	}
	curr := &RAPLReading{
		Packages: []RAPLZone{{Name: "package-0", EnergyUJ: 110_000_000, MaxRange: 262143328850}},
		DRAM:     []RAPLZone{{Name: "dram", EnergyUJ: 11_000_000, MaxRange: 262143328850}},
		Time:     now.Add(1 * time.Second),
	}

	power := CalculateRAPLPower(prev, curr)
	if power == nil {
		t.Fatal("Expected non-nil power for valid readings")
	}

	// 10,000,000 µJ / 1 second = 10 W
	expectedPackage := 10.0
	if power.PackageWatts < expectedPackage-0.01 || power.PackageWatts > expectedPackage+0.01 {
		t.Errorf("Expected package power ~%.1f W, got %.2f W", expectedPackage, power.PackageWatts)
	}

	// 1,000,000 µJ / 1 second = 1 W
	expectedDRAM := 1.0
	if power.DRAMWatts < expectedDRAM-0.01 || power.DRAMWatts > expectedDRAM+0.01 {
		t.Errorf("Expected DRAM power ~%.1f W, got %.2f W", expectedDRAM, power.DRAMWatts)
	}
}

func TestCalculateRAPLPowerZeroDelta(t *testing.T) {
	now := time.Now()
	prev := &RAPLReading{
		Packages: []RAPLZone{{Name: "package-0", EnergyUJ: 100_000_000}},
		Time:     now,
	}
	curr := &RAPLReading{
		Packages: []RAPLZone{{Name: "package-0", EnergyUJ: 100_000_000}},
		Time:     now, // Same timestamp — zero delta
	}

	power := CalculateRAPLPower(prev, curr)
	if power != nil {
		t.Error("Expected nil power for zero time delta")
	}
}

func TestEnergyDelta(t *testing.T) {
	tests := []struct {
		name     string
		prev     uint64
		curr     uint64
		maxRange uint64
		expected uint64
	}{
		{
			name:     "normal increment",
			prev:     100,
			curr:     200,
			maxRange: 1000,
			expected: 100,
		},
		{
			name:     "wraparound with max range",
			prev:     900,
			curr:     100,
			maxRange: 1000,
			expected: 200, // (1000 - 900) + 100
		},
		{
			name:     "wraparound without max range",
			prev:     ^uint64(0) - 9, // max - 9
			curr:     5,
			maxRange: 0,
			expected: 15, // wraps around 64-bit boundary
		},
		{
			name:     "no change",
			prev:     500,
			curr:     500,
			maxRange: 1000,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delta := energyDelta(tt.prev, tt.curr, tt.maxRange)
			if delta != tt.expected {
				t.Errorf("energyDelta(%d, %d, %d) = %d, want %d",
					tt.prev, tt.curr, tt.maxRange, delta, tt.expected)
			}
		})
	}
}

func TestFormatRAPLPower(t *testing.T) {
	// Test nil power
	result := FormatRAPLPower(nil)
	if result != "RAPL power data unavailable" {
		t.Errorf("Expected unavailable message, got %q", result)
	}

	// Test with power values
	power := &RAPLPower{PackageWatts: 45.67, DRAMWatts: 3.21}
	result = FormatRAPLPower(power)
	expected := "CPU Package: 45.67 W, DRAM: 3.21 W"
	if result != expected {
		t.Errorf("FormatRAPLPower() = %q, want %q", result, expected)
	}
}

func TestReadRAPLEnergyMissingFiles(t *testing.T) {
	origPath := SysPowercapPath
	defer func() { SysPowercapPath = origPath }()

	tmpDir := t.TempDir()
	SysPowercapPath = tmpDir

	// Create a zone directory without the required files
	zonePath := filepath.Join(tmpDir, "intel-rapl:0")
	if err := os.MkdirAll(zonePath, 0755); err != nil {
		t.Fatal(err)
	}

	// No name file → should return nil
	reading := ReadRAPLEnergy()
	if reading != nil {
		t.Error("Expected nil reading when zone has no name file")
	}

	// Add name but no energy_uj
	writeTestFile(t, filepath.Join(zonePath, "name"), "package-0")
	reading = ReadRAPLEnergy()
	if reading != nil {
		t.Error("Expected nil reading when zone has no energy_uj file")
	}
}

func TestCalculateRAPLPowerMultiSocket(t *testing.T) {
	now := time.Now()
	prev := &RAPLReading{
		Packages: []RAPLZone{
			{Name: "package-0", EnergyUJ: 100_000_000, MaxRange: 262143328850},
			{Name: "package-1", EnergyUJ: 200_000_000, MaxRange: 262143328850},
		},
		DRAM: []RAPLZone{
			{Name: "dram", EnergyUJ: 10_000_000, MaxRange: 262143328850},
			{Name: "dram", EnergyUJ: 20_000_000, MaxRange: 262143328850},
		},
		Time: now,
	}
	curr := &RAPLReading{
		Packages: []RAPLZone{
			{Name: "package-0", EnergyUJ: 115_000_000, MaxRange: 262143328850},
			{Name: "package-1", EnergyUJ: 220_000_000, MaxRange: 262143328850},
		},
		DRAM: []RAPLZone{
			{Name: "dram", EnergyUJ: 11_500_000, MaxRange: 262143328850},
			{Name: "dram", EnergyUJ: 22_000_000, MaxRange: 262143328850},
		},
		Time: now.Add(1 * time.Second),
	}

	power := CalculateRAPLPower(prev, curr)
	if power == nil {
		t.Fatal("Expected non-nil power for multi-socket readings")
	}

	// Socket 0: 15M µJ / 1s = 15W, Socket 1: 20M µJ / 1s = 20W → Total: 35W
	expectedPackage := 35.0
	if power.PackageWatts < expectedPackage-0.1 || power.PackageWatts > expectedPackage+0.1 {
		t.Errorf("Expected total package power ~%.0f W, got %.2f W", expectedPackage, power.PackageWatts)
	}

	// Socket 0 DRAM: 1.5M µJ / 1s = 1.5W, Socket 1 DRAM: 2M µJ / 1s = 2W → Total: 3.5W
	expectedDRAM := 3.5
	if power.DRAMWatts < expectedDRAM-0.1 || power.DRAMWatts > expectedDRAM+0.1 {
		t.Errorf("Expected total DRAM power ~%.1f W, got %.2f W", expectedDRAM, power.DRAMWatts)
	}
}
