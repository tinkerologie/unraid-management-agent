package collectors

import (
	"fmt"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func fakeNetworks() []dto.DockerNetworkInfo {
	return []dto.DockerNetworkInfo{
		{ID: "net-aaa", Name: "bridge", Driver: "bridge", Scope: "local", ContainerNames: []string{}},
		{ID: "net-bbb", Name: "host", Driver: "host", Scope: "host", ContainerNames: []string{}},
	}
}

func TestDockerNetworksCollector_PublishesAndDedupes(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicDockerNetworksUpdate.Name)
	defer hub.Unsub(sub)

	networks := fakeNetworks()

	c := NewDockerNetworksCollector(&domain.Context{Hub: hub})
	c.ListFn = func() ([]dto.DockerNetworkInfo, error) { return networks, nil }

	// First Collect → must publish with Count 2.
	c.Collect()
	select {
	case msg := <-sub:
		got, ok := msg.(*dto.DockerNetworkList)
		if !ok {
			t.Fatalf("expected *dto.DockerNetworkList, got %T", msg)
		}
		if got.Count != 2 {
			t.Fatalf("Count = %d, want 2", got.Count)
		}
		if len(got.Networks) != 2 {
			t.Fatalf("len(Networks) = %d, want 2", len(got.Networks))
		}
	case <-time.After(time.Second):
		t.Fatal("expected first publish, got none")
	}

	// Identical second Collect → must NOT publish (dedupe).
	c.Collect()
	select {
	case msg := <-sub:
		t.Fatalf("expected no re-publish on unchanged result, got %#v", msg)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestDockerNetworksCollector_NilListFnIsSafe(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicDockerNetworksUpdate.Name)
	defer hub.Unsub(sub)

	c := NewDockerNetworksCollector(&domain.Context{Hub: hub})
	c.Collect() // ListFn nil → must not panic, must not publish

	select {
	case msg := <-sub:
		t.Fatalf("expected no publish when ListFn is nil, got %#v", msg)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestDockerNetworksCollector_RepublishesOnChange(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicDockerNetworksUpdate.Name)
	defer hub.Unsub(sub)

	c := NewDockerNetworksCollector(&domain.Context{Hub: hub})
	c.ListFn = func() ([]dto.DockerNetworkInfo, error) { return fakeNetworks(), nil }

	c.Collect()
	select {
	case <-sub: // drain first publish
	case <-time.After(time.Second):
		t.Fatal("expected first publish, got none")
	}

	// Change the network list — a third network is added.
	c.ListFn = func() ([]dto.DockerNetworkInfo, error) {
		nets := fakeNetworks()
		nets = append(nets, dto.DockerNetworkInfo{
			ID: "net-ccc", Name: "custom", Driver: "bridge", Scope: "local", ContainerNames: []string{},
		})
		return nets, nil
	}
	c.Collect()

	select {
	case msg := <-sub:
		got, ok := msg.(*dto.DockerNetworkList)
		if !ok {
			t.Fatalf("expected *dto.DockerNetworkList, got %T", msg)
		}
		if got.Count != 3 {
			t.Fatalf("Count = %d, want 3", got.Count)
		}
	case <-time.After(time.Second):
		t.Fatal("expected republish after network list change, got none")
	}
}

func TestDockerNetworksCollector_ListErrorNoPublish(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicDockerNetworksUpdate.Name)
	defer hub.Unsub(sub)

	c := NewDockerNetworksCollector(&domain.Context{Hub: hub})
	c.ListFn = func() ([]dto.DockerNetworkInfo, error) { return nil, fmt.Errorf("docker error") }
	c.Collect()

	select {
	case <-sub:
		t.Fatal("expected no publish on list error")
	case <-time.After(150 * time.Millisecond):
	}
}
