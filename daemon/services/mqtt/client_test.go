package mqtt

import (
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestNewClient(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config, "test-server", "1.0.0")

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}

	if client.config == nil {
		t.Error("config is nil")
	}

	if client.hostname != "test-server" {
		t.Errorf("hostname = %q, want %q", client.hostname, "test-server")
	}

	if client.agentVersion != "1.0.0" {
		t.Errorf("agentVersion = %q, want %q", client.agentVersion, "1.0.0")
	}

	if client.deviceInfo == nil {
		t.Error("deviceInfo is nil")
	}

	if client.deviceInfo.Name != "test-server" {
		t.Errorf("deviceInfo.Name = %q, want %q", client.deviceInfo.Name, "test-server")
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	if config.Enabled {
		t.Error("default config should be disabled")
	}

	if config.Broker != "tcp://localhost:1883" {
		t.Errorf("Broker = %q, want %q", config.Broker, "tcp://localhost:1883")
	}

	if config.ClientID != "unraid-management-agent" {
		t.Errorf("ClientID = %q, want %q", config.ClientID, "unraid-management-agent")
	}

	if config.TopicPrefix != "unraid" {
		t.Errorf("TopicPrefix = %q, want %q", config.TopicPrefix, "unraid")
	}

	if config.QoS != 1 {
		t.Errorf("QoS = %d, want %d", config.QoS, 1)
	}

	if !config.RetainMessages {
		t.Error("RetainMessages should be true by default")
	}

	if config.ConnectTimeout != 30 {
		t.Errorf("ConnectTimeout = %d, want %d", config.ConnectTimeout, 30)
	}

	if config.KeepAlive != 60 {
		t.Errorf("KeepAlive = %d, want %d", config.KeepAlive, 60)
	}

	if !config.CleanSession {
		t.Error("CleanSession should be true by default")
	}

	if !config.AutoReconnect {
		t.Error("AutoReconnect should be true by default")
	}

	if !config.HomeAssistantMode {
		t.Error("HomeAssistantMode should be true by default")
	}

	if config.HADiscoveryPrefix != "homeassistant" {
		t.Errorf("HADiscoveryPrefix = %q, want %q", config.HADiscoveryPrefix, "homeassistant")
	}
}

func TestClientIsConnected(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config, "test-server", "1.0.0")

	if client.IsConnected() {
		t.Error("new client should not be connected")
	}
}

func TestClientGetStatus(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	client := NewClient(config, "test-server", "1.0.0")

	status := client.GetStatus()

	if status == nil {
		t.Fatal("GetStatus() returned nil")
	}

	if status.Connected {
		t.Error("status.Connected should be false for new client")
	}

	if !status.Enabled {
		t.Error("status.Enabled should be true")
	}

	if status.Broker != "tcp://localhost:1883" {
		t.Errorf("status.Broker = %q, want %q", status.Broker, "tcp://localhost:1883")
	}

	if status.ClientID != "unraid-management-agent" {
		t.Errorf("status.ClientID = %q, want %q", status.ClientID, "unraid-management-agent")
	}

	if status.TopicPrefix != "unraid" {
		t.Errorf("status.TopicPrefix = %q, want %q", status.TopicPrefix, "unraid")
	}

	if status.MessagesSent != 0 {
		t.Errorf("status.MessagesSent = %d, want 0", status.MessagesSent)
	}

	if status.MessagesErrors != 0 {
		t.Errorf("status.MessagesErrors = %d, want 0", status.MessagesErrors)
	}

	if status.Timestamp.IsZero() {
		t.Error("status.Timestamp should not be zero")
	}
}

func TestClientGetConfig(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.Username = "testuser"
	config.Password = "secret123" // Password should not be returned
	client := NewClient(config, "test-server", "1.0.0")

	returnedConfig := client.GetConfig()

	if returnedConfig == nil {
		t.Fatal("GetConfig() returned nil")
	}

	if !returnedConfig.Enabled {
		t.Error("Enabled should be true")
	}

	if returnedConfig.Username != "testuser" {
		t.Errorf("Username = %q, want %q", returnedConfig.Username, "testuser")
	}

	// Password should not be exposed in the returned config
	if returnedConfig.Password != "" {
		t.Error("Password should be empty in returned config (security)")
	}

	if returnedConfig.Broker != config.Broker {
		t.Errorf("Broker = %q, want %q", returnedConfig.Broker, config.Broker)
	}
}

func TestClientGetTopics(t *testing.T) {
	config := DefaultConfig()
	config.TopicPrefix = "unraid"
	client := NewClient(config, "test-server", "1.0.0")

	topics := client.GetTopics()

	if topics == nil {
		t.Fatal("GetTopics() returned nil")
	}

	testCases := []struct {
		name     string
		got      string
		expected string
	}{
		{"Status", topics.Status, "unraid/status"},
		{"System", topics.System, "unraid/system"},
		{"Array", topics.Array, "unraid/array"},
		{"Disks", topics.Disks, "unraid/disks"},
		{"Containers", topics.Containers, "unraid/docker/containers"},
		{"VMs", topics.VMs, "unraid/vm/list"},
		{"UPS", topics.UPS, "unraid/ups"},
		{"GPU", topics.GPU, "unraid/gpu"},
		{"Network", topics.Network, "unraid/network"},
		{"Shares", topics.Shares, "unraid/shares"},
		{"Notification", topics.Notification, "unraid/notifications"},
		{"Availability", topics.Availability, "unraid/availability"},
	}

	for _, tc := range testCases {
		if tc.got != tc.expected {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.expected)
		}
	}
}

