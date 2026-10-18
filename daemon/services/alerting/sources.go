package alerting

import (
	"fmt"
	"maps"
	"reflect"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// envSources maps each AlertEnv expression name to the collectors named in
// its source tag. Fields tagged source:"-" are absent.
var envSources = buildEnvSources()

// buildEnvSources maps each AlertEnv expression name to the collectors in its source tag.
func buildEnvSources() map[string][]string {
	t := reflect.TypeFor[dto.AlertEnv]()
	sources := make(map[string][]string, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		name := f.Tag.Get("expr")
		if name == "" {
			name = f.Name
		}
		if src := f.Tag.Get("source"); src != "" && src != "-" {
			sources[name] = strings.Split(src, ",")
		}
	}
	return sources
}

// allAlertSources returns every collector named in an AlertEnv source tag.
func allAlertSources() map[string]bool {
	all := make(map[string]bool)
	for _, collectors := range envSources {
		for _, c := range collectors {
			all[c] = true
		}
	}
	return all
}

// sourceVisitor adds the collectors behind each AlertEnv identifier it visits.
// Unknown identifiers (functions, let variables) are ignored.
type sourceVisitor map[string]bool

// Visit records each identifier the expression references.
func (v sourceVisitor) Visit(node *ast.Node) {
	if id, ok := (*node).(*ast.IdentifierNode); ok {
		for _, c := range envSources[id.Value] {
			v[c] = true
		}
	}
}

// addExpressionCollectors adds the collectors that expression reads to into.
// Member access such as "A.b" resolves through its root identifier "A".
func addExpressionCollectors(expression string, into map[string]bool) error {
	tree, err := parser.Parse(expression)
	if err != nil {
		return fmt.Errorf("parsing expression: %w", err)
	}
	ast.Walk(&tree.Node, sourceVisitor(into))
	return nil
}

// requiredCollectors returns the collectors that the enabled rules read. If an
// enabled rule cannot be parsed, every alert source is required.
func requiredCollectors(rules []dto.AlertRule) map[string]bool {
	required := make(map[string]bool)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if err := addExpressionCollectors(rule.Expression, required); err != nil {
			return allAlertSources()
		}
	}
	return required
}

// cloneCollectors returns a copy of m that is never nil.
func cloneCollectors(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	maps.Copy(out, m)
	return out
}
