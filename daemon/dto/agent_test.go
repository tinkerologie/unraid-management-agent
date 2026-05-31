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

func TestDefaultAgentConfigPhase2Defaults(t *testing.T) {
	cfg := DefaultAgentConfig()
	if cfg.WakeDebounceSecs <= 0 || cfg.WakeCooldownSecs <= 0 {
		t.Error("wake debounce/cooldown must be positive")
	}
	if cfg.MaxConcurrentSessions <= 0 {
		t.Error("max concurrent sessions must be positive")
	}
	if cfg.ApprovalTTLSecs <= 0 {
		t.Error("approval TTL must be positive")
	}
	if len(cfg.ForbidList) == 0 {
		t.Error("forbid-list should have default irreversible-op entries")
	}
}

func TestAgentSessionPendingApprovalRoundTrips(t *testing.T) {
	s := AgentSession{
		ID: "sess-1", Status: SessionAwaitingApproval,
		PendingApproval: &ApprovalRequest{ActionID: "act-1", ToolName: "stop_array", RiskTier: RiskHigh},
		Transcript:      []AgentMessage{{Role: "user", Content: "fix it"}},
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back AgentSession
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.PendingApproval == nil || back.PendingApproval.ToolName != "stop_array" {
		t.Fatalf("pending approval lost: %+v", back.PendingApproval)
	}
	if len(back.Transcript) != 1 || back.Transcript[0].Role != "user" {
		t.Fatalf("transcript lost: %+v", back.Transcript)
	}
}
