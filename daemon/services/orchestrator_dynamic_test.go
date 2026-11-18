package services

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/alerting"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/mqtt"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/scheduler"
)

func newModeOrchestrator(mode string) *Orchestrator {
	ctx := &domain.Context{Hub: domain.NewEventBus(10), Version: "test", CollectionMode: mode}
	o := CreateOrchestrator(ctx)
	o.collectorManager = NewCollectorManager(ctx, &sync.WaitGroup{})
	return o
}

func TestApplyAlertFloors(t *testing.T) {
	for _, tt := range []struct {
		mode       string
		wantFloors bool
	}{
		{domain.CollectionModeDynamic, true},
		{domain.CollectionModeManual, false},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			o := newModeOrchestrator(tt.mode)
			engine := alerting.NewEngine(alerting.NewStore(t.TempDir()), nil)
			o.applyAlertFloors(engine)
			// The listener is called immediately on registration.
			if got := o.collectorManager.floors.Load() != nil; got != tt.wantFloors {
				t.Errorf("floors set = %v, want %v", got, tt.wantFloors)
			}
		})
	}
}

func TestSubscribeMQTTEvents_DynamicRegistersDemand(t *testing.T) {
	runMQTTSubscription(t, domain.CollectionModeDynamic)
}

// Manual mode installs no MQTT demand: collector cadence ignores consumers.
func TestSubscribeMQTTEvents_ManualSkipsDemand(t *testing.T) {
	runMQTTSubscription(t, domain.CollectionModeManual)
}

func runMQTTSubscription(t *testing.T, mode string) {
	t.Helper()
	o := newModeOrchestrator(mode)
	o.mqttClient = mqtt.NewClient(&dto.MQTTConfig{Broker: "tcp://localhost:1883", DisabledTopics: []string{mqtt.TopicContainers}}, "test-host", "1.0.0", o.ctx)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		o.subscribeMQTTEvents(ctx, nil)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("subscribeMQTTEvents did not return")
	}
	// The broker is not connected, so MQTT makes no collector active.
	if o.collectorManager.Demand().Active("system") {
		t.Error("disconnected MQTT made system active")
	}
}

func TestMQTTDemandFollowsConnection(t *testing.T) {
	demand := scheduler.NewDemand(scheduler.RealClock{})
	handler := mqttDemand(demand, map[string]bool{"system": true})
	handler(true)
	if !demand.Active("system") || demand.Active("docker") {
		t.Errorf("connected: system active = %v, docker active = %v; want true, false", demand.Active("system"), demand.Active("docker"))
	}
	handler(false)
	if demand.Active("system") {
		t.Error("disconnected MQTT kept system active")
	}
}

func TestMQTTSelectedTopicsDemandWithBroker(t *testing.T) {
	brokerURL := os.Getenv("UMA_TEST_MQTT_BROKER")
	if brokerURL == "" {
		t.Skip("set UMA_TEST_MQTT_BROKER to a disposable broker")
	}
	o := newModeOrchestrator(domain.CollectionModeDynamic)
	cfg := mqtt.DefaultConfig()
	cfg.Broker = brokerURL
	cfg.ClientID = fmt.Sprintf("uma356-demand-%d", time.Now().UnixNano())
	cfg.TopicPrefix = cfg.ClientID
	cfg.HADiscoveryPrefix = cfg.ClientID + "-discovery"
	cfg.Enabled = true
	cfg.HomeAssistantMode = false
	cfg.DisabledTopics = []string{mqtt.TopicContainers, mqtt.TopicNotifications}
	o.mqttClient = mqtt.NewClient(cfg, "tower", "test", o.ctx)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := o.mqttClient.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	defer o.mqttClient.Disconnect()
	ready := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() { o.subscribeMQTTEvents(ctx, nil, func() { close(ready) }) })
	defer wg.Wait()
	defer cancel()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("MQTT subscriber did not start")
	}

	demand := o.collectorManager.Demand()
	for !demand.Active("system") {
		select {
		case <-ctx.Done():
			t.Fatal("enabled system telemetry did not activate MQTT collector demand")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if demand.Active("docker") {
		t.Error("disabled Docker telemetry activated collector demand")
	}
	if !demand.Active("notification") {
		t.Error("enabled notification events should still activate collector demand")
	}
	websocket := map[string]bool{constants.TopicContainerListUpdate.Name: true}
	demand.WSAdd(websocket)
	if !demand.Active("docker") {
		t.Error("MQTT topic filtering interfered with WebSocket demand for containers")
	}
	demand.WSRemove(websocket)
	if demand.Active("docker") {
		t.Error("Docker demand did not drop after the WebSocket subscriber left")
	}
}
