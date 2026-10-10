package domain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFile(t *testing.T) {
	t.Run("nonexistent file returns nil without error", func(t *testing.T) {
		cfg, err := LoadConfigFile(filepath.Join(t.TempDir(), "missing.yml"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg != nil {
			t.Errorf("expected nil config, got %+v", cfg)
		}
	})

	t.Run("valid config is parsed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yml")
		content := `
port: 9090
log_level: debug
read_only: true
tool_policy:
  container_start: ask
mqtt:
  enabled: true
  broker: mqtt.example.com
  port: 8883
discovery:
  enabled: false
  service_name: my-agent
intervals:
  system: 20
`
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		cfg, err := LoadConfigFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg == nil {
			t.Fatal("expected non-nil config")
		}
		if cfg.Port == nil || *cfg.Port != 9090 {
			t.Errorf("Port = %v, want 9090", cfg.Port)
		}
		if cfg.LogLevel == nil || *cfg.LogLevel != "debug" {
			t.Errorf("LogLevel = %v, want debug", cfg.LogLevel)
		}
		if cfg.ReadOnly == nil || !*cfg.ReadOnly {
			t.Errorf("ReadOnly = %v, want true", cfg.ReadOnly)
		}
		if got := cfg.ToolPolicy["container_start"]; got != "ask" {
			t.Errorf("ToolPolicy[container_start] = %q, want ask", got)
		}
		if cfg.MQTT == nil || cfg.MQTT.Broker == nil || *cfg.MQTT.Broker != "mqtt.example.com" {
			t.Errorf("MQTT broker not parsed: %+v", cfg.MQTT)
		}
		if cfg.Discovery == nil || cfg.Discovery.Enabled == nil || *cfg.Discovery.Enabled {
			t.Errorf("Discovery.Enabled = %v, want false", cfg.Discovery)
		}
		if cfg.Intervals == nil || cfg.Intervals.System == nil || *cfg.Intervals.System != 20 {
			t.Errorf("Intervals.System not parsed: %+v", cfg.Intervals)
		}
	})

	t.Run("invalid YAML returns error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.yml")
		if err := os.WriteFile(path, []byte("port: [not a number\n  broken:"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if _, err := LoadConfigFile(path); err == nil {
			t.Error("expected error for malformed YAML, got nil")
		}
	})
}

func TestDefaultDiscoveryConfig(t *testing.T) {
	cfg := DefaultDiscoveryConfig()
	if !cfg.Enabled {
		t.Error("expected discovery enabled by default")
	}
	if cfg.ServiceName != "" {
		t.Errorf("ServiceName = %q, want empty (hostname default)", cfg.ServiceName)
	}
}
