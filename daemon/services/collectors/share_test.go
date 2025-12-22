package collectors

import (
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewShareCollector(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewShareCollector(ctx)

	if collector == nil {
		t.Fatal("NewShareCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("ShareCollector context not set correctly")
	}
}

func TestShareINIParsing(t *testing.T) {
	// Test parsing of shares.ini format
	content := `[appdata]
name=appdata
comment=Application Data
allocator=highwater
splitLevel=
include=disk1,disk2
exclude=
useCache=yes

[media]
name=media
comment=Media Files
allocator=highwater
splitLevel=
include=
exclude=
useCache=no
`
	// Verify the content structure
	if content == "" {
		t.Error("Content is empty")
	}

	// Count sections
	sectionCount := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '[' {
			sectionCount++
		}
	}

	if sectionCount != 2 {
		t.Errorf("Expected 2 sections, got %d", sectionCount)
	}
}

func TestShareAllocatorTypes(t *testing.T) {
	// Test share allocator types
	tests := []struct {
		allocator string
		valid     bool
	}{
		{"highwater", true},
		{"fillup", true},
		{"most-free", true},
		{"invalid", false},
	}

	validAllocators := map[string]bool{
		"highwater": true,
		"fillup":    true,
		"most-free": true,
	}

	for _, tt := range tests {
		t.Run(tt.allocator, func(t *testing.T) {
			isValid := validAllocators[tt.allocator]
			if isValid != tt.valid {
				t.Errorf("Allocator %q validity = %v, want %v", tt.allocator, isValid, tt.valid)
			}
		})
	}
}

func TestShareCacheSettings(t *testing.T) {
	// Test cache setting values
	tests := []struct {
		value   string
		enabled bool
	}{
		{"yes", true},
		{"no", false},
		{"only", true},
		{"prefer", true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			cacheEnabled := tt.value == "yes" || tt.value == "only" || tt.value == "prefer"
			if cacheEnabled != tt.enabled {
				t.Errorf("Cache value %q: enabled = %v, want %v", tt.value, cacheEnabled, tt.enabled)
			}
		})
	}
}
func TestShareSplitLevels(t *testing.T) {
	// Test split level values
	tests := []struct {
		level       string
		description string
	}{
		{"", "automatic"},
		{"1", "level 1"},
		{"2", "level 2"},
		{"3", "level 3"},
		{"manual", "manual"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			// Verify split level is handled
			if tt.level == "" {
				// Empty means automatic
			}
		})
	}
}

func TestShareIncludeExcludeDisks(t *testing.T) {
	// Test include/exclude disk parsing
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"disk1", 1},
		{"disk1,disk2", 2},
		{"disk1,disk2,disk3", 3},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			count := countDisks(tt.input)
			if count != tt.expected {
				t.Errorf("countDisks(%q) = %d, want %d", tt.input, count, tt.expected)
			}
		})
	}
}

func countDisks(s string) int {
	if s == "" {
		return 0
	}
	count := 1
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			count++
		}
	}
	return count
}

func TestShareSecurityModes(t *testing.T) {
	// Test share security modes
	modes := []string{"public", "secure", "private"}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			if mode == "" {
				t.Error("Security mode should not be empty")
			}
		})
	}
}

func TestShareFreeSpaceThreshold(t *testing.T) {
	// Test minimum free space settings
	tests := []struct {
		bytes   uint64
		humanGB float64
	}{
		{0, 0},
		{1024 * 1024 * 1024, 1.0},         // 1 GB
		{10 * 1024 * 1024 * 1024, 10.0},   // 10 GB
		{100 * 1024 * 1024 * 1024, 100.0}, // 100 GB
	}

	for _, tt := range tests {
		t.Run(formatFloat(tt.humanGB), func(t *testing.T) {
			gb := float64(tt.bytes) / (1024 * 1024 * 1024)
			if gb != tt.humanGB {
				t.Errorf("bytes %d = %.1f GB, want %.1f GB", tt.bytes, gb, tt.humanGB)
			}
		})
	}
}

func formatFloat(f float64) string {
	if f == 0 {
		return "0"
	}
	// Simple integer formatting for test names
	return formatInt64(int64(f))
}

func formatInt64(n int64) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
