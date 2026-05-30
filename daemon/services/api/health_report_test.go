package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// ---------------------------------------------------------------------------
// BuildHealthReport unit tests
// ---------------------------------------------------------------------------

func boolPtr(b bool) *bool { return &b }

// TestBuildHealthReport_StoppedContainer verifies that a stopped container
// produces a finding with a start_container ActionRef pointing at the right ID.
func TestBuildHealthReport_StoppedContainer(t *testing.T) {
	containers := []dto.ContainerInfo{
		{ID: "abc123def456", Name: "plex", State: "exited"},
	}
	array := &dto.ArrayStatus{State: "Started"}

	report := BuildHealthReport(containers, array, nil, nil)

	// Must have at least one finding
	if len(report.Findings) == 0 {
		t.Fatal("expected at least one finding for stopped container, got none")
	}

	// Find the stopped-container finding
	var found bool
	for _, f := range report.Findings {
		if f.Severity != "info" && f.Severity != "warning" {
			continue
		}
		for _, a := range f.RecommendedActions {
			if a.Action == "start_container" && a.Target == "abc123def456" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected a start_container ActionRef targeting abc123def456; findings: %+v", report.Findings)
	}
}

// TestBuildHealthReport_ArrayNotStarted verifies that a non-Started array
// produces a critical finding.
func TestBuildHealthReport_ArrayNotStarted(t *testing.T) {
	array := &dto.ArrayStatus{State: "Stopped"}

	report := BuildHealthReport(nil, array, nil, nil)

	if report.Critical == 0 {
		t.Fatal("expected at least one critical finding for array not started")
	}

	var found bool
	for _, f := range report.Findings {
		if f.Severity == "critical" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a critical finding for array not started")
	}
}

// TestBuildHealthReport_DiskSMARTFailed verifies that a disk with a non-PASSED
// SMARTStatus produces a critical finding.
func TestBuildHealthReport_DiskSMARTFailed(t *testing.T) {
	array := &dto.ArrayStatus{State: "Started"}
	disks := []dto.DiskInfo{
		{ID: "disk1", Name: "Disk 1", SMARTStatus: "FAILED"},
	}

	report := BuildHealthReport(nil, array, disks, nil)

	if report.Critical == 0 {
		t.Fatal("expected a critical finding for SMART failure, got none")
	}
}

// TestBuildHealthReport_MultipleSignals tests the combined scenario from the
// acceptance criteria: one stopped container, array not Started, disk SMART
// not PASSED → correct severity counts and correct ActionRef target.
func TestBuildHealthReport_MultipleSignals(t *testing.T) {
	containers := []dto.ContainerInfo{
		{ID: "container001", Name: "nginx", State: "exited"},
	}
	array := &dto.ArrayStatus{State: "Stopped"}
	disks := []dto.DiskInfo{
		{ID: "disk1", Name: "Disk 1", SMARTStatus: "FAILED"},
	}

	report := BuildHealthReport(containers, array, disks, nil)

	// Should have ≥ 2 critical (array + disk) and ≥ 1 info/warning (container)
	if report.Critical < 2 {
		t.Errorf("expected at least 2 critical findings, got %d", report.Critical)
	}
	if report.Info+report.Warning < 1 {
		t.Errorf("expected at least 1 info/warning finding for stopped container")
	}

	// Verify ActionRef on the stopped-container finding
	var found bool
	for _, f := range report.Findings {
		for _, a := range f.RecommendedActions {
			if a.Action == "start_container" && a.Target == "container001" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected start_container ActionRef for container001")
	}

	// Verify critical findings appear before info/warning findings (sorted)
	if len(report.Findings) > 1 {
		first := report.Findings[0].Severity
		if first != "critical" {
			t.Errorf("findings should be sorted critical-first; first severity is %q", first)
		}
	}
}

// TestBuildHealthReport_RunningContainersNoFindings ensures that healthy
// running containers do not produce any start_container findings.
func TestBuildHealthReport_RunningContainersNoFindings(t *testing.T) {
	containers := []dto.ContainerInfo{
		{ID: "abc", Name: "plex", State: "running"},
		{ID: "def", Name: "sonarr", State: "running"},
	}
	array := &dto.ArrayStatus{State: "Started"}

	report := BuildHealthReport(containers, array, nil, nil)

	for _, f := range report.Findings {
		for _, a := range f.RecommendedActions {
			if a.Action == "start_container" {
				t.Errorf("unexpected start_container action for a running container: %+v", a)
			}
		}
	}
}

// TestBuildHealthReport_UpdateAvailableIsInfo verifies that a running container
// with an available update produces an info finding (not critical or warning).
func TestBuildHealthReport_UpdateAvailableIsInfo(t *testing.T) {
	avail := true
	containers := []dto.ContainerInfo{
		{ID: "abc", Name: "plex", State: "running", UpdateAvailable: &avail},
	}
	array := &dto.ArrayStatus{State: "Started"}

	report := BuildHealthReport(containers, array, nil, nil)

	if report.Info == 0 {
		t.Error("expected at least one info finding for update-available container")
	}
	if report.Critical > 0 || report.Warning > 0 {
		t.Error("update-available finding should not be critical or warning")
	}
}

// TestBuildHealthReport_DiskHighTemp verifies that a disk above the temperature
// threshold produces a warning finding (no action, describe only).
func TestBuildHealthReport_DiskHighTemp(t *testing.T) {
	array := &dto.ArrayStatus{State: "Started"}
	disks := []dto.DiskInfo{
		{ID: "disk1", Name: "Hot Disk", SMARTStatus: "PASSED", Temperature: 60},
	}

	report := BuildHealthReport(nil, array, disks, nil)

	if report.Warning == 0 {
		t.Fatal("expected a warning finding for high-temperature disk")
	}
}

// TestBuildHealthReport_NilArray verifies graceful handling of nil array.
func TestBuildHealthReport_NilArray(t *testing.T) {
	report := BuildHealthReport(nil, nil, nil, nil)
	// Should not panic and should return an empty report
	if report.Findings == nil {
		t.Error("findings should not be nil")
	}
}

// TestBuildHealthReport_FiringAlertAppearsInFindings verifies that firing alerts
// are included in the report.
func TestBuildHealthReport_FiringAlertAppearsInFindings(t *testing.T) {
	firing := []dto.AlertStatus{
		{RuleID: "cpu-high", RuleName: "High CPU", Severity: "warning", State: "firing", Message: "CPU > 90%"},
	}
	array := &dto.ArrayStatus{State: "Started"}

	report := BuildHealthReport(nil, array, nil, firing)

	if report.Warning == 0 {
		t.Error("expected a warning finding from firing alert")
	}
}

// TestBuildHealthReport_SeverityCounts verifies that Critical/Warning/Info
// counters match the slice contents.
func TestBuildHealthReport_SeverityCounts(t *testing.T) {
	containers := []dto.ContainerInfo{
		{ID: "c1", Name: "c1", State: "exited"},
	}
	array := &dto.ArrayStatus{State: "Stopped"}
	disks := []dto.DiskInfo{
		{ID: "d1", Name: "d1", SMARTStatus: "FAILED"},
		{ID: "d2", Name: "d2", SMARTStatus: "PASSED", Temperature: 57},
	}

	report := BuildHealthReport(containers, array, disks, nil)

	manualCritical, manualWarning, manualInfo := 0, 0, 0
	for _, f := range report.Findings {
		switch f.Severity {
		case "critical":
			manualCritical++
		case "warning":
			manualWarning++
		default:
			manualInfo++
		}
	}

	if report.Critical != manualCritical {
		t.Errorf("Critical count mismatch: field=%d, slice=%d", report.Critical, manualCritical)
	}
	if report.Warning != manualWarning {
		t.Errorf("Warning count mismatch: field=%d, slice=%d", report.Warning, manualWarning)
	}
	if report.Info != manualInfo {
		t.Errorf("Info count mismatch: field=%d, slice=%d", report.Info, manualInfo)
	}
}

// TestBuildHealthReport_HighRestartCountIsWarning verifies that a stopped container
// with high restart count produces a warning (not just info).
func TestBuildHealthReport_HighRestartCountIsWarning(t *testing.T) {
	containers := []dto.ContainerInfo{
		{ID: "c1", Name: "flapping", State: "exited", RestartCount: 5},
	}
	array := &dto.ArrayStatus{State: "Started"}

	report := BuildHealthReport(containers, array, nil, nil)

	if report.Warning == 0 {
		t.Error("expected a warning for container with high restart count")
	}
}

// ---------------------------------------------------------------------------
// handleHealthReport REST handler integration tests
// ---------------------------------------------------------------------------

// TestHandleHealthReport_ReturnsJSON verifies the endpoint returns valid JSON
// with the expected shape.
func TestHandleHealthReport_ReturnsJSON(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/health/report", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var report dto.HealthReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("response is not a valid HealthReport: %v", err)
	}

	// Findings must not be nil (may be empty when no signals)
	if report.Findings == nil {
		t.Error("findings field should not be nil")
	}

	if report.GeneratedAt.IsZero() {
		t.Error("generated_at should not be zero")
	}
}

// TestHandleHealthReport_ContentType verifies Content-Type is application/json.
func TestHandleHealthReport_ContentType(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/health/report", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

// TestHandleHealthReport_WithStoppedContainerInCache verifies that a container
// in the cache that is not running appears as a finding.
func TestHandleHealthReport_WithStoppedContainerInCache(t *testing.T) {
	server, _ := setupTestServer()

	// Seed the docker cache with a stopped container
	containers := []dto.ContainerInfo{
		{ID: "deadbeef1234", Name: "test-container", State: "exited"},
	}
	server.dockerCache.Store(&containers)

	req, err := http.NewRequest("GET", "/api/v1/health/report", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	var report dto.HealthReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("could not decode report: %v", err)
	}

	if len(report.Findings) == 0 {
		t.Fatal("expected at least one finding for stopped container")
	}

	var foundAction bool
	for _, f := range report.Findings {
		for _, a := range f.RecommendedActions {
			if a.Action == "start_container" && a.Target == "deadbeef1234" {
				foundAction = true
			}
		}
	}
	if !foundAction {
		t.Error("expected start_container ActionRef for deadbeef1234 in report findings")
	}
}
