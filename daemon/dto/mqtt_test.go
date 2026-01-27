package dto

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMQTTConfig(t *testing.T) {
	config := MQTTConfig{
		Enabled:           true,
		Broker:            "tcp://localhost:1883",
		Username:          "user",
		Password:          "secret",
		ClientID:          "test-client",
		TopicPrefix:       "unraid",
		QoS:               1,
		RetainMessages:    true,
		ConnectTimeout:    30,
		KeepAlive:         60,
		CleanSession:      true,
		AutoReconnect:     true,
		HomeAssistantMode: true,
		HADiscoveryPrefix: "homeassistant",
	}

	// Test JSON marshaling
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal MQTTConfig: %v", err)
	}

	var decoded MQTTConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal MQTTConfig: %v", err)
	}

	if decoded.Enabled != config.Enabled {
		t.Errorf("Enabled = %v, want %v", decoded.Enabled, config.Enabled)
	}
	if decoded.Broker != config.Broker {
		t.Errorf("Broker = %q, want %q", decoded.Broker, config.Broker)
	}
	if decoded.Username != config.Username {
		t.Errorf("Username = %q, want %q", decoded.Username, config.Username)
	}
	// Password has json:"-" tag, so it should NOT be serialized (security feature)
	if decoded.Password != "" {
		t.Errorf("Password should not be serialized, got %q", decoded.Password)
	}
	if decoded.ClientID != config.ClientID {
		t.Errorf("ClientID = %q, want %q", decoded.ClientID, config.ClientID)
	}
	if decoded.TopicPrefix != config.TopicPrefix {
		t.Errorf("TopicPrefix = %q, want %q", decoded.TopicPrefix, config.TopicPrefix)
	}
	if decoded.QoS != config.QoS {
		t.Errorf("QoS = %d, want %d", decoded.QoS, config.QoS)
	}
	if decoded.RetainMessages != config.RetainMessages {
		t.Errorf("RetainMessages = %v, want %v", decoded.RetainMessages, config.RetainMessages)
	}
	if decoded.ConnectTimeout != config.ConnectTimeout {
		t.Errorf("ConnectTimeout = %d, want %d", decoded.ConnectTimeout, config.ConnectTimeout)
	}
	if decoded.KeepAlive != config.KeepAlive {
		t.Errorf("KeepAlive = %d, want %d", decoded.KeepAlive, config.KeepAlive)
	}
	if decoded.CleanSession != config.CleanSession {
		t.Errorf("CleanSession = %v, want %v", decoded.CleanSession, config.CleanSession)
	}
	if decoded.AutoReconnect != config.AutoReconnect {
		t.Errorf("AutoReconnect = %v, want %v", decoded.AutoReconnect, config.AutoReconnect)
	}
	if decoded.HomeAssistantMode != config.HomeAssistantMode {
		t.Errorf("HomeAssistantMode = %v, want %v", decoded.HomeAssistantMode, config.HomeAssistantMode)
	}
	if decoded.HADiscoveryPrefix != config.HADiscoveryPrefix {
		t.Errorf("HADiscoveryPrefix = %q, want %q", decoded.HADiscoveryPrefix, config.HADiscoveryPrefix)
	}
}

func TestMQTTConfigPasswordNotSerialized(t *testing.T) {
	// Test that password is never serialized to JSON (security feature)
	config := MQTTConfig{
		Enabled:  true,
		Broker:   "tcp://localhost:1883",
		Password: "super_secret_password",
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)

	// Password should NOT appear in JSON output
	if contains(jsonStr, "password") {
		t.Errorf("Password should not be serialized to JSON: %s", jsonStr)
	}
	if contains(jsonStr, "super_secret_password") {
		t.Errorf("Password value should not appear in JSON: %s", jsonStr)
	}
}

