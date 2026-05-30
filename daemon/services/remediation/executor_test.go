package remediation

import (
	"context"
	"errors"
	"testing"
)

// fakeDockerActor records the most recent (method, target) call.
type fakeDockerActor struct {
	lastMethod string
	lastTarget string
	callCount  int
	returnErr  error
}

func (f *fakeDockerActor) Start(id string) error {
	f.lastMethod, f.lastTarget, f.callCount = "Start", id, f.callCount+1
	return f.returnErr
}

func (f *fakeDockerActor) Stop(id string) error {
	f.lastMethod, f.lastTarget, f.callCount = "Stop", id, f.callCount+1
	return f.returnErr
}

func (f *fakeDockerActor) Restart(id string) error {
	f.lastMethod, f.lastTarget, f.callCount = "Restart", id, f.callCount+1
	return f.returnErr
}

// fakeVMActor records the most recent (method, target) call.
type fakeVMActor struct {
	lastMethod string
	lastTarget string
	callCount  int
	returnErr  error
}

func (f *fakeVMActor) Start(name string) error {
	f.lastMethod, f.lastTarget, f.callCount = "Start", name, f.callCount+1
	return f.returnErr
}

func (f *fakeVMActor) Stop(name string) error {
	f.lastMethod, f.lastTarget, f.callCount = "Stop", name, f.callCount+1
	return f.returnErr
}

func (f *fakeVMActor) Restart(name string) error {
	f.lastMethod, f.lastTarget, f.callCount = "Restart", name, f.callCount+1
	return f.returnErr
}

func (f *fakeVMActor) ForceStop(name string) error {
	f.lastMethod, f.lastTarget, f.callCount = "ForceStop", name, f.callCount+1
	return f.returnErr
}

// validContainerID is a 12-char lowercase hex string accepted by ValidateContainerID.
const validContainerID = "a1b2c3d4e5f6"

// validVMName is a simple alphanumeric VM name accepted by ValidateVMName.
const validVMName = "my-vm"

func TestExecutor_DispatchAllActions(t *testing.T) {
	tests := []struct {
		name       string
		action     string
		target     string
		wantActor  string // "docker" or "vm"
		wantMethod string
	}{
		{
			name:       "restart_container routes to docker.Restart",
			action:     "restart_container",
			target:     validContainerID,
			wantActor:  "docker",
			wantMethod: "Restart",
		},
		{
			name:       "stop_container routes to docker.Stop",
			action:     "stop_container",
			target:     validContainerID,
			wantActor:  "docker",
			wantMethod: "Stop",
		},
		{
			name:       "start_container routes to docker.Start",
			action:     "start_container",
			target:     validContainerID,
			wantActor:  "docker",
			wantMethod: "Start",
		},
		{
			name:       "restart_vm routes to vm.Restart",
			action:     "restart_vm",
			target:     validVMName,
			wantActor:  "vm",
			wantMethod: "Restart",
		},
		{
			name:       "stop_vm routes to vm.Stop",
			action:     "stop_vm",
			target:     validVMName,
			wantActor:  "vm",
			wantMethod: "Stop",
		},
		{
			name:       "start_vm routes to vm.Start",
			action:     "start_vm",
			target:     validVMName,
			wantActor:  "vm",
			wantMethod: "Start",
		},
		{
			name:       "force_stop_vm routes to vm.ForceStop",
			action:     "force_stop_vm",
			target:     validVMName,
			wantActor:  "vm",
			wantMethod: "ForceStop",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			docker := &fakeDockerActor{}
			vm := &fakeVMActor{}
			exec := NewExecutor(docker, vm)

			ok, durationMs, err := exec.Execute(context.Background(), tc.action, tc.target)
			if err != nil {
				t.Fatalf("Execute(%q, %q) returned unexpected error: %v", tc.action, tc.target, err)
			}
			if !ok {
				t.Errorf("Execute(%q, %q) returned ok=false, want true", tc.action, tc.target)
			}
			if durationMs < 0 {
				t.Errorf("Execute(%q, %q) returned negative durationMs: %d", tc.action, tc.target, durationMs)
			}

			switch tc.wantActor {
			case "docker":
				if docker.callCount != 1 {
					t.Errorf("expected docker called once, got %d", docker.callCount)
				}
				if docker.lastMethod != tc.wantMethod {
					t.Errorf("docker method: got %q, want %q", docker.lastMethod, tc.wantMethod)
				}
				if docker.lastTarget != tc.target {
					t.Errorf("docker target: got %q, want %q", docker.lastTarget, tc.target)
				}
				if vm.callCount != 0 {
					t.Errorf("expected vm NOT called, got %d calls", vm.callCount)
				}
			case "vm":
				if vm.callCount != 1 {
					t.Errorf("expected vm called once, got %d", vm.callCount)
				}
				if vm.lastMethod != tc.wantMethod {
					t.Errorf("vm method: got %q, want %q", vm.lastMethod, tc.wantMethod)
				}
				if vm.lastTarget != tc.target {
					t.Errorf("vm target: got %q, want %q", vm.lastTarget, tc.target)
				}
				if docker.callCount != 0 {
					t.Errorf("expected docker NOT called, got %d calls", docker.callCount)
				}
			}
		})
	}
}

