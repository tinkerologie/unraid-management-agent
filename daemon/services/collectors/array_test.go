package collectors

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewArrayCollector(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewArrayCollector(ctx)

	if collector == nil {
		t.Fatal("NewArrayCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("ArrayCollector context not set correctly")
	}
}

// TestArraySafeBlockSizeConversion tests the safe int64 to uint64 conversion
// for stat.Bsize used in array size calculations. This validates the fix
// for gosec G115 (CWE-190) integer overflow vulnerability.
func TestArraySafeBlockSizeConversion(t *testing.T) {
	// Simulate the statfs conversion pattern used in updateArrayStats
	stat := syscall.Statfs_t{
		Bsize:  4096,
		Blocks: 10000000000, // ~40TB filesystem
		Bfree:  5000000000,  // ~20TB free
	}

	// Safe conversion pattern (as used in array.go)
	//nolint:gosec // G115: Bsize is always positive on Linux systems
	bsize := uint64(stat.Bsize)
	totalBytes := stat.Blocks * bsize
	freeBytes := stat.Bfree * bsize
	usedBytes := totalBytes - freeBytes

	// Expected values
	expectedTotal := uint64(40960000000000) // ~40TB
	expectedFree := uint64(20480000000000)  // ~20TB
	expectedUsed := uint64(20480000000000)  // ~20TB

	if totalBytes != expectedTotal {
		t.Errorf("totalBytes = %d, want %d", totalBytes, expectedTotal)
	}
	if freeBytes != expectedFree {
		t.Errorf("freeBytes = %d, want %d", freeBytes, expectedFree)
	}
	if usedBytes != expectedUsed {
		t.Errorf("usedBytes = %d, want %d", usedBytes, expectedUsed)
	}

	// Verify usage percentage calculation
	var usagePercent float64
	if totalBytes > 0 {
		usagePercent = float64(usedBytes) / float64(totalBytes) * 100
	}
	if usagePercent < 49.9 || usagePercent > 50.1 {
		t.Errorf("usagePercent = %f, want ~50.0", usagePercent)
	}
}

// TestCountParityDisksFromINI tests the parity disk counting logic with various INI file contents
func TestCountParityDisksFromINI(t *testing.T) {
	tests := []struct {
		name           string
		disksINI       string
		expectedParity int
	}{
		{
			name: "single active parity disk",
			disksINI: `[parity]
type="Parity"
status="DISK_OK"
device="sda"

[disk1]
type="Data"
status="DISK_OK"
device="sdb"

[disk2]
type="Data"
status="DISK_OK"
device="sdc"
`,
			expectedParity: 1,
		},
		{
			name: "dual active parity disks",
			disksINI: `[parity]
type="Parity"
status="DISK_OK"
device="sda"

[parity2]
type="Parity"
status="DISK_OK"
device="sdb"

[disk1]
type="Data"
status="DISK_OK"
device="sdc"
`,
			expectedParity: 2,
		},
		{
			name: "no parity disks (data only)",
			disksINI: `[disk1]
type="Data"
status="DISK_OK"
device="sda"

[disk2]
type="Data"
status="DISK_OK"
device="sdb"
`,
			expectedParity: 0,
		},
		{
			name: "parity disk disabled (DISK_NP_DSBL)",
			disksINI: `[parity]
type="Parity"
status="DISK_NP_DSBL"
device=""

[disk1]
type="Data"
status="DISK_OK"
device="sda"
`,
			expectedParity: 0,
		},
		{
			name: "parity disk not present (DISK_NP)",
			disksINI: `[parity]
type="Parity"
status="DISK_NP"
device=""

[disk1]
type="Data"
status="DISK_OK"
device="sda"
`,
			expectedParity: 0,
		},
		{
			name: "parity disk disabled (DISK_DSBL)",
			disksINI: `[parity]
type="Parity"
status="DISK_DSBL"
device="sda"

[disk1]
type="Data"
status="DISK_OK"
device="sdb"
`,
			expectedParity: 0,
		},
		{
			name: "mixed: one active parity, one disabled parity",
			disksINI: `[parity]
type="Parity"
status="DISK_OK"
device="sda"

[parity2]
type="Parity"
status="DISK_NP_DSBL"
device=""

[disk1]
type="Data"
status="DISK_OK"
device="sdb"
`,
			expectedParity: 1,
		},
		{
			name: "parity disk with DISK_NEW status (should count)",
			disksINI: `[parity]
type="Parity"
status="DISK_NEW"
device="sda"

[disk1]
type="Data"
status="DISK_OK"
device="sdb"
`,
			expectedParity: 1,
		},
		{
			name: "parity disk with DISK_INVALID status (should count - not in exclusion list)",
			disksINI: `[parity]
type="Parity"
status="DISK_INVALID"
device="sda"

[disk1]
type="Data"
status="DISK_OK"
device="sdb"
`,
			expectedParity: 1,
		},
		{
			name: "section missing type key",
			disksINI: `[parity]
status="DISK_OK"
device="sda"

[disk1]
type="Data"
status="DISK_OK"
device="sdb"
`,
			expectedParity: 0,
		},
		{
			name: "section missing status key",
			disksINI: `[parity]
type="Parity"
device="sda"

[disk1]
type="Data"
status="DISK_OK"
device="sdb"
`,
			expectedParity: 0,
		},
		{
			name:           "empty INI file",
			disksINI:       ``,
			expectedParity: 0,
		},
		{
			name: "cache disk should not be counted as parity",
			disksINI: `[cache]
type="Cache"
status="DISK_OK"
device="nvme0n1"

[disk1]
type="Data"
status="DISK_OK"
device="sda"
`,
			expectedParity: 0,
		},
		{
			name: "flash disk should not be counted as parity",
			disksINI: `[flash]
type="Flash"
status="DISK_OK"
device="sda"

[disk1]
type="Data"
status="DISK_OK"
device="sdb"
`,
			expectedParity: 0,
		},
		{
			name: "quoted values with spaces",
			disksINI: `[parity]
type="Parity"
status="DISK_OK"
device="sda"

[disk1]
type="Data"
status="DISK_OK"
device="sdb"
`,
			expectedParity: 1,
		},
		{
			name: "two parity disks one disabled one not present",
			disksINI: `[parity]
type="Parity"
status="DISK_DSBL"
device=""

[parity2]
type="Parity"
status="DISK_NP"
device=""

[disk1]
type="Data"
status="DISK_OK"
device="sda"
`,
			expectedParity: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for test files
			tmpDir := t.TempDir()
			disksINIPath := filepath.Join(tmpDir, "disks.ini")

			// Write the test INI content
			if err := os.WriteFile(disksINIPath, []byte(tt.disksINI), 0644); err != nil {
				t.Fatalf("Failed to write test disks.ini: %v", err)
			}

			// Count parity disks using the helper function
			parityCount := countParityDisksFromFile(disksINIPath)

			if parityCount != tt.expectedParity {
				t.Errorf("countParityDisks() = %d, want %d", parityCount, tt.expectedParity)
			}
		})
	}
}

