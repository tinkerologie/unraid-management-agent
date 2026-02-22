package domain

import (
	"testing"
)

func TestDefaultMQTTConfig(t *testing.T) {
	cfg := DefaultMQTTConfig()

	if cfg.Enabled {
		t.Error("Expected Enabled to be false")
	}
	if cfg.Broker != "" {
		t.Errorf("Expected empty broker, got %q", cfg.Broker)
	}
	if cfg.Port != 1883 {
		t.Errorf("Expected port 1883, got %d", cfg.Port)
	}
	if cfg.Username != "" {
		t.Errorf("Expected empty username, got %q", cfg.Username)
	}
	if cfg.Password != "" {
		t.Errorf("Expected empty password, got %q", cfg.Password)
	}
	if cfg.ClientID != "unraid-management-agent" {
		t.Errorf("Expected client ID 'unraid-management-agent', got %q", cfg.ClientID)
	}
	if cfg.UseTLS {
		t.Error("Expected UseTLS to be false")
	}
	if cfg.InsecureSkipVerify {
		t.Error("Expected InsecureSkipVerify to be false")
	}
	if cfg.TopicPrefix != "unraid" {
		t.Errorf("Expected topic prefix 'unraid', got %q", cfg.TopicPrefix)
	}
	if cfg.QoS != 0 {
		t.Errorf("Expected QoS 0, got %d", cfg.QoS)
	}
	if !cfg.RetainMessages {
		t.Error("Expected RetainMessages to be true")
	}
	if cfg.HomeAssistantMode {
		t.Error("Expected HomeAssistantMode to be false")
	}
	if cfg.HomeAssistantPrefix != "homeassistant" {
		t.Errorf("Expected HA prefix 'homeassistant', got %q", cfg.HomeAssistantPrefix)
	}
	if !cfg.DiscoveryEnabled {
		t.Error("Expected DiscoveryEnabled to be true")
	}
}

func TestToDTOConfig_TCP(t *testing.T) {
	cfg := MQTTConfig{
		Enabled:             true,
		Broker:              "mqtt.example.com",
		Port:                1883,
		Username:            "user",
		Password:            "pass",
		ClientID:            "test-client",
		UseTLS:              false,
		TopicPrefix:         "home/unraid",
		QoS:                 1,
		RetainMessages:      true,
		HomeAssistantMode:   true,
		HomeAssistantPrefix: "ha",
	}

	dto := cfg.ToDTOConfig()

	if dto == nil {
		t.Fatal("ToDTOConfig returned nil")
	}
	if dto.Broker != "tcp://mqtt.example.com:1883" {
		t.Errorf("Expected broker 'tcp://mqtt.example.com:1883', got %q", dto.Broker)
	}
	if !dto.Enabled {
		t.Error("Expected Enabled to be true")
	}
	if dto.ClientID != "test-client" {
		t.Errorf("Expected client ID 'test-client', got %q", dto.ClientID)
	}
	if dto.Username != "user" {
		t.Errorf("Expected username 'user', got %q", dto.Username)
	}
	if dto.Password != "pass" {
		t.Errorf("Expected password 'pass', got %q", dto.Password)
	}
	if dto.TopicPrefix != "home/unraid" {
		t.Errorf("Expected topic prefix 'home/unraid', got %q", dto.TopicPrefix)
	}
	if dto.QoS != 1 {
		t.Errorf("Expected QoS 1, got %d", dto.QoS)
	}
	if !dto.RetainMessages {
		t.Error("Expected RetainMessages to be true")
	}
	if dto.ConnectTimeout != 30 {
		t.Errorf("Expected ConnectTimeout 30, got %d", dto.ConnectTimeout)
	}
	if dto.KeepAlive != 60 {
		t.Errorf("Expected KeepAlive 60, got %d", dto.KeepAlive)
	}
	if !dto.CleanSession {
		t.Error("Expected CleanSession to be true")
	}
	if !dto.AutoReconnect {
		t.Error("Expected AutoReconnect to be true")
	}
	if !dto.HomeAssistantMode {
		t.Error("Expected HomeAssistantMode to be true")
	}
	if dto.HADiscoveryPrefix != "ha" {
		t.Errorf("Expected HA prefix 'ha', got %q", dto.HADiscoveryPrefix)
	}
}

func TestToDTOConfig_TLS(t *testing.T) {
	cfg := MQTTConfig{
		Broker: "mqtt.example.com",
		Port:   8883,
		UseTLS: true,
	}

	dto := cfg.ToDTOConfig()

	if dto.Broker != "ssl://mqtt.example.com:8883" {
		t.Errorf("Expected broker 'ssl://mqtt.example.com:8883', got %q", dto.Broker)
	}
}

func TestToDTOConfig_EmptyBroker(t *testing.T) {
	cfg := MQTTConfig{
		Broker: "",
		Port:   1883,
	}

	dto := cfg.ToDTOConfig()

	if dto.Broker != "" {
		t.Errorf("Expected empty broker URL, got %q", dto.Broker)
	}
}

func TestToDTOConfig_ZeroPort(t *testing.T) {
	cfg := MQTTConfig{
		Broker: "mqtt.example.com",
		Port:   0,
	}

	dto := cfg.ToDTOConfig()

	// When port is 0, broker stays as raw string (no protocol/port formatting)
	if dto.Broker != "mqtt.example.com" {
		t.Errorf("Expected raw broker 'mqtt.example.com' when port is 0, got %q", dto.Broker)
	}
}

func TestContextFields(t *testing.T) {
	ctx := Context{
		Config: Config{
			Version: "2025.01.01",
			Port:    8043,
		},
		Intervals: Intervals{
			System: 15,
			Array:  30,
		},
		MQTTConfig: DefaultMQTTConfig(),
	}

	if ctx.Version != "2025.01.01" {
		t.Errorf("Expected version '2025.01.01', got %q", ctx.Version)
	}
	if ctx.Port != 8043 {
		t.Errorf("Expected port 8043, got %d", ctx.Port)
	}
	if ctx.Intervals.System != 15 {
		t.Errorf("Expected system interval 15, got %d", ctx.Intervals.System)
	}
}
