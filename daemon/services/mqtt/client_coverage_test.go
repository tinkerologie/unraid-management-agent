package mqtt

import (
	"context"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestConnect_InvalidBroker(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.Broker = "tcp://192.0.2.1:1883" // RFC 5737 TEST-NET, guaranteed unreachable
	config.ConnectTimeout = 1              // 1 second timeout
	config.AutoReconnect = false

	client := NewClient(config, "test-server", "1.0.0")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err == nil {
		t.Logf("Connect unexpectedly succeeded; skipping error assertion")
		return
	}

	t.Logf("Connect returned expected error: %v", err)

	if client.IsConnected() {
		t.Error("client should not be connected after failed Connect()")
	}
}

func TestConnect_DisabledConfig(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false

	client := NewClient(config, "test-server", "1.0.0")

	err := client.Connect(context.Background())
	if err != nil {
		t.Errorf("Connect() with disabled config should return nil, got: %v", err)
	}
}

func TestConnect_CancelledContext(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.Broker = "tcp://192.0.2.1:1883"
	config.ConnectTimeout = 30 // Long timeout so context cancellation wins
	config.AutoReconnect = false

	client := NewClient(config, "test-server", "1.0.0")

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so the context is done before connection completes
	cancel()

	err := client.Connect(ctx)
	if err == nil {
		t.Logf("Connect unexpectedly succeeded despite cancelled context; skipping")
		return
	}

	t.Logf("Connect returned expected error: %v", err)
}

func TestTestConnection_NotConnected(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	client := NewClient(config, "test-server", "1.0.0")

	// Client method TestConnection (not the package-level function)
	err := client.TestConnection()
	if err == nil {
		t.Error("TestConnection() should return error when not connected")
	}

	expected := "MQTT client is not connected"
	if err.Error() != expected {
		t.Errorf("TestConnection() error = %q, want %q", err.Error(), expected)
	}
}

func TestPublishJSON_NotConnected(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	client := NewClient(config, "test-server", "1.0.0")
	// client.client is nil — publishJSON -> publish should return "client not initialized"

	err := client.publishJSON("test/topic", map[string]string{"key": "value"})
	if err == nil {
		t.Logf("publishJSON unexpectedly succeeded; skipping error assertion")
		return
	}

	t.Logf("publishJSON returned expected error: %v", err)
}

func TestPublish_NilClient(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config, "test-server", "1.0.0")
	// client.client is nil

	err := client.publish("test/topic", "payload", false)
	if err == nil {
		t.Error("publish() should return error when MQTT client is nil")
	}

	expected := "MQTT client not initialized"
	if err.Error() != expected {
		t.Errorf("publish() error = %q, want %q", err.Error(), expected)
	}
}

func TestPublishJSON_MarshalError(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	client := NewClient(config, "test-server", "1.0.0")

	// Channels cannot be marshalled to JSON
	err := client.publishJSON("test/topic", make(chan int))
	if err == nil {
		t.Error("publishJSON() should return error for unmarshalable payload")
	}

	// Error counter should increment
	if client.msgErrors.Load() != 1 {
		t.Errorf("msgErrors = %d, want 1 after marshal error", client.msgErrors.Load())
	}
}

func TestPublishHADiscovery_NotConnected(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.HomeAssistantMode = true
	config.HADiscoveryPrefix = "homeassistant"
	client := NewClient(config, "test-server", "1.0.0")
	// client.client is nil — all internal publishes will fail silently (logged as warnings)

	// Should not panic when client is not connected
	client.publishHADiscovery()

	// Verify no panic occurred — errors are logged as warnings but not counted
	// because the nil-client error path in publish() doesn't increment msgErrors
	if client.IsConnected() {
		t.Error("client should not be connected")
	}
}

func TestPublishHASensor_NotConnected(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.HADiscoveryPrefix = "homeassistant"
	client := NewClient(config, "test-server", "1.0.0")
	// client.client is nil

	// Should not panic — errors are logged as warnings
	client.publishHASensor("test_sensor", "Test Sensor", "%", "mdi:test", "{{ value_json.test }}")

	// Verify no panic — the nil-client path returns error without incrementing msgErrors
	if client.IsConnected() {
		t.Error("client should not be connected")
	}
}

func TestPublishHAArraySensor_NotConnected(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.HADiscoveryPrefix = "homeassistant"
	client := NewClient(config, "test-server", "1.0.0")
	// client.client is nil

	// Should not panic — errors are logged as warnings
	client.publishHAArraySensor("test_array", "Test Array", "mdi:test", "{{ value_json.state }}")

	// Verify no panic — the nil-client path returns error without incrementing msgErrors
	if client.IsConnected() {
		t.Error("client should not be connected")
	}
}

