package mcp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestToolGetStorageTopology(t *testing.T) {
	t.Run("pending before the first collection", func(t *testing.T) {
		server, _ := setupInitializedServer(t)
		cs, cleanup := connectClientToServer(t, server)
		defer cleanup()

		_, text := callToolJSON(t, cs, "get_storage_topology", nil)
		var got dto.StorageTopology
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatalf("invalid JSON %q: %v", text, err)
		}
		if got.State != dto.StorageTopologyStatePending {
			t.Errorf("state = %q, want %q", got.State, dto.StorageTopologyStatePending)
		}
		if got.Controllers == nil || got.Enclosures == nil || got.Drives == nil {
			t.Error("pending topology must serialise empty lists, not null")
		}
	})

	t.Run("returns the cached topology", func(t *testing.T) {
		server, mock := setupInitializedServer(t)
		mock.storageTopology = &dto.StorageTopology{
			State:       dto.StorageTopologyStateOK,
			Controllers: []dto.StorageController{{ID: "SN123", Model: "MegaRAID 9580-8i8e"}},
			Timestamp:   time.Now(),
		}
		cs, cleanup := connectClientToServer(t, server)
		defer cleanup()

		_, text := callToolJSON(t, cs, "get_storage_topology", nil)
		var got dto.StorageTopology
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatalf("invalid JSON %q: %v", text, err)
		}
		if got.State != dto.StorageTopologyStateOK || len(got.Controllers) != 1 || got.Controllers[0].ID != "SN123" {
			t.Errorf("unexpected topology: %+v", got)
		}
	})
}
