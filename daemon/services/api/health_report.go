package api

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// diskTempWarning is the temperature threshold (°C) above which a disk finding is emitted as a warning.
const diskTempWarning = 55.0

// BuildHealthReport aggregates health signals from plain data and returns a
// prioritised, ranked HealthReport. Keeping inputs as plain values makes the
// function unit-testable without a running Server.
func BuildHealthReport(
	containers []dto.ContainerInfo,
	array *dto.ArrayStatus,
	disks []dto.DiskInfo,
	firing []dto.AlertStatus,
) dto.HealthReport {
	var findings []dto.HealthFinding

	// ── Array ─────────────────────────────────────────────────────────────────

	if array != nil && array.State != "Started" {
		findings = append(findings, dto.HealthFinding{
			Severity: "critical",
			Title:    "Array not started",
			Detail:   fmt.Sprintf("Array is in state %q — data is inaccessible until the array is started.", array.State),
		})
	}

	// ── Disk health ───────────────────────────────────────────────────────────

	for _, d := range disks {
		if d.SMARTStatus != "" && d.SMARTStatus != "PASSED" {
			findings = append(findings, dto.HealthFinding{
				Severity: "critical",
				Title:    fmt.Sprintf("Disk %s SMART failure", diskLabel(d)),
				Detail:   fmt.Sprintf("Disk %s reported SMART status %q. Backup data and replace the disk.", diskLabel(d), d.SMARTStatus),
			})
		}
		if d.Temperature > diskTempWarning {
			findings = append(findings, dto.HealthFinding{
				Severity: "warning",
				Title:    fmt.Sprintf("Disk %s high temperature", diskLabel(d)),
				Detail:   fmt.Sprintf("Disk %s temperature is %.0f °C (threshold: %.0f °C). Improve airflow or reduce load.", diskLabel(d), d.Temperature, diskTempWarning),
			})
		}
	}

	// ── Container health ──────────────────────────────────────────────────────

	for _, c := range containers {
		if c.State != "running" {
			sev := "info"
			title := fmt.Sprintf("Container %q is not running", c.Name)
			detail := fmt.Sprintf("Container %q is in state %q.", c.Name, c.State)

			// Elevate to warning when restart count indicates repeated failures.
			if c.RestartCount > 3 {
				sev = "warning"
				title = fmt.Sprintf("Container %q is not running (restarted %d times)", c.Name, c.RestartCount)
				detail = fmt.Sprintf("Container %q is in state %q and has restarted %d times — it may be crash-looping.", c.Name, c.State, c.RestartCount)
			}

			findings = append(findings, dto.HealthFinding{
				Severity: sev,
				Title:    title,
				Detail:   detail,
				RecommendedActions: []dto.ActionRef{
					{
						Action: "start_container",
						Target: c.ID,
						Reason: fmt.Sprintf("Start container %q to restore service.", c.Name),
					},
				},
			})
			continue
		}

		// Running but update available — informational only (no executor action for updates).
		if c.UpdateAvailable != nil && *c.UpdateAvailable {
			findings = append(findings, dto.HealthFinding{
				Severity: "info",
				Title:    fmt.Sprintf("Container %q has an update available", c.Name),
				Detail:   fmt.Sprintf("A newer image is available for container %q. Update via the Docker UI or docker pull.", c.Name),
			})
		}
	}

	// ── Firing alerts ─────────────────────────────────────────────────────────

	for _, a := range firing {
		sev := a.Severity
		if sev == "" {
			sev = "warning"
		}
		msg := a.Message
		if msg == "" {
			msg = fmt.Sprintf("Alert rule %q is firing.", a.RuleName)
		}
		findings = append(findings, dto.HealthFinding{
			Severity: normalizeSeverity(sev),
			Title:    fmt.Sprintf("Firing alert: %s", a.RuleName),
			Detail:   msg,
		})
	}

	// ── Sort by severity (critical → warning → info) ──────────────────────────

	severityOrder := map[string]int{"critical": 0, "warning": 1, "info": 2}
	sort.SliceStable(findings, func(i, j int) bool {
		oi, ok := severityOrder[findings[i].Severity]
		if !ok {
			oi = severityOrder["info"]
		}
		oj, ok := severityOrder[findings[j].Severity]
		if !ok {
			oj = severityOrder["info"]
		}
		return oi < oj
	})

	// ── Count by severity ─────────────────────────────────────────────────────

	var critical, warning, info int
	for _, f := range findings {
		switch f.Severity {
		case "critical":
			critical++
		case "warning":
			warning++
		default:
			info++
		}
	}

	if findings == nil {
		findings = []dto.HealthFinding{}
	}

	return dto.HealthReport{
		Findings:    findings,
		Critical:    critical,
		Warning:     warning,
		Info:        info,
		GeneratedAt: time.Now(),
	}
}

// normalizeSeverity returns exactly one of "critical", "warning", or "info".
// Any unrecognised value is treated as "info" so that unknown severities sort
// and count consistently.
func normalizeSeverity(s string) string {
	switch s {
	case "critical", "warning", "info":
		return s
	default:
		return "info"
	}
}

// diskLabel returns a human-readable label for a disk.
func diskLabel(d dto.DiskInfo) string {
	if d.Name != "" {
		return d.Name
	}
	if d.Device != "" {
		return d.Device
	}
	return d.ID
}

// handleHealthReport godoc
//
//	@Summary		Get system health report
//	@Description	Aggregate health signals from array, disks, containers, and firing alerts into a prioritized list of findings with recommended actions
//	@Tags			System
//	@Produce		json
//	@Success		200	{object}	dto.HealthReport	"Health report"
//	@Router			/health/report [get]
func (s *Server) handleHealthReport(w http.ResponseWriter, _ *http.Request) {
	containers := s.GetDockerCache()
	if containers == nil {
		containers = []dto.ContainerInfo{}
	}

	disks := s.GetDisksCache()
	if disks == nil {
		disks = []dto.DiskInfo{}
	}

	var firing []dto.AlertStatus
	if s.alertEngine != nil {
		firing = s.alertEngine.GetFiringAlerts()
	}

	report := BuildHealthReport(containers, s.GetArrayCache(), disks, firing)

	// OS-resilience: surface any degraded/unavailable data sources in the report.
	if s.ctx.Platform != nil && s.ctx.Platform.DegradedCount() > 0 {
		report.DegradedSubsystems = &dto.DegradedSubsystems{
			Count: s.ctx.Platform.DegradedCount(),
			Items: s.ctx.Platform.Snapshot(),
		}
	}

	respondJSON(w, http.StatusOK, report)
}
