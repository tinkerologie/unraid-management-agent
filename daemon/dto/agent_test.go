package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentConfigOmitsAPIKey(t *testing.T) {
	cfg := DefaultAgentConfig()
	cfg.APIKey = "secret-key-value"
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "secret-key-value") {
		t.Fatalf("API key leaked into JSON: %s", b)
	}
}

func TestDefaultAgentConfigDefaults(t *testing.T) {
	cfg := DefaultAgentConfig()
	if cfg.Enabled {
		t.Error("agent must be disabled by default")
	}
	if cfg.MaxIterations <= 0 || cfg.MaxTokensPerSession <= 0 {
		t.Error("caps must be positive")
	}
	if cfg.Autonomy[RiskReadOnly] != ModeAuto {
		t.Errorf("read-only must default to auto, got %q", cfg.Autonomy[RiskReadOnly])
	}
}
