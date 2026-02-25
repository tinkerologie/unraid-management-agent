package services

import (
	"context"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/mqtt"
)

func TestCreateOrchestrator(t *testing.T) {
	hub := domain.NewEventBus(10)
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

func TestSubscribeMQTTEvents_NilClient(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	o := CreateOrchestrator(ctx)

	// mqttClient is nil — subscribeMQTTEvents should return immediately without panic
	goCtx := context.Background()

	done := make(chan struct{})
	go func() {
		defer close(done)
		o.subscribeMQTTEvents(goCtx, nil)
	}()

	select {
	case <-done:
		// Returned immediately as expected
	case <-time.After(2 * time.Second):
		t.Error("subscribeMQTTEvents did not return immediately for nil client")
	}
}

func TestSubscribeMQTTEvents_NotConnected(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	o := CreateOrchestrator(ctx)

	// Create an MQTT client that is NOT connected (disabled config)
	mqttConfig := &dto.MQTTConfig{
		Enabled: false,
		Broker:  "tcp://localhost:1883",
	}
	o.mqttClient = mqtt.NewClient(mqttConfig, "test-host", "1.0.0", ctx)

	// Client created but never connected — subscribeMQTTEvents should block on
	// the select loop but exit when the context is cancelled.
	goCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		o.subscribeMQTTEvents(goCtx, nil)
	}()

	select {
	case <-done:
		// Returned via context cancellation as expected
	case <-time.After(2 * time.Second):
		t.Error("subscribeMQTTEvents did not return for disconnected client")
	}
}
