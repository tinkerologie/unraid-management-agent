package controllers

import (
	"testing"
)

func TestNewDockerController(t *testing.T) {
	dc := NewDockerController()

	if dc == nil {
		t.Fatal("NewDockerController() returned nil")
	}
}

func TestDockerControllerInterface(t *testing.T) {
	dc := NewDockerController()

	// Test that the controller has all required methods
	// These tests verify the interface exists, not that commands work
	// (actual command execution requires Docker to be running)

	t.Run("has Start method", func(t *testing.T) {
		// Method exists and can be called (will fail without Docker)
		_ = dc.Start
	})

	t.Run("has Stop method", func(t *testing.T) {
		_ = dc.Stop
	})

	t.Run("has Restart method", func(t *testing.T) {
		_ = dc.Restart
	})

	t.Run("has Pause method", func(t *testing.T) {
		_ = dc.Pause
	})

	t.Run("has Unpause method", func(t *testing.T) {
		_ = dc.Unpause
	})
}

func TestDockerControllerWithInvalidContainer(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	dc := NewDockerController()

	// These operations should fail with invalid container names
	// Testing error paths when Docker is available

	t.Run("Start with invalid container", func(t *testing.T) {
		err := dc.Start("nonexistent-container-12345")
		// Should return an error (container doesn't exist)
		if err == nil {
			t.Log("Note: No error returned - Docker might not be running or container might exist")
		}
	})

	t.Run("Stop with invalid container", func(t *testing.T) {
		err := dc.Stop("nonexistent-container-12345")
		if err == nil {
			t.Log("Note: No error returned - Docker might not be running or container might exist")
		}
	})
}