func TestMQTTConfigJSONTags(t *testing.T) {
	config := MQTTConfig{
		Enabled:           true,
		Broker:            "tcp://localhost:1883",
		Username:          "testuser",
		Password:          "testpass", // Won't be serialized
		ClientID:          "my-client",
		TopicPrefix:       "home/unraid",
		QoS:               2,
		RetainMessages:    false,
		ConnectTimeout:    15,
		KeepAlive:         30,
		CleanSession:      false,
		AutoReconnect:     false,
		HomeAssistantMode: false,
		HADiscoveryPrefix: "ha",
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)

	// Check expected JSON keys (password excluded because json:"-")
	expectedKeys := []string{
		`"enabled":true`,
		`"broker":"tcp://localhost:1883"`,
		`"username":"testuser"`,
		`"client_id":"my-client"`,
		`"topic_prefix":"home/unraid"`,
		`"qos":2`,
		`"retain_messages":false`,
		`"connect_timeout":15`,
		`"keepalive":30`,
		`"clean_session":false`,
		`"auto_reconnect":false`,
		`"homeassistant_mode":false`,
		`"ha_discovery_prefix":"ha"`,
	}

	for _, key := range expectedKeys {
		if !contains(jsonStr, key) {
			t.Errorf("JSON missing expected key/value: %s\nJSON: %s", key, jsonStr)
		}
	}

	// Verify password is NOT in JSON
	if contains(jsonStr, `"password"`) {
		t.Errorf("Password should not be serialized (json:\"-\" tag)")
	}
}

func TestMQTTStatus(t *testing.T) {
	now := time.Now()
	status := MQTTStatus{
		Connected:      true,
		Enabled:        true,
		Broker:         "tcp://broker:1883",
		ClientID:       "test-client",
		TopicPrefix:    "unraid",
		LastConnected:  &now,
		LastDisconnect: nil,
		LastError:      "",
		MessagesSent:   100,
		MessagesErrors: 5,
		Uptime:         3600,
		Timestamp:      now,
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Failed to marshal MQTTStatus: %v", err)
	}

	var decoded MQTTStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal MQTTStatus: %v", err)
	}

	if decoded.Connected != status.Connected {
		t.Errorf("Connected = %v, want %v", decoded.Connected, status.Connected)
	}
	if decoded.Enabled != status.Enabled {
		t.Errorf("Enabled = %v, want %v", decoded.Enabled, status.Enabled)
	}
	if decoded.Broker != status.Broker {
		t.Errorf("Broker = %q, want %q", decoded.Broker, status.Broker)
	}
	if decoded.MessagesSent != status.MessagesSent {
		t.Errorf("MessagesSent = %d, want %d", decoded.MessagesSent, status.MessagesSent)
	}
	if decoded.MessagesErrors != status.MessagesErrors {
		t.Errorf("MessagesErrors = %d, want %d", decoded.MessagesErrors, status.MessagesErrors)
	}
	if decoded.Uptime != status.Uptime {
		t.Errorf("Uptime = %d, want %d", decoded.Uptime, status.Uptime)
	}
}

func TestMQTTMessage(t *testing.T) {
	msg := MQTTMessage{
		Topic:    "unraid/system",
		Payload:  `{"cpu":50.5}`,
		QoS:      1,
		Retained: true,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal MQTTMessage: %v", err)
	}

	var decoded MQTTMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal MQTTMessage: %v", err)
	}

	if decoded.Topic != msg.Topic {
		t.Errorf("Topic = %q, want %q", decoded.Topic, msg.Topic)
	}
	// Payload unmarshals as interface{}
	if decoded.QoS != msg.QoS {
		t.Errorf("QoS = %d, want %d", decoded.QoS, msg.QoS)
	}
	if decoded.Retained != msg.Retained {
		t.Errorf("Retained = %v, want %v", decoded.Retained, msg.Retained)
	}
}

func TestMQTTPublishRequest(t *testing.T) {
	req := MQTTPublishRequest{
		Topic:    "custom/topic",
		Payload:  map[string]interface{}{"key": "value", "num": 42},
		Retained: true,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal MQTTPublishRequest: %v", err)
	}

	var decoded MQTTPublishRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal MQTTPublishRequest: %v", err)
	}

	if decoded.Topic != req.Topic {
		t.Errorf("Topic = %q, want %q", decoded.Topic, req.Topic)
	}
	if decoded.Retained != req.Retained {
		t.Errorf("Retained = %v, want %v", decoded.Retained, req.Retained)
	}
	// Payload is interface{}, check it was preserved
	if decoded.Payload == nil {
		t.Error("Payload is nil, expected map")
	}
}

