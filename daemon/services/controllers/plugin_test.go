package controllers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
)

func TestNewPluginController(t *testing.T) {
	pc := NewPluginController()
	if pc == nil {
		t.Fatal("NewPluginController returned nil")
	}
	if pc.execOutput == nil {
		t.Error("Expected default execOutput to be set")
	}
}

func TestNewPluginControllerWithExec(t *testing.T) {
	called := false
	mockExec := func(_ string, _ ...string) (string, error) {
		called = true
		return "updated", nil
	}

	pc := NewPluginControllerWithExec(mockExec)
	if pc == nil {
		t.Fatal("Expected non-nil controller")
	}

	err := pc.UpdatePlugin("test-plugin")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !called {
		t.Error("Expected mockExec to be called")
	}
}

func TestPluginController_SetExec(t *testing.T) {
	pc := NewPluginController()
	called := false
	mockExec := func(_ string, _ ...string) (string, error) {
		called = true
		return "updated", nil
	}

	pc.SetExec(mockExec)
	err := pc.UpdatePlugin("test-plugin")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !called {
		t.Error("Expected mockExec to be called")
	}
}

func TestPluginController_UpdatePlugin_CommandArgs(t *testing.T) {
	tests := []struct {
		name         string
		pluginInput  string
		expectedFile string
	}{
		{
			name:         "bare plugin name without extension",
			pluginInput:  "appdata.cleanup.plus",
			expectedFile: "appdata.cleanup.plus.plg",
		},
		{
			name:         "plugin name already with .plg extension",
			pluginInput:  "test-plugin.plg",
			expectedFile: "test-plugin.plg",
		},
		{
			name:         "bare plugin name with dots",
			pluginInput:  "dynamix.system.stats",
			expectedFile: "dynamix.system.stats.plg",
		},
		{
			name:         "plugin name with absolute path and extension",
			pluginInput:  "/boot/config/plugins/test-plugin.plg",
			expectedFile: "test-plugin.plg",
		},
		{
			name:         "plugin name with relative directory path",
			pluginInput:  "plugins/nested/appdata.cleanup.plus",
			expectedFile: "appdata.cleanup.plus.plg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recordedCmd string
			var recordedArgs []string

			mockExec := func(cmd string, args ...string) (string, error) {
				recordedCmd = cmd
				recordedArgs = args
				return "plugin: updated", nil
			}

			pc := NewPluginControllerWithExec(mockExec)
			err := pc.UpdatePlugin(tt.pluginInput)
			if err != nil {
				t.Fatalf("UpdatePlugin(%q) unexpected error: %v", tt.pluginInput, err)
			}

			if recordedCmd != constants.PluginBin {
				t.Errorf("expected command %q, got %q", constants.PluginBin, recordedCmd)
			}

			if len(recordedArgs) != 2 {
				t.Fatalf("expected 2 args, got %d: %v", len(recordedArgs), recordedArgs)
			}

			if recordedArgs[0] != "update" {
				t.Errorf("expected arg[0] to be %q, got %q", "update", recordedArgs[0])
			}

			argFile := recordedArgs[1]
			if argFile != tt.expectedFile {
				t.Errorf("expected arg[1] to be %q, got %q", tt.expectedFile, argFile)
			}

			if strings.Contains(argFile, "/") || strings.Contains(argFile, "\\") {
				t.Errorf("arg[1] %q contains path separators", argFile)
			}

			if !strings.HasSuffix(argFile, ".plg") {
				t.Errorf("arg[1] %q does not end with .plg", argFile)
			}

			if strings.HasSuffix(argFile, ".plg.plg") {
				t.Errorf("arg[1] %q has duplicate .plg suffix", argFile)
			}
		})
	}
}

