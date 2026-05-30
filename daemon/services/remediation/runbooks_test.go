package remediation

import (
	"context"
	"testing"
)

func TestRunbooks(t *testing.T) {
	rbs := Runbooks()
	names := map[string]bool{}
	for _, r := range rbs {
		names[r.Name] = true
	}
	if !names["restart_unhealthy_containers"] || !names["update_outdated_containers"] {
		t.Fatalf("missing expected runbooks: %v", names)
	}
}

func TestRunRunbook_DryRunExecutesNothing(t *testing.T) {
	exec := NewExecutor(&fakeDockerActor{}, &fakeVMActor{})
	results, steps, err := RunRunbook(context.Background(), exec, "restart_unhealthy_containers", false, []string{"abc123def456"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("dry run executed %d actions, want 0", len(results))
	}
	if len(steps) != 1 {
		t.Errorf("want 1 planned step, got %d", len(steps))
	}
}

func TestRunRunbook_ConfirmExecutes(t *testing.T) {
	fd := &fakeDockerActor{}
	exec := NewExecutor(fd, &fakeVMActor{})
	results, _, err := RunRunbook(context.Background(), exec, "restart_unhealthy_containers", true, []string{"abc123def456"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Succeeded {
		t.Errorf("want 1 successful result, got %+v", results)
	}
}

func TestRunRunbook_Unknown(t *testing.T) {
	exec := NewExecutor(&fakeDockerActor{}, &fakeVMActor{})
	if _, _, err := RunRunbook(context.Background(), exec, "nope", true, nil); err == nil {
		t.Error("expected error for unknown runbook")
	}
}

func TestRunRunbook_UpdateOutdatedContainers_DescribeOnly(t *testing.T) {
	fd := &fakeDockerActor{}
	exec := NewExecutor(fd, &fakeVMActor{})

	// Even with confirm=true the update_outdated_containers runbook must never call the docker actor.
	results, steps, err := RunRunbook(context.Background(), exec, "update_outdated_containers", true, []string{"abc123def456"})
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 {
		t.Errorf("want 1 planned step, got %d", len(steps))
	}
	if steps[0].Action != "update_container" {
		t.Errorf("want action 'update_container', got %q", steps[0].Action)
	}
	if len(results) != 1 {
		t.Errorf("want 1 result (skipped), got %d", len(results))
	}
	if results[0].Succeeded {
		t.Error("update_container step should not have Succeeded=true")
	}
	if results[0].Error == "" {
		t.Error("expected non-empty error for unsupported action")
	}
	if fd.callCount != 0 {
		t.Errorf("docker actor should not have been called, got %d calls", fd.callCount)
	}
}

func TestRunRunbook_EmptyTargets(t *testing.T) {
	exec := NewExecutor(&fakeDockerActor{}, &fakeVMActor{})
	results, steps, err := RunRunbook(context.Background(), exec, "restart_unhealthy_containers", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("want 0 results for empty targets, got %d", len(results))
	}
	if len(steps) != 0 {
		t.Errorf("want 0 steps for empty targets, got %d", len(steps))
	}
}
