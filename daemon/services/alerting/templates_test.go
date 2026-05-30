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
