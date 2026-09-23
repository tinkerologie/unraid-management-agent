package collectors

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// setupOSUpdateTest redirects both candidate paths and the current-version path
// to a fresh temp directory, then restores originals after the test.
func setupOSUpdateTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	origPaths := osUpdateCandidatePaths
	origVersionPath := osCurrentVersionPath

	// Point candidate search at the temp dir; the files don't exist yet.
	osUpdateCandidatePaths = []string{
		filepath.Join(dir, "result.json"),
		filepath.Join(dir, "result"),
		filepath.Join(dir, "update.ini"),
	}
	// Point current-version reader at a temp file (injected per test case).
	osCurrentVersionPath = filepath.Join(dir, "unraid-version")

	t.Cleanup(func() {
		osUpdateCandidatePaths = origPaths
		osCurrentVersionPath = origVersionPath
	})

	return dir
}

// writeVersionFile writes /etc/unraid-version-style content to path.
func writeVersionFile(t *testing.T, path, version string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(`version="`+version+`"`+"\n"), 0o600); err != nil {
		t.Fatalf("writeVersionFile: %v", err)
	}
}

// writeLatestFile writes a latest-version candidate file (INI format).
func writeLatestFile(t *testing.T, dir, filename, version string) {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte("version="+version+"\n"), 0o600); err != nil {
		t.Fatalf("writeLatestFile: %v", err)
	}
}

// writeLatestJSONFile writes a latest-version candidate file (JSON format from unraidcheck).
func writeLatestJSONFile(t *testing.T, dir, filename, version string, isNewer bool) {
	t.Helper()
	path := filepath.Join(dir, filename)
	content := fmt.Sprintf(`{"version":%q,"isNewer":%t}`+"\n", version, isNewer)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeLatestJSONFile: %v", err)
	}
}

// ── 3-case status tests ─────────────────────────────────────────────────────

func TestOSUpdate_NoLatestFile_StatusUnknown(t *testing.T) {
	dir := setupOSUpdateTest(t)
	writeVersionFile(t, filepath.Join(dir, "unraid-version"), "7.2.0")
	// No candidate file created → unknown.

	hub := domain.NewEventBus(16)
	c := NewOSUpdateCollector(&domain.Context{Hub: hub})

	result, err := c.defaultCheck()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != dto.OSUpdateStatusUnknown {
		t.Errorf("Status = %q, want %q", result.Status, dto.OSUpdateStatusUnknown)
	}
	if result.UpdateAvailable {
		t.Error("UpdateAvailable should be false when status is unknown")
	}
	if result.LatestVersion != "" {
		t.Errorf("LatestVersion should be empty, got %q", result.LatestVersion)
	}
}

func TestOSUpdate_LatestGreaterThanCurrent_StatusAvailable(t *testing.T) {
	dir := setupOSUpdateTest(t)
	writeVersionFile(t, filepath.Join(dir, "unraid-version"), "7.2.0")
	writeLatestFile(t, dir, "result", "7.2.1")

	hub := domain.NewEventBus(16)
	c := NewOSUpdateCollector(&domain.Context{Hub: hub})

	result, err := c.defaultCheck()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != dto.OSUpdateStatusAvailable {
		t.Errorf("Status = %q, want %q", result.Status, dto.OSUpdateStatusAvailable)
	}
	if !result.UpdateAvailable {
		t.Error("UpdateAvailable should be true")
	}
	if result.LatestVersion != "7.2.1" {
		t.Errorf("LatestVersion = %q, want 7.2.1", result.LatestVersion)
	}
	if result.CurrentVersion != "7.2.0" {
		t.Errorf("CurrentVersion = %q, want 7.2.0", result.CurrentVersion)
	}
}

func TestOSUpdate_LatestEqualsCurrent_StatusUpToDate(t *testing.T) {
	dir := setupOSUpdateTest(t)
	writeVersionFile(t, filepath.Join(dir, "unraid-version"), "7.2.1")
	writeLatestFile(t, dir, "result", "7.2.1")

	hub := domain.NewEventBus(16)
	c := NewOSUpdateCollector(&domain.Context{Hub: hub})

	result, err := c.defaultCheck()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != dto.OSUpdateStatusUpToDate {
		t.Errorf("Status = %q, want %q", result.Status, dto.OSUpdateStatusUpToDate)
	}
	if result.UpdateAvailable {
		t.Error("UpdateAvailable should be false when up to date")
	}
}

