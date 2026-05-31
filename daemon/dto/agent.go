// daemon/dto/agent.go
package dto

import "time"

// RiskTier classifies how dangerous a tool's effect is.
type RiskTier string

const (
	RiskReadOnly RiskTier = "read_only" // never changes state
	RiskLow      RiskTier = "low"       // reversible, low blast radius (e.g. restart container)
	RiskHigh     RiskTier = "high"      // requires approval (e.g. stop array) — Phase 2
)

// AutonomyMode is how the policy gate treats a tier.
type AutonomyMode string

const (
	ModeAuto    AutonomyMode = "auto"    // execute without asking
	ModeApprove AutonomyMode = "approve" // require human approval (Phase 2)
	ModeForbid  AutonomyMode = "forbid"  // never execute
)

// AgentSessionStatus is the lifecycle state of a session.
type AgentSessionStatus string

const (
	SessionRunning          AgentSessionStatus = "running"
	SessionCompleted        AgentSessionStatus = "completed"
	SessionFailed           AgentSessionStatus = "failed"
	SessionCancelled        AgentSessionStatus = "cancelled"
	SessionAwaitingApproval AgentSessionStatus = "awaiting_approval"
)

// AgentConfig holds the agent's runtime configuration (persisted as JSON).
type AgentConfig struct {
	Enabled               bool                      `json:"enabled"`
	Provider              string                    `json:"provider"` // "anthropic" | "mock"
	Model                 string                    `json:"model"`
	Endpoint              string                    `json:"endpoint,omitempty"`
	APIKey                string                    `json:"-"` // never serialized; from env/secret file
	Autonomy              map[RiskTier]AutonomyMode `json:"autonomy"`
	MaxIterations         int                       `json:"max_iterations"`
	MaxTokensPerSession   int                       `json:"max_tokens_per_session"`
	SessionDeadlineSecs   int                       `json:"session_deadline_secs"`
	WakeDebounceSecs      int                       `json:"wake_debounce_secs"`
	WakeCooldownSecs      int                       `json:"wake_cooldown_secs"`
	MaxConcurrentSessions int                       `json:"max_concurrent_sessions"`
	ApprovalTTLSecs       int                       `json:"approval_ttl_secs"`
	ForbidList            []string                  `json:"forbid_list"`
	MemoryEnabled         bool                      `json:"memory_enabled"`
	MaxIncidents          int                       `json:"max_incidents"`
	RecallTopK            int                       `json:"recall_top_k"`
}

// DefaultAgentConfig returns safe defaults: disabled, conservative caps, tiered autonomy.
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		Enabled:               false,
		Provider:              "anthropic",
		Model:                 "claude-opus-4-8",
		Autonomy:              map[RiskTier]AutonomyMode{RiskReadOnly: ModeAuto, RiskLow: ModeAuto, RiskHigh: ModeApprove},
		MaxIterations:         12,
		MaxTokensPerSession:   60000,
		SessionDeadlineSecs:   180,
		WakeDebounceSecs:      30,
		WakeCooldownSecs:      300,
		MaxConcurrentSessions: 2,
		ApprovalTTLSecs:       3600,
		ForbidList:            []string{"format_disk", "clear_parity", "disable_parity", "partition_disk", "delete_array_disk"},
		MemoryEnabled:         true,
		MaxIncidents:          200,
		RecallTopK:            3,
	}
}

// AgentWakeEvent is published on the agent_wake topic to trigger an autonomous
// investigation. Source is "alert" or "watchdog"; Subsystem is the dedup key.
type AgentWakeEvent struct {
	Source    string    `json:"source"`
	Subsystem string    `json:"subsystem"`
	Severity  string    `json:"severity"`
	Title     string    `json:"title"`
	Detail    string    `json:"detail"`
	At        time.Time `json:"at"`
}

// AgentMsgToolCall is a tool call recorded inside a persisted transcript message.
type AgentMsgToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

// AgentMessage is a persisted conversation turn used to resume a paused loop.
type AgentMessage struct {
	Role       string             `json:"role"`
	Content    string             `json:"content,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	ToolCalls  []AgentMsgToolCall `json:"tool_calls,omitempty"`
}

// ApprovalRequest describes a high-risk tool call paused awaiting a human decision.
type ApprovalRequest struct {
	ActionID    string    `json:"action_id"`
	ToolName    string    `json:"tool_name"`
	Args        string    `json:"args"`
	RiskTier    RiskTier  `json:"risk_tier"`
	Reason      string    `json:"reason"`
	RequestedAt time.Time `json:"requested_at"`
}

// AgentToolCall records one tool invocation within a session.
type AgentToolCall struct {
	Name     string    `json:"name"`
	Args     string    `json:"args"` // raw JSON arguments
	RiskTier RiskTier  `json:"risk_tier"`
	Result   string    `json:"result"`
	Error    string    `json:"error,omitempty"`
	At       time.Time `json:"at"`
}

// AgentStep is one perceive→think→act iteration of the loop.
type AgentStep struct {
	Index     int             `json:"index"`
	Thought   string          `json:"thought,omitempty"`
	ToolCalls []AgentToolCall `json:"tool_calls,omitempty"`
	At        time.Time       `json:"at"`
}

// PlanStep is one step of a goal decomposition produced by the planner.
type PlanStep struct {
	Intent string `json:"intent"`
	Tool   string `json:"tool,omitempty"`
	Done   bool   `json:"done"`
}

// AgentIncident is an episodic memory record of a finished session.
type AgentIncident struct {
	ID        string    `json:"id"`
	Signature string    `json:"signature"`
	Goal      string    `json:"goal"`
	Outcome   string    `json:"outcome"`
	Summary   string    `json:"summary"`
	Actions   []string  `json:"actions,omitempty"`
	At        time.Time `json:"at"`
}

// PreferenceStatus is the lifecycle of a learned preference.
type PreferenceStatus string

const (
	PreferencePending PreferenceStatus = "pending"
	PreferenceActive  PreferenceStatus = "active"
)

// AgentPreference is a learned, suggest-not-mutate operator preference.
type AgentPreference struct {
	ID      string           `json:"id"`
	Kind    string           `json:"kind"`
	Subject string           `json:"subject"`
	Note    string           `json:"note,omitempty"`
	Status  PreferenceStatus `json:"status"`
	At      time.Time        `json:"at"`
}

// AgentSession is a full agent run (on-demand in Phase 1).
type AgentSession struct {
	ID              string             `json:"id"`
	Goal            string             `json:"goal"`
	Status          AgentSessionStatus `json:"status"`
	Steps           []AgentStep        `json:"steps"`
	Answer          string             `json:"answer,omitempty"`
	Error           string             `json:"error,omitempty"`
	TokensUsed      int                `json:"tokens_used"`
	StartedAt       time.Time          `json:"started_at"`
	EndedAt         *time.Time         `json:"ended_at,omitempty"`
	PendingApproval *ApprovalRequest   `json:"pending_approval,omitempty"`
	Transcript      []AgentMessage     `json:"transcript,omitempty"`
	Plan            []PlanStep         `json:"plan,omitempty"`
}
