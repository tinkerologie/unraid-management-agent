package controllers

import (
	"errors"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewArrayController(t *testing.T) {
	ctx := &domain.Context{}
	ac := NewArrayController(ctx)

	if ac == nil {
		t.Fatal("NewArrayController() returned nil")
	}

	if ac.ctx != ctx {
		t.Error("ArrayController context not set correctly")
	}
}

func TestArrayControllerInterface(t *testing.T) {
	ctx := &domain.Context{}
	ac := NewArrayController(ctx)

	// Test that the controller has all required methods
	// These tests verify the interface exists, not that commands work
	// (actual command execution requires Unraid mdcmd)

	t.Run("has StartArray method", func(t *testing.T) {
		_ = ac.StartArray
	})

	t.Run("has StopArray method", func(t *testing.T) {
		_ = ac.StopArray
	})

	t.Run("has StartParityCheck method", func(t *testing.T) {
		_ = ac.StartParityCheck
	})

	t.Run("has StopParityCheck method", func(t *testing.T) {
		_ = ac.StopParityCheck
	})

	t.Run("has PauseParityCheck method", func(t *testing.T) {
		_ = ac.PauseParityCheck
	})

	t.Run("has ResumeParityCheck method", func(t *testing.T) {
		_ = ac.ResumeParityCheck
	})

	t.Run("has SpinDownDisk method", func(t *testing.T) {
		_ = ac.SpinDownDisk
	})

	t.Run("has SpinUpDisk method", func(t *testing.T) {
		_ = ac.SpinUpDisk
	})
}

func TestArrayControllerCommandsWithMocks(t *testing.T) {
	var lastCmd []string
	ac := &ArrayController{
		isProcMdcmdAvailable: func() bool { return true },
		mdcmdWrite: func(args ...string) error {
			lastCmd = args
			return nil
		},
	}

	if err := ac.StartArray(); err != nil {
		t.Fatalf("StartArray failed: %v", err)
	}
	if len(lastCmd) != 1 || lastCmd[0] != "start" {
		t.Errorf("StartArray wrote %v, want ['start']", lastCmd)
	}

	if err := ac.StopArray(); err != nil {
		t.Fatalf("StopArray failed: %v", err)
	}
	if len(lastCmd) != 1 || lastCmd[0] != "stop" {
		t.Errorf("StopArray wrote %v, want ['stop']", lastCmd)
	}

	if err := ac.StartParityCheck(true); err != nil {
		t.Fatalf("StartParityCheck(true) failed: %v", err)
	}
	if len(lastCmd) != 2 || lastCmd[0] != "check" || lastCmd[1] != "CORRECT" {
		t.Errorf("StartParityCheck(true) wrote %v, want ['check', 'CORRECT']", lastCmd)
	}

	if err := ac.StartParityCheck(false); err != nil {
		t.Fatalf("StartParityCheck(false) failed: %v", err)
	}
	if len(lastCmd) != 2 || lastCmd[0] != "check" || lastCmd[1] != "NOCORRECT" {
		t.Errorf("StartParityCheck(false) wrote %v, want ['check', 'NOCORRECT']", lastCmd)
	}

	if err := ac.StopParityCheck(); err != nil {
		t.Fatalf("StopParityCheck failed: %v", err)
	}
	if len(lastCmd) != 1 || lastCmd[0] != "nocheck" {
		t.Errorf("StopParityCheck wrote %v, want ['nocheck']", lastCmd)
	}

	if err := ac.PauseParityCheck(); err != nil {
		t.Fatalf("PauseParityCheck failed: %v", err)
	}
	if len(lastCmd) != 1 || lastCmd[0] != "pause" {
		t.Errorf("PauseParityCheck wrote %v, want ['pause']", lastCmd)
	}

	if err := ac.ResumeParityCheck(); err != nil {
		t.Fatalf("ResumeParityCheck failed: %v", err)
	}
	if len(lastCmd) != 1 || lastCmd[0] != "resume" {
		t.Errorf("ResumeParityCheck wrote %v, want ['resume']", lastCmd)
	}
}

func TestArrayControllerDiskOperationsWithMocks(t *testing.T) {
	t.Run("emhttpd socket path with startState", func(t *testing.T) {
		var lastParams map[string]string
		ac := &ArrayController{
			isEmhttpdAvailable: func() bool { return true },
			readStartState:     func() string { return "STARTED" },
			emhttpdRequest: func(params map[string]string) error {
				lastParams = params
				return nil
			},
		}

		if err := ac.SpinUpDisk("disk1"); err != nil {
			t.Fatalf("SpinUpDisk failed: %v", err)
		}
		if lastParams["cmdSpinup"] != "disk1" || lastParams["startState"] != "STARTED" {
			t.Errorf("SpinUpDisk params = %v, want cmdSpinup=disk1, startState=STARTED", lastParams)
		}

		if err := ac.SpinDownDisk("disk1"); err != nil {
			t.Fatalf("SpinDownDisk failed: %v", err)
		}
		if lastParams["cmdSpindown"] != "disk1" || lastParams["startState"] != "STARTED" {
			t.Errorf("SpinDownDisk params = %v, want cmdSpindown=disk1, startState=STARTED", lastParams)
		}
	})

	t.Run("fallback to proc mdcmd when socket unavailable", func(t *testing.T) {
		var lastCmd []string
		ac := &ArrayController{
			isEmhttpdAvailable:   func() bool { return false },
			isProcMdcmdAvailable: func() bool { return true },
			mdcmdWrite: func(args ...string) error {
				lastCmd = args
				return nil
			},
		}

		if err := ac.SpinUpDisk("disk2"); err != nil {
			t.Fatalf("SpinUpDisk fallback failed: %v", err)
		}
		if len(lastCmd) != 2 || lastCmd[0] != "spinup" || lastCmd[1] != "disk2" {
			t.Errorf("SpinUpDisk wrote %v, want ['spinup', 'disk2']", lastCmd)
		}

		if err := ac.SpinDownDisk("disk2"); err != nil {
			t.Fatalf("SpinDownDisk fallback failed: %v", err)
		}
		if len(lastCmd) != 2 || lastCmd[0] != "spindown" || lastCmd[1] != "disk2" {
			t.Errorf("SpinDownDisk wrote %v, want ['spindown', 'disk2']", lastCmd)
		}
	})

	t.Run("ClearDiskStats with emhttpd", func(t *testing.T) {
		var lastParams map[string]string
		ac := &ArrayController{
			isEmhttpdAvailable: func() bool { return true },
			emhttpdRequest: func(params map[string]string) error {
				lastParams = params
				return nil
			},
		}
		if err := ac.ClearDiskStats(); err != nil {
			t.Fatalf("ClearDiskStats failed: %v", err)
		}
		if lastParams["clearStatistics"] != "true" {
			t.Errorf("ClearDiskStats params = %v, want clearStatistics=true", lastParams)
		}
	})
}

func TestEmcmdSpinRejectsWhitespaceDiskNames(t *testing.T) {
	tests := []struct {
		name     string
		diskName string
	}{
		{name: "space", diskName: "disk 1"},
		{name: "tab", diskName: "disk\t1"},
		{name: "newline", diskName: "disk1\nstart"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &ArrayController{}
			err := controller.emcmdSpin("cmdSpinup", "spinup", tt.diskName)
			if err == nil {
				t.Fatalf("emcmdSpin(%q) expected error for whitespace disk name", tt.diskName)
			}
			if !strings.Contains(err.Error(), "must not contain whitespace") {
				t.Fatalf("emcmdSpin(%q) error = %v, want whitespace validation error", tt.diskName, err)
			}
		})
	}
}

