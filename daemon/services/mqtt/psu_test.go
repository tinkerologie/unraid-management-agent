package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestPSUPublicationDiscoveryAndRemoval(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.HomeAssistantMode = true
	c := NewClient(config, "test-server", "test", nil)
	if err := c.PublishPSUStatus(nil); err != nil {
		t.Fatal(err)
	}
	capture := &psuCaptureClient{published: map[string]string{}}
	c.client = capture
	c.connected.Store(true)
	zero := 0.0
	status := &dto.PSUStatus{State: dto.PSUStateOK, Devices: []dto.PSUDevice{{ID: "psu-a", Model: "HX1000i", State: dto.PSUStateOK, Sensors: []dto.PSUSensor{
		{Name: "output_power", Label: "DC output", Kind: "power", Unit: "W", Value: &zero},
		{Name: "input_power", Label: "AC input", Kind: "power", Unit: "W"},
	}}}}
	if err := c.PublishPSUStatus(status); err != nil {
		t.Fatal(err)
	}
	waitPSUDiscovery(t, c)
	state := capture.snapshot()[c.buildTopic("psu")]
	if !strings.Contains(state, `"value":0`) || !strings.Contains(state, `"value":null`) {
		t.Fatal("zero/null state not preserved", state)
	}
	topic := "homeassistant/sensor/test-server/psu_psu_a_output_power/config"
	var discovery map[string]any
	if err := json.Unmarshal([]byte(capture.snapshot()[topic]), &discovery); err != nil {
		t.Fatal(err, capture.snapshot())
	}
	if discovery["device_class"] != "power" || discovery["availability_mode"] != "all" {
		t.Fatal(discovery)
	}
	availability := discovery["availability"].([]any)
	if len(availability) != 2 || !strings.Contains(availability[1].(map[string]any)["value_template"].(string), "is not none") {
		t.Fatal("missing per-sensor availability", discovery)
	}
	for topic := range capture.snapshot() {
		if strings.Contains(topic, "input_power/config") {
			t.Fatal("unknown measurement discovered")
		}
	}
	if err := c.PublishPSUStatus(dto.NewPSUStatus(dto.PSUStateNotDetected, time.Now())); err != nil {
		t.Fatal(err)
	}
	waitPSUDiscovery(t, c)
	if capture.snapshot()[topic] != "" {
		t.Fatal("disconnected entity retained")
	}
	config.HomeAssistantMode = false
	c.publishPSUDiscovery(context.Background(), status)
	if err := c.PublishPSUStatus(nil); err != nil {
		t.Fatal(err)
	}
}

type psuFailureToken struct{ doneToken }

func (psuFailureToken) Error() error { return errors.New("broker rejected publish") }

type psuFailureClient struct{ capturingClient }

func (*psuFailureClient) Publish(string, byte, bool, any) pahomqtt.Token { return psuFailureToken{} }

func TestPSUPublishErrorIsPropagated(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	c := NewClient(config, "test", "test", nil)
	c.client = &psuFailureClient{}
	c.connected.Store(true)
	if err := c.PublishPSUStatus(dto.NewPSUStatus(dto.PSUStateNotDetected, time.Now())); err == nil {
		t.Fatal("broker error swallowed")
	}
}

// psuCaptureClient permits concurrent state and discovery publishes.
type psuCaptureClient struct {
	pahomqtt.Client
	mu             sync.Mutex
	published      map[string]string
	discoveryToken pahomqtt.Token
	entered        chan struct{}
}

func (c *psuCaptureClient) Publish(topic string, _ byte, _ bool, payload any) pahomqtt.Token {
	c.mu.Lock()
	c.published[topic], _ = payload.(string)
	token := c.discoveryToken
	c.mu.Unlock()
	if strings.HasPrefix(topic, "homeassistant/") && token != nil {
		select {
		case c.entered <- struct{}{}:
		default:
		}
		return token
	}
	return doneToken{}
}
func (c *psuCaptureClient) snapshot() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string]string, len(c.published))
	for topic, payload := range c.published {
		result[topic] = payload
	}
	return result
}
func waitPSUDiscovery(t *testing.T, c *Client) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c.psuDiscoveryMu.Lock()
		idle := !c.psuDiscoveryRunning && c.psuDiscoveryPending == nil
		c.psuDiscoveryMu.Unlock()
		if idle {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("PSU discovery did not drain")
}

type psuGateToken struct {
	doneToken
	release <-chan struct{}
}

