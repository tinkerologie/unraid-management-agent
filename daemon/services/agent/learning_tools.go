package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/remediation"
)

// RegisterLearningTools adds suggest-not-mutate learning tools to the registry.
func (s *Service) RegisterLearningTools(reg *tools.Registry) {
	reg.Register(tools.Tool{
		Name:        "propose_preference",
		Description: "Propose an operator preference for review (e.g. always auto-approve restarting a specific container). Stored PENDING — never takes effect until the operator confirms it.",
		RiskTier:    dto.RiskReadOnly,
		Schema:      []byte(`{"type":"object","properties":{"kind":{"type":"string"},"subject":{"type":"string"},"note":{"type":"string"}},"required":["kind","subject"]}`),
		Invoke: func(_ context.Context, argsJSON string) (string, error) {
			var a struct {
				Kind    string `json:"kind"`
				Subject string `json:"subject"`
				Note    string `json:"note"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
				return "", fmt.Errorf("parse args: %w", err)
			}
			if s.memory == nil {
				return "Memory disabled; cannot store preference.", nil
			}
			id := fmt.Sprintf("pref-%d", s.nextPrefSeq())
			s.memory.AddPreference(dto.AgentPreference{ID: id, Kind: a.Kind, Subject: a.Subject, Note: a.Note, Status: dto.PreferencePending})
			if err := s.memory.Save(); err != nil {
				return "", fmt.Errorf("save preference: %w", err)
			}
			return fmt.Sprintf("Proposed preference %s (%s: %s) — PENDING operator confirmation; it will not take effect until confirmed.", id, a.Kind, a.Subject), nil
		},
	})
	reg.Register(tools.Tool{
		Name:        "propose_runbook",
		Description: "Propose a named remediation runbook for operator review. Does not execute anything.",
		RiskTier:    dto.RiskReadOnly,
		Schema:      []byte(`{"type":"object","properties":{"name":{"type":"string"},"description":{"type":"string"}},"required":["name","description"]}`),
		Invoke: func(_ context.Context, argsJSON string) (string, error) {
			var a struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
				return "", fmt.Errorf("parse args: %w", err)
			}
			if s.runbooks == nil {
				return "Runbook store unavailable.", nil
			}
			s.runbooks.Add(remediation.Runbook{Name: a.Name, Description: a.Description})
			if err := s.runbooks.Save(); err != nil {
				return "", fmt.Errorf("save runbook: %w", err)
			}
			return fmt.Sprintf("Proposed runbook %q for operator review.", a.Name), nil
		},
	})
}
