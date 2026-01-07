package lib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSysfsFile(t *testing.T) {
	// Create a temp file for testing
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_file")

	// Test reading non-existent file
	result := readSysfsFile("/non/existent/path")
	if result != "" {
		t.Errorf("Expected empty string for non-existent file, got %s", result)
	}

	// Test reading existing file
	testContent := "  test content with whitespace  \n"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result = readSysfsFile(testFile)
	expected := "test content with whitespace"
	if result != expected {
		t.Errorf("readSysfsFile() = %q, expected %q", result, expected)
	}
}

func TestIsSysfsDMIAvailable(t *testing.T) {
	// This test checks if the function works correctly
	// The actual result depends on the system
	result := IsSysfsDMIAvailable()
	t.Logf("IsSysfsDMIAvailable() = %v (system-dependent)", result)

	// Test should not panic
	_ = result
}

func TestParseBIOSInfoSysfs(t *testing.T) {
	bios, err := ParseBIOSInfoSysfs()
	if err != nil {
		t.Fatalf("ParseBIOSInfoSysfs() returned error: %v", err)
	}

	if bios == nil {
		t.Fatal("ParseBIOSInfoSysfs() returned nil")
	}

	// In test environment, values may be empty but should not panic
	t.Logf("BIOS Vendor: %s", bios.Vendor)
	t.Logf("BIOS Version: %s", bios.Version)
	t.Logf("BIOS Release Date: %s", bios.ReleaseDate)
}

func TestParseBaseboardInfoSysfs(t *testing.T) {
	baseboard, err := ParseBaseboardInfoSysfs()
	if err != nil {
		t.Fatalf("ParseBaseboardInfoSysfs() returned error: %v", err)
	}

	if baseboard == nil {
		t.Fatal("ParseBaseboardInfoSysfs() returned nil")
	}

	// In test environment, values may be empty but should not panic
	t.Logf("Board Manufacturer: %s", baseboard.Manufacturer)
	t.Logf("Board Product: %s", baseboard.ProductName)
	t.Logf("Board Version: %s", baseboard.Version)
}

func TestParseSystemInfoSysfs(t *testing.T) {
	sysInfo := ParseSystemInfoSysfs()

	if sysInfo == nil {
		t.Fatal("ParseSystemInfoSysfs() returned nil")
	}

	// In test environment, values may be empty but should not panic
	t.Logf("Product Name: %s", sysInfo["product_name"])
	t.Logf("Product Family: %s", sysInfo["product_family"])
	t.Logf("Chassis Type: %s", sysInfo["chassis_type"])
	t.Logf("Chassis Vendor: %s", sysInfo["chassis_vendor"])
}
