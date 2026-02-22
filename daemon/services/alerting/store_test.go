package alerting

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
		if store.filePath != filepath.Join(DefaultConfigDir, AlertsConfigFile) {
			t.Errorf("expected default path, got %s", store.filePath)
		}
	})

	t.Run("custom config dir", func(t *testing.T) {
		store := NewStore("/tmp/test-alerts")
		if store.filePath != "/tmp/test-alerts/alerts.json" {
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
	rules := store.GetRules()
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

func TestStoreLoadInvalid(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := os.WriteFile(filepath.Join(dir, AlertsConfigFile), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Load(); err == nil {
		t.Error("Load should error on invalid JSON")
	}
}

func TestStoreCRUD(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	rule := dto.AlertRule{
		ID:         "test-rule-1",
		Name:       "High CPU",
		Expression: "CPU > 90",
		Severity:   "warning",
		Channels:   []string{"unraid"},
		Enabled:    true,
	}

	// Create
	if err := store.CreateRule(rule); err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}

	// Get
	got, err := store.GetRule("test-rule-1")
	if err != nil {
		t.Fatalf("GetRule failed: %v", err)
	}
	if got.Name != "High CPU" {
		t.Errorf("expected name 'High CPU', got '%s'", got.Name)
	}
	if got.CooldownMinutes != 5 {
		t.Errorf("expected default cooldown 5, got %d", got.CooldownMinutes)
	}

	// List
	rules := store.GetRules()
	if len(rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(rules))
	}

	// Update
	rule.Name = "Very High CPU"
	rule.Expression = "CPU > 95"
	if err := store.UpdateRule(rule); err != nil {
		t.Fatalf("UpdateRule failed: %v", err)
	}
	got, _ = store.GetRule("test-rule-1")
	if got.Name != "Very High CPU" {
		t.Errorf("expected updated name, got '%s'", got.Name)
	}

	// Delete
	if err := store.DeleteRule("test-rule-1"); err != nil {
		t.Fatalf("DeleteRule failed: %v", err)
	}
	rules = store.GetRules()
	if len(rules) != 0 {
		t.Errorf("expected 0 rules after delete, got %d", len(rules))
	}
}

func TestStoreDuplicateID(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	rule := dto.AlertRule{ID: "dup", Name: "First", Expression: "CPU > 50", Severity: "info", Enabled: true}
	if err := store.CreateRule(rule); err != nil {
		t.Fatal(err)
	}
	err := store.CreateRule(rule)
	if err == nil {
		t.Error("expected error for duplicate ID")
	}
}

func TestStoreMaxRules(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	for i := range MaxAlertRules {
		rule := dto.AlertRule{
			ID:         fmt.Sprintf("rule-%d", i),
			Name:       fmt.Sprintf("Rule %d", i),
			Expression: "CPU > 50",
			Severity:   "info",
			Enabled:    true,
		}
		if err := store.CreateRule(rule); err != nil {
			t.Fatalf("CreateRule %d failed: %v", i, err)
		}
	}
	err := store.CreateRule(dto.AlertRule{ID: "overflow", Name: "Too Many", Expression: "CPU > 50", Severity: "info"})
	if err == nil {
		t.Error("expected error when exceeding max rules")
	}
}

func TestStoreGetNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	_, err := store.GetRule("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent rule")
	}
}

func TestStoreUpdateNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	err := store.UpdateRule(dto.AlertRule{ID: "nonexistent", Name: "Test", Expression: "CPU > 50"})
	if err == nil {
		t.Error("expected error for updating nonexistent rule")
	}
}

func TestStoreDeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	err := store.DeleteRule("nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent rule")
	}
}

func TestStoreGetEnabledRules(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	store.CreateRule(dto.AlertRule{ID: "r1", Name: "Enabled", Expression: "CPU > 50", Severity: "info", Enabled: true})
	store.CreateRule(dto.AlertRule{ID: "r2", Name: "Disabled", Expression: "CPU > 50", Severity: "info", Enabled: false})
	store.CreateRule(dto.AlertRule{ID: "r3", Name: "Enabled2", Expression: "CPU > 50", Severity: "info", Enabled: true})
	enabled := store.GetEnabledRules()
	if len(enabled) != 2 {
		t.Errorf("expected 2 enabled rules, got %d", len(enabled))
	}
}

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()

	// Create and write
	store1 := NewStore(dir)
	store1.CreateRule(dto.AlertRule{ID: "persist", Name: "Persistent", Expression: "CPU > 50", Severity: "info", Enabled: true})

	// Load in a new store instance
	store2 := NewStore(dir)
	if err := store2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	rules := store2.GetRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule after reload, got %d", len(rules))
	}
	if rules[0].ID != "persist" {
		t.Errorf("expected rule ID 'persist', got '%s'", rules[0].ID)
	}
}
