package remediation

import (
	"context"
	"fmt"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// RunbookStep is one action in a runbook (target may be resolved dynamically).
type RunbookStep struct {
	Action string `json:"action"`
	Target string `json:"target,omitempty"` // empty => resolved at run time
	Reason string `json:"reason,omitempty"`
}

// Runbook is a named, reviewed remediation sequence.
type Runbook struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Steps       []RunbookStep `json:"steps"`
}

// Runbooks returns the static, reviewed set of runbooks.
func Runbooks() []Runbook {
	return []Runbook{
		{
			Name:        "restart_unhealthy_containers",
			Description: "Restart containers that are not running (stopped/exited).",
			Steps:       nil, // targets resolved dynamically from the provided unhealthy set
		},
		{
			Name: "update_outdated_containers",
			Description: "Recreate containers that have an image update available. " +
				"This runbook is DESCRIBE-ONLY — the planned steps use action \"update_container\" " +
				"which is not in SupportedActions(), so RunRunbook will never execute them. " +
				"The caller is responsible for manual pull/recreate operations.",
			Steps: nil, // describe-only; resolved dynamically; no destructive default
		},
	}
}

// supportedActionsSet returns a set of supported action strings for fast lookup.
func supportedActionsSet() map[string]bool {
	set := make(map[string]bool)
	for _, a := range SupportedActions() {
		set[a] = true
	}
	return set
}

// RunRunbook looks up the named runbook, builds concrete steps from targets, and
// optionally executes them via exec.
//
//   - If confirm is false, no actions are ever executed; the planned steps are returned.
//   - If confirm is true, each planned step whose Action is in SupportedActions() is executed
//     via exec. Steps with unsupported actions (e.g. "update_container") are recorded as
//     non-executed results with an explanatory error.
//
// For "restart_unhealthy_containers" each entry in targets becomes one
// {Action:"restart_container", Target:<id>} step.
// For "update_outdated_containers" each target becomes a DESCRIBE-ONLY
// {Action:"update_container", Target:<id>} step (never executed).
func RunRunbook(ctx context.Context, exec *Executor, name string, confirm bool, targets []string) ([]dto.ActionResult, []RunbookStep, error) {
	// Look up the runbook.
	var found *Runbook
	for _, rb := range Runbooks() {
		rb := rb // capture loop variable
		if rb.Name == name {
			found = &rb
			break
		}
	}
	if found == nil {
		return nil, nil, fmt.Errorf("unknown runbook: %q", name)
	}

	// Build concrete steps from targets.
	var steps []RunbookStep
	switch name {
	case "restart_unhealthy_containers":
		for _, id := range targets {
			steps = append(steps, RunbookStep{
				Action: "restart_container",
				Target: id,
				Reason: "container is not running",
			})
		}
	case "update_outdated_containers":
		// Describe-only: "update_container" is not in SupportedActions and will never execute.
		for _, id := range targets {
			steps = append(steps, RunbookStep{
				Action: "update_container",
				Target: id,
				Reason: "image update available — manual pull/recreate required",
			})
		}
	}

	// Dry-run path: return the plan without executing anything.
	if !confirm {
		return nil, steps, nil
	}

	// Execute path: run supported steps, record unsupported ones as skipped.
	supported := supportedActionsSet()
	results := make([]dto.ActionResult, 0, len(steps))
	for _, step := range steps {
		if !supported[step.Action] {
			results = append(results, dto.ActionResult{
				Action:    step.Action,
				Target:    step.Target,
				Succeeded: false,
				Error:     fmt.Sprintf("action %q is not executable (describe-only runbook step)", step.Action),
			})
			continue
		}

		ok, dur, err := exec.Execute(ctx, step.Action, step.Target)
		ar := dto.ActionResult{
			Action:     step.Action,
			Target:     step.Target,
			Succeeded:  ok,
			DurationMs: dur,
		}
		if err != nil {
			ar.Error = err.Error()
		}
		results = append(results, ar)
	}

	return results, steps, nil
}
