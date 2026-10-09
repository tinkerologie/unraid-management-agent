package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestHandleStorageTopology(t *testing.T) {
	t.Run("pending before the first collection", func(t *testing.T) {
		server, _ := setupTestServer()
		rr := httptest.NewRecorder()
		server.router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/storage/topology", nil))

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var got dto.StorageTopology
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if got.State != dto.StorageTopologyStatePending {
			t.Errorf("state = %q, want pending", got.State)
		}
		if got.Controllers == nil || got.Enclosures == nil || got.Drives == nil {
			t.Error("lists must be empty arrays, not null")
		}
	})

	t.Run("serves the topology published on the event bus", func(t *testing.T) {
		server, _ := setupTestServer()
		dispatch := buildCacheDispatch(cacheBindings())
		update, ok := dispatch[reflect.TypeFor[*dto.StorageTopology]()]
		if !ok {
			t.Fatal("no cache binding for *dto.StorageTopology")
		}
		update(server.CacheStore, &dto.StorageTopology{
			State:      dto.StorageTopologyStateOK,
			Enclosures: []dto.StorageEnclosure{{ID: "50050cc100000000", Product: "DS424IOM12A"}},
		})

		rr := httptest.NewRecorder()
		server.router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/storage/topology", nil))
		var got dto.StorageTopology
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if got.State != dto.StorageTopologyStateOK || len(got.Enclosures) != 1 || got.Enclosures[0].Product != "DS424IOM12A" {
			t.Errorf("unexpected topology: %+v", got)
		}
	})

	t.Run("topology updates are broadcast to websocket clients", func(t *testing.T) {
		if !slices.Contains(broadcastTopicNames(), constants.TopicStorageTopologyUpdate.Name) {
			t.Errorf("%s is not broadcast", constants.TopicStorageTopologyUpdate.Name)
		}
	})
}
