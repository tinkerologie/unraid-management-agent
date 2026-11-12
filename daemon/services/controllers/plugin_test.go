package controllers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestNewPluginController(t *testing.T) {
	if NewPluginController() == nil {
		t.Fatal("NewPluginController returned nil")
	}
}

func TestPluginControllerChecksOnlyRegisteredTargets(t *testing.T) {
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
	pc.targets = func(context.Context) ([]string, error) {
		return []string{"fixture.plg", "--invalid.plg", "invalid;target.plg"}, nil
	}
	if _, err := pc.CheckPluginUpdates(t.Context()); err != nil {
		t.Fatalf("CheckPluginUpdates returned error: %v", err)
	}

	if gotCmd != constants.PluginBin {
		t.Errorf("expected command %q, got %q", constants.PluginBin, gotCmd)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "check" || gotArgs[1] != "fixture.plg" {
		t.Errorf("expected args [check fixture.plg], got %v", gotArgs)
	}
	if !gotDeadline {
		t.Error("expected the check command to run with a deadline")
	}
}

func TestPluginControllerCancellationReleasesSlotWithoutResolvingStaleData(t *testing.T) {
	original := pluginCheckExec
	t.Cleanup(func() { pluginCheckExec = original })

	started := make(chan struct{})
	pluginCheckExec = func(ctx context.Context, _ string, _ ...string) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pc := NewPluginController()
	pc.targets = func(context.Context) ([]string, error) { return []string{"fixture.plg"}, nil }
	pc.resolve = func(context.Context) (*dto.PluginList, error) {
		return nil, errors.New("canceled refresh must not resolve cached data")
	}

	done := make(chan error, 1)
	go func() {
		_, err := pc.CheckPluginUpdates(ctx)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}

	waiter, cancelWaiter := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancelWaiter()
	if _, err := pc.CheckPluginUpdates(waiter); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("parallel refresh returned %v", err)
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled refresh returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh did not stop after cancellation")
	}
	if len(pluginCheckSlot) != 0 {
		t.Fatal("canceled refresh retained the check slot")
	}
}