// TestArrayDiskClearStats tests the ClearDiskStats method.
func TestArrayDiskClearStats(t *testing.T) {
	ctx := &domain.Context{}
	ac := NewArrayController(ctx)

	t.Run("has ClearDiskStats method", func(t *testing.T) {
		// Verify the method exists on the controller.
		_ = ac.ClearDiskStats
	})

	t.Run("capability gate: socket absent returns socket-unavailable error", func(t *testing.T) {
		// In CI / non-Unraid environments the emhttpd socket is absent.
		// ClearDiskStats must return a socket-unavailable error, not panic.
		err := ac.ClearDiskStats()
		if err == nil {
			// Socket is present (running on Unraid hardware); nothing to assert here.
			t.Log("Note: emhttpd socket present — ClearDiskStats reached the emhttpd socket")
			return
		}
		// Error must be non-nil and reference the socket when unavailable.
		if err.Error() == "" {
			t.Error("ClearDiskStats returned a non-nil error with empty message")
		}
		if !strings.Contains(err.Error(), "emhttpd socket") {
			t.Errorf("expected socket-unavailable error, got: %v", err)
		}
		t.Logf("ClearDiskStats (no socket) correctly returned: %v", err)
	})
}

func TestArrayController_SeamFallbacksAndErrors(t *testing.T) {
	t.Run("spin fallback to mdcmd when emhttpd socket unavailable", func(t *testing.T) {
		var mdcmdCalled bool
		c := &ArrayController{
			isEmhttpdAvailable:   func() bool { return false },
			isProcMdcmdAvailable: func() bool { return true },
			mdcmdWrite: func(args ...string) error {
				mdcmdCalled = true
				return nil
			},
		}
		if err := c.SpinUpDisk("disk1"); err != nil || !mdcmdCalled {
			t.Errorf("expected fallback to mdcmdWrite, err: %v, called: %v", err, mdcmdCalled)
		}
	})

	t.Run("spin error when emhttpdRequest fails", func(t *testing.T) {
		c := &ArrayController{
			isEmhttpdAvailable: func() bool { return true },
			readStartState:     func() string { return "" },
			emhttpdRequest: func(params map[string]string) error {
				return errors.New("emhttpd fail")
			},
		}
		if err := c.SpinDownDisk("disk1"); err == nil {
			t.Error("expected error from emhttpd fail")
		}
	})

	t.Run("ClearDiskStats success when socket available", func(t *testing.T) {
		var reqSent bool
		c := &ArrayController{
			isEmhttpdAvailable: func() bool { return true },
			emhttpdRequest: func(params map[string]string) error {
				if params["clearStatistics"] == "true" {
					reqSent = true
				}
				return nil
			},
		}
		if err := c.ClearDiskStats(); err != nil || !reqSent {
			t.Errorf("ClearDiskStats err: %v, reqSent: %v", err, reqSent)
		}
	})

	t.Run("ClearDiskStats error when emhttpdRequest fails", func(t *testing.T) {
		c := &ArrayController{
			isEmhttpdAvailable: func() bool { return true },
			emhttpdRequest: func(params map[string]string) error {
				return errors.New("clear fail")
			},
		}
		if err := c.ClearDiskStats(); err == nil {
			t.Error("expected error from clear fail")
		}
	})

	t.Run("mdcmd fallback when proc mdcmd unavailable", func(t *testing.T) {
		c := &ArrayController{
			isProcMdcmdAvailable: func() bool { return false },
			execCommand: func(cmd string, args ...string) ([]string, error) {
				return nil, nil
			},
		}
		// In test env, mdcmd binary is not in /usr/local/sbin, so guard returns error
		_ = c.StartArray()
	})
}