func (t psuGateToken) WaitTimeout(timeout time.Duration) bool {
	select {
	case <-t.release:
		return true
	case <-time.After(timeout):
		return false
	}
}
func TestPSUSlowDiscoveryCoalescesWithoutBlockingState(t *testing.T) {
	config := DefaultConfig()
	config.Enabled, config.HomeAssistantMode = true, true
	c := NewClient(config, "test", "test", nil)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	capture := &psuCaptureClient{published: map[string]string{}, discoveryToken: psuGateToken{release: release}, entered: make(chan struct{}, 10)}
	c.client = capture
	c.connected.Store(true)
	value := 12.0
	status := &dto.PSUStatus{State: dto.PSUStateOK, Devices: []dto.PSUDevice{{ID: "old", Sensors: []dto.PSUSensor{{Name: "voltage", Value: &value}}}}}
	if err := c.PublishPSUStatus(status); err != nil {
		t.Fatal(err)
	}
	select {
	case <-capture.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	// Discovery is still awaiting its ack. Even a burst must not block state or
	// unrelated subscriber work, spawn more workers, or accumulate snapshots.
	done := make(chan error, 1)
	latest := dto.NewPSUStatus(dto.PSUStateNotDetected, time.Now())
	go func() {
		for i := 0; i < 100; i++ {
			if err := c.PublishPSUStatus(status); err != nil {
				done <- err
				return
			}
		}
		if err := c.PublishPSUStatus(latest); err != nil {
			done <- err
			return
		}
		done <- c.PublishArrayStatus(&dto.ArrayStatus{})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("discovery blocked subscriber")
	}
	c.psuDiscoveryMu.Lock()
	pending := c.psuDiscoveryPending
	running := c.psuDiscoveryRunning
	c.psuDiscoveryMu.Unlock()
	if !running || pending == nil || pending.status != latest {
		t.Fatal("latest snapshot not coalesced")
	}
	select {
	case <-capture.entered:
		t.Fatal("concurrent discovery worker started")
	default:
	}
	unblock()
	waitPSUDiscovery(t, c)
	if capture.snapshot()["homeassistant/sensor/test/psu_old_voltage/config"] != "" {
		t.Fatal("old discovery survived final removal")
	}
}
func TestPSUDiscoveryCancellationPreservesPartialCleanup(t *testing.T) {
	config := DefaultConfig()
	config.Enabled, config.HomeAssistantMode = true, true
	c := NewClient(config, "test", "test", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.connectContext = ctx
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	capture := &psuCaptureClient{published: map[string]string{}, discoveryToken: psuGateToken{release: release}, entered: make(chan struct{}, 10)}
	c.client = capture
	c.connected.Store(true)
	value := 1.0
	status := &dto.PSUStatus{Devices: []dto.PSUDevice{{ID: "old", Sensors: []dto.PSUSensor{{Name: "first", Value: &value}, {Name: "second", Value: &value}}}}}
	if err := c.PublishPSUStatus(status); err != nil {
		t.Fatal(err)
	}
	select {
	case <-capture.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	cancel()
	// A new connection queues its fresh inventory behind the old in-flight ack.
	c.mu.Lock()
	c.connectContext = context.Background()
	c.mu.Unlock()
	if err := c.PublishPSUStatus(dto.NewPSUStatus(dto.PSUStateNotDetected, time.Now())); err != nil {
		t.Fatal(err)
	}
	unblock()
	waitPSUDiscovery(t, c)
	published := capture.snapshot()
	if _, ok := published["homeassistant/sensor/test/psu_old_second/config"]; ok {
		t.Fatal("canceled connection continued discovery")
	}
	if published["homeassistant/sensor/test/psu_old_first/config"] != "" {
		t.Fatal("partial canceled discovery not removed")
	}
	canceled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	c.publishPSUDiscovery(canceled, status)
}

// A worker panic must not wedge pending work or lose partially published IDs.
type psuPanicToken struct {
	psuGateToken
	once *sync.Once
}

func (t psuPanicToken) WaitTimeout(timeout time.Duration) bool {
	if !t.psuGateToken.WaitTimeout(timeout) {
		return false
	}
	t.once.Do(func() { panic("simulated broker token panic") })
	return true
}
func TestPSUDiscoveryWorkerRecoversPendingUpdate(t *testing.T) {
	config := DefaultConfig()
	config.Enabled, config.HomeAssistantMode = true, true
	c := NewClient(config, "test", "test", nil)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	capture := &psuCaptureClient{published: map[string]string{}, discoveryToken: psuPanicToken{psuGateToken{release: release}, &sync.Once{}}, entered: make(chan struct{}, 10)}
	c.client = capture
	c.connected.Store(true)
	value := 1.0
	status := &dto.PSUStatus{Devices: []dto.PSUDevice{{ID: "old", Sensors: []dto.PSUSensor{{Name: "power", Value: &value}}}}}
	if err := c.PublishPSUStatus(status); err != nil {
		t.Fatal(err)
	}
	select {
	case <-capture.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	if err := c.PublishPSUStatus(dto.NewPSUStatus(dto.PSUStateNotDetected, time.Now())); err != nil {
		t.Fatal(err)
	}
	unblock()
	waitPSUDiscovery(t, c)
	if capture.snapshot()["homeassistant/sensor/test/psu_old_power/config"] != "" {
		t.Fatal("pending cleanup lost after panic")
	}
	c.connected.Store(false)
	c.publishPSUDiscovery(context.Background(), status)
}