func TestClientGetTopicsWithEmptyPrefix(t *testing.T) {
	config := DefaultConfig()
	config.TopicPrefix = ""
	client := NewClient(config, "test-server", "1.0.0")

	topics := client.GetTopics()

	if topics.System != "system" {
		t.Errorf("System = %q, want %q", topics.System, "system")
	}

	if topics.Array != "array" {
		t.Errorf("Array = %q, want %q", topics.Array, "array")
	}
}

func TestClientGetTopicsWithCustomPrefix(t *testing.T) {
	config := DefaultConfig()
	config.TopicPrefix = "homelab/server1"
	client := NewClient(config, "test-server", "1.0.0")

	topics := client.GetTopics()

	if topics.System != "homelab/server1/system" {
		t.Errorf("System = %q, want %q", topics.System, "homelab/server1/system")
	}
}

func TestBuildTopic(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		suffix   string
		expected string
	}{
		{
			name:     "with prefix",
			prefix:   "unraid",
			suffix:   "system",
			expected: "unraid/system",
		},
		{
			name:     "empty prefix",
			prefix:   "",
			suffix:   "system",
			expected: "system",
		},
		{
			name:     "nested suffix",
			prefix:   "unraid",
			suffix:   "docker/containers",
			expected: "unraid/docker/containers",
		},
		{
			name:     "custom prefix",
			prefix:   "homelab/server1",
			suffix:   "system",
			expected: "homelab/server1/system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.TopicPrefix = tt.prefix
			client := NewClient(config, "test", "1.0.0")

			got := client.buildTopic(tt.suffix)
			if got != tt.expected {
				t.Errorf("buildTopic(%q) = %q, want %q", tt.suffix, got, tt.expected)
			}
		})
	}
}

func TestShouldPublish(t *testing.T) {
	// Test shouldPublish logic with various conditions
	// Note: shouldPublish also checks c.client != nil, so without an actual
	// MQTT client connection, it will always return false

	t.Run("disabled client", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = false
		client := NewClient(config, "test", "1.0.0")

		got := client.shouldPublish()
		if got != false {
			t.Errorf("shouldPublish() = %v, want false (disabled)", got)
		}
	})

	t.Run("enabled but not connected", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		client := NewClient(config, "test", "1.0.0")

		got := client.shouldPublish()
		if got != false {
			t.Errorf("shouldPublish() = %v, want false (not connected)", got)
		}
	})

	t.Run("enabled connected but no client", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		client := NewClient(config, "test", "1.0.0")
		client.connected.Store(true)

		// Without c.client being set, shouldPublish returns false
		got := client.shouldPublish()
		if got != false {
			t.Errorf("shouldPublish() = %v, want false (no client instance)", got)
		}
	})
}

func TestPublishMethodsWithoutConnection(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	client := NewClient(config, "test", "1.0.0")

	// All publish methods should return nil when not enabled/connected
	t.Run("PublishSystemInfo", func(t *testing.T) {
		err := client.PublishSystemInfo(&dto.SystemInfo{})
		if err != nil {
			t.Errorf("PublishSystemInfo() error = %v, want nil", err)
		}
	})

	t.Run("PublishArrayStatus", func(t *testing.T) {
		err := client.PublishArrayStatus(&dto.ArrayStatus{})
		if err != nil {
			t.Errorf("PublishArrayStatus() error = %v, want nil", err)
		}
	})

	t.Run("PublishDisks", func(t *testing.T) {
		err := client.PublishDisks([]dto.DiskInfo{})
		if err != nil {
			t.Errorf("PublishDisks() error = %v, want nil", err)
		}
	})

	t.Run("PublishContainers", func(t *testing.T) {
		err := client.PublishContainers([]dto.ContainerInfo{})
		if err != nil {
			t.Errorf("PublishContainers() error = %v, want nil", err)
		}
	})

	t.Run("PublishVMs", func(t *testing.T) {
		err := client.PublishVMs([]dto.VMInfo{})
		if err != nil {
			t.Errorf("PublishVMs() error = %v, want nil", err)
		}
	})

	t.Run("PublishUPSStatus", func(t *testing.T) {
		err := client.PublishUPSStatus(&dto.UPSStatus{})
		if err != nil {
			t.Errorf("PublishUPSStatus() error = %v, want nil", err)
		}
	})

	t.Run("PublishGPUMetrics", func(t *testing.T) {
		err := client.PublishGPUMetrics([]*dto.GPUMetrics{})
		if err != nil {
			t.Errorf("PublishGPUMetrics() error = %v, want nil", err)
		}
	})

	t.Run("PublishNetworkInfo", func(t *testing.T) {
		err := client.PublishNetworkInfo([]dto.NetworkInfo{})
		if err != nil {
			t.Errorf("PublishNetworkInfo() error = %v, want nil", err)
		}
	})

	t.Run("PublishShares", func(t *testing.T) {
		err := client.PublishShares([]dto.ShareInfo{})
		if err != nil {
			t.Errorf("PublishShares() error = %v, want nil", err)
		}
	})

	t.Run("PublishNotifications", func(t *testing.T) {
		err := client.PublishNotifications(&dto.NotificationList{})
		if err != nil {
			t.Errorf("PublishNotifications() error = %v, want nil", err)
		}
	})
}

