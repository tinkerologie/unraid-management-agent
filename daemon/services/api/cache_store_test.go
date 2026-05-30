package api

import (
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestGetDockerCacheMerge(t *testing.T) {
	var cs CacheStore
	containers := []dto.ContainerInfo{
		{ID: "abc123", Name: "plex"},
		{ID: "def456", Name: "sonarr"},
	}
	cs.dockerCache.Store(&containers)

	// No updates cache yet → all unknown, raw slice untouched.
	got := cs.GetDockerCache()
	for _, c := range got {
		if c.UpdateStatus != dto.UpdateStatusUnknown {
			t.Errorf("%s: status = %q, want unknown", c.Name, c.UpdateStatus)
		}
	}
	if containers[0].UpdateStatus != "" {
		t.Error("raw stored slice was mutated")
	}

	// Publish update result for plex only, with a non-zero Timestamp.
	cs.dockerUpdatesCache.Store(&dto.ContainerUpdatesResult{
		Containers: []dto.ContainerUpdateInfo{
			{ContainerID: "abc123", ContainerName: "plex", CurrentDigest: "sha256:a", LatestDigest: "sha256:b", UpdateAvailable: true},
		},
		TotalCount: 1, UpdatesAvailable: 1, Timestamp: time.Now(),
	})

	got = cs.GetDockerCache()
	byName := map[string]dto.ContainerInfo{}
	for _, c := range got {
		byName[c.Name] = c
	}
	if byName["plex"].UpdateStatus != dto.UpdateStatusAvailable {
		t.Errorf("plex status = %q, want update_available", byName["plex"].UpdateStatus)
	}
	if byName["plex"].UpdateAvailable == nil || !*byName["plex"].UpdateAvailable {
		t.Error("plex UpdateAvailable should be non-nil true")
	}
	if byName["plex"].UpdateChecked == nil {
		t.Error("plex UpdateChecked should be non-nil when Timestamp is set")
	}
	if byName["sonarr"].UpdateStatus != dto.UpdateStatusUnknown {
		t.Errorf("sonarr status = %q, want unknown", byName["sonarr"].UpdateStatus)
	}
	if byName["sonarr"].UpdateChecked != nil {
		t.Error("sonarr UpdateChecked should be nil for unmatched container")
	}
}
