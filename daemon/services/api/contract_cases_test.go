package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/platform"
)

func restContract(id, path, model, description string, status int, configure func(*Server), semantics ...string) contractCapture {
	return contractCapture{
		contractCase: contractCase{ID: id, Transport: "rest", Method: http.MethodGet,
			Path: "/api/v1" + path, Status: status, Model: model, Case: description,
			Semantics: semantics, Origin: "producer-reachable"},
		configure: configure,
	}
}

func eventContract[T any](id, event, model, description string, topic domain.Topic[T], data T, semantics ...string) contractCapture {
	return contractCapture{
		contractCase: contractCase{ID: id, Transport: "websocket", Event: event, Envelope: "dto.WSEvent",
			Model: model, Case: description, Semantics: semantics, Origin: "producer-reachable"},
		publish: func(hub *domain.EventBus) { domain.Publish(hub, topic, data) },
	}
}

func normalizeTimestamp(c contractCapture) contractCapture {
	c.NormalizedFields = []string{"/timestamp"}
	return c
}

func syntheticContract(c contractCapture) contractCapture {
	c.Origin = "dto-constructed"
	return c
}

func producerContractCases() []contractCapture {
	return slices.Concat(networkAndPoolContracts(), powerContracts(), notificationContracts(), shareAndHealthContracts())
}

func networkAndPoolContracts() []contractCapture {
	status := dto.NetworkServicesStatus{
		SMB:           dto.NetworkServiceInfo{Name: "SMB", Port: 445, Running: true, Enabled: true},
		NFS:           dto.NetworkServiceInfo{Name: "NFS", Port: 2049},
		AFP:           dto.NetworkServiceInfo{Name: "AFP", Port: 548},
		FTP:           dto.NetworkServiceInfo{Name: "FTP", Port: 21},
		SSH:           dto.NetworkServiceInfo{Name: "SSH", Port: 22},
		Telnet:        dto.NetworkServiceInfo{Name: "Telnet", Port: 23},
		Avahi:         dto.NetworkServiceInfo{Name: "Avahi", Port: 5353},
		NetBIOS:       dto.NetworkServiceInfo{Name: "NetBIOS", Port: 137},
		WSD:           dto.NetworkServiceInfo{Name: "WSD", Port: 3702},
		WireGuard:     dto.NetworkServiceInfo{Name: "WireGuard"},
		UPNP:          dto.NetworkServiceInfo{Name: "UPnP"},
		NTP:           dto.NetworkServiceInfo{Name: "NTP", Port: 123},
		SyslogServer:  dto.NetworkServiceInfo{Name: "Syslog", Port: 514, Running: true, Enabled: true},
		TotalServices: 13, EnabledServices: 2, RunningServices: 2, Timestamp: contractTime,
	}
	pools := []dto.ZFSPool{
		{Name: "sample", Health: "DEGRADED", Timestamp: contractTime, CorruptedFiles: []string{"/mnt/sample/example.bin"}},
		{Name: "empty", Health: "ONLINE", Timestamp: contractTime, CorruptedFiles: []string{}},
		{Name: "unknown", Health: "ONLINE", Timestamp: contractTime},
	}
	setPools := func(s *Server) { s.zfsPoolsCache.Store(&pools) }
	setManager := func(s *Server) {
		s.collectorManager = &fixtureCollectorManager{mockCollectorManager{statuses: map[string]*dto.CollectorStatus{
			"disk": {Name: "disk", Enabled: true, Interval: 30, Status: "running", LastRun: &contractTime,
				Mode: "dynamic", EffectiveInterval: 30, Reason: "active", PublishCount: 2},
		}}}
	}
	return []contractCapture{
		syntheticContract(restContract("network-services", "/settings/network-services", "dto.NetworkServicesStatus", "Syslog wire key, counts and omitted port", 200,
			func(s *Server) {
				s.networkServicesStatus = func() (*dto.NetworkServicesStatus, error) { return &status, nil }
			}, "object", "alias", "omitted")),
		normalizeTimestamp(restContract("network-services-error", "/settings/network-services", "dto.Response", "Provider failure", 500,
			func(s *Server) {
				s.networkServicesStatus = func() (*dto.NetworkServicesStatus, error) { return nil, errors.New("synthetic unavailable source") }
			}, "object")),
		syntheticContract(restContract("zfs-pools", "/zfs/pools", "[]dto.ZFSPool", "Corrupted files populated, empty and null", 200, setPools, "list", "null")),
		restContract("zfs-pool", "/zfs/pools/sample", "dto.ZFSPool", "One pool object", 200, setPools, "object"),
		normalizeTimestamp(restContract("zfs-pool-missing", "/zfs/pools/missing", "dto.Response", "Unknown pool", 404, setPools, "object")),
		syntheticContract(eventContract("zfs-pools-event", "zfs_pools_update", "[]dto.ZFSPool", "Array-valued pool event", constants.TopicZFSPoolsUpdate, pools, "list", "null")),
		restContract("collector-status-list", "/collectors/status", "dto.CollectorsStatusResponse", "Full collector status list", 200, setManager, "object", "distinct-model"),
		normalizeTimestamp(restContract("collector-status", "/collectors/disk", "dto.CollectorResponse", "Full collector status object", 200, setManager, "object", "distinct-model")),
		syntheticContract(eventContract("collector-state-event", "update", "dto.CollectorStateEvent", "Partial collector update in generic envelope",
			constants.TopicCollectorStateChange, dto.CollectorStateEvent{Event: "collector_state_change", Collector: "disk", Status: "stopped", Interval: 30, Timestamp: contractTime}, "partial", "distinct-model")),
	}
}

