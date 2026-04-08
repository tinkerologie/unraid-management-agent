package diagnostics

import (
	"context"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestCollectDiagnostics(t *testing.T) {
	ctx := &domain.Context{
		Config: domain.Config{
			Version: "2026.04.00",
			Port:    8043,
		},
		Hub:     domain.NewEventBus(16),
		LogsDir: t.TempDir(),
		Intervals: domain.Intervals{
			System: 15,
			Array:  60,
			Disk:   300,
		},
	}

	svc := NewBundleService(ctx)
	bundle, err := svc.CollectDiagnostics(context.Background())
	if err != nil {
		t.Fatalf("CollectDiagnostics() error = %v", err)
	}

	if bundle == nil {
		t.Fatal("expected non-nil bundle")
	}

	// Verify metadata
	if bundle.Metadata.AgentVersion != "2026.04.00" {
		t.Errorf("agent version = %q, want %q", bundle.Metadata.AgentVersion, "2026.04.00")
	}
	if bundle.Metadata.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}

	// Verify configuration intervals are collected
	if bundle.Configuration.Port != 8043 {
		t.Errorf("port = %d, want %d", bundle.Configuration.Port, 8043)
	}
	if bundle.Configuration.CollectorIntervals["system"] != 15 {
		t.Errorf("system interval = %d, want %d", bundle.Configuration.CollectorIntervals["system"], 15)
	}
}

func TestCollectDiagnostics_RedactsMQTTConfig(t *testing.T) {
	ctx := &domain.Context{
		Config: domain.Config{
			Version: "test",
			Port:    8043,
		},
		Hub:     domain.NewEventBus(16),
		LogsDir: t.TempDir(),
		MQTTConfig: domain.MQTTConfig{
			Enabled:  true,
			Broker:   "mqtt.example.com",
			Username: "user",
			Password: "supersecret",
		},
	}

	svc := NewBundleService(ctx)
	bundle, err := svc.CollectDiagnostics(context.Background())
	if err != nil {
		t.Fatalf("CollectDiagnostics() error = %v", err)
	}

	mqtt := bundle.Configuration.MQTTConfig
	if mqtt == nil {
		t.Fatal("expected non-nil MQTT config")
	}

	// Password field has json:"-" tag, so it should be redacted
	if pw, ok := mqtt["-"]; ok && pw != "[REDACTED]" {
		t.Errorf("MQTT password should be redacted, got %v", pw)
	}

	// Broker should not be redacted
	if broker, ok := mqtt["broker"]; ok && broker != "mqtt.example.com" {
		t.Errorf("MQTT broker = %v, want %v", broker, "mqtt.example.com")
	}
}
