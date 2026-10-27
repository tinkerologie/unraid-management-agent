package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestHandlePSU(t *testing.T) {
	t.Run("pending before the first collection", func(t *testing.T) {
		server, _ := setupTestServer()
		rr := httptest.NewRecorder()
		server.router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/psu", nil))

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var got dto.PSUStatus
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if got.State != dto.PSUStatePending {
			t.Errorf("state = %q, want pending", got.State)
		}
		if got.Devices == nil {
			t.Error("PSUs must be an empty array, not null")
		}
	})

	t.Run("serves the PSUs published on the event bus", func(t *testing.T) {
		server, _ := setupTestServer()
		dispatch := buildCacheDispatch(cacheBindings())
		update, ok := dispatch[reflect.TypeFor[*dto.PSUStatus]()]
		if !ok {
			t.Fatal("no cache binding for *dto.PSUStatus")
		}
		update(server.CacheStore, &dto.PSUStatus{
			State:   dto.PSUStateOK,
			Devices: []dto.PSUDevice{{ID: "psu-test", Model: "HX1000i"}},
		})

		rr := httptest.NewRecorder()
		server.router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/psu", nil))
		var got dto.PSUStatus
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if got.State != dto.PSUStateOK || len(got.Devices) != 1 || got.Devices[0].Model != "HX1000i" {
			t.Errorf("unexpected PSUs: %+v", got)
		}
	})

	t.Run("card updates are broadcast to websocket clients", func(t *testing.T) {
		if !slices.Contains(broadcastTopicNames(), constants.TopicPSUStatusUpdate.Name) {
			t.Errorf("%s is not broadcast", constants.TopicPSUStatusUpdate.Name)
		}
	})
}

func TestPSUEventCacheDisconnection(t *testing.T) {
	server, app := setupTestServer()
	app.Hub = domain.NewEventBus(10)
	cancel := startSubscribeToEvents(t, server)
	defer cancel()
	app.Hub.Pub(&dto.PSUStatus{State: dto.PSUStateOK, Devices: []dto.PSUDevice{{ID: "psu-a"}}}, constants.TopicPSUStatusUpdate.Name)
	eventually(t, func() bool { return server.GetPSUCache() != nil && len(server.GetPSUCache().Devices) == 1 })
	app.Hub.Pub(dto.NewPSUStatus(dto.PSUStateNotDetected, time.Now()), constants.TopicPSUStatusUpdate.Name)
	eventually(t, func() bool {
		return server.GetPSUCache().State == dto.PSUStateNotDetected && len(server.GetPSUCache().Devices) == 0
	})
}
