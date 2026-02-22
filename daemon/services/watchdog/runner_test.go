package watchdog

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestRunnerGetStatuses_Empty(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	runner := NewRunner(store)

	statuses := runner.GetStatuses()
	if len(statuses) != 0 {
		t.Errorf("expected 0 statuses, got %d", len(statuses))
	}
}

func TestRunnerGetHistory_Empty(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	runner := NewRunner(store)

	history := runner.GetHistory()
	if len(history) != 0 {
		t.Errorf("expected 0 history events, got %d", len(history))
	}
}

func TestRunnerRunSingleCheck_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	runner := NewRunner(store)

	_, err := runner.RunSingleCheck(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent check")
	}
}

func TestRunnerRunSingleCheck_HTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	store := NewStore(dir)
	store.CreateCheck(dto.HealthCheck{
		ID:             "http-test",
		Name:           "HTTP Test",
		Type:           dto.HealthCheckHTTP,
		Target:         srv.URL,
		TimeoutSeconds: 5,
		SuccessCode:    200,
		Enabled:        true,
	})

	runner := NewRunner(store)
	status, err := runner.RunSingleCheck(context.Background(), "http-test")
	if err != nil {
		t.Fatalf("RunSingleCheck failed: %v", err)
	}
	if !status.Healthy {
		t.Errorf("expected healthy, got unhealthy: %s", status.LastError)
	}
	if status.CheckID != "http-test" {
		t.Errorf("expected check ID 'http-test', got '%s'", status.CheckID)
	}
}

func TestRunnerRunSingleCheck_TCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	dir := t.TempDir()
	store := NewStore(dir)
	store.CreateCheck(dto.HealthCheck{
		ID:             "tcp-test",
		Name:           "TCP Test",
		Type:           dto.HealthCheckTCP,
		Target:         ln.Addr().String(),
		TimeoutSeconds: 5,
		Enabled:        true,
	})

	runner := NewRunner(store)
	status, err := runner.RunSingleCheck(context.Background(), "tcp-test")
	if err != nil {
		t.Fatalf("RunSingleCheck failed: %v", err)
	}
	if !status.Healthy {
		t.Errorf("expected healthy, got unhealthy: %s", status.LastError)
	}
}

func TestRunnerRunSingleCheck_FailingHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	dir := t.TempDir()
	store := NewStore(dir)
	store.CreateCheck(dto.HealthCheck{
		ID:             "http-fail",
		Name:           "HTTP Fail",
		Type:           dto.HealthCheckHTTP,
		Target:         srv.URL,
		TimeoutSeconds: 5,
		SuccessCode:    200,
		Enabled:        true,
	})

	runner := NewRunner(store)
	status, err := runner.RunSingleCheck(context.Background(), "http-fail")
	if err != nil {
		t.Fatalf("RunSingleCheck failed: %v", err)
	}
	if status.Healthy {
		t.Error("expected unhealthy status for 503 response")
	}
	if status.ConsecutiveFails != 1 {
		t.Errorf("expected 1 consecutive fail, got %d", status.ConsecutiveFails)
	}
}

func TestRunnerCleanupCheck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	dir := t.TempDir()
	store := NewStore(dir)
	store.CreateCheck(dto.HealthCheck{
		ID:             "cleanup-test",
		Name:           "Cleanup",
		Type:           dto.HealthCheckTCP,
		Target:         ln.Addr().String(),
		TimeoutSeconds: 2,
		Enabled:        true,
	})

	runner := NewRunner(store)

	_, err = runner.RunSingleCheck(context.Background(), "cleanup-test")
	if err != nil {
		t.Fatalf("RunSingleCheck failed: %v", err)
	}

	status, err := runner.GetStatus("cleanup-test")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status == nil {
		t.Fatal("expected status to exist after running check")
	}
	if status.CheckID != "cleanup-test" {
		t.Fatalf("expected check ID, got '%s'", status.CheckID)
	}

	runner.CleanupCheck("cleanup-test")

	status, err = runner.GetStatus("cleanup-test")
	if err != nil {
		t.Fatalf("GetStatus error: %v", err)
	}
	if status != nil {
		t.Error("expected nil status after cleanup")
	}
}

func TestRunnerGetUnhealthyChecks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	store := NewStore(dir)
	store.CreateCheck(dto.HealthCheck{
		ID:             "healthy-one",
		Name:           "Healthy",
		Type:           dto.HealthCheckHTTP,
		Target:         srv.URL,
		TimeoutSeconds: 5,
		SuccessCode:    200,
		Enabled:        true,
	})
	store.CreateCheck(dto.HealthCheck{
		ID:             "unhealthy-one",
		Name:           "Unhealthy",
		Type:           dto.HealthCheckHTTP,
		Target:         "http://192.0.2.1:1",
		TimeoutSeconds: 1,
		SuccessCode:    200,
		Enabled:        true,
	})

	runner := NewRunner(store)
	runner.RunSingleCheck(context.Background(), "healthy-one")
	runner.RunSingleCheck(context.Background(), "unhealthy-one")

	unhealthy := runner.GetUnhealthyChecks()
	if len(unhealthy) != 1 {
		t.Errorf("expected 1 unhealthy, got %d", len(unhealthy))
	}
	if len(unhealthy) > 0 && unhealthy[0].CheckID != "unhealthy-one" {
		t.Errorf("expected unhealthy-one, got %s", unhealthy[0].CheckID)
	}
}