func TestOSUpdate_JSONResult(t *testing.T) {
	testCases := []struct {
		name                string
		currentVersion      string
		jsonContent         string
		wantStatus          string
		wantUpdateAvailable bool
		wantLatestVersion   string
	}{
		{
			name:                "isNewer true",
			currentVersion:      "7.2.3",
			jsonContent:         `{"version":"7.3.2","isNewer":true}`,
			wantStatus:          dto.OSUpdateStatusAvailable,
			wantUpdateAvailable: true,
			wantLatestVersion:   "7.3.2",
		},
		{
			name:                "isNewer false with same version",
			currentVersion:      "7.3.2",
			jsonContent:         `{"version":"7.3.2","isNewer":false}`,
			wantStatus:          dto.OSUpdateStatusUpToDate,
			wantUpdateAvailable: false,
			wantLatestVersion:   "7.3.2",
		},
		{
			name:                "isNewer false despite differing version",
			currentVersion:      "7.2.0",
			jsonContent:         `{"version":"7.3.0","isNewer":false}`,
			wantStatus:          dto.OSUpdateStatusUpToDate,
			wantUpdateAvailable: false,
			wantLatestVersion:   "7.3.0",
		},
		{
			name:                "no isNewer property, candidate version is newer",
			currentVersion:      "7.2.0",
			jsonContent:         `{"version":"7.2.5"}`,
			wantStatus:          dto.OSUpdateStatusAvailable,
			wantUpdateAvailable: true,
			wantLatestVersion:   "7.2.5",
		},
		{
			name:                "no isNewer property, candidate version is older",
			currentVersion:      "7.3.2",
			jsonContent:         `{"version":"7.2.0"}`,
			wantStatus:          dto.OSUpdateStatusUpToDate,
			wantUpdateAvailable: false,
			wantLatestVersion:   "7.2.0",
		},
		{
			name:                "no isNewer property, candidate version is equal",
			currentVersion:      "7.3.2",
			jsonContent:         `{"version":"7.3.2"}`,
			wantStatus:          dto.OSUpdateStatusUpToDate,
			wantUpdateAvailable: false,
			wantLatestVersion:   "7.3.2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupOSUpdateTest(t)
			writeVersionFile(t, filepath.Join(dir, "unraid-version"), tc.currentVersion)
			path := filepath.Join(dir, "result.json")
			if err := os.WriteFile(path, []byte(tc.jsonContent+"\n"), 0o600); err != nil {
				t.Fatalf("write result.json: %v", err)
			}

			hub := domain.NewEventBus(16)
			c := NewOSUpdateCollector(&domain.Context{Hub: hub})

			result, err := c.defaultCheck()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", result.Status, tc.wantStatus)
			}
			if result.UpdateAvailable != tc.wantUpdateAvailable {
				t.Errorf("UpdateAvailable = %v, want %v", result.UpdateAvailable, tc.wantUpdateAvailable)
			}
			if result.LatestVersion != tc.wantLatestVersion {
				t.Errorf("LatestVersion = %q, want %q", result.LatestVersion, tc.wantLatestVersion)
			}
		})
	}
}

