package collectors

import (
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestDockerUpdateCollector_PublishesAndDedupes(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicDockerUpdatesUpdate.Name)
	defer hub.Unsub(sub)

	result := &dto.ContainerUpdatesResult{
		Containers: []dto.ContainerUpdateInfo{
			{ContainerID: "abc123", ContainerName: "plex", LatestDigest: "sha256:b", CurrentDigest: "sha256:a", UpdateAvailable: true},
		},
		TotalCount: 1, UpdatesAvailable: 1,
	}

	c := NewDockerUpdateCollector(&domain.Context{Hub: hub})
	c.CheckFn = func() (*dto.ContainerUpdatesResult, error) { return result, nil }

	c.Collect()
	select {
	case msg := <-sub:
		got, ok := msg.(*dto.ContainerUpdatesResult)
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

func TestDockerUpdateCollector_NilCheckFnIsSafe(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicDockerUpdatesUpdate.Name)
	defer hub.Unsub(sub)

	c := NewDockerUpdateCollector(&domain.Context{Hub: hub})
	c.Collect() // CheckFn nil → must not panic, must not publish

	select {
	case msg := <-sub:
		t.Fatalf("expected no publish when CheckFn is nil, got %#v", msg)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestDockerUpdateCollector_RepublishesOnChange(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicDockerUpdatesUpdate.Name)
	defer hub.Unsub(sub)

	c := NewDockerUpdateCollector(&domain.Context{Hub: hub})

	c.CheckFn = func() (*dto.ContainerUpdatesResult, error) {
		return &dto.ContainerUpdatesResult{
			Containers: []dto.ContainerUpdateInfo{{ContainerID: "a", UpdateAvailable: false}},
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
	c.CheckFn = func() (*dto.ContainerUpdatesResult, error) {
		return &dto.ContainerUpdatesResult{
			Containers:       []dto.ContainerUpdateInfo{{ContainerID: "a", UpdateAvailable: true}},
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