func TestPublishCustomWithoutConnection(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	client := NewClient(config, "test", "1.0.0")
	// Not connected

	err := client.PublishCustom("test/topic", map[string]string{"key": "value"}, false)
	if err == nil {
		t.Error("PublishCustom() should return error when not connected")
	}
}

func TestClientDisconnect(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config, "test", "1.0.0")

	// Disconnect should not panic when client is nil
	client.Disconnect()

	if client.IsConnected() {
		t.Error("client should not be connected after Disconnect()")
	}
}

func TestMessageCounters(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config, "test", "1.0.0")

	// Initial counters should be zero
	status := client.GetStatus()
	if status.MessagesSent != 0 {
		t.Errorf("initial MessagesSent = %d, want 0", status.MessagesSent)
	}
	if status.MessagesErrors != 0 {
		t.Errorf("initial MessagesErrors = %d, want 0", status.MessagesErrors)
	}
}

func TestHandleConnect(t *testing.T) {
	config := DefaultConfig()
	config.HomeAssistantMode = false // Disable HA discovery for this test
	client := NewClient(config, "test-server", "1.0.0")

	// Simulate connection
	client.handleConnect()

	if !client.IsConnected() {
		t.Error("IsConnected() should be true after handleConnect()")
	}

	if client.lastConnect == nil {
		t.Error("lastConnect should be set after handleConnect()")
	}

	if client.lastError != "" {
		t.Errorf("lastError = %q, want empty", client.lastError)
	}
}

func TestHandleDisconnect(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config, "test-server", "1.0.0")

	// First connect, then disconnect
	client.connected.Store(true)

	client.handleDisconnect(nil)

	if client.IsConnected() {
		t.Error("IsConnected() should be false after handleDisconnect()")
	}

	if client.lastDisconn == nil {
		t.Error("lastDisconn should be set after handleDisconnect()")
	}
}

func TestHandleDisconnectWithError(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config, "test-server", "1.0.0")
	client.connected.Store(true)

	testErr := &testError{msg: "connection lost"}
	client.handleDisconnect(testErr)

	if client.lastError != "connection lost" {
		t.Errorf("lastError = %q, want %q", client.lastError, "connection lost")
	}
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestDeviceInfo(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config, "My Server Name", "2.0.0")

	if client.deviceInfo == nil {
		t.Fatal("deviceInfo is nil")
	}

	if len(client.deviceInfo.Identifiers) != 1 {
		t.Errorf("Identifiers length = %d, want 1", len(client.deviceInfo.Identifiers))
	}

	expectedID := "unraid_My_Server_Name"
	if client.deviceInfo.Identifiers[0] != expectedID {
		t.Errorf("Identifiers[0] = %q, want %q", client.deviceInfo.Identifiers[0], expectedID)
	}

	if client.deviceInfo.Name != "My Server Name" {
		t.Errorf("Name = %q, want %q", client.deviceInfo.Name, "My Server Name")
	}

	if client.deviceInfo.Manufacturer != "Lime Technology" {
		t.Errorf("Manufacturer = %q, want %q", client.deviceInfo.Manufacturer, "Lime Technology")
	}

	if client.deviceInfo.Model != "Unraid Server" {
		t.Errorf("Model = %q, want %q", client.deviceInfo.Model, "Unraid Server")
	}

	if client.deviceInfo.SWVersion != "2.0.0" {
		t.Errorf("SWVersion = %q, want %q", client.deviceInfo.SWVersion, "2.0.0")
	}
}

func TestTestConnection(t *testing.T) {
	// Test with invalid broker (should fail fast)
	result := TestConnection("tcp://invalid-broker:1883", "", "", "test-client", 1*time.Second)

	if result == nil {
		t.Fatal("TestConnection() returned nil")
	}

	// Connection should fail to invalid broker
	if result.Success {
		t.Error("TestConnection() should fail for invalid broker")
	}

	if result.Message == "" {
		t.Error("Message should not be empty on failure")
	}

	if result.Latency <= 0 {
		t.Error("Latency should be > 0")
	}

	if result.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestTestConnectionWithEmptyClientID(t *testing.T) {
	// Test that empty client ID gets a default
	result := TestConnection("tcp://invalid:1883", "", "", "", 500*time.Millisecond)

	if result == nil {
		t.Fatal("TestConnection() returned nil")
	}

	// Should not panic and should return a result
	if result.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}
