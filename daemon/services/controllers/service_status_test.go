package controllers

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeStatus reports a service as running when its rc script name contains
// "docker" or "nginx", so the result order can be checked.
func fakeStatus(command string) (string, error) {
	if strings.Contains(command, "docker") || strings.Contains(command, "nginx") {
		return "running", nil
	}
	return "not running", errors.New("exit status 1")
}

// TestListServiceStatuses_Order checks results come back in ValidServiceNames order with the right values.
func TestListServiceStatuses_Order(t *testing.T) {
	sc := &ServiceController{statusOutput: func(command string, _ ...string) (string, error) {
		return fakeStatus(command)
	}}
	statuses := sc.ListServiceStatuses()
	names := ValidServiceNames()
	if len(statuses) != len(names) {
		t.Fatalf("got %d statuses, want %d", len(statuses), len(names))
	}
	for i, status := range statuses {
		if status.Name != names[i] {
			t.Errorf("statuses[%d].Name = %q, want %q (ValidServiceNames order)", i, status.Name, names[i])
		}
		want := status.Name == "docker" || status.Name == "nginx"
		if status.Running != want {
			t.Errorf("%s running = %v, want %v", status.Name, status.Running, want)
		}
	}
}

// TestListServiceStatuses_RunsChecksConcurrently checks the status checks run concurrently, bounded by statusWorkers.
func TestListServiceStatuses_RunsChecksConcurrently(t *testing.T) {
	// Every check blocks until the test releases it. Reaching statusWorkers
	// checks in flight at once is only possible if they run concurrently;
	// a sequential implementation would stall at one and hit the timeout
	// below, which only fires on failure (no sleeps, no timing assertions).
	var inFlight, maxInFlight atomic.Int32
	var reachedOnce sync.Once
	reached := make(chan struct{})
	release := make(chan struct{})

	sc := &ServiceController{statusOutput: func(command string, _ ...string) (string, error) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			m := maxInFlight.Load()
			if n <= m || maxInFlight.CompareAndSwap(m, n) {
				break
			}
		}
		if n == statusWorkers {
			reachedOnce.Do(func() { close(reached) })
		}
		<-release
		return fakeStatus(command)
	}}

	done := make(chan []ServiceStatus, 1)
	go func() { done <- sc.ListServiceStatuses() }()

	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatalf("only %d of %d status checks ran at the same time", maxInFlight.Load(), statusWorkers)
	}
	close(release)
	statuses := <-done

	if got := maxInFlight.Load(); got != statusWorkers {
		t.Errorf("max concurrent checks = %d, want exactly %d (bounded worker count)", got, statusWorkers)
	}
	if len(statuses) != len(ValidServiceNames()) {
		t.Errorf("got %d statuses, want %d", len(statuses), len(ValidServiceNames()))
	}
}