func TestIsVersionNewer(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{"7.3.0", "7.2.0", true},
		{"7.2.1", "7.2.0", true},
		{"v7.2.1", "7.2.0", true},
		{"7.2.0", "7.2.1", false},
		{"7.2.0", "7.2.0", false},
		{"7.1.0", "7.2.0", false},
		{"7.0.0-beta.2", "7.0.0-beta.1", false},
		{"", "7.2.0", false},
		{"7.2.0", "", true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s vs %s", tt.latest, tt.current), func(t *testing.T) {
			got := isVersionNewer(tt.latest, tt.current)
			if got != tt.want {
				t.Errorf("isVersionNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

// ── Publish / dedupe tests (mirroring plugin_update_test) ───────────────────

func TestOSUpdateCollector_PublishesAndDedupes(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicOSUpdateUpdate.Name)
	defer hub.Unsub(sub)

	result := &dto.OSUpdateStatus{
		CurrentVersion:  "7.2.0",
		LatestVersion:   "7.2.1",
		UpdateAvailable: true,
		Status:          dto.OSUpdateStatusAvailable,
		Timestamp:       time.Now(),
	}

	c := NewOSUpdateCollector(&domain.Context{Hub: hub})
	c.CheckFn = func() (*dto.OSUpdateStatus, error) { return result, nil }

	c.Collect()
	select {
	case msg := <-sub:
		got, ok := msg.(*dto.OSUpdateStatus)
		if !ok || !got.UpdateAvailable {
			t.Fatalf("unexpected first publish: %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("expected first publish, got none")
	}

	// Identical result → must NOT publish again.
	c.Collect()
	select {
	case msg := <-sub:
		t.Fatalf("expected no re-publish on unchanged result, got %#v", msg)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestOSUpdateCollector_NilCheckFnIsSafe(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicOSUpdateUpdate.Name)
	defer hub.Unsub(sub)

	c := NewOSUpdateCollector(&domain.Context{Hub: hub})
	c.CheckFn = nil
	c.Collect() // must not panic, must not publish

	select {
	case msg := <-sub:
		t.Fatalf("expected no publish when CheckFn is nil, got %#v", msg)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestOSUpdateCollector_RepublishesOnChange(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicOSUpdateUpdate.Name)
	defer hub.Unsub(sub)

	c := NewOSUpdateCollector(&domain.Context{Hub: hub})
	c.CheckFn = func() (*dto.OSUpdateStatus, error) {
		return &dto.OSUpdateStatus{
			CurrentVersion: "7.2.0",
			Status:         dto.OSUpdateStatusUnknown,
			Timestamp:      time.Now(),
		}, nil
	}
	c.Collect()

	select {
	case <-sub: // drain first publish
	case <-time.After(time.Second):
		t.Fatal("expected first publish, got none")
	}

	// Change status → must publish again.
	c.CheckFn = func() (*dto.OSUpdateStatus, error) {
		return &dto.OSUpdateStatus{
			CurrentVersion:  "7.2.0",
			LatestVersion:   "7.2.1",
			UpdateAvailable: true,
			Status:          dto.OSUpdateStatusAvailable,
			Timestamp:       time.Now(),
		}, nil
	}
	c.Collect()

	select {
	case <-sub: // success
	case <-time.After(time.Second):
		t.Fatal("expected republish after status change, got none")
	}
}

func TestOSUpdateCollector_CheckErrorNoPublish(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicOSUpdateUpdate.Name)
	defer hub.Unsub(sub)

	c := NewOSUpdateCollector(&domain.Context{Hub: hub})
	c.CheckFn = func() (*dto.OSUpdateStatus, error) { return nil, fmt.Errorf("boom") }
	c.Collect()

	select {
	case <-sub:
		t.Fatal("expected no publish on check error")
	case <-time.After(150 * time.Millisecond):
	}
}

// ── Notification baseline-then-notify ───────────────────────────────────────

func TestOSUpdateNotify_FiresOnNewTransitionOnly(t *testing.T) {
	hub := domain.NewEventBus(16)
	var notified []string

	c := NewOSUpdateCollector(&domain.Context{Hub: hub})
	c.NotifyFn = func(latest string) { notified = append(notified, latest) }

	// Step 1: first run with update available — sets baseline, must NOT notify.
	c.CheckFn = func() (*dto.OSUpdateStatus, error) {
		return &dto.OSUpdateStatus{
			CurrentVersion: "7.2.0", LatestVersion: "7.2.1",
			UpdateAvailable: true, Status: dto.OSUpdateStatusAvailable,
			Timestamp: time.Now(),
		}, nil
	}
	c.Collect()
	if len(notified) != 0 {
		t.Fatalf("first run should not notify, got %v", notified)
	}

	// Step 2: same state — no transition, must NOT notify.
	c.Collect()
	if len(notified) != 0 {
		t.Fatalf("second run (same state) should not notify, got %v", notified)
	}
}

func TestOSUpdateNotify_FiresAfterBaselineIfNewUpdateAppears(t *testing.T) {
	hub := domain.NewEventBus(16)
	var notified []string

	c := NewOSUpdateCollector(&domain.Context{Hub: hub})
	c.NotifyFn = func(latest string) { notified = append(notified, latest) }

	// Step 1: baseline — up to date, no notify.
	c.CheckFn = func() (*dto.OSUpdateStatus, error) {
		return &dto.OSUpdateStatus{
			CurrentVersion:  "7.2.0",
			UpdateAvailable: false, Status: dto.OSUpdateStatusUpToDate,
			Timestamp: time.Now(),
		}, nil
	}
	c.Collect()
	if len(notified) != 0 {
		t.Fatalf("baseline run should not notify, got %v", notified)
	}

	// Step 2: update appears — must notify.
	c.CheckFn = func() (*dto.OSUpdateStatus, error) {
		return &dto.OSUpdateStatus{
			CurrentVersion: "7.2.0", LatestVersion: "7.2.1",
			UpdateAvailable: true, Status: dto.OSUpdateStatusAvailable,
			Timestamp: time.Now(),
		}, nil
	}
	c.Collect()
	if len(notified) != 1 || notified[0] != "7.2.1" {
		t.Fatalf("expected notify [7.2.1], got %v", notified)
	}
}

func TestOSUpdateNotify_NilIsSafe(t *testing.T) {
	hub := domain.NewEventBus(16)
	c := NewOSUpdateCollector(&domain.Context{Hub: hub})
	c.CheckFn = func() (*dto.OSUpdateStatus, error) {
		return &dto.OSUpdateStatus{
			CurrentVersion: "7.2.0", LatestVersion: "7.2.1",
			UpdateAvailable: true, Status: dto.OSUpdateStatusAvailable,
			Timestamp: time.Now(),
		}, nil
	}
	// NotifyFn is nil — must not panic.
	c.Collect()
	c.Collect()
}

// ── Second candidate file fallback ──────────────────────────────────────────

func TestOSUpdate_FallbackToSecondCandidatePath(t *testing.T) {
	dir := setupOSUpdateTest(t)
	writeVersionFile(t, filepath.Join(dir, "unraid-version"), "7.2.0")
	// Write to the SECOND candidate (update.ini), not the first (result).
	writeLatestFile(t, dir, "update.ini", "7.2.2")

	hub := domain.NewEventBus(16)
	c := NewOSUpdateCollector(&domain.Context{Hub: hub})

	result, err := c.defaultCheck()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != dto.OSUpdateStatusAvailable {
		t.Errorf("Status = %q, want update_available", result.Status)
	}
	if result.LatestVersion != "7.2.2" {
		t.Errorf("LatestVersion = %q, want 7.2.2", result.LatestVersion)
	}
}
