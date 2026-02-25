// Package mqtt provides MQTT client functionality for the Unraid Management Agent.
package mqtt

import (
	"encoding/json"
	"fmt"
	"strings"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/controllers"
)

// subscribeCommandTopics subscribes to all command topics for switches and buttons.
// Uses a combination of wildcard subscriptions for per-entity commands and
// direct subscriptions for fixed-path commands.
func (c *Client) subscribeCommandTopics() {
	if c.client == nil || !c.client.IsConnected() {
		return
	}

	// Subscribe to all commands under cmd/# using the wildcard router
	cmdTopic := c.buildTopic("cmd/#")
	token := c.client.Subscribe(cmdTopic, byte(c.config.QoS), func(_ pahomqtt.Client, msg pahomqtt.Message) {
		c.handleCommand(msg)
	})
	token.Wait()
	if token.Error() != nil {
		logger.Error("MQTT: Failed to subscribe to command topics: %v", token.Error())
		return
	}

	logger.Success("MQTT: Subscribed to command topic %s", cmdTopic)
}

// buildCommandTopic constructs a command topic for a specific entity.
func (c *Client) buildCommandTopic(parts ...string) string {
	suffix := "cmd/" + strings.Join(parts, "/")
	return c.buildTopic(suffix)
}

// handleCommand routes incoming MQTT command messages to the appropriate handler.
func (c *Client) handleCommand(msg pahomqtt.Message) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("MQTT: PANIC in command handler for %s: %v", msg.Topic(), r)
		}
	}()

	topic := msg.Topic()
	payload := strings.TrimSpace(string(msg.Payload()))
	prefix := c.buildTopic("cmd/")

	if !strings.HasPrefix(topic, prefix) {
		return
	}

	// Strip prefix to get relative path: "docker/plex/set", "array/set", etc.
	relative := topic[len(prefix):]
	parts := strings.Split(relative, "/")

	logger.Info("MQTT: Command received: %s → %s", relative, payload)

	var err error

	switch {
	// Docker: docker/{name}/set (switch), docker/{name}/{action} (button)
	case len(parts) == 3 && parts[0] == "docker" && parts[2] == "set":
		err = c.execDockerSwitch(parts[1], payload)
	case len(parts) == 3 && parts[0] == "docker":
		err = c.execDockerButton(parts[1], parts[2])

	// VM: vm/{name}/set (switch), vm/{name}/{action} (button)
	case len(parts) == 3 && parts[0] == "vm" && parts[2] == "set":
		err = c.execVMSwitch(parts[1], payload)
	case len(parts) == 3 && parts[0] == "vm":
		err = c.execVMButton(parts[1], parts[2])

	// Array: array/set (switch)
	case len(parts) == 2 && parts[0] == "array" && parts[1] == "set":
		err = c.execArraySwitch(payload)

	// Parity: array/parity/{action} (buttons)
	case len(parts) == 3 && parts[0] == "array" && parts[1] == "parity":
		err = c.execParityButton(parts[2])

	// Disk: disk/{name}/spin_up, disk/{name}/spin_down (buttons)
	case len(parts) == 3 && parts[0] == "disk" && parts[2] == "spin_up":
		err = c.execDiskSpin(parts[1], "up")
	case len(parts) == 3 && parts[0] == "disk" && parts[2] == "spin_down":
		err = c.execDiskSpin(parts[1], "down")

	// Service: service/{name}/set (switch)
	case len(parts) == 3 && parts[0] == "service" && parts[2] == "set":
		err = c.execServiceSwitch(parts[1], payload)

	// System: system/reboot, system/shutdown (buttons)
	case len(parts) == 2 && parts[0] == "system":
		err = c.execSystemButton(parts[1])

	// Notifications: notifications/archive_all (button)
	case len(parts) == 2 && parts[0] == "notifications" && parts[1] == "archive_all":
		err = c.execArchiveAllNotifications()

	default:
		logger.Debug("MQTT: Unhandled command topic: %s", relative)
		return
	}

	// Publish result
	c.publishCommandResult(topic, err)
}

// publishCommandResult publishes a success/error result on the command topic.
func (c *Client) publishCommandResult(topic string, err error) {
	result := map[string]any{"success": err == nil}
	if err != nil {
		result["error"] = err.Error()
		logger.Error("MQTT: Command failed on %s: %v", topic, err)
	}
	data, jsonErr := json.Marshal(result)
	if jsonErr != nil {
		return
	}
	_ = c.publish(topic+"/result", string(data), false)
}

// --- Docker ---

func (c *Client) execDockerSwitch(nameID, payload string) error {
	ctrl := controllers.NewDockerController()
	defer func() {
		if err := ctrl.Close(); err != nil {
			logger.Debug("MQTT: Failed to close Docker controller: %v", err)
		}
	}()

	switch strings.ToUpper(payload) {
	case "ON":
		logger.Info("MQTT: Starting container %s", nameID)
		return ctrl.Start(nameID)
	case "OFF":
		logger.Info("MQTT: Stopping container %s", nameID)
		return ctrl.Stop(nameID)
	default:
		return fmt.Errorf("invalid docker switch payload: %s (expected ON/OFF)", payload)
	}
}

