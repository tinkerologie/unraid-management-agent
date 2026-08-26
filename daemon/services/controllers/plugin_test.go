package controllers

import (
	"errors"
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
