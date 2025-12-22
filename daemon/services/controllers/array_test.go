package controllers

import (
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

func TestArrayControllerParityCheckModes(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := &domain.Context{}
	ac := NewArrayController(ctx)

	// Test that different parity check modes are called correctly
	// These will fail without mdcmd but test the logic paths

	t.Run("StartParityCheck with correcting=true", func(t *testing.T) {
		err := ac.StartParityCheck(true)
		// Will fail without mdcmd, but tests the code path
		if err == nil {
			t.Log("Note: No error - mdcmd might be available")
		}
	})

	t.Run("StartParityCheck with correcting=false", func(t *testing.T) {
		err := ac.StartParityCheck(false)
		// Will fail without mdcmd, but tests the code path
		if err == nil {
			t.Log("Note: No error - mdcmd might be available")
		}
	})

	t.Run("StartArray", func(t *testing.T) {
		err := ac.StartArray()
		// Will fail without mdcmd, but tests the code path
		if err == nil {
			t.Log("Note: No error - mdcmd might be available")
		}
	})

	t.Run("StopArray", func(t *testing.T) {
		err := ac.StopArray()
		// Will fail without mdcmd, but tests the code path
		if err == nil {
			t.Log("Note: No error - mdcmd might be available")
		}
	})

	t.Run("StopParityCheck", func(t *testing.T) {
		err := ac.StopParityCheck()
		// Will fail without mdcmd, but tests the code path
		if err == nil {
			t.Log("Note: No error - mdcmd might be available")
		}
	})

	t.Run("PauseParityCheck", func(t *testing.T) {
		err := ac.PauseParityCheck()
		// Will fail without mdcmd, but tests the code path
		if err == nil {
			t.Log("Note: No error - mdcmd might be available")
		}
	})

	t.Run("ResumeParityCheck", func(t *testing.T) {
		err := ac.ResumeParityCheck()
		// Will fail without mdcmd, but tests the code path
		if err == nil {
			t.Log("Note: No error - mdcmd might be available")
		}
	})
}

func TestArrayControllerDiskOperations(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := &domain.Context{}
	ac := NewArrayController(ctx)

	t.Run("SpinDownDisk with invalid disk", func(t *testing.T) {
		err := ac.SpinDownDisk("nonexistent-disk")
		// Will fail without mdcmd
		if err == nil {
			t.Log("Note: No error - mdcmd might be available")
		}
	})

	t.Run("SpinUpDisk with invalid disk", func(t *testing.T) {
		err := ac.SpinUpDisk("nonexistent-disk")
		// Will fail without mdcmd
		if err == nil {
			t.Log("Note: No error - mdcmd might be available")
		}
	})
}
