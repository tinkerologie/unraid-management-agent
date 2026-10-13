package controllers

import (
	"context"
	"errors"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
)

func TestNewPluginController(t *testing.T) {
	if NewPluginController() == nil {
		t.Fatal("NewPluginController returned nil")
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