func TestRunnerHistoryOnStateTransition(t *testing.T) {
	// History only records state transitions (healthy->unhealthy or unhealthy->healthy)
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError) // fail first 2
		} else {
			w.WriteHeader(http.StatusOK) // then recover
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	store := NewStore(dir)
	store.CreateCheck(dto.HealthCheck{
		ID:             "transition-test",
		Name:           "Transitions",
		Type:           dto.HealthCheckHTTP,
		Target:         srv.URL,
		TimeoutSeconds: 5,
		SuccessCode:    200,
		Enabled:        true,
	})

	runner := NewRunner(store)

	// Run 1: healthy->unhealthy = 1 event (transition)
	runner.RunSingleCheck(context.Background(), "transition-test")
	// Run 2: still unhealthy = no new event
	runner.RunSingleCheck(context.Background(), "transition-test")
	// Run 3: unhealthy->healthy = 1 event (recovery)
	runner.RunSingleCheck(context.Background(), "transition-test")

	history := runner.GetHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 history events (transition + recovery), got %d", len(history))
	}

	// Most recent first (reverse chronological)
	if history[0].State != "healthy" {
		t.Errorf("expected most recent event to be 'healthy' recovery, got '%s'", history[0].State)
	}
	if history[1].State != "unhealthy" {
		t.Errorf("expected older event to be 'unhealthy', got '%s'", history[1].State)
	}
}

func TestRunnerHistoryMaxSize(t *testing.T) {
	// To fill history, we need state transitions. Alternate healthy/unhealthy.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		if callCount%2 == 1 {
			w.WriteHeader(http.StatusInternalServerError) // odd = fail
		} else {
			w.WriteHeader(http.StatusOK) // even = recover
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	store := NewStore(dir)
	store.CreateCheck(dto.HealthCheck{
		ID:             "max-hist",
		Name:           "Max History",
		Type:           dto.HealthCheckHTTP,
		Target:         srv.URL,
		TimeoutSeconds: 5,
		SuccessCode:    200,
		Enabled:        true,
	})

	runner := NewRunner(store)

	// Each call alternates state, producing a history event each time
	for range MaxHistoryEvents + 20 {
		runner.RunSingleCheck(context.Background(), "max-hist")
	}

	history := runner.GetHistory()
	if len(history) > MaxHistoryEvents {
		t.Errorf("history should be capped at %d, got %d", MaxHistoryEvents, len(history))
	}
}

func TestRunnerStartStop(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	runner := NewRunner(store)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runner.Start(ctx)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runner did not stop within timeout")
	}
}

func TestRunnerConsecutiveFails(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	store.CreateCheck(dto.HealthCheck{
		ID:             "consec-fail",
		Name:           "Consecutive",
		Type:           dto.HealthCheckTCP,
		Target:         "127.0.0.1:1",
		TimeoutSeconds: 1,
		Enabled:        true,
	})

	runner := NewRunner(store)

	for range 3 {
		runner.RunSingleCheck(context.Background(), "consec-fail")
	}

	status, err := runner.GetStatus("consec-fail")
	if err != nil {
		t.Fatal(err)
	}
	if status == nil {
		t.Fatal("expected status to exist")
	}
	if status.ConsecutiveFails != 3 {
		t.Errorf("expected 3 consecutive fails, got %d", status.ConsecutiveFails)
	}
	if status.Healthy {
		t.Error("expected unhealthy")
	}
}

func TestRunnerRecovery(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	store := NewStore(dir)
	store.CreateCheck(dto.HealthCheck{
		ID:             "recovery-test",
		Name:           "Recovery",
		Type:           dto.HealthCheckHTTP,
		Target:         srv.URL,
		TimeoutSeconds: 5,
		SuccessCode:    200,
		Enabled:        true,
	})

	runner := NewRunner(store)

	runner.RunSingleCheck(context.Background(), "recovery-test")
	runner.RunSingleCheck(context.Background(), "recovery-test")
	status, _ := runner.GetStatus("recovery-test")
	if status.Healthy {
		t.Error("should be unhealthy after 2 fails")
	}

	runner.RunSingleCheck(context.Background(), "recovery-test")
	status, _ = runner.GetStatus("recovery-test")
	if !status.Healthy {
		t.Error("should be healthy after recovery")
	}
	if status.ConsecutiveFails != 0 {
		t.Errorf("consecutive fails should be 0 after recovery, got %d", status.ConsecutiveFails)
	}
}
