// Package mqtt provides MQTT client functionality for the Unraid Management Agent.
// It enables publishing system metrics and events to MQTT brokers for integration
// with home automation systems like Home Assistant.
package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// Client represents an MQTT client that publishes Unraid metrics and events.
type Client struct {
	config       *dto.MQTTConfig
	client       pahomqtt.Client
	mu           sync.RWMutex
	connected    atomic.Bool
	startTime    time.Time
	lastConnect  *time.Time
	lastDisconn  *time.Time
	lastError    string
	msgSent      atomic.Int64
	msgErrors    atomic.Int64
	deviceInfo   *dto.HADeviceInfo
	hostname     string
	agentVersion string
	tracker      *discoveryTracker
	domainCtx    *domain.Context // domain context for controllers (array, system)

	// connectCancel cancels the context for goroutines spawned by handleConnect.
	// Protected by mu.
	connectCancel context.CancelFunc
}

// NewClient creates a new MQTT client with the given configuration.
func NewClient(config *dto.MQTTConfig, hostname, agentVersion string, domainCtx *domain.Context) *Client {
	return &Client{
		config:       config,
		hostname:     hostname,
		agentVersion: agentVersion,
		tracker:      newDiscoveryTracker(),
		domainCtx:    domainCtx,
		deviceInfo: &dto.HADeviceInfo{
			Identifiers:  []string{fmt.Sprintf("unraid_%s", strings.ReplaceAll(hostname, " ", "_"))},
			Name:         hostname,
			Manufacturer: "Lime Technology",
			Model:        "Unraid Server",
			SWVersion:    agentVersion,
		},
	}
}