func TestMQTTPublishResponse(t *testing.T) {
	now := time.Now()
	resp := MQTTPublishResponse{
		Success:   true,
		Message:   "Message published",
		Topic:     "unraid/test",
		Timestamp: now,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal MQTTPublishResponse: %v", err)
	}

	var decoded MQTTPublishResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal MQTTPublishResponse: %v", err)
	}

	if decoded.Success != resp.Success {
		t.Errorf("Success = %v, want %v", decoded.Success, resp.Success)
	}
	if decoded.Message != resp.Message {
		t.Errorf("Message = %q, want %q", decoded.Message, resp.Message)
	}
	if decoded.Topic != resp.Topic {
		t.Errorf("Topic = %q, want %q", decoded.Topic, resp.Topic)
	}
}

func TestMQTTTestRequest(t *testing.T) {
	req := MQTTTestRequest{
		Broker:   "tcp://test:1883",
		Username: "admin",
		Password: "pass",
		ClientID: "test-conn",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal MQTTTestRequest: %v", err)
	}

	var decoded MQTTTestRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal MQTTTestRequest: %v", err)
	}

	if decoded.Broker != req.Broker {
		t.Errorf("Broker = %q, want %q", decoded.Broker, req.Broker)
	}
	if decoded.Username != req.Username {
		t.Errorf("Username = %q, want %q", decoded.Username, req.Username)
	}
	if decoded.Password != req.Password {
		t.Errorf("Password = %q, want %q", decoded.Password, req.Password)
	}
	if decoded.ClientID != req.ClientID {
		t.Errorf("ClientID = %q, want %q", decoded.ClientID, req.ClientID)
	}
}

func TestMQTTTestResponse(t *testing.T) {
	now := time.Now()
	resp := MQTTTestResponse{
		Success:   true,
		Message:   "Connection successful",
		Latency:   50,
		Timestamp: now,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal MQTTTestResponse: %v", err)
	}

	var decoded MQTTTestResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal MQTTTestResponse: %v", err)
	}

	if decoded.Success != resp.Success {
		t.Errorf("Success = %v, want %v", decoded.Success, resp.Success)
	}
	if decoded.Message != resp.Message {
		t.Errorf("Message = %q, want %q", decoded.Message, resp.Message)
	}
	if decoded.Latency != resp.Latency {
		t.Errorf("Latency = %d, want %d", decoded.Latency, resp.Latency)
	}
}

func TestHADiscoveryConfig(t *testing.T) {
	haConfig := HADiscoveryConfig{
		Name:              "CPU Usage",
		UniqueID:          "unraid_server_cpu_usage",
		StateTopic:        "unraid/system",
		AvailabilityTopic: "unraid/availability",
		DeviceClass:       "power_factor",
		UnitOfMeasurement: "%",
		ValueTemplate:     "{{ value_json.cpu_usage | round(1) }}",
		Icon:              "mdi:cpu-64-bit",
		Device: &HADeviceInfo{
			Identifiers:  []string{"unraid_myserver"},
			Name:         "My Server",
			Manufacturer: "Lime Technology",
			Model:        "Unraid Server",
			SWVersion:    "6.12.4",
		},
	}

	data, err := json.Marshal(haConfig)
	if err != nil {
		t.Fatalf("Failed to marshal HADiscoveryConfig: %v", err)
	}

	var decoded HADiscoveryConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal HADiscoveryConfig: %v", err)
	}

	if decoded.Name != haConfig.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, haConfig.Name)
	}
	if decoded.UniqueID != haConfig.UniqueID {
		t.Errorf("UniqueID = %q, want %q", decoded.UniqueID, haConfig.UniqueID)
	}
	if decoded.StateTopic != haConfig.StateTopic {
		t.Errorf("StateTopic = %q, want %q", decoded.StateTopic, haConfig.StateTopic)
	}
	if decoded.AvailabilityTopic != haConfig.AvailabilityTopic {
		t.Errorf("AvailabilityTopic = %q, want %q", decoded.AvailabilityTopic, haConfig.AvailabilityTopic)
	}
	if decoded.DeviceClass != haConfig.DeviceClass {
		t.Errorf("DeviceClass = %q, want %q", decoded.DeviceClass, haConfig.DeviceClass)
	}
	if decoded.UnitOfMeasurement != haConfig.UnitOfMeasurement {
		t.Errorf("UnitOfMeasurement = %q, want %q", decoded.UnitOfMeasurement, haConfig.UnitOfMeasurement)
	}
	if decoded.ValueTemplate != haConfig.ValueTemplate {
		t.Errorf("ValueTemplate = %q, want %q", decoded.ValueTemplate, haConfig.ValueTemplate)
	}
	if decoded.Icon != haConfig.Icon {
		t.Errorf("Icon = %q, want %q", decoded.Icon, haConfig.Icon)
	}
	if decoded.Device == nil {
		t.Fatal("Device is nil")
	}
	if decoded.Device.Name != haConfig.Device.Name {
		t.Errorf("Device.Name = %q, want %q", decoded.Device.Name, haConfig.Device.Name)
	}
}

