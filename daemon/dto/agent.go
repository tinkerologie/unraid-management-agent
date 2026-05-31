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
	SessionRunning   AgentSessionStatus = "running"
	SessionCompleted AgentSessionStatus = "completed"
	SessionFailed    AgentSessionStatus = "failed"
	SessionCancelled AgentSessionStatus = "cancelled"
)

// AgentConfig holds the agent's runtime configuration (persisted as JSON).
type AgentConfig struct {
	Enabled             bool                      `json:"enabled"`
	Provider            string                    `json:"provider"` // "anthropic" | "mock"
	Model               string                    `json:"model"`
	Endpoint            string                    `json:"endpoint,omitempty"`
	APIKey              string                    `json:"-"` // never serialized; from env/secret file
	Autonomy            map[RiskTier]AutonomyMode `json:"autonomy"`
	MaxIterations       int                       `json:"max_iterations"`
	MaxTokensPerSession int                       `json:"max_tokens_per_session"`
	SessionDeadlineSecs int                       `json:"session_deadline_secs"`
}

// DefaultAgentConfig returns safe defaults: disabled, conservative caps, tiered autonomy.
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		Enabled:             false,
		Provider:            "anthropic",
		Model:               "claude-opus-4-8",
		Autonomy:            map[RiskTier]AutonomyMode{RiskReadOnly: ModeAuto, RiskLow: ModeAuto, RiskHigh: ModeApprove},
		MaxIterations:       12,
		MaxTokensPerSession: 60000,
		SessionDeadlineSecs: 180,
	}
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

// AgentSession is a full agent run (on-demand in Phase 1).
type AgentSession struct {
	ID         string             `json:"id"`
	Goal       string             `json:"goal"`
	Status     AgentSessionStatus `json:"status"`
	Steps      []AgentStep        `json:"steps"`
	Answer     string             `json:"answer,omitempty"`
	Error      string             `json:"error,omitempty"`
	TokensUsed int                `json:"tokens_used"`
	StartedAt  time.Time          `json:"started_at"`
	EndedAt    *time.Time         `json:"ended_at,omitempty"`
}
