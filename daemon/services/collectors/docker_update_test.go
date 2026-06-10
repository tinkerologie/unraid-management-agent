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
	c.CheckFn = func(_ context.Context) (*dto.ContainerUpdatesResult, error) { return result, nil }

	c.Collect(context.Background())
	select {
	case msg := <-sub:
		got, ok := msg.(*dto.ContainerUpdatesResult)
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

func TestDockerUpdateCollector_NilCheckFnIsSafe(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicDockerUpdatesUpdate.Name)
	defer hub.Unsub(sub)

	c := NewDockerUpdateCollector(&domain.Context{Hub: hub})
	c.Collect(context.Background()) // CheckFn nil → must not panic, must not publish

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

	c.CheckFn = func(_ context.Context) (*dto.ContainerUpdatesResult, error) {
		return &dto.ContainerUpdatesResult{
			Containers: []dto.ContainerUpdateInfo{{ContainerID: "a", UpdateAvailable: false}},
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
	c.CheckFn = func(_ context.Context) (*dto.ContainerUpdatesResult, error) {
		return &dto.ContainerUpdatesResult{
			Containers:       []dto.ContainerUpdateInfo{{ContainerID: "a", UpdateAvailable: true}},
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

func TestDockerUpdateCollector_CheckErrorNoPublish(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicDockerUpdatesUpdate.Name)
	defer hub.Unsub(sub)
	c := NewDockerUpdateCollector(&domain.Context{Hub: hub})
	c.CheckFn = func(_ context.Context) (*dto.ContainerUpdatesResult, error) { return nil, fmt.Errorf("boom") }
	c.Collect(context.Background())
	select {
	case <-sub:
		t.Fatal("expected no publish on check error")
	case <-time.After(150 * time.Millisecond):
	}
}

func TestDockerUpdateNotify_FiresOnNewTransitionOnly(t *testing.T) {
	hub := domain.NewEventBus(16)
	var notified []string
	c := NewDockerUpdateCollector(&domain.Context{Hub: hub, DockerUpdateNotify: true})
	c.NotifyFn = func(names []string) { notified = append(notified, names...) }

	step1 := &dto.ContainerUpdatesResult{
		Containers: []dto.ContainerUpdateInfo{{ContainerID: "a", ContainerName: "plex", LatestDigest: "x", UpdateAvailable: true}},
		TotalCount: 1, UpdatesAvailable: 1,
	}
	c.CheckFn = func(_ context.Context) (*dto.ContainerUpdatesResult, error) { return step1, nil }
	c.Collect(context.Background()) // baseline → no notify
	if len(notified) != 0 {
		t.Fatalf("first run should not notify, got %v", notified)
	}

	step2 := &dto.ContainerUpdatesResult{
		Containers: []dto.ContainerUpdateInfo{
			{ContainerID: "a", ContainerName: "plex", LatestDigest: "x", UpdateAvailable: true},
			{ContainerID: "b", ContainerName: "sonarr", LatestDigest: "y", UpdateAvailable: true},
		},
		TotalCount: 2, UpdatesAvailable: 2,
	}
	c.CheckFn = func(_ context.Context) (*dto.ContainerUpdatesResult, error) { return step2, nil }
	c.Collect(context.Background()) // sonarr newly available → notify only sonarr
	if len(notified) != 1 || notified[0] != "sonarr" {
		t.Fatalf("expected notify [sonarr], got %v", notified)
	}
}

func TestDockerUpdateCollector_CheckFnContextHasDeadline(t *testing.T) {
	hub := domain.NewEventBus(16)
	c := NewDockerUpdateCollector(&domain.Context{Hub: hub})

	var gotDeadline bool
	c.CheckFn = func(ctx context.Context) (*dto.ContainerUpdatesResult, error) {
		_, gotDeadline = ctx.Deadline()
		return nil, nil
	}
	c.Collect(context.Background())
	if !gotDeadline {
		t.Fatal("CheckFn context must carry a deadline so registry checks fail fast")
	}
}

func TestDockerUpdateCollector_CheckFnContextCancelledOnShutdown(t *testing.T) {
	hub := domain.NewEventBus(16)
	c := NewDockerUpdateCollector(&domain.Context{Hub: hub})

	runCtx, cancel := context.WithCancel(context.Background())
	cancel() // simulate collector shutdown before the check runs

	var checkErr error
	c.CheckFn = func(ctx context.Context) (*dto.ContainerUpdatesResult, error) {
		checkErr = ctx.Err()
		return nil, ctx.Err()
	}
	c.Collect(runCtx)
	if checkErr == nil {
		t.Fatal("CheckFn context must be cancelled when the lifecycle context is cancelled")
	}
}

func TestDockerUpdateNotify_DisabledByDefault(t *testing.T) {
	hub := domain.NewEventBus(16)
	called := false
	c := NewDockerUpdateCollector(&domain.Context{Hub: hub}) // DockerUpdateNotify false
	c.NotifyFn = func(names []string) { called = true }
	c.CheckFn = func(_ context.Context) (*dto.ContainerUpdatesResult, error) {
		return &dto.ContainerUpdatesResult{Containers: []dto.ContainerUpdateInfo{{ContainerID: "a", ContainerName: "plex", LatestDigest: "x", UpdateAvailable: true}}, TotalCount: 1, UpdatesAvailable: 1}, nil
	}
	c.Collect(context.Background())
	c.Collect(context.Background())
	if called {
		t.Fatal("notify must not fire when DockerUpdateNotify is false")
	}
}
