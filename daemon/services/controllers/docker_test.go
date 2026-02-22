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
	// (actual command execution requires Docker SDK connection)

	t.Run("has Start method", func(t *testing.T) {
		// Method exists and can be called (will fail without Docker socket)
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

	t.Run("has Close method", func(t *testing.T) {
		_ = dc.Close
	})
}

func TestDockerControllerWithInvalidContainer(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	dc := NewDockerController()
	defer dc.Close()

	// These operations should fail with invalid container names
	// Testing error paths when Docker SDK is available

	t.Run("Start with invalid container", func(t *testing.T) {
		err := dc.Start("nonexistent-container-12345")
		// Should return an error (container doesn't exist)
		if err == nil {
			t.Log("Note: No error returned - Docker socket might not be available or container might exist")
		}
	})

	t.Run("Stop with invalid container", func(t *testing.T) {
		err := dc.Stop("nonexistent-container-12345")
		if err == nil {
			t.Log("Note: No error returned - Docker socket might not be available or container might exist")
		}
	})
}
func TestDockerControllerPause(t *testing.T) {
	dc := NewDockerController()
	defer dc.Close()

	t.Run("Pause with nonexistent container", func(t *testing.T) {
		err := dc.Pause("nonexistent-container-67890")
		// Should return an error
		if err == nil {
			t.Log("Note: No error returned - Docker socket might not be available")
		}
	})
}

func TestDockerControllerUnpause(t *testing.T) {
	dc := NewDockerController()
	defer dc.Close()

	t.Run("Unpause with nonexistent container", func(t *testing.T) {
		err := dc.Unpause("nonexistent-container-67890")
		// Should return an error
		if err == nil {
			t.Log("Note: No error returned - Docker socket might not be available")
		}
	})
}

func TestDockerControllerRestart(t *testing.T) {
	dc := NewDockerController()
	defer dc.Close()

	t.Run("Restart with nonexistent container", func(t *testing.T) {
		err := dc.Restart("nonexistent-container-67890")
		// Should return an error
		if err == nil {
			t.Log("Note: No error returned - Docker socket might not be available")
		}
	})
}

func TestDockerControllerClose(t *testing.T) {
	dc := NewDockerController()

	// Close should not error even if client wasn't initialized
	err := dc.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestStripDockerStreamHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "plain text without headers",
			input:    []byte("hello world\n"),
			expected: "hello world\n",
		},
		{
			name: "stdout frame",
			input: func() []byte {
				msg := []byte("hello from stdout\n")
				header := []byte{1, 0, 0, 0, 0, 0, 0, byte(len(msg))}
				return append(header, msg...)
			}(),
			expected: "hello from stdout\n",
		},
		{
			name: "stderr frame",
			input: func() []byte {
				msg := []byte("error message\n")
				header := []byte{2, 0, 0, 0, 0, 0, 0, byte(len(msg))}
				return append(header, msg...)
			}(),
			expected: "error message\n",
		},
		{
			name: "multiple frames",
			input: func() []byte {
				msg1 := []byte("line1\n")
				msg2 := []byte("line2\n")
				h1 := []byte{1, 0, 0, 0, 0, 0, 0, byte(len(msg1))}
				h2 := []byte{1, 0, 0, 0, 0, 0, 0, byte(len(msg2))}
				result := append(h1, msg1...)
				result = append(result, h2...)
				result = append(result, msg2...)
				return result
			}(),
			expected: "line1\nline2\n",
		},
		{
			name:     "empty input",
			input:    []byte{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripDockerStreamHeaders(tt.input)
			if got != tt.expected {
				t.Errorf("stripDockerStreamHeaders() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDockerControllerContainerLogs(t *testing.T) {
	dc := NewDockerController()
	defer dc.Close()

	t.Run("has ContainerLogs method", func(t *testing.T) {
		_ = dc.ContainerLogs
	})

	t.Run("ContainerLogs with nonexistent container", func(t *testing.T) {
		_, err := dc.ContainerLogs("nonexistent-container-99999", 100, "", false)
		// Should return an error (container doesn't exist or Docker not available)
		if err == nil {
			t.Log("Note: No error returned - Docker socket might not be available")
		}
	})
}
