package controllers

import (
	"strings"
	"testing"
)

func TestNewRemoteShareController(t *testing.T) {
	rc := NewRemoteShareController()
	if rc == nil {
		t.Fatal("NewRemoteShareController returned nil")
	}
}

// TestRemoteShareControllerRejectsInvalidSource verifies that invalid sources
// are rejected by validation before any attempt to invoke the control script.
func TestRemoteShareControllerRejectsInvalidSource(t *testing.T) {
	rc := NewRemoteShareController()

	tests := []struct {
		name   string
		source string
	}{
		{"empty", ""},
		{"plain path", "/mnt/user/backup"},
		{"leading hyphen", "-rf"},
		{"bare hostname", "nothost"},
		{"path traversal", "//server/../../etc/passwd"},
		{"smb missing share", "//server"},
		{"null byte", "//server/share\x00"},
		{"blank whitespace", "   "},
		{"excessively long", "//s/" + strings.Repeat("a", 10000)},
	}

	for _, tt := range tests {
		t.Run("mount/"+tt.name, func(t *testing.T) {
			if err := rc.Mount(tt.source); err == nil {
				t.Errorf("Mount(%q) expected validation error, got nil", tt.source)
			}
		})
		t.Run("unmount/"+tt.name, func(t *testing.T) {
			if err := rc.Unmount(tt.source); err == nil {
				t.Errorf("Unmount(%q) expected validation error, got nil", tt.source)
			}
		})
	}
}

// TestRemoteShareControllerValidSourceReachesExec confirms a well-formed source
// passes validation and proceeds to execution (which fails in the test
// environment because the control script is absent — surfaced as a mount/umount
// error, not a validation error).
func TestRemoteShareControllerValidSourceReachesExec(t *testing.T) {
	rc := NewRemoteShareController()

	if err := rc.Mount("//127.0.0.1/test"); err != nil &&
		!strings.Contains(err.Error(), "mount remote share") {
		t.Errorf("Mount: expected execution-stage error, got: %v", err)
	}
	// Unmount maps to the rc.unassigned "umount" verb, so the error reads
	// "umount remote share".
	if err := rc.Unmount("//127.0.0.1/test"); err != nil &&
		!strings.Contains(err.Error(), "umount remote share") {
		t.Errorf("Unmount: expected execution-stage error, got: %v", err)
	}
}