func TestHADiscoveryConfigOmitEmpty(t *testing.T) {
	// Test that optional fields are omitted when empty
	haConfig := HADiscoveryConfig{
		Name:       "Test Sensor",
		UniqueID:   "test_sensor",
		StateTopic: "test/topic",
	}

	data, err := json.Marshal(haConfig)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)

	// These should not be present when empty
	unwantedKeys := []string{
		"availability_topic",
		"device_class",
		"state_class",
		"unit_of_measurement",
		"value_template",
		"icon",
		"device",
	}

	for _, key := range unwantedKeys {
		if contains(jsonStr, key) {
			t.Errorf("JSON should not contain %q when empty", key)
		}
	}
}

func TestHADeviceInfo(t *testing.T) {
	device := HADeviceInfo{
		Identifiers:  []string{"unraid_server", "unraid_nas"},
		Name:         "My Unraid Server",
		Manufacturer: "Lime Technology",
		Model:        "Unraid Server",
		SWVersion:    "6.12.4",
		HWVersion:    "1.0",
		ConfigURL:    "http://tower.local",
	}

	data, err := json.Marshal(device)
	if err != nil {
		t.Fatalf("Failed to marshal HADeviceInfo: %v", err)
	}

	var decoded HADeviceInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal HADeviceInfo: %v", err)
	}

	if len(decoded.Identifiers) != 2 {
		t.Errorf("Identifiers length = %d, want 2", len(decoded.Identifiers))
	}
	if decoded.Name != device.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, device.Name)
	}
	if decoded.Manufacturer != device.Manufacturer {
		t.Errorf("Manufacturer = %q, want %q", decoded.Manufacturer, device.Manufacturer)
	}
	if decoded.Model != device.Model {
		t.Errorf("Model = %q, want %q", decoded.Model, device.Model)
	}
	if decoded.SWVersion != device.SWVersion {
		t.Errorf("SWVersion = %q, want %q", decoded.SWVersion, device.SWVersion)
	}
	if decoded.HWVersion != device.HWVersion {
		t.Errorf("HWVersion = %q, want %q", decoded.HWVersion, device.HWVersion)
	}
	if decoded.ConfigURL != device.ConfigURL {
		t.Errorf("ConfigURL = %q, want %q", decoded.ConfigURL, device.ConfigURL)
	}
}

