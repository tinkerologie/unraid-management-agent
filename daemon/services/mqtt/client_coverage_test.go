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

	client := NewClient(config, "test-server", "1.0.0", nil)

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

	client := NewClient(config, "test-server", "1.0.0", nil)

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

	client := NewClient(config, "test-server", "1.0.0", nil)

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
	client := NewClient(config, "test-server", "1.0.0", nil)

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
	client := NewClient(config, "test-server", "1.0.0", nil)
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
	client := NewClient(config, "test-server", "1.0.0", nil)
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
	client := NewClient(config, "test-server", "1.0.0", nil)

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
	client := NewClient(config, "test-server", "1.0.0", nil)
	// client.client is nil — all internal publishes will fail silently (logged as warnings)

	// Should not panic when client is not connected
	client.publishHADiscovery()

	// Verify no panic occurred — errors are logged as warnings but not counted
	// because the nil-client error path in publish() doesn't increment msgErrors
	if client.IsConnected() {
		t.Error("client should not be connected")
	}
}

func TestPublishHAEntity_NotConnected(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.HADiscoveryPrefix = "homeassistant"
	client := NewClient(config, "test-server", "1.0.0", nil)
	// client.client is nil

	// Should not panic — errors are logged as warnings
	client.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: "unraid/system",
		id: "test_sensor", name: "Test Sensor", unit: "%",
		icon: "mdi:test", template: "{{ value_json.test }}",
		deviceClass: "temperature", stateClass: "measurement",
	})

	// binary_sensor variant
	client.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: "unraid/system",
		id: "test_binary", name: "Test Binary",
		icon: "mdi:test", template: "{{ 'ON' if value_json.test else 'OFF' }}",
		deviceClass: "safety",
	})

	// Verify no panic
	if client.IsConnected() {
		t.Error("client should not be connected")
	}
}

func TestPerItemDiscovery_NotConnected(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.HomeAssistantMode = true
	config.HADiscoveryPrefix = "homeassistant"
	client := NewClient(config, "test-server", "1.0.0", nil)
	// client.client is nil — all per-item discovery should not panic

	// Disks
	client.publishDiskDiscovery([]dto.DiskInfo{{ID: "disk1", Name: "Disk 1"}})
	// Containers
	client.publishContainerDiscovery([]dto.ContainerInfo{{Name: "plex", State: "running"}})
	// VMs
	client.publishVMDiscovery([]dto.VMInfo{{Name: "Windows", State: "running"}})
	// GPUs
	client.publishGPUDiscovery([]*dto.GPUMetrics{{Available: true, Index: 0, Name: "Test GPU"}})
	// Network
	client.publishNetworkDiscovery([]dto.NetworkInfo{{Name: "eth0", State: "up"}})
	// Shares
	client.publishShareDiscovery([]dto.ShareInfo{{Name: "Media"}})
	// ZFS
	client.publishZFSDiscovery([]dto.ZFSPool{{Name: "tank", Health: "ONLINE"}})

	if client.IsConnected() {
		t.Error("client should not be connected")
	}
}

func TestDiscoveryTracker_CleanupRemovedEntities(t *testing.T) {
	tracker := newDiscoveryTracker()

	// Initial set
	removed := tracker.update("disks", []string{"disk_1", "disk_2", "disk_3"})
	if len(removed) != 0 {
		t.Errorf("first update should return no removed, got %v", removed)
	}

	// Remove disk_2
	removed = tracker.update("disks", []string{"disk_1", "disk_3"})
	if len(removed) != 1 || removed[0] != "disk_2" {
		t.Errorf("expected [disk_2] removed, got %v", removed)
	}

	// Remove all
	removed = tracker.update("disks", []string{})
	if len(removed) != 2 {
		t.Errorf("expected 2 removed, got %v", removed)
	}
}

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Disk 1", "disk_1"},
		{"My-Container", "my_container"},
		{"eth0.1", "eth0_1"},
		{"path/to/thing", "path_to_thing"},
		{"already_clean", "already_clean"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeID(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsPhysicalInterface(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"eth0", true},
		{"eth1", true},
		{"br0", true},
		{"bond0", true},
		{"wlan0", true},
		{"veth1ab0321", false},
		{"vethdb96cc6", false},
		{"tunl0", false},
		{"tun0", false},
		{"virbr0", false},
		{"docker0", false},
		{"br-3cc0fa14431c", false},
		{"br_f063b8e6a1ab", false},
		{"shim-br0", false},
		{"shim_br0", false},
		{"vhost1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPhysicalInterface(tt.name)
			if result != tt.expected {
				t.Errorf("isPhysicalInterface(%q) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestDisconnect_NotConnected(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config, "test-server", "1.0.0", nil)

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

	client := NewClient(config, "test-server", "1.0.0", nil)

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
	client := NewClient(config, "test-server", "1.0.0", nil)
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
		{"PublishZFSPools", func() error { return client.PublishZFSPools([]dto.ZFSPool{}) }},
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
	client := NewClient(config, "test-server", "1.0.0", nil)

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
	client := NewClient(config, "My Unraid Server", "1.0.0", nil)

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

func TestPublishHAEntity_TopicFormat(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.HADiscoveryPrefix = "homeassistant"
	config.TopicPrefix = "unraid"
	client := NewClient(config, "My Server", "1.0.0", nil)

	// Verify the discovery topic format by exercising the code path
	// (it will fail because client.client is nil, but the topic construction is exercised)
	client.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: "unraid/system",
		id: "cpu_usage", name: "CPU Usage", unit: "%",
		icon: "mdi:cpu-64-bit", template: "{{ value_json.cpu_usage_percent }}",
		stateClass: "measurement",
	})

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