// Connect establishes a connection to the MQTT broker.
func (c *Client) Connect(ctx context.Context) error {
	if !c.config.Enabled {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	opts := pahomqtt.NewClientOptions()
	opts.AddBroker(c.config.Broker)
	opts.SetClientID(c.config.ClientID)
	opts.SetCleanSession(c.config.CleanSession)
	opts.SetAutoReconnect(c.config.AutoReconnect)
	opts.SetConnectTimeout(time.Duration(c.config.ConnectTimeout) * time.Second)
	opts.SetKeepAlive(time.Duration(c.config.KeepAlive) * time.Second)

	if c.config.Username != "" {
		opts.SetUsername(c.config.Username)
	}
	if c.config.Password != "" {
		opts.SetPassword(c.config.Password)
	}

	// Set will message for availability
	availabilityTopic := c.buildTopic("availability")
	opts.SetWill(availabilityTopic, "offline", byte(c.config.QoS), true)

	// Connection handlers
	opts.SetOnConnectHandler(func(_ pahomqtt.Client) {
		c.handleConnect()
	})

	opts.SetConnectionLostHandler(func(_ pahomqtt.Client, err error) {
		c.handleDisconnect(err)
	})

	opts.SetReconnectingHandler(func(_ pahomqtt.Client, _ *pahomqtt.ClientOptions) {
		logger.Debug("MQTT: Attempting to reconnect...")
	})

	c.client = pahomqtt.NewClient(opts)
	c.startTime = time.Now()

	logger.Info("MQTT: Connecting to broker %s...", c.config.Broker)

	token := c.client.Connect()

	// Wait with context for connection
	done := make(chan bool)
	go func() {
		token.Wait()
		done <- true
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("connection cancelled: %w", ctx.Err())
	case <-done:
		if token.Error() != nil {
			c.lastError = token.Error().Error()
			return fmt.Errorf("failed to connect: %w", token.Error())
		}
	}

	return nil
}

// handleConnect is called when connection is established.
func (c *Client) handleConnect() {
	c.mu.Lock()
	// Cancel any goroutines from a previous connection cycle.
	if c.connectCancel != nil {
		c.connectCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.connectCancel = cancel
	c.mu.Unlock()

	c.connected.Store(true)
	now := time.Now()
	c.lastConnect = &now
	c.lastError = ""

	logger.Success("MQTT: Connected to broker %s", c.config.Broker)

	// Publish availability
	availabilityTopic := c.buildTopic("availability")
	_ = c.publish(availabilityTopic, "online", true)

	// Publish Home Assistant discovery if enabled
	if c.config.HomeAssistantMode {
		go func() {
			// Run discovery, initial states, then subscribe — sequentially —
			// so command subscriptions exist only after discovery completes.
			if ctx.Err() != nil {
				return
			}
			c.publishHADiscovery()
			if ctx.Err() != nil {
				return
			}
			c.publishServiceStates()
			if ctx.Err() != nil {
				return
			}
			c.subscribeCommandTopics()
		}()
	}
}

// handleDisconnect is called when connection is lost.
func (c *Client) handleDisconnect(err error) {
	c.connected.Store(false)
	now := time.Now()
	c.lastDisconn = &now

	// Cancel any in-flight connect goroutines.
	c.mu.Lock()
	if c.connectCancel != nil {
		c.connectCancel()
		c.connectCancel = nil
	}
	c.mu.Unlock()

	if err != nil {
		c.lastError = err.Error()
		logger.Warning("MQTT: Connection lost: %v", err)
	} else {
		logger.Info("MQTT: Disconnected from broker")
	}
}

// Disconnect closes the MQTT connection gracefully.
func (c *Client) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Cancel any in-flight connect goroutines.
	if c.connectCancel != nil {
		c.connectCancel()
		c.connectCancel = nil
	}

	if c.client != nil && c.client.IsConnected() {
		// Publish offline status
		availabilityTopic := c.buildTopic("availability")
		_ = c.publish(availabilityTopic, "offline", true)

		c.client.Disconnect(250)
		c.connected.Store(false)
		logger.Info("MQTT: Disconnected from broker")
	}
}

// IsConnected returns true if the client is connected to the broker.
func (c *Client) IsConnected() bool {
	return c.connected.Load()
}

// TestConnection tests the MQTT connection by attempting a quick connect/disconnect.
func (c *Client) TestConnection() error {
	if !c.IsConnected() {
		return fmt.Errorf("MQTT client is not connected")
	}

	// Publish a test message to verify the connection is working
	testTopic := fmt.Sprintf("%s/test", c.config.TopicPrefix)
	testPayload := map[string]any{
		"test":      true,
		"timestamp": time.Now().Unix(),
	}

	if err := c.publishJSON(testTopic, testPayload); err != nil {
		return fmt.Errorf("failed to publish test message: %w", err)
	}

	return nil
}

// GetStatus returns the current MQTT client status.
func (c *Client) GetStatus() *dto.MQTTStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var uptime int64
	if !c.startTime.IsZero() && c.connected.Load() {
		uptime = int64(time.Since(c.startTime).Seconds())
	}

	return &dto.MQTTStatus{
		Connected:      c.connected.Load(),
		Enabled:        c.config.Enabled,
		Broker:         c.config.Broker,
		ClientID:       c.config.ClientID,
		TopicPrefix:    c.config.TopicPrefix,
		LastConnected:  c.lastConnect,
		LastDisconnect: c.lastDisconn,
		LastError:      c.lastError,
		MessagesSent:   c.msgSent.Load(),
		MessagesErrors: c.msgErrors.Load(),
		Uptime:         uptime,
		Timestamp:      time.Now(),
	}
}

// GetConfig returns the current MQTT configuration (without password).
func (c *Client) GetConfig() *dto.MQTTConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy without the password
	return &dto.MQTTConfig{
		Enabled:           c.config.Enabled,
		Broker:            c.config.Broker,
		ClientID:          c.config.ClientID,
		Username:          c.config.Username,
		TopicPrefix:       c.config.TopicPrefix,
		QoS:               c.config.QoS,
		RetainMessages:    c.config.RetainMessages,
		ConnectTimeout:    c.config.ConnectTimeout,
		KeepAlive:         c.config.KeepAlive,
		CleanSession:      c.config.CleanSession,
		AutoReconnect:     c.config.AutoReconnect,
		HomeAssistantMode: c.config.HomeAssistantMode,
		HADiscoveryPrefix: c.config.HADiscoveryPrefix,
	}
}