func (c *Client) execDockerButton(nameID, action string) error {
	ctrl := controllers.NewDockerController()
	defer func() {
		if err := ctrl.Close(); err != nil {
			logger.Debug("MQTT: Failed to close Docker controller: %v", err)
		}
	}()

	switch action {
	case "restart":
		logger.Info("MQTT: Restarting container %s", nameID)
		return ctrl.Restart(nameID)
	case "pause":
		logger.Info("MQTT: Pausing container %s", nameID)
		return ctrl.Pause(nameID)
	case "unpause":
		logger.Info("MQTT: Unpausing container %s", nameID)
		return ctrl.Unpause(nameID)
	default:
		return fmt.Errorf("unknown docker button action: %s", action)
	}
}

// --- VM ---

func (c *Client) execVMSwitch(nameID, payload string) error {
	ctrl := controllers.NewVMController()

	switch strings.ToUpper(payload) {
	case "ON":
		logger.Info("MQTT: Starting VM %s", nameID)
		return ctrl.Start(nameID)
	case "OFF":
		logger.Info("MQTT: Stopping VM %s", nameID)
		return ctrl.Stop(nameID)
	default:
		return fmt.Errorf("invalid VM switch payload: %s (expected ON/OFF)", payload)
	}
}

func (c *Client) execVMButton(nameID, action string) error {
	ctrl := controllers.NewVMController()

	switch action {
	case "restart":
		logger.Info("MQTT: Restarting VM %s", nameID)
		return ctrl.Restart(nameID)
	case "pause":
		logger.Info("MQTT: Pausing VM %s", nameID)
		return ctrl.Pause(nameID)
	case "resume":
		logger.Info("MQTT: Resuming VM %s", nameID)
		return ctrl.Resume(nameID)
	case "hibernate":
		logger.Info("MQTT: Hibernating VM %s", nameID)
		return ctrl.Hibernate(nameID)
	case "force_stop":
		logger.Info("MQTT: Force-stopping VM %s", nameID)
		return ctrl.ForceStop(nameID)
	default:
		return fmt.Errorf("unknown VM button action: %s", action)
	}
}

// --- Array ---

func (c *Client) execArraySwitch(payload string) error {
	if c.domainCtx == nil {
		return fmt.Errorf("domain context not available for array control")
	}
	ctrl := controllers.NewArrayController(c.domainCtx)

	switch strings.ToUpper(payload) {
	case "ON":
		logger.Info("MQTT: Starting array")
		return ctrl.StartArray()
	case "OFF":
		logger.Info("MQTT: Stopping array")
		return ctrl.StopArray()
	default:
		return fmt.Errorf("invalid array switch payload: %s (expected ON/OFF)", payload)
	}
}

func (c *Client) execParityButton(action string) error {
	if c.domainCtx == nil {
		return fmt.Errorf("domain context not available for parity control")
	}
	ctrl := controllers.NewArrayController(c.domainCtx)

	switch action {
	case "start":
		logger.Info("MQTT: Starting parity check")
		return ctrl.StartParityCheck(false)
	case "stop":
		logger.Info("MQTT: Stopping parity check")
		return ctrl.StopParityCheck()
	case "pause":
		logger.Info("MQTT: Pausing parity check")
		return ctrl.PauseParityCheck()
	case "resume":
		logger.Info("MQTT: Resuming parity check")
		return ctrl.ResumeParityCheck()
	default:
		return fmt.Errorf("unknown parity action: %s", action)
	}
}

func (c *Client) execDiskSpin(nameID, direction string) error {
	if c.domainCtx == nil {
		return fmt.Errorf("domain context not available for disk control")
	}
	ctrl := controllers.NewArrayController(c.domainCtx)

	switch direction {
	case "up":
		logger.Info("MQTT: Spinning up disk %s", nameID)
		return ctrl.SpinUpDisk(nameID)
	case "down":
		logger.Info("MQTT: Spinning down disk %s", nameID)
		return ctrl.SpinDownDisk(nameID)
	default:
		return fmt.Errorf("unknown spin direction: %s", direction)
	}
}

// --- Services ---

func (c *Client) execServiceSwitch(nameID, payload string) error {
	ctrl := controllers.NewServiceController()

	var err error
	switch strings.ToUpper(payload) {
	case "ON":
		logger.Info("MQTT: Starting service %s", nameID)
		err = ctrl.StartService(nameID)
	case "OFF":
		logger.Info("MQTT: Stopping service %s", nameID)
		err = ctrl.StopService(nameID)
	default:
		return fmt.Errorf("invalid service switch payload: %s (expected ON/OFF)", payload)
	}

	// Publish updated service states so HA switches reflect the change
	if err == nil {
		go c.publishServiceStates()
	}

	return err
}

// --- System ---

func (c *Client) execSystemButton(action string) error {
	if c.domainCtx == nil {
		return fmt.Errorf("domain context not available for system control")
	}
	ctrl := controllers.NewSystemController(c.domainCtx)

	switch action {
	case "reboot":
		logger.Info("MQTT: Initiating system reboot")
		return ctrl.Reboot()
	case "shutdown":
		logger.Info("MQTT: Initiating system shutdown")
		return ctrl.Shutdown()
	default:
		return fmt.Errorf("unknown system action: %s", action)
	}
}

// --- Notifications ---

func (c *Client) execArchiveAllNotifications() error {
	logger.Info("MQTT: Archiving all notifications")
	return controllers.ArchiveAllNotifications()
}