func TestPluginController_UpdatePlugin_Error(t *testing.T) {
	mockExec := func(_ string, _ ...string) (string, error) {
		return "plugin: download failed", errors.New("exit status 1")
	}

	pc := NewPluginControllerWithExec(mockExec)
	err := pc.UpdatePlugin("my-plugin")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "my-plugin") {
		t.Errorf("expected error to mention plugin name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "download failed") {
		t.Errorf("expected error to include command output, got: %v", err)
	}
}

func TestPluginController_CheckPluginUpdates_RunsCheckall(t *testing.T) {
	original := pluginCheckExec
	t.Cleanup(func() { pluginCheckExec = original })

	var gotCmd string
	var gotArgs []string
	var gotDeadline bool
	pluginCheckExec = func(ctx context.Context, cmd string, args ...string) (string, error) {
		gotCmd = cmd
		gotArgs = args
		_, gotDeadline = ctx.Deadline()
		return "", errors.New("exit status 1")
	}

	pc := NewPluginController()
	if _, err := pc.CheckPluginUpdates(context.Background()); err != nil {
		t.Fatalf("CheckPluginUpdates returned error: %v", err)
	}

	if gotCmd != constants.PluginBin {
		t.Errorf("expected command %q, got %q", constants.PluginBin, gotCmd)
	}
	// "plugin check" without a plugin file only prints usage; "checkall"
	// downloads update metadata for every installed plugin.
	if len(gotArgs) != 1 || gotArgs[0] != "checkall" {
		t.Errorf("expected args [checkall], got %v", gotArgs)
	}
	if !gotDeadline {
		t.Error("expected the check command to run with a deadline")
	}
}

// writePluginFiles creates .plg files (and a dangling symlink) in a temporary
// plugin directory and points pluginsConfigDir at it for the test.
func writePluginFiles(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		// File name matches the requested name; its entity name differs.
		"exact.plg": "<!DOCTYPE PLUGIN [\n<!ENTITY name \"something-else\">\n]>\n",
		// Installed under a file name that differs from its entity name.
		"disklocation-master.plg": "<?xml version='1.0'?>\n<!DOCTYPE PLUGIN [\n" +
			"<!ENTITY name      \"disklocation\">\n<!ENTITY branch \"master\">\n]>\n",
		// Name entity only after the DOCTYPE section: must not be matched.
		"late.plg": "<!DOCTYPE PLUGIN [\n<!ENTITY version \"1.0\">\n]>\n" +
			"<!ENTITY name \"late-name\">\n",
		// No DOCTYPE terminator and no name entity.
		"attributes.plg": "<PLUGIN name=\"attributes-name\" version=\"1.0\">\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "missing-target"), filepath.Join(dir, "broken.plg")); err != nil {
		t.Fatal(err)
	}

	original := pluginsConfigDir
	pluginsConfigDir = dir
	t.Cleanup(func() { pluginsConfigDir = original })
}

func TestResolvePluginFile(t *testing.T) {
	writePluginFiles(t)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"file name match wins", "exact", "exact.plg"},
		{"name entity differs from file name", "disklocation", "disklocation-master.plg"},
		{"name entity after DOCTYPE is ignored", "late-name", "late-name.plg"},
		{"PLUGIN attributes are not entities", "attributes-name", "attributes-name.plg"},
		{"unknown plugin falls back to its name", "not-installed", "not-installed.plg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolvePluginFile(tt.input); got != tt.expected {
				t.Errorf("resolvePluginFile(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestPluginController_UpdatePlugin_UsesInstalledFileName(t *testing.T) {
	writePluginFiles(t)

	var recordedArgs []string
	pc := NewPluginControllerWithExec(func(_ string, args ...string) (string, error) {
		recordedArgs = args
		return "plugin: updated", nil
	})

	if err := pc.UpdatePlugin("disklocation"); err != nil {
		t.Fatalf("UpdatePlugin returned error: %v", err)
	}
	if len(recordedArgs) != 2 || recordedArgs[1] != "disklocation-master.plg" {
		t.Errorf("expected update of disklocation-master.plg, got args %v", recordedArgs)
	}
}