// GetTopics returns the MQTT topics used by the client.
func (c *Client) GetTopics() *dto.MQTTTopics {
	return &dto.MQTTTopics{
		Status:       c.buildTopic("status"),
		System:       c.buildTopic("system"),
		Array:        c.buildTopic("array"),
		Disks:        c.buildTopic("disks"),
		Containers:   c.buildTopic("docker/containers"),
		VMs:          c.buildTopic("vm/list"),
		UPS:          c.buildTopic("ups"),
		GPU:          c.buildTopic("gpu"),
		Network:      c.buildTopic("network"),
		Shares:       c.buildTopic("shares"),
		Notification: c.buildTopic("notifications"),
		ZFSPools:     c.buildTopic("zfs/pools"),
		Availability: c.buildTopic("availability"),
	}
}

// PublishSystemInfo publishes system information to MQTT.
func (c *Client) PublishSystemInfo(info *dto.SystemInfo) error {
	if !c.shouldPublish() {
		return nil
	}
	return c.publishJSON(c.buildTopic("system"), info)
}

// PublishArrayStatus publishes array status to MQTT.
func (c *Client) PublishArrayStatus(status *dto.ArrayStatus) error {
	if !c.shouldPublish() {
		return nil
	}
	return c.publishJSON(c.buildTopic("array"), status)
}

// PublishDisks publishes disk information to MQTT.
func (c *Client) PublishDisks(disks []dto.DiskInfo) error {
	if !c.shouldPublish() {
		return nil
	}
	err := c.publishJSON(c.buildTopic("disks"), disks)
	// Publish per-disk topics and HA discovery
	go c.publishDiskDiscovery(disks)
	return err
}

// PublishContainers publishes Docker container information to MQTT.
func (c *Client) PublishContainers(containers []dto.ContainerInfo) error {
	if !c.shouldPublish() {
		return nil
	}
	err := c.publishJSON(c.buildTopic("docker/containers"), containers)
	// Publish per-container topics and HA discovery
	go c.publishContainerDiscovery(containers)
	return err
}

// PublishVMs publishes VM information to MQTT.
func (c *Client) PublishVMs(vms []dto.VMInfo) error {
	if !c.shouldPublish() {
		return nil
	}
	err := c.publishJSON(c.buildTopic("vm/list"), vms)
	// Publish per-VM topics and HA discovery
	go c.publishVMDiscovery(vms)
	return err
}

// PublishUPSStatus publishes UPS status to MQTT.
func (c *Client) PublishUPSStatus(ups *dto.UPSStatus) error {
	if !c.shouldPublish() {
		return nil
	}
	return c.publishJSON(c.buildTopic("ups"), ups)
}

// PublishGPUMetrics publishes GPU metrics to MQTT.
func (c *Client) PublishGPUMetrics(gpus []*dto.GPUMetrics) error {
	if !c.shouldPublish() {
		return nil
	}
	err := c.publishJSON(c.buildTopic("gpu"), gpus)
	// Publish per-GPU topics and HA discovery
	go c.publishGPUDiscovery(gpus)
	return err
}

// PublishNetworkInfo publishes network information to MQTT.
func (c *Client) PublishNetworkInfo(network []dto.NetworkInfo) error {
	if !c.shouldPublish() {
		return nil
	}
	err := c.publishJSON(c.buildTopic("network"), network)
	// Publish per-interface topics and HA discovery
	go c.publishNetworkDiscovery(network)
	return err
}

// PublishShares publishes share information to MQTT.
func (c *Client) PublishShares(shares []dto.ShareInfo) error {
	if !c.shouldPublish() {
		return nil
	}
	err := c.publishJSON(c.buildTopic("shares"), shares)
	// Publish per-share topics and HA discovery
	go c.publishShareDiscovery(shares)
	return err
}

