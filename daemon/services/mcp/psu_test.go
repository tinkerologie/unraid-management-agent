package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestToolGetPSU(t *testing.T) {
	t.Run("listed as a read-only tool", func(t *testing.T) {
		server, _ := setupInitializedServer(t)
		cs, cleanup := connectClientToServer(t, server)
		defer cleanup()

		result, err := cs.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatalf("ListTools error: %v", err)
		}
		for _, tool := range result.Tools {
			if tool.Name != "get_psu_status" {
				continue
			}
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				t.Errorf("get_psu_status annotations = %+v, want ReadOnlyHint", tool.Annotations)
			}
			return
		}
		t.Error("expected get_psu_status tool to be registered")
	})

	t.Run("pending before the first collection", func(t *testing.T) {
		server, _ := setupInitializedServer(t)
		cs, cleanup := connectClientToServer(t, server)
		defer cleanup()

		_, text := callToolJSON(t, cs, "get_psu_status", nil)
		var got dto.PSUStatus
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatalf("invalid JSON %q: %v", text, err)
		}
		if got.State != dto.PSUStatePending {
			t.Errorf("state = %q, want %q", got.State, dto.PSUStatePending)
		}
		if got.Devices == nil {
			t.Error("pending list must serialise an empty card list, not null")
		}
	})

	t.Run("returns the cached PSUs", func(t *testing.T) {
		server, mock := setupInitializedServer(t)
		mock.psuStatus = &dto.PSUStatus{
			State:     dto.PSUStateOK,
			Devices:   []dto.PSUDevice{{ID: "psu-test", Model: "HX1000i"}},
			Timestamp: time.Now(),
		}
		cs, cleanup := connectClientToServer(t, server)
		defer cleanup()

		_, text := callToolJSON(t, cs, "get_psu_status", nil)
		var got dto.PSUStatus
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatalf("invalid JSON %q: %v", text, err)
		}
		if got.State != dto.PSUStateOK || len(got.Devices) != 1 || got.Devices[0].ID != "psu-test" {
			t.Errorf("unexpected PSUs: %+v", got)
		}
	})
}
