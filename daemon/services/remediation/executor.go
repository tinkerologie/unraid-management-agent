package remediation

import (
	"context"
	"fmt"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
)

// dockerActor is the subset of DockerController methods remediation needs.
type dockerActor interface {
	Start(id string) error
	Stop(id string) error
	Restart(id string) error
}

// vmActor is the subset of VMController methods remediation needs.
type vmActor interface {
	Start(name string) error
	Stop(name string) error
	Restart(name string) error
	ForceStop(name string) error
}

// Executor maps action strings to controller operations with input validation.
type Executor struct {
	docker dockerActor
	vm     vmActor
}

// NewExecutor creates an Executor backed by the provided docker and vm actors.
func NewExecutor(docker dockerActor, vm vmActor) *Executor {
	return &Executor{docker: docker, vm: vm}
}

// Execute validates the target and dispatches the action. It returns whether
// the action succeeded, how long it took in milliseconds, and any error.
// Unknown actions and invalid targets return an error WITHOUT calling a controller.
func (e *Executor) Execute(_ context.Context, action, target string) (ok bool, durationMs int64, err error) {
	// Validate target before dispatching.
	switch action {
	case "restart_container", "stop_container", "start_container":
		if verr := lib.ValidateContainerID(target); verr != nil {
			return false, 0, fmt.Errorf("invalid container target %q: %w", target, verr)
		}
	case "restart_vm", "stop_vm", "start_vm", "force_stop_vm":
		if verr := lib.ValidateVMName(target); verr != nil {
			return false, 0, fmt.Errorf("invalid vm target %q: %w", target, verr)
		}
	default:
		return false, 0, fmt.Errorf("unknown remediation action: %q", action)
	}

	start := time.Now()

	switch action {
	case "restart_container":
		err = e.docker.Restart(target)
	case "stop_container":
		err = e.docker.Stop(target)
	case "start_container":
		err = e.docker.Start(target)
	case "restart_vm":
		err = e.vm.Restart(target)
	case "stop_vm":
		err = e.vm.Stop(target)
	case "start_vm":
		err = e.vm.Start(target)
	case "force_stop_vm":
		err = e.vm.ForceStop(target)
	}

	durationMs = time.Since(start).Milliseconds()
	return err == nil, durationMs, err
}

// SupportedActions returns the action strings the executor understands.
func SupportedActions() []string {
	return []string{
		"restart_container",
		"stop_container",
		"start_container",
		"restart_vm",
		"stop_vm",
		"start_vm",
		"force_stop_vm",
	}
}