// PublishNotifications publishes notifications to MQTT.
func (c *Client) PublishNotifications(notifications *dto.NotificationList) error {
	if !c.shouldPublish() {
		return nil
	}
	return c.publishJSON(c.buildTopic("notifications"), notifications)
}

// PublishZFSPools publishes ZFS pool information to MQTT.
func (c *Client) PublishZFSPools(pools []dto.ZFSPool) error {
	if !c.shouldPublish() {
		return nil
	}
	err := c.publishJSON(c.buildTopic("zfs/pools"), pools)
	// Publish per-pool topics and HA discovery
	go c.publishZFSDiscovery(pools)
	return err
}

// PublishCustom publishes a custom message to the specified topic.
func (c *Client) PublishCustom(topic string, payload any, retained bool) error {
	if !c.shouldPublish() {
		return fmt.Errorf("MQTT client not connected")
	}
	fullTopic := c.buildTopic(topic)
	return c.publishJSON(fullTopic, payload)
}

// shouldPublish checks if the client is ready to publish.
func (c *Client) shouldPublish() bool {
	return c.config.Enabled && c.connected.Load() && c.client != nil
}

// publish publishes a string payload to the specified topic.
func (c *Client) publish(topic, payload string, retained bool) error {
	if c.client == nil {
		return fmt.Errorf("MQTT client not initialized")
	}

	token := c.client.Publish(topic, byte(c.config.QoS), retained, payload)
	token.Wait()

	if token.Error() != nil {
		c.msgErrors.Add(1)
		logger.Debug("MQTT: Failed to publish to %s: %v", topic, token.Error())
		return token.Error()
	}

	c.msgSent.Add(1)
	logger.Debug("MQTT: Published to %s", topic)
	return nil
}

// publishJSON publishes a JSON-encoded payload to the specified topic.
func (c *Client) publishJSON(topic string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		c.msgErrors.Add(1)
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return c.publish(topic, string(data), c.config.RetainMessages)
}

// buildTopic constructs a full topic path with the configured prefix.
func (c *Client) buildTopic(suffix string) string {
	if c.config.TopicPrefix == "" {
		return suffix
	}
	return fmt.Sprintf("%s/%s", c.config.TopicPrefix, suffix)
}

// NOTE: publishHADiscovery, publishHAEntity, and all per-item discovery
// methods are in ha_discovery.go

// TestConnection tests connectivity to an MQTT broker.
func TestConnection(broker, username, password, clientID string, timeout time.Duration) *dto.MQTTTestResponse {
	start := time.Now()

	if clientID == "" {
		clientID = "unraid-mqtt-test"
	}

	opts := pahomqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID)
	opts.SetConnectTimeout(timeout)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(false)

	if username != "" {
		opts.SetUsername(username)
	}
	if password != "" {
		opts.SetPassword(password)
	}

	client := pahomqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()

	latency := time.Since(start).Milliseconds()

	if token.Error() != nil {
		return &dto.MQTTTestResponse{
			Success:   false,
			Message:   fmt.Sprintf("Connection failed: %v", token.Error()),
			Latency:   latency,
			Timestamp: time.Now(),
		}
	}

	client.Disconnect(100)

	tlsEnabled := strings.HasPrefix(broker, "ssl://") || strings.HasPrefix(broker, "tls://")

	return &dto.MQTTTestResponse{
		Success:     true,
		Message:     "Connection successful",
		Latency:     latency,
		TLSEnabled:  tlsEnabled,
		ProtocolVer: "3.1.1",
		Timestamp:   time.Now(),
	}
}

// DefaultConfig returns the default MQTT configuration.
func DefaultConfig() *dto.MQTTConfig {
	return &dto.MQTTConfig{
		Enabled:           false,
		Broker:            "tcp://localhost:1883",
		ClientID:          "unraid-management-agent",
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
}