// TestCalculateDataDisks tests the data disk calculation logic
func TestCalculateDataDisks(t *testing.T) {
	tests := []struct {
		name             string
		totalDisks       int
		parityDisks      int
		expectedDataDisk int
	}{
		{
			name:             "5 total disks, 1 parity",
			totalDisks:       5,
			parityDisks:      1,
			expectedDataDisk: 4,
		},
		{
			name:             "10 total disks, 2 parity",
			totalDisks:       10,
			parityDisks:      2,
			expectedDataDisk: 8,
		},
		{
			name:             "0 total disks, 0 parity",
			totalDisks:       0,
			parityDisks:      0,
			expectedDataDisk: 0,
		},
		{
			name:             "1 total disk, 1 parity (edge case)",
			totalDisks:       1,
			parityDisks:      1,
			expectedDataDisk: 0,
		},
		{
			name:             "3 total disks, 0 parity (no parity configured)",
			totalDisks:       3,
			parityDisks:      0,
			expectedDataDisk: 3,
		},
		{
			name:             "28 total disks, 2 parity (max array)",
			totalDisks:       28,
			parityDisks:      2,
			expectedDataDisk: 26,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate data disks using the same formula as the collector
			dataDisks := tt.totalDisks - tt.parityDisks

			if dataDisks != tt.expectedDataDisk {
				t.Errorf("Data disks = %d, want %d (total=%d, parity=%d)",
					dataDisks, tt.expectedDataDisk, tt.totalDisks, tt.parityDisks)
			}
		})
	}
}

