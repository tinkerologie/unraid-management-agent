package alerting

import (
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestEvaluatorCompileRule(t *testing.T) {
	eval := NewEvaluator()

	t.Run("valid expression", func(t *testing.T) {
		rule := dto.AlertRule{ID: "r1", Expression: "CPU > 90"}
		if err := eval.CompileRule(rule); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("invalid expression", func(t *testing.T) {
		rule := dto.AlertRule{ID: "r2", Expression: "??? invalid !!!"}
		if err := eval.CompileRule(rule); err == nil {
			t.Error("expected error for invalid expression")
		}
	})

	t.Run("non-boolean expression", func(t *testing.T) {
		rule := dto.AlertRule{ID: "r3", Expression: "CPU + 1"}
		if err := eval.CompileRule(rule); err == nil {
			t.Error("expected error for non-boolean expression")
		}
	})
}

func TestEvaluatorCompileRules(t *testing.T) {
	eval := NewEvaluator()

	rules := []dto.AlertRule{
		{ID: "ok1", Expression: "CPU > 50"},
		{ID: "bad1", Expression: "??? bad"},
		{ID: "ok2", Expression: "RAMUsedPct > 80"},
	}

	errs := eval.CompileRules(rules)
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}

func TestEvaluatorRemoveRule(t *testing.T) {
	eval := NewEvaluator()
	rule := dto.AlertRule{ID: "removeme", Expression: "CPU > 50"}
	eval.CompileRule(rule)

	eval.RemoveRule("removeme")

	// Evaluate should skip removed rules
	results := eval.Evaluate(dto.AlertEnv{CPU: 99}, []dto.AlertRule{rule})
	for _, r := range results {
		if r.Rule.ID == "removeme" && r.Transitioned {
			t.Error("rule should not fire after removal")
		}
	}
}

func TestEvaluatorStateTransitions(t *testing.T) {
	eval := NewEvaluator()

	rule := dto.AlertRule{
		ID:              "cpu-high",
		Name:            "High CPU",
		Expression:      "CPU > 90",
		DurationSeconds: 0, // Immediate
		Severity:        "critical",
		Enabled:         true,
	}
	eval.CompileRule(rule)

	// First eval with high CPU — should transition to firing
	results := eval.Evaluate(dto.AlertEnv{CPU: 95}, []dto.AlertRule{rule})
	firingFound := false
	for _, r := range results {
		if r.Rule.ID == "cpu-high" && r.Transitioned && r.NewState == "firing" {
			firingFound = true
		}
	}
	if !firingFound {
		t.Error("expected rule to transition to firing")
	}

	// Second eval with high CPU — should NOT transition again (already firing)
	results = eval.Evaluate(dto.AlertEnv{CPU: 95}, []dto.AlertRule{rule})
	for _, r := range results {
		if r.Rule.ID == "cpu-high" && r.Transitioned {
			t.Error("rule should not re-transition while still firing")
		}
	}

	// Eval with low CPU — should resolve
	results = eval.Evaluate(dto.AlertEnv{CPU: 50}, []dto.AlertRule{rule})
	resolvedFound := false
	for _, r := range results {
		if r.Rule.ID == "cpu-high" && r.Transitioned && r.NewState == "ok" && r.PrevState == "firing" {
			resolvedFound = true
		}
	}
	if !resolvedFound {
		t.Error("expected rule to resolve")
	}
}

func TestEvaluatorDuration(t *testing.T) {
	eval := NewEvaluator()

	rule := dto.AlertRule{
		ID:              "cpu-sustained",
		Name:            "Sustained CPU",
		Expression:      "CPU > 90",
		DurationSeconds: 60, // Must be true for 60s
		Severity:        "warning",
		Enabled:         true,
	}
	eval.CompileRule(rule)

	// First eval — should go to pending, not firing
	results := eval.Evaluate(dto.AlertEnv{CPU: 95}, []dto.AlertRule{rule})
	for _, r := range results {
		if r.Rule.ID == "cpu-sustained" && r.Transitioned && r.NewState == "firing" {
			t.Error("rule should not fire immediately with duration > 0")
		}
	}

	// Verify it's in pending state
	statuses := eval.GetStatuses([]dto.AlertRule{rule})
	for _, s := range statuses {
		if s.RuleID == "cpu-sustained" && s.State != "pending" {
			t.Errorf("expected pending state, got %s", s.State)
		}
	}
}

func TestEvaluatorGetStatuses(t *testing.T) {
	eval := NewEvaluator()

	rules := []dto.AlertRule{
		{ID: "r1", Name: "Rule 1", Expression: "CPU > 90", Severity: "warning", Enabled: true},
		{ID: "r2", Name: "Rule 2", Expression: "RAMUsedPct > 80", Severity: "info", Enabled: true},
	}
	eval.CompileRules(rules)

	// Fire rule 1
	eval.Evaluate(dto.AlertEnv{CPU: 95, RAMUsedPct: 50}, rules)

	statuses := eval.GetStatuses(rules)
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}

	for _, s := range statuses {
		if s.RuleID == "r1" && s.State != "firing" {
			t.Errorf("expected r1 to be firing, got %s", s.State)
		}
		if s.RuleID == "r2" && s.State != "ok" {
			t.Errorf("expected r2 to be ok, got %s", s.State)
		}
	}
}

func TestEvaluatorGetFiringAlerts(t *testing.T) {
	eval := NewEvaluator()

	rules := []dto.AlertRule{
		{ID: "r1", Name: "Firing", Expression: "CPU > 90", Severity: "critical", Enabled: true},
		{ID: "r2", Name: "Not Firing", Expression: "CPU > 99", Severity: "info", Enabled: true},
	}
	eval.CompileRules(rules)

	eval.Evaluate(dto.AlertEnv{CPU: 95}, rules)

	firing := eval.GetFiringAlerts(rules)
	if len(firing) != 1 {
		t.Fatalf("expected 1 firing alert, got %d", len(firing))
	}
	if firing[0].RuleID != "r1" {
		t.Errorf("expected r1 to be firing, got %s", firing[0].RuleID)
	}
}

func TestEvaluatorStringComparison(t *testing.T) {
	eval := NewEvaluator()

	rule := dto.AlertRule{
		ID:         "array-stopped",
		Name:       "Array Stopped",
		Expression: `ArrayState != "Started"`,
		Severity:   "critical",
		Enabled:    true,
	}
	eval.CompileRule(rule)

	// Array stopped
	results := eval.Evaluate(dto.AlertEnv{ArrayState: "Stopped"}, []dto.AlertRule{rule})
	found := false
	for _, r := range results {
		if r.Rule.ID == "array-stopped" && r.Transitioned && r.NewState == "firing" {
			found = true
		}
	}
	if !found {
		t.Error("expected rule to fire for stopped array")
	}

	// Array started — should resolve
	results = eval.Evaluate(dto.AlertEnv{ArrayState: "Started"}, []dto.AlertRule{rule})
	resolved := false
	for _, r := range results {
		if r.Rule.ID == "array-stopped" && r.Transitioned && r.NewState == "ok" {
			resolved = true
		}
	}
	if !resolved {
		t.Error("expected rule to resolve when array started")
	}
}
