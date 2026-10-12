package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestToolGetNetworkCards(t *testing.T) {
	t.Run("listed as a read-only tool", func(t *testing.T) {
		server, _ := setupInitializedServer(t)
		cs, cleanup := connectClientToServer(t, server)
		defer cleanup()

		result, err := cs.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatalf("ListTools error: %v", err)
		}
		for _, tool := range result.Tools {
			if tool.Name != "get_network_cards" {
				continue
			}
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				t.Errorf("get_network_cards annotations = %+v, want ReadOnlyHint", tool.Annotations)
			}
			return
		}
		t.Error("expected get_network_cards tool to be registered")
	})

	t.Run("pending before the first collection", func(t *testing.T) {
		server, _ := setupInitializedServer(t)
		cs, cleanup := connectClientToServer(t, server)
		defer cleanup()

		_, text := callToolJSON(t, cs, "get_network_cards", nil)
		var got dto.NICList
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatalf("invalid JSON %q: %v", text, err)
		}
		if got.State != dto.NICStatePending {
			t.Errorf("state = %q, want %q", got.State, dto.NICStatePending)
		}
		if got.Cards == nil {
			t.Error("pending list must serialise an empty card list, not null")
		}
	})

	t.Run("returns the cached cards", func(t *testing.T) {
		server, mock := setupInitializedServer(t)
		mock.nicList = &dto.NICList{
			State:     dto.NICStateOK,
			Cards:     []dto.NICCard{{ID: "0000:41:00", Model: "ConnectX-6 Dx"}},
			Timestamp: time.Now(),
		}
		cs, cleanup := connectClientToServer(t, server)
		defer cleanup()

		_, text := callToolJSON(t, cs, "get_network_cards", nil)
		var got dto.NICList
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatalf("invalid JSON %q: %v", text, err)
		}
		if got.State != dto.NICStateOK || len(got.Cards) != 1 || got.Cards[0].ID != "0000:41:00" {
			t.Errorf("unexpected cards: %+v", got)
		}
	})
}
