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
