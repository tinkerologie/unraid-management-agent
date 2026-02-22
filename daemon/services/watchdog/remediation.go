package watchdog

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/controllers"
)

// Remediator executes remediation actions when health checks fail.
type Remediator struct{}

// NewRemediator creates a new Remediator.
func NewRemediator() *Remediator {
	return &Remediator{}
}

// Execute runs the remediation action specified in the health check's OnFail field.
// Supported actions: "notify", "restart_container:<id>", "webhook:<url>".
func (r *Remediator) Execute(ctx context.Context, check dto.HealthCheck, result ProbeResult) error {
	action := check.OnFail
	if action == "" {
		return nil
	}

	switch {
	case action == "notify":
		return r.notifyUnraid(check, result)
	case strings.HasPrefix(action, "restart_container:"):
		containerID := strings.TrimPrefix(action, "restart_container:")
		return r.restartContainer(ctx, containerID)
	case strings.HasPrefix(action, "webhook:"):
		url := strings.TrimPrefix(action, "webhook:")
		return r.callWebhook(ctx, check, result, url)
	default:
		return fmt.Errorf("unknown remediation action: %s", action)
	}
}

// notifyUnraid creates an Unraid system notification about the health check failure.
func (r *Remediator) notifyUnraid(check dto.HealthCheck, result ProbeResult) error {
	msg := fmt.Sprintf("Health check '%s' (%s) failed: %s", check.Name, check.Target, result.Error)
	importance := "warning"

	err := controllers.CreateNotification("Health Check", check.Name, msg, importance, "")
	if err != nil {
		return fmt.Errorf("sending unraid notification: %w", err)
	}

	logger.Info("Watchdog: Sent Unraid notification for '%s'", check.Name)
	return nil
}

// restartContainer restarts a Docker container as remediation.
func (r *Remediator) restartContainer(_ context.Context, containerID string) error {
	dc := controllers.NewDockerController()
	defer func() { _ = dc.Close() }()

	logger.Warning("Watchdog: Restarting container '%s' as remediation", containerID)

	if err := dc.Restart(containerID); err != nil {
		return fmt.Errorf("restarting container '%s': %w", containerID, err)
	}

	logger.Success("Watchdog: Container '%s' restarted successfully", containerID)
	return nil
}

// callWebhook sends an HTTP POST to the webhook URL with failure details.
func (r *Remediator) callWebhook(ctx context.Context, check dto.HealthCheck, result ProbeResult, url string) error {
	payload := fmt.Sprintf(
		`{"check_id":"%s","check_name":"%s","target":"%s","error":"%s","timestamp":"%s"}`,
		check.ID, check.Name, check.Target, result.Error, time.Now().UTC().Format(time.RFC3339),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(payload))
	if err != nil {
		return fmt.Errorf("creating webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req) //nolint:gosec //#nosec G704 -- Webhook URL is user-configured
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	logger.Info("Watchdog: Webhook called for '%s' (status %d)", check.Name, resp.StatusCode)
	return nil
}
