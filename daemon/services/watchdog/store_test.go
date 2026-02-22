package watchdog

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestNewStore(t *testing.T) {
	t.Run("default config dir", func(t *testing.T) {
		store := NewStore("")
		if store.filePath != filepath.Join(DefaultConfigDir, HealthChecksConfigFile) {
			t.Errorf("expected default path, got %s", store.filePath)
		}
	})

	t.Run("custom config dir", func(t *testing.T) {
		store := NewStore("/tmp/test-hc")
		if store.filePath != "/tmp/test-hc/healthchecks.json" {
			t.Errorf("expected custom path, got %s", store.filePath)
		}
	})
}

func TestStoreLoadMissing(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatalf("Load should not error on missing file: %v", err)
	}
	if len(store.GetChecks()) != 0 {
		t.Error("expected 0 checks")
	}
}

func TestStoreLoadInvalid(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := os.WriteFile(filepath.Join(dir, HealthChecksConfigFile), []byte("bad json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Load(); err == nil {
		t.Error("expected error on invalid JSON")
	}
}

func TestStoreCRUD(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	check := dto.HealthCheck{
		ID:      "plex-http",
		Name:    "Plex Web",
		Type:    dto.HealthCheckHTTP,
		Target:  "http://localhost:32400/web",
		OnFail:  "notify",
		Enabled: true,
	}

	// Create
	if err := store.CreateCheck(check); err != nil {
		t.Fatalf("CreateCheck failed: %v", err)
	}

	// Verify defaults applied
	got, err := store.GetCheck("plex-http")
	if err != nil {
		t.Fatalf("GetCheck failed: %v", err)
	}
	if got.IntervalSeconds != DefaultIntervalSeconds {
		t.Errorf("expected default interval %d, got %d", DefaultIntervalSeconds, got.IntervalSeconds)
	}
	if got.TimeoutSeconds != DefaultTimeoutSeconds {
		t.Errorf("expected default timeout %d, got %d", DefaultTimeoutSeconds, got.TimeoutSeconds)
	}
	if got.SuccessCode != DefaultSuccessCode {
		t.Errorf("expected default success code %d, got %d", DefaultSuccessCode, got.SuccessCode)
	}

	// List
	checks := store.GetChecks()
	if len(checks) != 1 {
		t.Errorf("expected 1 check, got %d", len(checks))
	}

	// Update
	check.Name = "Plex Media Server"
	check.IntervalSeconds = 60
	if err := store.UpdateCheck(check); err != nil {
		t.Fatalf("UpdateCheck failed: %v", err)
	}
	got, _ = store.GetCheck("plex-http")
	if got.Name != "Plex Media Server" {
		t.Errorf("expected updated name, got '%s'", got.Name)
	}

	// Delete
	if err := store.DeleteCheck("plex-http"); err != nil {
		t.Fatalf("DeleteCheck failed: %v", err)
	}
	if len(store.GetChecks()) != 0 {
		t.Error("expected 0 checks after delete")
	}
}

func TestStoreDuplicateID(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	check := dto.HealthCheck{ID: "dup", Name: "Test", Type: dto.HealthCheckTCP, Target: "localhost:80", Enabled: true}
	if err := store.CreateCheck(check); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateCheck(check); err == nil {
		t.Error("expected error for duplicate ID")
	}
}

func TestStoreMaxChecks(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	for i := range MaxHealthChecks {
		check := dto.HealthCheck{
			ID:      fmt.Sprintf("check-%d", i),
			Name:    fmt.Sprintf("Check %d", i),
			Type:    dto.HealthCheckTCP,
			Target:  "localhost:80",
			Enabled: true,
		}
		if err := store.CreateCheck(check); err != nil {
			t.Fatalf("CreateCheck %d failed: %v", i, err)
		}
	}
	err := store.CreateCheck(dto.HealthCheck{ID: "overflow", Name: "Too Many", Type: dto.HealthCheckTCP, Target: "localhost:80"})
	if err == nil {
		t.Error("expected error when exceeding max checks")
	}
}

func TestStoreNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if _, err := store.GetCheck("nope"); err == nil {
		t.Error("expected error for nonexistent check")
	}
	if err := store.UpdateCheck(dto.HealthCheck{ID: "nope"}); err == nil {
		t.Error("expected error for update nonexistent")
	}
	if err := store.DeleteCheck("nope"); err == nil {
		t.Error("expected error for delete nonexistent")
	}
}

func TestStoreGetEnabledChecks(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	store.CreateCheck(dto.HealthCheck{ID: "a", Name: "A", Type: dto.HealthCheckTCP, Target: "localhost:80", Enabled: true})
	store.CreateCheck(dto.HealthCheck{ID: "b", Name: "B", Type: dto.HealthCheckTCP, Target: "localhost:80", Enabled: false})
	store.CreateCheck(dto.HealthCheck{ID: "c", Name: "C", Type: dto.HealthCheckTCP, Target: "localhost:80", Enabled: true})
	enabled := store.GetEnabledChecks()
	if len(enabled) != 2 {
		t.Errorf("expected 2 enabled, got %d", len(enabled))
	}
}

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()
	store1 := NewStore(dir)
	store1.CreateCheck(dto.HealthCheck{ID: "persist", Name: "Persistent", Type: dto.HealthCheckHTTP, Target: "http://localhost", Enabled: true})

	store2 := NewStore(dir)
	if err := store2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	checks := store2.GetChecks()
	if len(checks) != 1 {
		t.Fatalf("expected 1 check after reload, got %d", len(checks))
	}
	if checks[0].ID != "persist" {
		t.Errorf("expected ID 'persist', got '%s'", checks[0].ID)
	}
}

func TestStoreDefaultsNotAppliedForTCP(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	store.CreateCheck(dto.HealthCheck{
		ID:      "tcp-test",
		Name:    "TCP Test",
		Type:    dto.HealthCheckTCP,
		Target:  "localhost:8080",
		Enabled: true,
	})
	got, _ := store.GetCheck("tcp-test")
	if got.SuccessCode != 0 {
		t.Errorf("TCP check should not have SuccessCode set, got %d", got.SuccessCode)
	}
}
