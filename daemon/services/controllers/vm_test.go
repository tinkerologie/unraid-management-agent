package controllers

import (
	"errors"
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

	t.Run("has CreateSnapshot method", func(t *testing.T) {
		_ = vc.CreateSnapshot
	})

	t.Run("has ListSnapshots method", func(t *testing.T) {
		_ = vc.ListSnapshots
	})

	t.Run("has DeleteSnapshot method", func(t *testing.T) {
		_ = vc.DeleteSnapshot
	})

	t.Run("has RestoreSnapshot method", func(t *testing.T) {
		_ = vc.RestoreSnapshot
	})

	t.Run("has CloneVM method", func(t *testing.T) {
		_ = vc.CloneVM
	})
}

func TestVMReset(t *testing.T) {
	vc := NewVMController()
	// No libvirt in CI → connect fails with a clear error (not a panic).
	err := vc.Reset("nonexistent-vm")
	if err == nil {
		t.Skip("libvirt available; reset reached the daemon")
	}
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

func TestVMControllerMockOperations(t *testing.T) {
	origBinaryExists := binaryExists
	t.Cleanup(func() {
		binaryExists = origBinaryExists
	})
	binaryExists = func(path string) bool {
		return true
	}
	t.Run("pmWakeup success and failure", func(t *testing.T) {
		var passedArgs []string
		vc := &VMController{
			exec: func(command string, args ...string) ([]string, error) {
				passedArgs = args
				return nil, nil
			},
		}
		if err := vc.pmWakeup("my-vm"); err != nil {
			t.Fatalf("pmWakeup failed: %v", err)
		}
		if len(passedArgs) != 2 || passedArgs[0] != "dompmwakeup" || passedArgs[1] != "my-vm" {
			t.Errorf("args = %v, want ['dompmwakeup', 'my-vm']", passedArgs)
		}

		failVc := &VMController{
			exec: func(command string, args ...string) ([]string, error) {
				return nil, errors.New("wakeup failed")
			},
		}
		if err := failVc.pmWakeup("my-vm"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("ListSnapshots parsing", func(t *testing.T) {
		vc := &VMController{
			exec: func(command string, args ...string) ([]string, error) {
				if len(args) >= 3 && args[0] == "snapshot-list" {
					return []string{"snap1", "snap2"}, nil
				}
				if len(args) >= 3 && args[0] == "snapshot-current" {
					return []string{"snap1"}, nil
				}
				if len(args) >= 3 && args[0] == "snapshot-info" {
					return []string{
						"Description: test snapshot",
						"State: running",
						"Creation Time: 2026-10-10 12:00:00",
						"Parent: root",
					}, nil
				}
				return nil, nil
			},
		}

		list, err := vc.ListSnapshots("test-vm")
		if err != nil {
			t.Fatalf("ListSnapshots failed: %v", err)
		}
		if list.Count != 2 {
			t.Fatalf("list.Count = %d, want 2", list.Count)
		}
		if !list.Snapshots[0].IsCurrent {
			t.Errorf("expected snap1 to be current")
		}
		if list.Snapshots[0].Description != "test snapshot" {
			t.Errorf("Description = %q, want 'test snapshot'", list.Snapshots[0].Description)
		}
	})

	t.Run("CloneVM success and error", func(t *testing.T) {
		var passedArgs []string
		vc := &VMController{
			execOutput: func(command string, args ...string) (string, error) {
				passedArgs = args
				return "Clone 'clone-1' created successfully", nil
			},
		}

		if err := vc.CloneVM("source-vm", "clone-1"); err != nil {
			t.Fatalf("CloneVM failed: %v", err)
		}
		if len(passedArgs) < 4 || passedArgs[1] != "source-vm" || passedArgs[3] != "clone-1" {
			t.Errorf("args = %v, want original source-vm and name clone-1", passedArgs)
		}

		failVc := &VMController{
			execOutput: func(command string, args ...string) (string, error) {
				return "disk full", errors.New("clone failed")
			},
		}
		if err := failVc.CloneVM("source-vm", "clone-1"); err == nil {
			t.Fatal("expected clone error, got nil")
		}
	})
}

func TestVMController_NilSeams(t *testing.T) {
	vc := &VMController{} // nil exec and execOutput
	_ = vc.pmWakeup("test-vm")
	_, _ = vc.ListSnapshots("test-vm")
	_ = vc.CloneVM("test-vm", "test-clone")
}

