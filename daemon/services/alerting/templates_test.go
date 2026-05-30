package alerting

import (
	"testing"

	"github.com/expr-lang/expr"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestAlertRuleTemplates(t *testing.T) {
	tmpls := AlertRuleTemplates()
	if len(tmpls) == 0 {
		t.Fatal("expected non-empty templates")
	}
	for _, tmpl := range tmpls {
		if tmpl.Enabled {
			t.Errorf("template %q should be disabled by default", tmpl.ID)
		}
		if tmpl.ID == "" || tmpl.Name == "" || tmpl.Expression == "" {
			t.Errorf("template missing id/name/expression: %+v", tmpl)
		}
		if _, err := expr.Compile(tmpl.Expression, expr.Env(dto.AlertEnv{}), expr.AsBool()); err != nil {
			t.Errorf("template %q expression does not compile: %v", tmpl.ID, err)
		}
	}
}

func TestRuleFromTemplate(t *testing.T) {
	t.Run("known id returns ok=true and enabled rule", func(t *testing.T) {
		rule, ok := RuleFromTemplate("tmpl-array-fill", nil)
		if !ok {
			t.Fatal("expected ok=true for known template id")
		}
		if !rule.Enabled {
			t.Error("expected Enabled=true")
		}
		if rule.ID != "tmpl-array-fill" {
			t.Errorf("expected ID=tmpl-array-fill, got %q", rule.ID)
		}
	})

	t.Run("default channel is unraid when none given", func(t *testing.T) {
		rule, ok := RuleFromTemplate("tmpl-array-fill", nil)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if len(rule.Channels) != 1 || rule.Channels[0] != "unraid" {
			t.Errorf("expected channels=[unraid], got %v", rule.Channels)
		}
	})

	t.Run("custom channels are respected", func(t *testing.T) {
		channels := []string{"slack://token", "discord://webhook"}
		rule, ok := RuleFromTemplate("tmpl-disk-temp-climb", channels)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if len(rule.Channels) != 2 || rule.Channels[0] != channels[0] || rule.Channels[1] != channels[1] {
			t.Errorf("expected custom channels %v, got %v", channels, rule.Channels)
		}
	})

	t.Run("unknown id returns ok=false", func(t *testing.T) {
		_, ok := RuleFromTemplate("tmpl-does-not-exist", nil)
		if ok {
			t.Error("expected ok=false for unknown template id")
		}
	})

	t.Run("all known templates can be built", func(t *testing.T) {
		for _, tmpl := range AlertRuleTemplates() {
			rule, ok := RuleFromTemplate(tmpl.ID, nil)
			if !ok {
				t.Errorf("RuleFromTemplate(%q) returned ok=false unexpectedly", tmpl.ID)
			}
			if !rule.Enabled {
				t.Errorf("rule for template %q should be Enabled", tmpl.ID)
			}
			if rule.ID != tmpl.ID {
				t.Errorf("rule ID mismatch: want %q got %q", tmpl.ID, rule.ID)
			}
		}
	})
}
