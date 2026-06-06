package alerting

import (
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestBuiltinSubsystemDegradedRule(t *testing.T) {
	var rule dto.AlertRule
	for _, r := range BuiltinRules() {
		if r.ID == "subsystem-degraded" {
			rule = r
		}
	}
	if rule.ID == "" {
		t.Fatal("BuiltinRules() missing subsystem-degraded rule")
	}
	if !rule.Enabled {
		t.Error("subsystem-degraded rule must be enabled by default")
	}

	eval := NewEvaluator()
	eval.CompileRule(rule)

	// Transitions to firing when a subsystem is degraded.
	res := eval.Evaluate(dto.AlertEnv{DegradedSubsystemCount: 1}, []dto.AlertRule{rule})
	firing := false
	for _, r := range res {
		if r.Rule.ID == rule.ID && r.Transitioned && r.NewState == "firing" {
			firing = true
		}
	}
	if !firing {
		t.Errorf("expected transition to firing with DegradedSubsystemCount=1, got %+v", res)
	}

	// Transitions to resolved when all healthy again.
	res = eval.Evaluate(dto.AlertEnv{DegradedSubsystemCount: 0}, []dto.AlertRule{rule})
	resolved := false
	for _, r := range res {
		if r.Rule.ID == rule.ID && r.Transitioned && r.NewState == "ok" {
			resolved = true
		}
	}
	if !resolved {
		t.Errorf("expected transition to ok with DegradedSubsystemCount=0, got %+v", res)
	}
}
