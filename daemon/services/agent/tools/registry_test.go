package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

type fakeState struct{}

func (fakeState) SystemJSON() (any, bool) { return map[string]string{"host": "tower"}, true }
func (fakeState) ArrayJSON() (any, bool)  { return map[string]string{"state": "STARTED"}, true }
func (fakeState) DockerJSON() (any, bool) { return []string{"plex"}, true }

type fakeDocker struct{ restarted string }

func (f *fakeDocker) Restart(id string) error { f.restarted = id; return nil }

func TestBuildDefaultTiersAndInvoke(t *testing.T) {
	fd := &fakeDocker{}
	reg := BuildDefault(fakeState{}, fd)

	sys, ok := reg.Get("get_system_info")
	if !ok || sys.RiskTier != dto.RiskReadOnly {
		t.Fatalf("get_system_info missing or wrong tier: %+v", sys)
	}
	res, err := sys.Invoke(context.Background(), "{}")
	if err != nil || res == "" {
		t.Fatalf("invoke get_system_info: %q err=%v", res, err)
	}

	rc, ok := reg.Get("restart_container")
	if !ok || rc.RiskTier != dto.RiskLow {
		t.Fatalf("restart_container missing or wrong tier: %+v", rc)
	}
	// ValidateContainerID requires 12 or 64 lowercase hex characters.
	// Use a valid 12-hex container ID.
	if _, err := rc.Invoke(context.Background(), `{"container_id":"abc123def456"}`); err != nil {
		t.Fatalf("invoke restart_container: %v", err)
	}
	if fd.restarted != "abc123def456" {
		t.Fatalf("expected restart of abc123def456, got %q", fd.restarted)
	}

	if len(reg.Schemas()) == 0 {
		t.Fatal("expected non-empty schemas")
	}
	_ = json.RawMessage(nil)
}
