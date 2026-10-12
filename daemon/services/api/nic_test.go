package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestHandleNetworkCards(t *testing.T) {
	t.Run("pending before the first collection", func(t *testing.T) {
		server, _ := setupTestServer()
		rr := httptest.NewRecorder()
		server.router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/network/cards", nil))

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var got dto.NICList
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if got.State != dto.NICStatePending {
			t.Errorf("state = %q, want pending", got.State)
		}
		if got.Cards == nil {
			t.Error("cards must be an empty array, not null")
		}
	})

	t.Run("serves the cards published on the event bus", func(t *testing.T) {
		server, _ := setupTestServer()
		dispatch := buildCacheDispatch(cacheBindings())
		update, ok := dispatch[reflect.TypeFor[*dto.NICList]()]
		if !ok {
			t.Fatal("no cache binding for *dto.NICList")
		}
		update(server.CacheStore, &dto.NICList{
			State: dto.NICStateOK,
			Cards: []dto.NICCard{{ID: "0000:41:00", Model: "ConnectX-6 Dx"}},
		})

		rr := httptest.NewRecorder()
		server.router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/network/cards", nil))
		var got dto.NICList
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if got.State != dto.NICStateOK || len(got.Cards) != 1 || got.Cards[0].Model != "ConnectX-6 Dx" {
			t.Errorf("unexpected cards: %+v", got)
		}
	})

	t.Run("card updates are broadcast to websocket clients", func(t *testing.T) {
		if !slices.Contains(broadcastTopicNames(), constants.TopicNICUpdate.Name) {
			t.Errorf("%s is not broadcast", constants.TopicNICUpdate.Name)
		}
	})
}
