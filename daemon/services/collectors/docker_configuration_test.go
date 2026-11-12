package collectors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestDockerPublishesConfiguredMemoryForStoppedContainers(t *testing.T) {
	const list = `[
	  {"Id":"aaaaaaaaaaaa1","Names":["/capped"],"State":"exited"},
	  {"Id":"bbbbbbbbbbbb2","Names":["/unlimited"],"State":"running"},
	  {"Id":"cccccccccccc3","Names":["/unavailable"],"State":"exited"}
	]`
	var cappedInspections atomic.Int32
	hub := domain.NewEventBus(10)
	sub := hub.Sub(constants.TopicContainerListUpdate.Name)
	t.Cleanup(func() { hub.Unsub(sub) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("API-Version", "1.56")
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_, _ = fmt.Fprint(w, list)
		case strings.HasSuffix(r.URL.Path, "/aaaaaaaaaaaa1/json"):
			cappedInspections.Add(1)
			_, _ = fmt.Fprint(w, `{"HostConfig":{"Memory":268435456}}`)
		case strings.HasSuffix(r.URL.Path, "/bbbbbbbbbbbb2/json"):
			_, _ = fmt.Fprint(w, `{"HostConfig":{"Memory":0}}`)
		default:
			http.Error(w, "inspect unavailable", http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("DOCKER_HOST", server.URL)
	collector := NewDockerCollector(&domain.Context{Hub: hub})
	t.Cleanup(collector.Close)
	for range 2 {
		if err := collector.Refresh(t.Context()); err != nil {
			t.Fatal(err)
		}
		select {
		case event := <-sub:
			data, err := json.Marshal(event.([]*dto.ContainerInfo))
			if err != nil {
				t.Fatal(err)
			}
			var got []map[string]any
			if err := json.Unmarshal(data, &got); err != nil || len(got) != 3 {
				t.Fatalf("published %s, error %v", data, err)
			}
			if got[0]["configured_memory_limit_bytes"] != float64(268435456) || got[0]["memory_limit_bytes"] != float64(0) {
				t.Fatalf("stopped container limits = %v", got[0])
			}
			if got[1]["configured_memory_limit_bytes"] != float64(0) {
				t.Fatalf("unlimited container = %v", got[1])
			}
			if _, ok := got[2]["configured_memory_limit_bytes"]; ok {
				t.Fatalf("unavailable cap was published: %v", got[2])
			}
		case <-time.After(time.Second):
			t.Fatal("Docker refresh did not publish")
		}
	}
	if got := cappedInspections.Load(); got != 1 {
		t.Fatalf("stopped container was inspected %d times before expiry", got)
	}
}

func TestDockerPublishesSortedNetworksAndPrimaryAddress(t *testing.T) {
	const id = "11111111111111"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("API-Version", "1.56")
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_, _ = fmt.Fprint(w, `[{"Id":"`+id+`","Names":["/dual"],"State":"running"}]`)
		case strings.HasSuffix(r.URL.Path, "/"+id+"/json"):
			_, _ = fmt.Fprint(w, `{"HostConfig":{"NetworkMode":"net-z"},"NetworkSettings":{"Networks":{
			  "alpha":{"NetworkID":"net-a","IPAddress":"192.0.2.2","MacAddress":"02:42:00:00:00:02"},
			  "zulu":{"NetworkID":"net-z","IPAddress":"198.51.100.2","GlobalIPv6Address":"2001:db8::2","MacAddress":"02:42:00:00:00:03"}
			}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("DOCKER_HOST", server.URL)
	hub := domain.NewEventBus(10)
	sub := hub.Sub(constants.TopicContainerListUpdate.Name)
	t.Cleanup(func() { hub.Unsub(sub) })
	collector := NewDockerCollector(&domain.Context{Hub: hub})
	t.Cleanup(collector.Close)
	for range 4 {
		if err := collector.Refresh(t.Context()); err != nil {
			t.Fatal(err)
		}
		select {
		case event := <-sub:
			got := event.([]*dto.ContainerInfo)[0]
			data, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			want := `"networks":[{"network":"alpha","network_id":"net-a","ip_address":"192.0.2.2","mac_address":"02:42:00:00:00:02"},{"network":"zulu","network_id":"net-z","ip_address":"198.51.100.2","ipv6_address":"2001:db8::2","mac_address":"02:42:00:00:00:03"}]`
			if got.IPAddress != "198.51.100.2" || got.MACAddress != "02:42:00:00:00:03" || !strings.Contains(string(data), want) {
				t.Fatalf("unexpected attachment data: %s", data)
			}
		case <-time.After(time.Second):
			t.Fatal("Docker refresh did not publish")
		}
	}
}

func TestDefaultDockerModePrefersBridgeAddress(t *testing.T) {
	ip, mac := selectLegacyEndpoint("default", []dto.ContainerNetworkAttachment{
		{Network: "alpha", IPAddress: "192.0.2.2", MACAddress: "02:42:00:00:00:02"},
		{Network: "bridge", IPAddress: "198.51.100.2", MACAddress: "02:42:00:00:00:03"},
	})
	if ip != "198.51.100.2" || mac != "02:42:00:00:00:03" {
		t.Fatalf("default mode selected %s / %s instead of bridge", ip, mac)
	}
}