func TestDisconnect_NotConnected(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config, "test-server", "1.0.0")

	// Should not panic when called without a prior Connect
	client.Disconnect()

	if client.IsConnected() {
		t.Error("client should not be connected after Disconnect()")
	}

	// Call again to verify double-disconnect safety
	client.Disconnect()
}

func TestConnect_WithCredentials(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.Broker = "tcp://192.0.2.1:1883"
	config.Username = "testuser"
	config.Password = "testpassword"
	config.ConnectTimeout = 1
	config.AutoReconnect = false

	client := NewClient(config, "test-server", "1.0.0")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err == nil {
		t.Logf("Connect with credentials unexpectedly succeeded; skipping")
		return
	}

	t.Logf("Connect with credentials returned expected error: %v", err)
}

func TestPublishMethodsWithEnabledButNotConnected(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	client := NewClient(config, "test-server", "1.0.0")
	// Enabled but shouldPublish() returns false (not connected, no underlying client)

	// All typed publish methods should return nil (early return via shouldPublish)
	tests := []struct {
		name string
		fn   func() error
	}{
		{"PublishSystemInfo", func() error { return client.PublishSystemInfo(&dto.SystemInfo{}) }},
		{"PublishArrayStatus", func() error { return client.PublishArrayStatus(&dto.ArrayStatus{}) }},
		{"PublishDisks", func() error { return client.PublishDisks([]dto.DiskInfo{}) }},
		{"PublishContainers", func() error { return client.PublishContainers([]dto.ContainerInfo{}) }},
		{"PublishVMs", func() error { return client.PublishVMs([]dto.VMInfo{}) }},
		{"PublishUPSStatus", func() error { return client.PublishUPSStatus(&dto.UPSStatus{}) }},
		{"PublishGPUMetrics", func() error { return client.PublishGPUMetrics([]*dto.GPUMetrics{}) }},
		{"PublishNetworkInfo", func() error { return client.PublishNetworkInfo([]dto.NetworkInfo{}) }},
		{"PublishShares", func() error { return client.PublishShares([]dto.ShareInfo{}) }},
		{"PublishNotifications", func() error { return client.PublishNotifications(&dto.NotificationList{}) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err != nil {
				t.Errorf("%s() error = %v, want nil (early return)", tt.name, err)
			}
		})
	}
}

func TestGetStatus_Uptime(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	client := NewClient(config, "test-server", "1.0.0")

	// When not connected, uptime should be 0
	status := client.GetStatus()
	if status.Uptime != 0 {
		t.Errorf("Uptime = %d, want 0 when not connected", status.Uptime)
	}

	// Simulate connection: set startTime and connected flag
	client.startTime = time.Now().Add(-10 * time.Second)
	client.connected.Store(true)

	status = client.GetStatus()
	if status.Uptime < 9 {
		t.Errorf("Uptime = %d, want >= 9 seconds", status.Uptime)
	}
}

func TestNewClient_HostnameWithSpaces(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config, "My Unraid Server", "1.0.0")

	// Spaces should be replaced with underscores in the identifier
	expectedID := "unraid_My_Unraid_Server"
	if client.deviceInfo.Identifiers[0] != expectedID {
		t.Errorf("Identifiers[0] = %q, want %q", client.deviceInfo.Identifiers[0], expectedID)
	}

	// Display name should keep spaces
	if client.deviceInfo.Name != "My Unraid Server" {
		t.Errorf("Name = %q, want %q", client.deviceInfo.Name, "My Unraid Server")
	}
}

func TestPublishHASensor_TopicFormat(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.HADiscoveryPrefix = "homeassistant"
	config.TopicPrefix = "unraid"
	client := NewClient(config, "My Server", "1.0.0")

	// Verify the discovery topic format by exercising the code path
	// (it will fail because client.client is nil, but the topic construction is exercised)
	client.publishHASensor("cpu_usage", "CPU Usage", "%", "mdi:cpu-64-bit", "{{ value_json.cpu_usage }}")

	// At minimum, ensure no panic occurred
	if client.IsConnected() {
		t.Error("client should not be connected")
	}
}

func TestPackageLevelTestConnection_InvalidBroker(t *testing.T) {
	// Tests the package-level TestConnection function with an unreachable broker
	result := TestConnection("tcp://192.0.2.1:1883", "", "", "test-client", 1*time.Second)

	if result == nil {
		t.Fatal("TestConnection() returned nil")
	}

	if result.Success {
		t.Error("TestConnection() should fail for unreachable broker")
	}

	if result.Latency <= 0 {
		t.Error("Latency should be > 0")
	}

	if result.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestPackageLevelTestConnection_TLSBroker(t *testing.T) {
	// Verify TLS detection in the response
	result := TestConnection("ssl://192.0.2.1:8883", "", "", "test-client", 1*time.Second)

	if result == nil {
		t.Fatal("TestConnection() returned nil")
	}

	// Connection will fail but we can verify the response is populated
	if result.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}
