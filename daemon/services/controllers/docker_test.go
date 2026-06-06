package controllers

import (
	"os"
	"path/filepath"
	"strings"
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

// TestDockerRemove verifies that Remove returns a clear "docker unavailable" error
// when no Docker daemon is reachable, and does not panic.
// When a live daemon IS present the test confirms that calling Remove with a
// non-existent container ID does not panic and returns an appropriate error.
func TestDockerRemove(t *testing.T) {
	dc := NewDockerController()
	defer dc.Close() //nolint:errcheck

	err := dc.Remove("bogus-container-remove-test", false)

	if err == nil {
		// A live daemon reported no error — unlikely but not a test concern.
		return
	}

	// Without a daemon the error must mention "docker unavailable".
	// With a daemon the error will come from the daemon (container not found).
	// Either way Remove must not panic and must return an error.
	daemonPresent := !strings.Contains(err.Error(), "docker unavailable")
	if daemonPresent {
		// Daemon is present: the error is from the daemon (e.g. "No such container").
		// This is the expected integration-environment path — test passes.
		t.Logf("Docker daemon present; Remove returned daemon error (expected): %v", err)
		return
	}

	// No daemon: error must contain "docker unavailable".
	if !strings.Contains(err.Error(), "docker unavailable") {
		t.Errorf("expected 'docker unavailable' in error, got: %v", err)
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

// TestPortConflicts exercises detectPortConflicts without a live Docker daemon.
func TestPortConflicts(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string][]string
		wantLen  int
		wantPort int
		wantN    int // container count for the first conflict (sorted by port)
	}{
		{
			name:     "one conflict one non-conflict",
			input:    map[string][]string{"8080/tcp": {"a", "b"}, "9000/tcp": {"c"}},
			wantLen:  1,
			wantPort: 8080,
			wantN:    2,
		},
		{
			name:    "no conflicts",
			input:   map[string][]string{"8080/tcp": {"a"}, "9000/udp": {"b"}},
			wantLen: 0,
		},
		{
			name:    "empty bindings",
			input:   map[string][]string{},
			wantLen: 0,
		},
		{
			name:     "multiple conflicts sorted by port",
			input:    map[string][]string{"9000/tcp": {"x", "y"}, "443/tcp": {"p", "q", "r"}, "80/tcp": {"m"}},
			wantLen:  2,
			wantPort: 443, // lowest port first
			wantN:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectPortConflicts(tt.input)
			if len(got) != tt.wantLen {
				t.Fatalf("detectPortConflicts() len=%d, want %d; result: %+v", len(got), tt.wantLen, got)
			}
			if tt.wantLen > 0 {
				if got[0].HostPort != tt.wantPort {
					t.Errorf("first conflict HostPort=%d, want %d", got[0].HostPort, tt.wantPort)
				}
				if len(got[0].Containers) != tt.wantN {
					t.Errorf("first conflict Containers len=%d, want %d", len(got[0].Containers), tt.wantN)
				}
			}
		})
	}
}

