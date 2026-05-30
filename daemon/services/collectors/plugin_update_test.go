package collectors

import (
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
	c.CheckFn = func() (*dto.PluginList, error) { return result, nil }

	c.Collect()
	select {
	case msg := <-sub:
		got, ok := msg.(*dto.PluginList)
		if !ok || got.UpdatesAvailable != 1 {
			t.Fatalf("unexpected first publish: %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("expected first publish, got none")
	}

	c.Collect() // identical → must NOT publish
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
	c.Collect() // CheckFn nil → must not panic, must not publish

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

	c.CheckFn = func() (*dto.PluginList, error) {
		return &dto.PluginList{
			Plugins:    []dto.PluginInfo{{Name: "myplugin", Version: "1.0", UpdateAvailable: false}},
			TotalCount: 1,
		}, nil
	}
	c.Collect()

	select {
	case <-sub: // drain first publish
	case <-time.After(time.Second):
		t.Fatal("expected first publish, got none")
	}

	// flip UpdateAvailable — signature changes, so a second publish must occur
	c.CheckFn = func() (*dto.PluginList, error) {
		return &dto.PluginList{
			Plugins:          []dto.PluginInfo{{Name: "myplugin", Version: "1.0", UpdateAvailable: true, LatestVersion: "1.1"}},
			TotalCount:       1,
			UpdatesAvailable: 1,
		}, nil
	}
	c.Collect()

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
	c.CheckFn = func() (*dto.PluginList, error) { return nil, fmt.Errorf("boom") }
	c.Collect()
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
	c.CheckFn = func() (*dto.PluginList, error) { return step1, nil }
	c.Collect() // baseline → no notify
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
	c.CheckFn = func() (*dto.PluginList, error) { return step2, nil }
	c.Collect() // dynamix.system.stats newly available → notify only that one
	if len(notified) != 1 || notified[0] != "dynamix.system.stats" {
		t.Fatalf("expected notify [dynamix.system.stats], got %v", notified)
	}
}

func TestPluginUpdateNotify_NotifyFnNilIsSafe(t *testing.T) {
	hub := domain.NewEventBus(16)
	c := NewPluginUpdateCollector(&domain.Context{Hub: hub}) // NotifyFn nil
	c.CheckFn = func() (*dto.PluginList, error) {
		return &dto.PluginList{
			Plugins:          []dto.PluginInfo{{Name: "myplugin", Version: "1.0", UpdateAvailable: true, LatestVersion: "1.1"}},
			TotalCount:       1,
			UpdatesAvailable: 1,
		}, nil
	}
	// Must not panic
	c.Collect()
	c.Collect()
}