type fixtureCollectorManager struct{ mockCollectorManager }

func (m *fixtureCollectorManager) GetAllStatus() dto.CollectorsStatusResponse {
	result := m.mockCollectorManager.GetAllStatus()
	result.Timestamp = contractTime
	result.Mode = "dynamic"
	return result
}

func powerContracts() []contractCapture {
	zero := 0.0
	ups := &dto.UPSStatus{Connected: true, Status: "OL", LoadPercent: &zero, Timestamp: contractTime}
	primary := &dto.NUTStatus{Connected: true, DeviceName: "sample-ups", Status: "OL", Timestamp: contractTime}
	secondary := &dto.NUTStatus{Connected: true, DeviceName: "sample-ats", Type: "ats", Timestamp: contractTime}
	nut := &dto.NUTResponse{Installed: true, Running: true, Status: primary, Statuses: []*dto.NUTStatus{primary, secondary},
		Devices: []dto.NUTDevice{{Name: primary.DeviceName, Available: true}, {Name: secondary.DeviceName, Available: true}}, Timestamp: contractTime}
	return []contractCapture{
		restContract("ups-measurements", "/ups", "dto.UPSStatus", "Zero and unknown measurements", 200,
			func(s *Server) { s.upsCache.Store(ups) }, "zero", "null"),
		normalizeTimestamp(restContract("ups-empty", "/ups", "dto.UPSStatus", "Disconnected fallback", 200, nil, "null")),
		eventContract("ups-event", "ups_status_update", "*dto.UPSStatus", "Basic UPS event", constants.TopicUPSStatusUpdate, ups, "distinct-model"),
		normalizeTimestamp(restContract("nut-not-installed", "/nut", "dto.NUTResponse", "NUT unavailable", 200, nil, "omitted")),
		restContract("nut-stopped", "/nut", "dto.NUTResponse", "NUT installed but stopped", 200,
			func(s *Server) { s.nutCache.Store(&dto.NUTResponse{Installed: true, Timestamp: contractTime}) }, "omitted"),
		restContract("nut-running", "/nut", "dto.NUTResponse", "Multiple NUT devices and primary alias", 200,
			func(s *Server) { s.nutCache.Store(nut) }, "alias", "list", "null"),
		eventContract("nut-event", "nut_status_update", "*dto.NUTResponse", "NUT response distinct from UPS event", constants.TopicNUTStatusUpdate, nut, "distinct-model"),
	}
}

func notificationContracts() []contractCapture {
	list := &dto.NotificationList{Timestamp: contractTime,
		Overview:      dto.NotificationOverview{Unread: dto.NotificationCounts{Info: 1, Total: 1}},
		Notifications: []dto.Notification{{ID: "sample-notification", Subject: "Example", Importance: "info", Type: "unread", Timestamp: contractTime}}}
	setList := func(s *Server) { s.notificationsCache.Store(list) }
	return []contractCapture{
		restContract("notifications", "/notifications", "dto.NotificationList", "List and overview with omitted title and link", 200, setList, "object", "omitted", "zero"),
		normalizeTimestamp(restContract("notifications-fallback", "/notifications", "dto.NotificationList", "Fallback empty list", 200, nil, "list")),
		restContract("notifications-collected-empty", "/notifications", "dto.NotificationList", "Collected nil list", 200,
			func(s *Server) { s.notificationsCache.Store(&dto.NotificationList{Timestamp: contractTime}) }, "null"),
		restContract("notifications-unread", "/notifications/unread", "notification wrapper", "Unread list and count wrapper", 200, setList, "object"),
		restContract("notifications-overview", "/notifications/overview", "dto.NotificationOverview", "Overview and zero archive counts", 200, setList, "object", "zero"),
		normalizeTimestamp(restContract("notification-missing", "/notifications/missing", "dto.Response", "Unknown notification", 404, setList, "object")),
		eventContract("notifications-event", "notifications_update", "*dto.NotificationList", "List envelope contains overview", constants.TopicNotificationsUpdate, list, "object"),
	}
}