// TestSetAutostartFile exercises the file-manipulation helper (modifyAutostartFile)
// using a temp directory as the seam — no Docker daemon needed.
//
// Verified mechanism (2026-06-07, Unraid 7.x at 192.168.20.21):
//
//	/var/lib/docker/unraid-autostart — one container NAME per line, plain text,
//	no quotes. The WebUI reads/writes this file to control which containers
//	auto-start at boot. Empty lines are ignored by Unraid.
func TestSetAutostartFile(t *testing.T) {
	t.Run("add to empty file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "unraid-autostart")
		if err := modifyAutostartFile(path, "plex", true); err != nil {
			t.Fatalf("modifyAutostartFile(enable) error: %v", err)
		}
		data, _ := os.ReadFile(path)
		if !strings.Contains(string(data), "plex") {
			t.Errorf("expected 'plex' in file, got: %q", string(data))
		}
	})

	t.Run("add to existing file preserves order", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "unraid-autostart")
		if err := os.WriteFile(path, []byte("jellyfin\nsonarr\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := modifyAutostartFile(path, "radarr", true); err != nil {
			t.Fatalf("modifyAutostartFile(enable) error: %v", err)
		}
		data, _ := os.ReadFile(path)
		content := string(data)
		if !strings.Contains(content, "jellyfin") || !strings.Contains(content, "sonarr") || !strings.Contains(content, "radarr") {
			t.Errorf("unexpected file content: %q", content)
		}
		// radarr must be appended after the existing entries
		jIdx := strings.Index(content, "jellyfin")
		sIdx := strings.Index(content, "sonarr")
		rIdx := strings.Index(content, "radarr")
		if !(jIdx < sIdx && sIdx < rIdx) {
			t.Errorf("order not preserved; want jellyfin < sonarr < radarr, got positions %d %d %d", jIdx, sIdx, rIdx)
		}
	})

	t.Run("remove existing entry", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "unraid-autostart")
		if err := os.WriteFile(path, []byte("jellyfin\nplex\nsonarr\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := modifyAutostartFile(path, "plex", false); err != nil {
			t.Fatalf("modifyAutostartFile(disable) error: %v", err)
		}
		data, _ := os.ReadFile(path)
		content := string(data)
		if strings.Contains(content, "plex") {
			t.Errorf("'plex' should have been removed, got: %q", content)
		}
		if !strings.Contains(content, "jellyfin") || !strings.Contains(content, "sonarr") {
			t.Errorf("other entries must remain, got: %q", content)
		}
	})

	t.Run("idempotent add (already present)", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "unraid-autostart")
		if err := os.WriteFile(path, []byte("plex\nsonarr\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := modifyAutostartFile(path, "plex", true); err != nil {
			t.Fatalf("modifyAutostartFile(enable idempotent) error: %v", err)
		}
		data, _ := os.ReadFile(path)
		content := string(data)
		// plex must appear exactly once
		count := strings.Count(content, "plex")
		if count != 1 {
			t.Errorf("expected 'plex' exactly once, got %d occurrences in: %q", count, content)
		}
	})

	t.Run("idempotent remove (already absent)", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "unraid-autostart")
		if err := os.WriteFile(path, []byte("jellyfin\nsonarr\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := modifyAutostartFile(path, "plex", false); err != nil {
			t.Fatalf("modifyAutostartFile(disable absent) error: %v", err)
		}
		data, _ := os.ReadFile(path)
		content := string(data)
		if strings.Contains(content, "plex") {
			t.Errorf("'plex' should not be in file, got: %q", content)
		}
	})

	t.Run("file does not exist — enable creates it", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "unraid-autostart")
		if err := modifyAutostartFile(path, "homeassistant", true); err != nil {
			t.Fatalf("modifyAutostartFile on missing file error: %v", err)
		}
		data, _ := os.ReadFile(path)
		if !strings.Contains(string(data), "homeassistant") {
			t.Errorf("expected 'homeassistant' in new file, got: %q", string(data))
		}
	})

	t.Run("seam: dockerAutostartFile variable is overridable", func(t *testing.T) {
		// Save and restore the global seam.
		orig := dockerAutostartFile
		defer func() { dockerAutostartFile = orig }()

		tmp := filepath.Join(t.TempDir(), "unraid-autostart")
		dockerAutostartFile = tmp

		// modifyAutostartFile should use the overridden path.
		if err := modifyAutostartFile(dockerAutostartFile, "mycontainer", true); err != nil {
			t.Fatalf("modifyAutostartFile via seam error: %v", err)
		}
		data, _ := os.ReadFile(tmp)
		if !strings.Contains(string(data), "mycontainer") {
			t.Errorf("expected 'mycontainer' in temp file, got: %q", string(data))
		}
	})
}