func TestExecutor_InvalidContainerID(t *testing.T) {
	docker := &fakeDockerActor{}
	vm := &fakeVMActor{}
	exec := NewExecutor(docker, vm)

	// "bad id!" contains a space and exclamation mark — fails ValidateContainerID.
	ok, _, err := exec.Execute(context.Background(), "restart_container", "bad id!")
	if err == nil {
		t.Fatal("expected error for invalid container ID, got nil")
	}
	if ok {
		t.Error("expected ok=false for invalid container ID")
	}
	if docker.callCount != 0 {
		t.Errorf("docker should NOT have been called, got %d calls", docker.callCount)
	}
}

func TestExecutor_InvalidVMName(t *testing.T) {
	docker := &fakeDockerActor{}
	vm := &fakeVMActor{}
	exec := NewExecutor(docker, vm)

	// Empty string fails ValidateVMName.
	ok, _, err := exec.Execute(context.Background(), "start_vm", "")
	if err == nil {
		t.Fatal("expected error for empty VM name, got nil")
	}
	if ok {
		t.Error("expected ok=false for empty VM name")
	}
	if vm.callCount != 0 {
		t.Errorf("vm should NOT have been called, got %d calls", vm.callCount)
	}
}

func TestExecutor_UnknownAction(t *testing.T) {
	docker := &fakeDockerActor{}
	vm := &fakeVMActor{}
	exec := NewExecutor(docker, vm)

	ok, _, err := exec.Execute(context.Background(), "frobnicate", validContainerID)
	if err == nil {
		t.Fatal("expected error for unknown action, got nil")
	}
	if ok {
		t.Error("expected ok=false for unknown action")
	}
	if docker.callCount != 0 {
		t.Errorf("docker should NOT have been called, got %d calls", docker.callCount)
	}
	if vm.callCount != 0 {
		t.Errorf("vm should NOT have been called, got %d calls", vm.callCount)
	}
}

func TestExecutor_ControllerError(t *testing.T) {
	wantErr := errors.New("connection refused")
	docker := &fakeDockerActor{returnErr: wantErr}
	vm := &fakeVMActor{}
	exec := NewExecutor(docker, vm)

	ok, _, err := exec.Execute(context.Background(), "restart_container", validContainerID)
	if err == nil {
		t.Fatal("expected error propagated from controller, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error: got %v, want to wrap %v", err, wantErr)
	}
	if ok {
		t.Error("expected ok=false when controller returns error")
	}
}

func TestSupportedActions(t *testing.T) {
	actions := SupportedActions()
	want := map[string]bool{
		"restart_container": true,
		"stop_container":    true,
		"start_container":   true,
		"restart_vm":        true,
		"stop_vm":           true,
		"start_vm":          true,
		"force_stop_vm":     true,
	}
	if len(actions) != len(want) {
		t.Errorf("SupportedActions() returned %d actions, want %d", len(actions), len(want))
	}
	for _, a := range actions {
		if !want[a] {
			t.Errorf("unexpected action in SupportedActions(): %q", a)
		}
	}
}