func shareAndHealthContracts() []contractCapture {
	zero, measured := uint64(0), uint64(25)
	yes, no := true, false
	emptyLocations := []string{}
	shares := []dto.ShareInfo{
		{Name: "measured", Used: &measured, Exclusive: &yes, StorageUsed: 60, StorageFree: 40, StorageTotal: 100,
			StorageUsagePercent: 60, Free: 40, Total: 100, UsagePercent: 60, Timestamp: contractTime},
		{Name: "zero", Used: &zero, Exclusive: &no, Locations: &emptyLocations, LocationsUnchecked: []string{"disk1"}, Timestamp: contractTime},
		{Name: "unknown", Timestamp: contractTime},
	}
	setHealth := func(s *Server) {
		reg := platform.NewRegistry()
		reg.SetClock(func() time.Time { return contractTime })
		reg.SetCapabilities(dto.Capabilities{UnraidVersion: "7.3.1", CheckedAt: contractTime, Items: []dto.Capability{
			{Name: "hardware.gpu", Available: true, State: dto.CapabilityPresent},
			{Name: "hardware.nut", State: dto.CapabilityAbsent},
			{Name: "hardware.psu", State: dto.CapabilityUnknown, Detail: "synthetic probe timeout"},
		}})
		reg.Healthy("array")
		reg.Healthy("shares")
		reg.Report("shares", dto.SourceUnavailable, "synthetic unavailable source", nil)
		reg.Report("ups", dto.SourceDisabled, "synthetic disabled source", nil)
		s.ctx.Platform = reg
	}
	return []contractCapture{
		syntheticContract(restContract("shares-usage", "/shares", "[]dto.ShareInfo", "Measured, zero and omitted usage beside backing values", 200,
			func(s *Server) { s.sharesCache.Store(&shares) }, "list", "zero", "omitted", "alias")),
		restContract("shares-empty", "/shares", "[]dto.ShareInfo", "No collected shares", 200, nil, "list"),
		syntheticContract(eventContract("shares-null-event", "share_list_update", "[]dto.ShareInfo", "Nil slice produces null data",
			constants.TopicShareListUpdate, nil, "null")),
		restContract("capabilities", "/capabilities", "dto.Capabilities", "Present, absent and unknown capability states", 200, setHealth, "object", "list"),
		normalizeTimestamp(restContract("source-health", "/diagnostics/self-test", "selfTestResponse", "Healthy, unavailable and disabled sources", 200, setHealth, "object", "list")),
		syntheticContract(eventContract("source-health-event", "source_status_changed", "dto.SourceStatus", "Degraded source-health event",
			constants.TopicSourceStatusChanged, dto.SourceStatus{Subsystem: "shares", State: dto.SourceDegraded, Reason: "synthetic incomplete sample", LastChecked: contractTime}, "object")),
	}
}

func TestProducerContractAccess(t *testing.T) {
	s := newContractServer(t)
	calls := 0
	s.networkServicesStatus = func() (*dto.NetworkServicesStatus, error) {
		calls++
		return &dto.NetworkServicesStatus{}, nil
	}
	for _, tc := range []struct {
		bearer string
		want   int
	}{{"", 401}, {"wrong-synthetic-token", 401}, {contractBearer, 200}} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/network-services", nil)
		if tc.bearer != "" {
			req.Header.Set("Authorization", "Bearer "+tc.bearer)
		}
		rec := httptest.NewRecorder()
		s.GetRouter().ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("REST status %d, want %d", rec.Code, tc.want)
		}
	}
	if calls != 1 {
		t.Fatalf("network services provider invoked %d times, want 1 authorized invocation", calls)
	}

	server := httptest.NewServer(s.GetRouter())
	defer server.Close()
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	for _, tc := range []struct {
		headers http.Header
		want    int
	}{
		{nil, http.StatusUnauthorized},
		{http.Header{"Authorization": {"Bearer " + contractBearer}, "Origin": {"https://untrusted.example"}}, http.StatusForbidden},
	} {
		conn, response, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/ws", tc.headers)
		if conn != nil {
			_ = conn.Close()
		}
		if response != nil {
			_ = response.Body.Close()
		}
		if err == nil || response == nil || response.StatusCode != tc.want {
			t.Fatalf("WebSocket rejection should be HTTP %d, got %v", tc.want, err)
		}
	}
}
