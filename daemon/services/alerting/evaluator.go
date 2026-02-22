package alerting

import (
	"fmt"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// ruleState tracks the internal evaluation state for a single rule.
type ruleState struct {
	pendingSince time.Time // When the rule first started evaluating to true
	firingAt     time.Time // When the rule transitioned to firing
	evalCount    int64
	state        string // "ok", "pending", "firing"
}

// Evaluator compiles and evaluates alert rule expressions against cached system data.
type Evaluator struct {
	mu       sync.RWMutex
	programs map[string]*vm.Program // Compiled expressions keyed by rule ID
	states   map[string]*ruleState
}

// NewEvaluator creates a new rule evaluator.
func NewEvaluator() *Evaluator {
	return &Evaluator{
		programs: make(map[string]*vm.Program),
		states:   make(map[string]*ruleState),
	}
}

// CompileRule compiles an expression string and validates it against AlertEnv.
// Returns an error if the expression is invalid.
func (e *Evaluator) CompileRule(rule dto.AlertRule) error {
	program, err := expr.Compile(rule.Expression, expr.Env(dto.AlertEnv{}), expr.AsBool())
	if err != nil {
		return fmt.Errorf("invalid expression for rule %s: %w", rule.ID, err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.programs[rule.ID] = program
	if _, ok := e.states[rule.ID]; !ok {
		e.states[rule.ID] = &ruleState{state: "ok"}
	}

	return nil
}

// CompileRules compiles all rules, returning errors for any that fail.
func (e *Evaluator) CompileRules(rules []dto.AlertRule) []error {
	var errs []error
	for _, rule := range rules {
		if err := e.CompileRule(rule); err != nil {
			errs = append(errs, err)
			logger.Warning("Alerting: Failed to compile rule %s: %v", rule.ID, err)
		}
	}
	return errs
}

// RemoveRule removes a compiled rule and its state.
func (e *Evaluator) RemoveRule(ruleID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.programs, ruleID)
	delete(e.states, ruleID)
}

// EvaluateResult holds the result of evaluating a single rule.
type EvaluateResult struct {
	Rule         dto.AlertRule
	Transitioned bool   // True if state changed (ok→firing or firing→resolved)
	NewState     string // "ok", "pending", "firing"
	PrevState    string
}

// Evaluate runs all compiled rule expressions against the given environment.
// Returns results only for rules that had a state transition.
func (e *Evaluator) Evaluate(env dto.AlertEnv, rules []dto.AlertRule) []EvaluateResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	var transitions []EvaluateResult

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		program, ok := e.programs[rule.ID]
		if !ok {
			continue
		}

		state, ok := e.states[rule.ID]
		if !ok {
			state = &ruleState{state: "ok"}
			e.states[rule.ID] = state
		}
		state.evalCount++

		// Evaluate the expression
		output, err := expr.Run(program, env)
		if err != nil {
			logger.Warning("Alerting: Error evaluating rule %s: %v", rule.ID, err)
			continue
		}

		triggered, ok := output.(bool)
		if !ok {
			logger.Warning("Alerting: Rule %s expression did not return bool", rule.ID)
			continue
		}

		prevState := state.state

		if triggered {
			switch state.state {
			case "ok":
				if rule.DurationSeconds > 0 {
					// Transition to pending
					state.state = "pending"
					state.pendingSince = now
				} else {
					// Immediate firing
					state.state = "firing"
					state.firingAt = now
					transitions = append(transitions, EvaluateResult{
						Rule:         rule,
						Transitioned: true,
						NewState:     "firing",
						PrevState:    prevState,
					})
				}

			case "pending":
				// Check if duration has elapsed
				elapsed := now.Sub(state.pendingSince)
				if elapsed >= time.Duration(rule.DurationSeconds)*time.Second {
					state.state = "firing"
					state.firingAt = now
					transitions = append(transitions, EvaluateResult{
						Rule:         rule,
						Transitioned: true,
						NewState:     "firing",
						PrevState:    prevState,
					})
				}
				// else: stays pending

			case "firing":
				// Already firing, check cooldown for re-notification
				// (actual re-notification is handled by the engine)
			}
		} else {
			// Expression is false
			switch state.state {
			case "pending":
				// Reset to ok — condition wasn't sustained
				state.state = "ok"
				state.pendingSince = time.Time{}

			case "firing":
				// Transition to resolved
				state.state = "ok"
				transitions = append(transitions, EvaluateResult{
					Rule:         rule,
					Transitioned: true,
					NewState:     "ok",
					PrevState:    prevState,
				})
				state.firingAt = time.Time{}
			}
			// "ok" stays "ok"
		}
	}

	return transitions
}

// GetStatuses returns the current status of all tracked rules.
func (e *Evaluator) GetStatuses(rules []dto.AlertRule) []dto.AlertStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var statuses []dto.AlertStatus
	for _, rule := range rules {
		state, ok := e.states[rule.ID]
		if !ok {
			statuses = append(statuses, dto.AlertStatus{
				RuleID:   rule.ID,
				RuleName: rule.Name,
				State:    "ok",
				Severity: rule.Severity,
			})
			continue
		}

		status := dto.AlertStatus{
			RuleID:    rule.ID,
			RuleName:  rule.Name,
			State:     state.state,
			Severity:  rule.Severity,
			EvalCount: state.evalCount,
		}

		switch state.state {
		case "pending":
			status.Since = state.pendingSince
		case "firing":
			status.Since = state.firingAt
		}

		statuses = append(statuses, status)
	}

	return statuses
}

// GetFiringAlerts returns statuses only for rules currently in "firing" state.
func (e *Evaluator) GetFiringAlerts(rules []dto.AlertRule) []dto.AlertStatus {
	all := e.GetStatuses(rules)
	var firing []dto.AlertStatus
	for _, s := range all {
		if s.State == "firing" {
			firing = append(firing, s)
		}
	}
	return firing
}