func TestMQTTTopics(t *testing.T) {
	topics := MQTTTopics{
		Status:       "unraid/status",
		System:       "unraid/system",
		Array:        "unraid/array",
		Disks:        "unraid/disks",
		Containers:   "unraid/docker/containers",
		VMs:          "unraid/vm/list",
		UPS:          "unraid/ups",
		GPU:          "unraid/gpu",
		Network:      "unraid/network",
		Shares:       "unraid/shares",
		Notification: "unraid/notifications",
		Availability: "unraid/availability",
	}

	data, err := json.Marshal(topics)
	if err != nil {
		t.Fatalf("Failed to marshal MQTTTopics: %v", err)
	}

	var decoded MQTTTopics
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal MQTTTopics: %v", err)
	}

	if decoded.Status != topics.Status {
		t.Errorf("Status = %q, want %q", decoded.Status, topics.Status)
	}
	if decoded.System != topics.System {
		t.Errorf("System = %q, want %q", decoded.System, topics.System)
	}
	if decoded.Array != topics.Array {
		t.Errorf("Array = %q, want %q", decoded.Array, topics.Array)
	}
	if decoded.Disks != topics.Disks {
		t.Errorf("Disks = %q, want %q", decoded.Disks, topics.Disks)
	}
	if decoded.Containers != topics.Containers {
		t.Errorf("Containers = %q, want %q", decoded.Containers, topics.Containers)
	}
	if decoded.VMs != topics.VMs {
		t.Errorf("VMs = %q, want %q", decoded.VMs, topics.VMs)
	}
	if decoded.UPS != topics.UPS {
		t.Errorf("UPS = %q, want %q", decoded.UPS, topics.UPS)
	}
	if decoded.GPU != topics.GPU {
		t.Errorf("GPU = %q, want %q", decoded.GPU, topics.GPU)
	}
	if decoded.Network != topics.Network {
		t.Errorf("Network = %q, want %q", decoded.Network, topics.Network)
	}
	if decoded.Shares != topics.Shares {
		t.Errorf("Shares = %q, want %q", decoded.Shares, topics.Shares)
	}
	if decoded.Notification != topics.Notification {
		t.Errorf("Notification = %q, want %q", decoded.Notification, topics.Notification)
	}
	if decoded.Availability != topics.Availability {
		t.Errorf("Availability = %q, want %q", decoded.Availability, topics.Availability)
	}
}

func TestMQTTEnableRequest(t *testing.T) {
	req := MQTTEnableRequest{
		Enabled: true,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal MQTTEnableRequest: %v", err)
	}

	var decoded MQTTEnableRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal MQTTEnableRequest: %v", err)
	}

	if decoded.Enabled != req.Enabled {
		t.Errorf("Enabled = %v, want %v", decoded.Enabled, req.Enabled)
	}
}

func TestMQTTConfigUpdateRequest(t *testing.T) {
	qos := 2
	retain := false
	haMode := true
	req := MQTTConfigUpdateRequest{
		Broker:            "tcp://new-broker:1883",
		Username:          "newuser",
		Password:          "newpass",
		TopicPrefix:       "homelab",
		QoS:               &qos,
		RetainMessages:    &retain,
		HomeAssistantMode: &haMode,
		HADiscoveryPrefix: "ha",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal MQTTConfigUpdateRequest: %v", err)
	}

	var decoded MQTTConfigUpdateRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal MQTTConfigUpdateRequest: %v", err)
	}

	if decoded.Broker != req.Broker {
		t.Errorf("Broker = %q, want %q", decoded.Broker, req.Broker)
	}
	if decoded.Username != req.Username {
		t.Errorf("Username = %q, want %q", decoded.Username, req.Username)
	}
	if decoded.Password != req.Password {
		t.Errorf("Password = %q, want %q", decoded.Password, req.Password)
	}
	if decoded.TopicPrefix != req.TopicPrefix {
		t.Errorf("TopicPrefix = %q, want %q", decoded.TopicPrefix, req.TopicPrefix)
	}
	if decoded.QoS == nil || *decoded.QoS != *req.QoS {
		t.Errorf("QoS = %v, want %v", decoded.QoS, req.QoS)
	}
	if decoded.RetainMessages == nil || *decoded.RetainMessages != *req.RetainMessages {
		t.Errorf("RetainMessages = %v, want %v", decoded.RetainMessages, req.RetainMessages)
	}
	if decoded.HomeAssistantMode == nil || *decoded.HomeAssistantMode != *req.HomeAssistantMode {
		t.Errorf("HomeAssistantMode = %v, want %v", decoded.HomeAssistantMode, req.HomeAssistantMode)
	}
	if decoded.HADiscoveryPrefix != req.HADiscoveryPrefix {
		t.Errorf("HADiscoveryPrefix = %q, want %q", decoded.HADiscoveryPrefix, req.HADiscoveryPrefix)
	}
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsAt(s, substr)
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