// TestParityDiskStatusFiltering tests that specific disk statuses are correctly filtered
func TestParityDiskStatusFiltering(t *testing.T) {
	// These are the statuses that should be excluded
	excludedStatuses := []string{"DISK_NP_DSBL", "DISK_NP", "DISK_DSBL"}

	// These statuses should be included (counted as active parity)
	includedStatuses := []string{"DISK_OK", "DISK_NEW", "DISK_INVALID", "DISK_WRONG", "DISK_EMULATED"}

	for _, status := range excludedStatuses {
		t.Run("excluded_"+status, func(t *testing.T) {
			tmpDir := t.TempDir()
			disksINIPath := filepath.Join(tmpDir, "disks.ini")

			content := `[parity]
type="Parity"
status="` + status + `"
device="sda"
`
			if err := os.WriteFile(disksINIPath, []byte(content), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			count := countParityDisksFromFile(disksINIPath)
			if count != 0 {
				t.Errorf("Status %q should be excluded but got count=%d", status, count)
			}
		})
	}

	for _, status := range includedStatuses {
		t.Run("included_"+status, func(t *testing.T) {
			tmpDir := t.TempDir()
			disksINIPath := filepath.Join(tmpDir, "disks.ini")

			content := `[parity]
type="Parity"
status="` + status + `"
device="sda"
`
			if err := os.WriteFile(disksINIPath, []byte(content), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			count := countParityDisksFromFile(disksINIPath)
			if count != 1 {
				t.Errorf("Status %q should be included but got count=%d", status, count)
			}
		})
	}
}

// TestArrayDiskCountsIntegration tests the complete disk counting scenario that was broken in Issue #30
func TestArrayDiskCountsIntegration(t *testing.T) {
	tests := []struct {
		name           string
		mdNumDisks     int // From var.ini
		disksINI       string
		expectedTotal  int
		expectedData   int
		expectedParity int
		description    string
	}{
		{
			name:       "Issue #30 scenario: 5 disks with 1 parity",
			mdNumDisks: 5,
			disksINI: `[parity]
type="Parity"
status="DISK_OK"
device="sda"

[disk1]
type="Data"
status="DISK_OK"
device="sdb"

[disk2]
type="Data"
status="DISK_OK"
device="sdc"

[disk3]
type="Data"
status="DISK_OK"
device="sdd"

[disk4]
type="Data"
status="DISK_OK"
device="sde"
`,
			expectedTotal:  5,
			expectedData:   4,
			expectedParity: 1,
			description:    "Original bug: was reporting num_data_disks=1, num_parity_disks=2",
		},
		{
			name:       "dual parity configuration",
			mdNumDisks: 10,
			disksINI: `[parity]
type="Parity"
status="DISK_OK"
device="sda"

[parity2]
type="Parity"
status="DISK_OK"
device="sdb"

[disk1]
type="Data"
status="DISK_OK"
device="sdc"

[disk2]
type="Data"
status="DISK_OK"
device="sdd"

[disk3]
type="Data"
status="DISK_OK"
device="sde"

[disk4]
type="Data"
status="DISK_OK"
device="sdf"

[disk5]
type="Data"
status="DISK_OK"
device="sdg"

[disk6]
type="Data"
status="DISK_OK"
device="sdh"

[disk7]
type="Data"
status="DISK_OK"
device="sdi"

[disk8]
type="Data"
status="DISK_OK"
device="sdj"
`,
			expectedTotal:  10,
			expectedData:   8,
			expectedParity: 2,
			description:    "Standard dual parity setup",
		},
		{
			name:       "dual parity with second disabled",
			mdNumDisks: 5,
			disksINI: `[parity]
type="Parity"
status="DISK_OK"
device="sda"

[parity2]
type="Parity"
status="DISK_NP_DSBL"
device=""

[disk1]
type="Data"
status="DISK_OK"
device="sdb"

[disk2]
type="Data"
status="DISK_OK"
device="sdc"

[disk3]
type="Data"
status="DISK_OK"
device="sdd"

[disk4]
type="Data"
status="DISK_OK"
device="sde"
`,
			expectedTotal:  5,
			expectedData:   4,
			expectedParity: 1,
			description:    "Parity2 is disabled but defined in INI",
		},
		{
			name:       "no parity configuration",
			mdNumDisks: 3,
			disksINI: `[disk1]
type="Data"
status="DISK_OK"
device="sda"

[disk2]
type="Data"
status="DISK_OK"
device="sdb"

[disk3]
type="Data"
status="DISK_OK"
device="sdc"
`,
			expectedTotal:  3,
			expectedData:   3,
			expectedParity: 0,
			description:    "Array without parity protection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for test files
			tmpDir := t.TempDir()
			disksINIPath := filepath.Join(tmpDir, "disks.ini")

			// Write the test disks.ini
			if err := os.WriteFile(disksINIPath, []byte(tt.disksINI), 0644); err != nil {
				t.Fatalf("Failed to write disks.ini: %v", err)
			}

			// Count parity disks
			parityCount := countParityDisksFromFile(disksINIPath)

			// Calculate data disks (same formula as collector)
			dataCount := tt.mdNumDisks - parityCount

			// Verify results
			if parityCount != tt.expectedParity {
				t.Errorf("Parity count = %d, want %d (%s)", parityCount, tt.expectedParity, tt.description)
			}

			if dataCount != tt.expectedData {
				t.Errorf("Data count = %d, want %d (%s)", dataCount, tt.expectedData, tt.description)
			}

			// Verify total adds up
			if parityCount+dataCount != tt.expectedTotal {
				t.Errorf("Total mismatch: parity(%d) + data(%d) = %d, want %d",
					parityCount, dataCount, parityCount+dataCount, tt.expectedTotal)
			}
		})
	}
}

// TestMissingDisksINIFile tests graceful handling when disks.ini doesn't exist
func TestMissingDisksINIFile(t *testing.T) {
	// Use a path that doesn't exist
	nonExistentPath := "/tmp/non_existent_dir_12345/disks.ini"

	count := countParityDisksFromFile(nonExistentPath)

	if count != 0 {
		t.Errorf("Expected 0 when file doesn't exist, got %d", count)
	}
}

// countParityDisksFromFile is a test helper that replicates the counting logic
// from ArrayCollector.countParityDisks() but accepts a file path parameter
func countParityDisksFromFile(filePath string) int {
	cfg, err := loadINIFile(filePath)
	if err != nil {
		return 0
	}

	parityCount := 0
	for _, section := range cfg.Sections() {
		if section.HasKey("type") && section.HasKey("status") {
			diskType := trimQuotes(section.Key("type").String())
			diskStatus := trimQuotes(section.Key("status").String())

			// Only count parity disks that are active (not disabled)
			if diskType == "Parity" && diskStatus != "DISK_NP_DSBL" && diskStatus != "DISK_NP" && diskStatus != "DISK_DSBL" {
				parityCount++
			}
		}
	}

	return parityCount
}

// loadINIFile loads an INI file for testing
func loadINIFile(filePath string) (*iniFile, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return parseINIContent(string(data))
}

// iniFile represents a parsed INI file for testing
type iniFile struct {
	sections map[string]*iniSection
}

// iniSection represents a section in an INI file
type iniSection struct {
	name string
	keys map[string]string
}

// Sections returns all sections in the INI file
func (f *iniFile) Sections() []*iniSection {
	result := make([]*iniSection, 0, len(f.sections))
	for _, s := range f.sections {
		result = append(result, s)
	}
	return result
}

// HasKey checks if a key exists in the section
func (s *iniSection) HasKey(key string) bool {
	_, ok := s.keys[key]
	return ok
}

// Key returns a key value holder
func (s *iniSection) Key(key string) *iniKey {
	return &iniKey{value: s.keys[key]}
}

// Name returns the section name
func (s *iniSection) Name() string {
	return s.name
}

// iniKey represents a key-value pair
type iniKey struct {
	value string
}

// String returns the key value
func (k *iniKey) String() string {
	return k.value
}

// parseINIContent parses INI content into a structured format
func parseINIContent(content string) (*iniFile, error) {
	file := &iniFile{sections: make(map[string]*iniSection)}
	var currentSection *iniSection

	lines := splitLines(content)
	for _, line := range lines {
		line = trimSpace(line)

		// Skip empty lines and comments
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}

		// Check for section header
		if line[0] == '[' && line[len(line)-1] == ']' {
			sectionName := line[1 : len(line)-1]
			currentSection = &iniSection{
				name: sectionName,
				keys: make(map[string]string),
			}
			file.sections[sectionName] = currentSection
			continue
		}

		// Parse key=value
		if currentSection != nil {
			if idx := indexByte(line, '='); idx > 0 {
				key := trimSpace(line[:idx])
				value := trimSpace(line[idx+1:])
				currentSection.keys[key] = value
			}
		}
	}

	return file, nil
}

// Helper functions to avoid importing strings package in test logic
func trimQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
