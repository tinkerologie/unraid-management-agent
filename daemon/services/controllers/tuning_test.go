package controllers

import (
	"testing"
)

func TestTuningController_SetDiskCache_Validation(t *testing.T) {
	c := NewTuningController()

	tests := []struct {
		name           string
		bgRatio, ratio int
		writebackCenti int
		expireCenti    int
		wantErr        bool
		errContains    string
	}{
		{"valid defaults", 10, 20, 500, 3000, false, ""},
		{"valid equal ratios", 50, 50, 500, 3000, false, ""},
		{"valid zeros", 0, 0, 0, 0, false, ""},
		{"bg_ratio > ratio", 30, 20, 500, 3000, true, "must not exceed"},
		{"bg_ratio negative", -1, 20, 500, 3000, true, "must be 0-100"},
		{"bg_ratio over 100", 101, 20, 500, 3000, true, "must be 0-100"},
		{"ratio negative", 10, -1, 500, 3000, true, "must be 0-100"},
		{"ratio over 100", 10, 101, 500, 3000, true, "must be 0-100"},
		{"writeback negative", 10, 20, -1, 3000, true, "must be non-negative"},
		{"expire negative", 10, 20, 500, -1, true, "must be non-negative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.SetDiskCache(tt.bgRatio, tt.ratio, tt.writebackCenti, tt.expireCenti)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				if tt.errContains != "" {
					if !containsStr(err.Error(), tt.errContains) {
						t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
					}
				}
			} else if err != nil {
				// Allow runtime/write errors (no /proc/sys) but catch validation rejections for valid inputs
				if containsStr(err.Error(), "invalid") || containsStr(err.Error(), "must be") || containsStr(err.Error(), "must not") {
					t.Errorf("validation failed for valid input: %v", err)
				}
			}
		})
	}
}

func TestTuningController_SetInotifyLimits_Validation(t *testing.T) {
	c := NewTuningController()

	tests := []struct {
		name        string
		watches     int
		instances   int
		events      int
		wantErr     bool
		errContains string
	}{
		{"valid defaults", 524288, 128, 16384, false, ""},
		{"valid minimums", 1, 1, 1, false, ""},
		{"watches zero", 0, 128, 16384, true, "must be positive"},
		{"watches negative", -1, 128, 16384, true, "must be positive"},
		{"instances zero", 524288, 0, 16384, true, "must be positive"},
		{"instances negative", 524288, -1, 16384, true, "must be positive"},
		{"events zero", 524288, 128, 0, true, "must be positive"},
		{"events negative", 524288, 128, -1, true, "must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.SetInotifyLimits(tt.watches, tt.instances, tt.events)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				if tt.errContains != "" {
					if !containsStr(err.Error(), tt.errContains) {
						t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
					}
				}
			} else if err != nil {
				// Allow runtime/write errors (no /proc/sys) but catch validation rejections for valid inputs
				if containsStr(err.Error(), "must be") {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestTuningController_SetTurboBoost_NotAvailable(t *testing.T) {
	c := NewTuningController()
	// Without calling Initialize(), turboAvailable is false
	err := c.SetTurboBoost(true)
	if err == nil {
		t.Fatal("expected error when turbo not available")
	}
	if !containsStr(err.Error(), "not available") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewTuningController(t *testing.T) {
	c := NewTuningController()
	if c == nil {
		t.Fatal("NewTuningController returned nil")
	}
	if c.turboAvailable {
		t.Error("turboAvailable should be false before Initialize()")
	}
}

// containsStr is a test helper to check if a string contains a substring.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
