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

func TestDefaultAgentConfigPhase3Defaults(t *testing.T) {
	cfg := DefaultAgentConfig()
	if !cfg.MemoryEnabled {
		t.Error("memory should be enabled by default")
	}
	if cfg.MaxIncidents <= 0 || cfg.RecallTopK <= 0 {
		t.Error("MaxIncidents and RecallTopK must be positive")
	}
}

func TestAgentIncidentAndPreferenceRoundTrip(t *testing.T) {
	inc := AgentIncident{ID: "inc-1", Signature: "watchdog:Plex HTTP", Goal: "fix plex", Outcome: "completed", Summary: "restarted plex", Actions: []string{"restart_container"}}
	pref := AgentPreference{ID: "pref-1", Kind: "auto_approve_tool", Subject: "restart_container", Status: PreferencePending}
	for _, v := range []any{inc, pref} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if len(b) == 0 {
			t.Fatal("empty marshal")
		}
	}
	sess := AgentSession{ID: "s1", Plan: []PlanStep{{Intent: "check disk", Tool: "get_system_info"}}}
	b, _ := json.Marshal(sess)
	var back AgentSession
	if err := json.Unmarshal(b, &back); err != nil || len(back.Plan) != 1 || back.Plan[0].Intent != "check disk" {
		t.Fatalf("plan round-trip failed: %+v err=%v", back.Plan, err)
	}
}
