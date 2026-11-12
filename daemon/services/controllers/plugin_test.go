package controllers

import (
	"context"
	"errors"
	"slices"
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
		return "", nil
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

func TestPluginControllerReportsInventoryErrorsAndOnlyInstallableUpdates(t *testing.T) {
	pc := NewPluginController()
	offline := errors.New("fixture inventory unavailable")
	pc.targets = func(context.Context) ([]string, error) { return nil, offline }
	if _, err := pc.CheckPluginUpdates(t.Context()); !errors.Is(err, offline) {
		t.Fatal("expected registration error:", err)
	}
	pc.targets = func(context.Context) ([]string, error) { return nil, nil }
	pc.resolve = func(context.Context) (*dto.PluginList, error) { return nil, offline }
	if _, err := pc.CheckPluginUpdates(t.Context()); !errors.Is(err, offline) {
		t.Fatal("expected resolution error:", err)
	}
	pc.resolve = func(context.Context) (*dto.PluginList, error) {
		return &dto.PluginList{UpdatesAvailable: 1, Plugins: []dto.PluginInfo{
			{Name: "installable", UpdateAvailable: true, UpdateStatus: "update_available"},
			{Name: "incompatible", UpdateStatus: "update_incompatible"},
		}}, nil
	}
	got, err := pc.CheckPluginUpdates(t.Context())
	if err != nil || len(got) != 1 || got[0].Name != "installable" {
		t.Fatalf("unexpected installable updates: %+v, %v", got, err)
	}
}

func TestPluginControllerDoesNotPublishCachedUpdatesAfterTotalRefreshFailure(t *testing.T) {
	original := pluginCheckExec
	t.Cleanup(func() { pluginCheckExec = original })
	offline := errors.New("fixture mirror unavailable")
	var targets []string
	pluginCheckExec = func(_ context.Context, _ string, args ...string) (string, error) {
		targets = append(targets, args[1])
		return "", offline
	}
	pc := NewPluginController()
	pc.targets = func(context.Context) ([]string, error) {
		return []string{"first.plg", "second.plg"}, nil
	}
	pc.resolve = func(context.Context) (*dto.PluginList, error) {
		t.Fatal("a total refresh failure must not publish cached results")
		return nil, nil
	}
	if _, err := pc.CheckPluginUpdates(t.Context()); !errors.Is(err, offline) {
		t.Fatalf("refresh did not report failure: %v", err)
	}
	if !slices.Equal(targets, []string{"first.plg", "second.plg"}) {
		t.Fatalf("unexpected native check order: %v", targets)
	}
}

func TestPluginControllerAllowsResultsAfterPartialRefreshFailure(t *testing.T) {
	original := pluginCheckExec
	t.Cleanup(func() { pluginCheckExec = original })
	pluginCheckExec = func(_ context.Context, _ string, args ...string) (string, error) {
		if args[1] == "offline.plg" {
			return "", errors.New("fixture mirror unavailable")
		}
		return "", nil
	}
	pc := NewPluginController()
	pc.targets = func(context.Context) ([]string, error) {
		return []string{"offline.plg", "validated.plg"}, nil
	}
	pc.resolve = func(context.Context) (*dto.PluginList, error) {
		return &dto.PluginList{Plugins: []dto.PluginInfo{
			{Name: "validated", UpdateStatus: "validation_failed"},
		}}, nil
	}
	if updates, err := pc.CheckPluginUpdates(t.Context()); err != nil || len(updates) != 0 {
		t.Fatalf("unexpected partial refresh result: %v, %v", updates, err)
	}
}
