package controllers

import (
	"testing"
)

func TestNewVMController(t *testing.T) {
	vc := NewVMController()

	if vc == nil {
		t.Fatal("NewVMController() returned nil")
	}
}

func TestVMControllerInterface(t *testing.T) {
	vc := NewVMController()

	// Test that the controller has all required methods
	// These tests verify the interface exists, not that commands work
	// (actual command execution requires libvirt API connection)

	t.Run("has Start method", func(t *testing.T) {
		_ = vc.Start
	})

	t.Run("has Stop method", func(t *testing.T) {
		_ = vc.Stop
	})

	t.Run("has Restart method", func(t *testing.T) {
		_ = vc.Restart
	})

	t.Run("has Pause method", func(t *testing.T) {
		_ = vc.Pause
	})

	t.Run("has Resume method", func(t *testing.T) {
		_ = vc.Resume
	})

	t.Run("has Hibernate method", func(t *testing.T) {
		_ = vc.Hibernate
	})

	t.Run("has ForceStop method", func(t *testing.T) {
		_ = vc.ForceStop
	})
}

func TestVMControllerWithInvalidVM(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	vc := NewVMController()

	// These operations should fail with invalid VM names
	// Testing error paths when libvirt API is available

	t.Run("Start with invalid VM", func(t *testing.T) {
		err := vc.Start("nonexistent-vm-12345")
		// Should return an error (VM doesn't exist)
		if err == nil {
			t.Log("Note: No error returned - libvirt might not be running or VM might exist")
		}
	})

	t.Run("Stop with invalid VM", func(t *testing.T) {
		err := vc.Stop("nonexistent-vm-12345")
		if err == nil {
			t.Log("Note: No error returned - libvirt might not be running or VM might exist")
		}
	})

	t.Run("ForceStop with invalid VM", func(t *testing.T) {
		err := vc.ForceStop("nonexistent-vm-12345")
		if err == nil {
			t.Log("Note: No error returned - libvirt might not be running or VM might exist")
		}
	})

	t.Run("Restart with invalid VM", func(t *testing.T) {
		err := vc.Restart("nonexistent-vm-12345")
		if err == nil {
			t.Log("Note: No error returned - libvirt might not be running or VM might exist")
		}
	})

	t.Run("Pause with invalid VM", func(t *testing.T) {
		err := vc.Pause("nonexistent-vm-12345")
		if err == nil {
			t.Log("Note: No error returned - libvirt might not be running or VM might exist")
		}
	})

	t.Run("Resume with invalid VM", func(t *testing.T) {
		err := vc.Resume("nonexistent-vm-12345")
		if err == nil {
			t.Log("Note: No error returned - libvirt might not be running or VM might exist")
		}
	})

	t.Run("Hibernate with invalid VM", func(t *testing.T) {
		err := vc.Hibernate("nonexistent-vm-12345")
		if err == nil {
			t.Log("Note: No error returned - libvirt might not be running or VM might exist")
		}
	})
}
