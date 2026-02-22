package alerting

import (
	"fmt"
	"strings"

	"github.com/nicholas-fedor/shoutrrr"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/controllers"
)

// Dispatcher sends alert notifications via configured channels.
type Dispatcher struct{}

// NewDispatcher creates a new alert notification dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{}
}

// Dispatch sends an alert event to all channels configured on the rule.
func (d *Dispatcher) Dispatch(rule dto.AlertRule, event dto.AlertEvent) {
	message := d.formatMessage(event)

	for _, channel := range rule.Channels {
		if err := d.sendToChannel(channel, message, event); err != nil {
			logger.Error("Alerting: Failed to dispatch to channel %s for rule %s: %v",
				channelType(channel), rule.ID, err)
		}
	}
}

// sendToChannel sends a message to a single channel.
func (d *Dispatcher) sendToChannel(channel, message string, event dto.AlertEvent) error {
	if channel == "unraid" {
		return d.sendToUnraid(event)
	}

	// Use shoutrrr for all other channel types (ntfy, gotify, discord, slack, webhook, etc.)
	return d.sendViaShoutrrr(channel, message)
}

// sendToUnraid sends a notification via the Unraid notification system.
func (d *Dispatcher) sendToUnraid(event dto.AlertEvent) error {
	importance := "normal"
	switch event.Severity {
	case "critical":
		importance = "alert"
	case "warning":
		importance = "warning"
	case "info":
		importance = "normal"
	}

	subject := fmt.Sprintf("Alert: %s", event.RuleName)
	if event.State == "resolved" {
		subject = fmt.Sprintf("Resolved: %s", event.RuleName)
	}

	return controllers.CreateNotification(
		"Management Agent Alert",
		subject,
		event.Message,
		importance,
		"",
	)
}

// sendViaShoutrrr sends a notification via shoutrrr URL.
func (d *Dispatcher) sendViaShoutrrr(url, message string) error {
	err := shoutrrr.Send(url, message)
	if err != nil {
		return fmt.Errorf("shoutrrr error: %w", err)
	}
	return nil
}

// formatMessage creates a human-readable notification message.
func (d *Dispatcher) formatMessage(event dto.AlertEvent) string {
	var sb strings.Builder

	if event.State == "firing" {
		sb.WriteString(fmt.Sprintf("🔔 ALERT [%s]: %s\n", strings.ToUpper(event.Severity), event.RuleName))
	} else {
		sb.WriteString(fmt.Sprintf("✅ RESOLVED: %s\n", event.RuleName))
	}

	sb.WriteString(event.Message)
	sb.WriteString(fmt.Sprintf("\nTime: %s", event.FiredAt.Format("2006-01-02 15:04:05")))

	if event.State == "resolved" && !event.ResolvedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("\nResolved: %s", event.ResolvedAt.Format("2006-01-02 15:04:05")))
	}

	return sb.String()
}

// channelType returns a display-friendly name for a channel URL.
func channelType(ch string) string {
	if ch == "unraid" {
		return "unraid"
	}
	if before, _, ok := strings.Cut(ch, "://"); ok {
		return before
	}
	return "unknown"
}
