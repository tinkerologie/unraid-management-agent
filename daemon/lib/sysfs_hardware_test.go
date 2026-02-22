package lib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSysfsFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_file")

	// Test reading non-existent file
	result := readSysfsFile("/non/existent/path")
	if result != "" {
		t.Errorf("Expected empty string for non-existent file, got %s", result)
	}

	// Test reading existing file
	testContent := "  test content with whitespace  \n"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result = readSysfsFile(testFile)
	expected := "test content with whitespace"
	if result != expected {
		t.Errorf("readSysfsFile() = %q, expected %q", result, expected)
	}
}

// setupSysfsDMI creates a temp DMI directory with synthetic sysfs files
// and overrides SysfsDMIPath for the test, restoring it on cleanup.
func setupSysfsDMI(t *testing.T, files map[string]string) {
	t.Helper()
	tmpDir := t.TempDir()
	orig := SysfsDMIPath
	SysfsDMIPath = tmpDir
	t.Cleanup(func() { SysfsDMIPath = orig })

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}
}

func TestIsSysfsDMIAvailable(t *testing.T) {
	t.Run("available", func(t *testing.T) {
		setupSysfsDMI(t, map[string]string{"bios_vendor": "TestVendor"})
		if !IsSysfsDMIAvailable() {
			t.Error("expected IsSysfsDMIAvailable()=true with temp dir")
		}
	})
	t.Run("unavailable", func(t *testing.T) {
		orig := SysfsDMIPath
		SysfsDMIPath = "/nonexistent/path/dmi/id"
		defer func() { SysfsDMIPath = orig }()
		if IsSysfsDMIAvailable() {
			t.Error("expected IsSysfsDMIAvailable()=false with nonexistent path")
		}
	})
}

func TestParseBIOSInfoSysfs(t *testing.T) {
	t.Run("with_data", func(t *testing.T) {
		setupSysfsDMI(t, map[string]string{
			"bios_vendor":  "American Megatrends Inc.",
			"bios_version": "3.40",
			"bios_date":    "12/25/2023",
			"bios_release": "5.17",
		})
		bios, err := ParseBIOSInfoSysfs()
		if err != nil {
			t.Fatalf("ParseBIOSInfoSysfs() error: %v", err)
		}
		if bios.Vendor != "American Megatrends Inc." {
			t.Errorf("Vendor = %q, want %q", bios.Vendor, "American Megatrends Inc.")
		}
		if bios.Version != "3.40" {
			t.Errorf("Version = %q, want %q", bios.Version, "3.40")
		}
		if bios.ReleaseDate != "12/25/2023" {
			t.Errorf("ReleaseDate = %q, want %q", bios.ReleaseDate, "12/25/2023")
		}
		if bios.Revision != "5.17" {
			t.Errorf("Revision = %q, want %q", bios.Revision, "5.17")
		}
	})
	t.Run("empty_dir", func(t *testing.T) {
		setupSysfsDMI(t, map[string]string{})
		bios, err := ParseBIOSInfoSysfs()
		if err != nil {
			t.Fatalf("ParseBIOSInfoSysfs() error: %v", err)
		}
		if bios.Vendor != "" {
			t.Errorf("Vendor = %q, want empty", bios.Vendor)
		}
		if bios.Revision != "" {
			t.Errorf("Revision = %q, want empty", bios.Revision)
		}
	})
	t.Run("no_release", func(t *testing.T) {
		setupSysfsDMI(t, map[string]string{
			"bios_vendor":  "Phoenix",
			"bios_version": "1.0",
			"bios_date":    "01/01/2020",
		})
		bios, err := ParseBIOSInfoSysfs()
		if err != nil {
			t.Fatalf("ParseBIOSInfoSysfs() error: %v", err)
		}
		if bios.Vendor != "Phoenix" {
			t.Errorf("Vendor = %q, want %q", bios.Vendor, "Phoenix")
		}
		if bios.Revision != "" {
			t.Errorf("Revision = %q, want empty when bios_release missing", bios.Revision)
		}
	})
}

func TestParseBaseboardInfoSysfs(t *testing.T) {
	t.Run("with_data", func(t *testing.T) {
		setupSysfsDMI(t, map[string]string{
			"board_vendor":    "ASRock",
			"board_name":      "X570 Taichi",
			"board_version":   "1.0",
			"board_serial":    "SN123456",
			"board_asset_tag": "AT9999",
		})
		bb, err := ParseBaseboardInfoSysfs()
		if err != nil {
			t.Fatalf("ParseBaseboardInfoSysfs() error: %v", err)
		}
		if bb.Manufacturer != "ASRock" {
			t.Errorf("Manufacturer = %q, want ASRock", bb.Manufacturer)
		}
		if bb.ProductName != "X570 Taichi" {
			t.Errorf("ProductName = %q, want X570 Taichi", bb.ProductName)
		}
		if bb.Version != "1.0" {
			t.Errorf("Version = %q, want 1.0", bb.Version)
		}
		if bb.SerialNumber != "SN123456" {
			t.Errorf("SerialNumber = %q, want SN123456", bb.SerialNumber)
		}
		if bb.AssetTag != "AT9999" {
			t.Errorf("AssetTag = %q, want AT9999", bb.AssetTag)
		}
	})
	t.Run("empty_dir", func(t *testing.T) {
		setupSysfsDMI(t, map[string]string{})
		bb, err := ParseBaseboardInfoSysfs()
		if err != nil {
			t.Fatalf("ParseBaseboardInfoSysfs() error: %v", err)
		}
		if bb.Manufacturer != "" {
			t.Errorf("Manufacturer = %q, want empty", bb.Manufacturer)
		}
	})
}

func TestParseSystemInfoSysfs(t *testing.T) {
	t.Run("with_data", func(t *testing.T) {
		setupSysfsDMI(t, map[string]string{
			"product_name":   "PowerEdge R730",
			"product_family": "PowerEdge",
			"product_serial": "SERIAL123",
			"product_uuid":   "4C4C4544-0042-5810-8052-C4C04F325332",
			"product_sku":    "SKU999",
			"sys_vendor":     "Dell Inc.",
			"chassis_vendor": "Dell Inc.",
			"chassis_type":   "23",
		})
		info := ParseSystemInfoSysfs()
		checks := map[string]string{
			"product_name":   "PowerEdge R730",
			"product_family": "PowerEdge",
			"product_serial": "SERIAL123",
			"product_uuid":   "4C4C4544-0042-5810-8052-C4C04F325332",
			"product_sku":    "SKU999",
			"sys_vendor":     "Dell Inc.",
			"chassis_vendor": "Dell Inc.",
			"chassis_type":   "23",
		}
		for key, want := range checks {
			if got := info[key]; got != want {
				t.Errorf("info[%q] = %q, want %q", key, got, want)
			}
		}
	})
	t.Run("empty_dir", func(t *testing.T) {
		setupSysfsDMI(t, map[string]string{})
		info := ParseSystemInfoSysfs()
		if len(info) != 8 {
			t.Errorf("expected 8 keys, got %d", len(info))
		}
		for key, val := range info {
			if val != "" {
				t.Errorf("info[%q] = %q, want empty", key, val)
			}
		}
	})
}
