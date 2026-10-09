package collectors

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestPluginUpdateCollector_PublishesAndDedupes(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicPluginUpdatesUpdate.Name)
	defer hub.Unsub(sub)

	result := &dto.PluginList{
		Plugins: []dto.PluginInfo{
			{Name: "community.applications", Version: "2025.10.27", UpdateAvailable: true, LatestVersion: "2025.10.28"},
		},
		TotalCount: 1, UpdatesAvailable: 1,
	}

	c := NewPluginUpdateCollector(&domain.Context{Hub: hub})
	c.CheckFn = func(_ context.Context) (*dto.PluginList, error) { return result, nil }

	c.Collect(context.Background())
	select {
	case msg := <-sub:
		got, ok := msg.(*dto.PluginList)
		if !ok || got.UpdatesAvailable != 1 {
			t.Fatalf("unexpected first publish: %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("expected first publish, got none")
	}

	c.Collect(context.Background()) // identical → must NOT publish
	select {
	case msg := <-sub:
		t.Fatalf("expected no re-publish on unchanged result, got %#v", msg)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestPluginUpdateCollector_NilCheckFnIsSafe(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicPluginUpdatesUpdate.Name)
	defer hub.Unsub(sub)

	c := NewPluginUpdateCollector(&domain.Context{Hub: hub})
	c.Collect(context.Background()) // CheckFn nil → must not panic, must not publish

	select {
	case msg := <-sub:
		t.Fatalf("expected no publish when CheckFn is nil, got %#v", msg)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestPluginUpdateCollector_RepublishesOnChange(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicPluginUpdatesUpdate.Name)
	defer hub.Unsub(sub)

	c := NewPluginUpdateCollector(&domain.Context{Hub: hub})

	c.CheckFn = func(_ context.Context) (*dto.PluginList, error) {
		return &dto.PluginList{
			Plugins:    []dto.PluginInfo{{Name: "myplugin", Version: "1.0", UpdateAvailable: false}},
			TotalCount: 1,
		}, nil
	}
	c.Collect(context.Background())

	select {
	case <-sub: // drain first publish
	case <-time.After(time.Second):
		t.Fatal("expected first publish, got none")
	}

	// flip UpdateAvailable — signature changes, so a second publish must occur
	c.CheckFn = func(_ context.Context) (*dto.PluginList, error) {
		return &dto.PluginList{
			Plugins:          []dto.PluginInfo{{Name: "myplugin", Version: "1.0", UpdateAvailable: true, LatestVersion: "1.1"}},
			TotalCount:       1,
			UpdatesAvailable: 1,
		}, nil
	}
	c.Collect(context.Background())

	select {
	case <-sub: // success: changed signature triggered republish
	case <-time.After(time.Second):
		t.Fatal("expected republish after signature change, got none")
	}
}

func TestPluginUpdateCollector_CheckErrorNoPublish(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicPluginUpdatesUpdate.Name)
	defer hub.Unsub(sub)
	c := NewPluginUpdateCollector(&domain.Context{Hub: hub})
	c.CheckFn = func(_ context.Context) (*dto.PluginList, error) { return nil, fmt.Errorf("boom") }
	c.Collect(context.Background())
	select {
	case <-sub:
		t.Fatal("expected no publish on check error")
	case <-time.After(150 * time.Millisecond):
	}
}

func TestPluginUpdateNotify_FiresOnNewTransitionOnly(t *testing.T) {
	hub := domain.NewEventBus(16)
	var notified []string
	c := NewPluginUpdateCollector(&domain.Context{Hub: hub})
	c.NotifyFn = func(names []string) { notified = append(notified, names...) }

	step1 := &dto.PluginList{
		Plugins:    []dto.PluginInfo{{Name: "community.applications", Version: "1.0", UpdateAvailable: true, LatestVersion: "1.1"}},
		TotalCount: 1, UpdatesAvailable: 1,
	}
	c.CheckFn = func(_ context.Context) (*dto.PluginList, error) { return step1, nil }
	c.Collect(context.Background()) // baseline → no notify
	if len(notified) != 0 {
		t.Fatalf("first run should not notify, got %v", notified)
	}

	step2 := &dto.PluginList{
		Plugins: []dto.PluginInfo{
			{Name: "community.applications", Version: "1.0", UpdateAvailable: true, LatestVersion: "1.1"},
			{Name: "dynamix.system.stats", Version: "2.0", UpdateAvailable: true, LatestVersion: "2.1"},
		},
		TotalCount: 2, UpdatesAvailable: 2,
	}
	c.CheckFn = func(_ context.Context) (*dto.PluginList, error) { return step2, nil }
	c.Collect(context.Background()) // dynamix.system.stats newly available → notify only that one
	if len(notified) != 1 || notified[0] != "dynamix.system.stats" {
		t.Fatalf("expected notify [dynamix.system.stats], got %v", notified)
	}
}

func TestPluginUpdateCollector_CheckFnContextHasDeadline(t *testing.T) {
	hub := domain.NewEventBus(16)
	c := NewPluginUpdateCollector(&domain.Context{Hub: hub})

	var gotDeadline bool
	c.CheckFn = func(ctx context.Context) (*dto.PluginList, error) {
		_, gotDeadline = ctx.Deadline()
		return nil, nil
	}
	c.Collect(context.Background())
	if !gotDeadline {
		t.Fatal("CheckFn context must carry a deadline so the check command fails fast")
	}
}

func TestPluginUpdateNotify_NotifyFnNilIsSafe(t *testing.T) {
	hub := domain.NewEventBus(16)
	c := NewPluginUpdateCollector(&domain.Context{Hub: hub}) // NotifyFn nil
	c.CheckFn = func(_ context.Context) (*dto.PluginList, error) {
		return &dto.PluginList{
			Plugins:          []dto.PluginInfo{{Name: "myplugin", Version: "1.0", UpdateAvailable: true, LatestVersion: "1.1"}},
			TotalCount:       1,
			UpdatesAvailable: 1,
		}, nil
	}
	// Must not panic
	c.Collect(context.Background())
	c.Collect(context.Background())
}

func TestPluginUpdateCollector_PublishesFirstEmptyResult(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicPluginUpdatesUpdate.Name)
	defer hub.Unsub(sub)

	c := NewPluginUpdateCollector(&domain.Context{Hub: hub})
	c.CheckFn = func(_ context.Context) (*dto.PluginList, error) {
		return &dto.PluginList{}, nil // no plugin has an update
	}

	// An empty result has the same signature as the initial state; it must
	// still be published so the cache records that the check ran.
	c.Collect(context.Background())
	select {
	case msg := <-sub:
		got, ok := msg.(*dto.PluginList)
		if !ok {
			t.Fatalf("unexpected publish payload: %#v", msg)
		}
		if got.Timestamp.IsZero() {
			t.Error("expected published result to carry the check time")
		}
	case <-time.After(time.Second):
		t.Fatal("expected the first (empty) result to be published")
	}

	c.Collect(context.Background()) // unchanged → no re-publish
	select {
	case msg := <-sub:
		t.Fatalf("expected no re-publish on unchanged empty result, got %#v", msg)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestPluginUpdateCollector_KeepsProvidedTimestamp(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicPluginUpdatesUpdate.Name)
	defer hub.Unsub(sub)

	checked := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	c := NewPluginUpdateCollector(&domain.Context{Hub: hub})
	c.CheckFn = func(_ context.Context) (*dto.PluginList, error) {
		return &dto.PluginList{Timestamp: checked}, nil
	}

	c.Collect(context.Background())
	select {
	case msg := <-sub:
		if got := msg.(*dto.PluginList); !got.Timestamp.Equal(checked) {
			t.Errorf("expected timestamp %v to be kept, got %v", checked, got.Timestamp)
		}
	case <-time.After(time.Second):
		t.Fatal("expected a publish")
	}
}
