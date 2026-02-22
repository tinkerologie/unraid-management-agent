package services

import (
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/mqtt"
)

func TestCreateOrchestrator(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{
		Hub:    hub,
		Config: domain.Config{Version: "test", Port: 8080},
	}

	o := CreateOrchestrator(ctx)
	if o == nil {
		t.Fatal("CreateOrchestrator returned nil")
	}
	if o.ctx != ctx {
		t.Error("Orchestrator ctx not set correctly")
	}
	if o.mqttClient != nil {
		t.Error("Expected mqttClient to be nil initially")
	}
	if o.collectorManager != nil {
		t.Error("Expected collectorManager to be nil initially")
	}
}

func TestHandleMQTTEvent_NilClient(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	o := CreateOrchestrator(ctx)

	// mqttClient is nil — all of these should return early without panic
	testCases := []struct {
		name string
		msg  any
	}{
		{"SystemInfo", &dto.SystemInfo{Hostname: "test"}},
		{"ArrayStatus", &dto.ArrayStatus{State: "Started"}},
		{"DiskInfo", []dto.DiskInfo{{Name: "disk1"}}},
		{"ShareInfo", []dto.ShareInfo{{Name: "appdata"}}},
		{"ContainerInfo", []*dto.ContainerInfo{{ID: "abc", Name: "plex"}}},
		{"VMInfo", []*dto.VMInfo{{ID: "1", Name: "win10"}}},
		{"UPSStatus", &dto.UPSStatus{Status: "OL"}},
		{"GPUMetrics", []*dto.GPUMetrics{{Name: "RTX 3080"}}},
		{"NetworkInfo", []dto.NetworkInfo{{Name: "eth0"}}},
		{"NotificationList", &dto.NotificationList{}},
		{"UnknownType", "some string"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic
			o.handleMQTTEvent(tc.msg)
		})
	}
}

func TestHandleMQTTEvent_NotConnected(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	o := CreateOrchestrator(ctx)

	// Create an MQTT client that is NOT connected (disabled config)
	mqttConfig := &dto.MQTTConfig{
		Enabled: false,
		Broker:  "tcp://localhost:1883",
	}
	o.mqttClient = mqtt.NewClient(mqttConfig, "test-host", "1.0.0")
	// Client created but never connected — IsConnected() returns false

	testCases := []struct {
		name string
		msg  any
	}{
		{"SystemInfo", &dto.SystemInfo{Hostname: "test"}},
		{"ArrayStatus", &dto.ArrayStatus{State: "Started"}},
		{"DiskInfo", []dto.DiskInfo{{Name: "disk1"}}},
		{"ShareInfo", []dto.ShareInfo{{Name: "appdata"}}},
		{"ContainerInfo", []*dto.ContainerInfo{{ID: "abc", Name: "plex"}}},
		{"VMInfo", []*dto.VMInfo{{ID: "1", Name: "win10"}}},
		{"UPSStatus", &dto.UPSStatus{Status: "OL"}},
		{"GPUMetrics", []*dto.GPUMetrics{{Name: "RTX 3080"}}},
		{"NetworkInfo", []dto.NetworkInfo{{Name: "eth0"}}},
		{"NotificationList", &dto.NotificationList{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic — early return because not connected
			o.handleMQTTEvent(tc.msg)
		})
	}
}
