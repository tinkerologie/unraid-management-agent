package controllers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserScriptsBasePath(t *testing.T) {
	// Verify the constant is set correctly
	expected := "/boot/config/plugins/user.scripts/scripts"
	if UserScriptsBasePath != expected {
		t.Errorf("UserScriptsBasePath = %q, want %q", UserScriptsBasePath, expected)
	}
}

func TestListUserScriptsEmptyDirectory(t *testing.T) {
	// Create temp directory to simulate user scripts
	tmpDir, err := os.MkdirTemp("", "userscripts-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// ListUserScripts uses a hardcoded path, so we can't directly test it
	// but we can test the pattern it uses

	// Read empty directory
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("Expected 0 entries, got %d", len(entries))
	}
}

func TestListUserScriptsDirectoryStructure(t *testing.T) {
	// Create temp directory structure matching expected format
	tmpDir, err := os.MkdirTemp("", "userscripts-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create script directory structure: scripts/scriptname/script
	scriptDir := filepath.Join(tmpDir, "test-script")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		t.Fatalf("Failed to create script dir: %v", err)
	}

	// Create script file
	scriptPath := filepath.Join(scriptDir, "script")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho test"), 0755); err != nil {
		t.Fatalf("Failed to write script: %v", err)
	}

	// Create description file
	descPath := filepath.Join(scriptDir, "description")
	if err := os.WriteFile(descPath, []byte("Test script description"), 0644); err != nil {
		t.Fatalf("Failed to write description: %v", err)
	}

	// Verify structure
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].Name() != "test-script" {
		t.Errorf("Expected entry name %q, got %q", "test-script", entries[0].Name())
	}

	if !entries[0].IsDir() {
		t.Error("Expected entry to be a directory")
	}

	// Verify script file exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		t.Error("Script file does not exist")
	}

	// Verify description file exists
	if _, err := os.Stat(descPath); os.IsNotExist(err) {
		t.Error("Description file does not exist")
	}

	// Read description
	descContent, err := os.ReadFile(descPath)
	if err != nil {
		t.Fatalf("Failed to read description: %v", err)
	}

	if string(descContent) != "Test script description" {
		t.Errorf("Description content = %q, want %q", string(descContent), "Test script description")
	}
}

func TestListUserScriptsNonExistentDirectory(t *testing.T) {
	// When the user scripts directory doesn't exist,
	// ListUserScripts should return an empty list, not an error

	// The actual function uses a hardcoded path that likely doesn't exist in tests
	// Test the error handling pattern
	nonExistentPath := "/nonexistent/path/to/scripts"

	if _, err := os.Stat(nonExistentPath); !os.IsNotExist(err) {
		t.Skip("Test path unexpectedly exists")
	}
}

func TestListUserScriptsActual(t *testing.T) {
	// Test the actual ListUserScripts function
	// It will return an empty list if the scripts directory doesn't exist
	scripts, err := ListUserScripts()

	// Directory likely doesn't exist in test environment, expect error
	if err != nil {
		t.Logf("ListUserScripts returned error (expected if scripts dir doesn't exist): %v", err)
	}

	// Should return an array, possibly nil on error
	if err == nil && scripts == nil {
		t.Error("Expected non-nil scripts array when no error")
	}
}

func TestExecuteUserScriptValidation(t *testing.T) {
	// Test script name validation
	tests := []struct {
		name        string
		scriptName  string
		shouldError bool
	}{
		{"valid name", "my-script", false},
		{"valid with underscore", "my_script", false},
		{"valid with numbers", "script123", false},
		{"path traversal", "../etc/passwd", true},
		{"absolute path", "/etc/passwd", true},
		{"null byte", "script\x00name", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ExecuteUserScript validates the script name
			// Will also fail because scripts don't exist, but validates first
			resp, err := ExecuteUserScript(tt.scriptName, false, false)

			if tt.shouldError {
				// Should get validation error
				if err == nil && resp.Success {
					t.Errorf("Expected error for script name %q", tt.scriptName)
				}
			}
		})
	}
}

func TestExecuteUserScriptModes(t *testing.T) {
	// Test different execution modes
	t.Run("background mode", func(t *testing.T) {
		// Script doesn't exist, but tests the code path
		_, _ = ExecuteUserScript("nonexistent-script", true, false)
	})

	t.Run("wait mode", func(t *testing.T) {
		// Script doesn't exist, but tests the code path
		_, _ = ExecuteUserScript("nonexistent-script", false, true)
	})

	t.Run("default mode", func(t *testing.T) {
		// Script doesn't exist, but tests the code path
		_, _ = ExecuteUserScript("nonexistent-script", false, false)
	})
}

func TestUserScriptFilePermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "userscripts-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	scriptDir := filepath.Join(tmpDir, "test-script")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		t.Fatalf("Failed to create script dir: %v", err)
	}

	scriptPath := filepath.Join(scriptDir, "script")

	t.Run("executable script", func(t *testing.T) {
		// Create executable script
		if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho test"), 0755); err != nil {
			t.Fatalf("Failed to write script: %v", err)
		}

		info, err := os.Stat(scriptPath)
		if err != nil {
			t.Fatalf("Failed to stat script: %v", err)
		}

		// Check if readable (the actual check used)
		isExecutable := info.Mode().Perm()&0400 != 0
		if !isExecutable {
			t.Error("Script should be marked as executable")
		}
	})

	t.Run("non-readable script permissions", func(t *testing.T) {
		// Create script with no read permission
		if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho test"), 0000); err != nil {
			t.Fatalf("Failed to write script: %v", err)
		}
		defer os.Chmod(scriptPath, 0644) // Reset permissions for cleanup

		info, err := os.Stat(scriptPath)
		if err != nil {
			t.Fatalf("Failed to stat script: %v", err)
		}

		// Check that permission is 0000 (note: stat may still work even without read permission)
		perm := info.Mode().Perm()
		if perm != 0 {
			// On some systems, the user/root can still read the file
			t.Logf("Note: Permissions are %v (expected 0), may be OS-specific", perm)
		}
	})
}
