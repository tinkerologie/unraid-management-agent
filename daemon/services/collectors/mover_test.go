package collectors

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// ---------------------------------------------------------------------------
// parseMoverLog tests
// ---------------------------------------------------------------------------

// TestParseMoverLog_FullRun feeds a fixture that matches the assumed log format
// and asserts that start/finish/files/bytes are parsed correctly.
func TestParseMoverLog_FullRun(t *testing.T) {
	fixture := `
Started (Mon May 30 03:40:00 UTC 2026)
Moving /mnt/cache/appdata/plex → /mnt/user/appdata/plex
Mover: 1024 files (5368709120 bytes) moved
Finished (Mon May 30 03:52:00 UTC 2026)
`
	r := strings.NewReader(fixture)
	start, finish, files, bytes := parseMoverLog(r)

	if start.IsZero() {
		t.Error("expected non-zero start time")
	}
	if finish.IsZero() {
		t.Error("expected non-zero finish time")
	}
	if !start.Before(finish) {
		t.Errorf("expected start (%v) before finish (%v)", start, finish)
	}

	duration := finish.Sub(start)
	if duration < 11*time.Minute || duration > 13*time.Minute {
		t.Errorf("expected duration ~12 minutes, got %v", duration)
	}

	if files != 1024 {
		t.Errorf("expected files=1024, got %d", files)
	}
	if bytes != 5368709120 {
		t.Errorf("expected bytes=5368709120, got %d", bytes)
	}
}

// TestParseMoverLog_Idle asserts that an empty/no-run log returns all zero values.
func TestParseMoverLog_Idle(t *testing.T) {
	r := strings.NewReader("")
	start, finish, files, bytes := parseMoverLog(r)

	if !start.IsZero() {
		t.Errorf("expected zero start time, got %v", start)
	}
	if !finish.IsZero() {
		t.Errorf("expected zero finish time, got %v", finish)
	}
	if files != 0 {
		t.Errorf("expected files=0, got %d", files)
	}
	if bytes != 0 {
		t.Errorf("expected bytes=0, got %d", bytes)
	}
}

// TestParseMoverLog_NoFinish checks partial logs (started but not finished).
func TestParseMoverLog_NoFinish(t *testing.T) {
	fixture := "Started (Mon May 30 03:40:00 UTC 2026)\n"
	r := strings.NewReader(fixture)
	start, finish, _, _ := parseMoverLog(r)

	if start.IsZero() {
		t.Error("expected non-zero start time")
	}
	if !finish.IsZero() {
		t.Errorf("expected zero finish time for in-progress run, got %v", finish)
	}
}

// TestParseMoverLog_MultiRun asserts that only the LAST run is returned when
// the log contains multiple completed runs.
func TestParseMoverLog_MultiRun(t *testing.T) {
	fixture := `
Started (Mon May 29 03:40:00 UTC 2026)
Mover: 10 files (1000 bytes) moved
Finished (Mon May 29 03:50:00 UTC 2026)
Started (Mon May 30 03:40:00 UTC 2026)
Mover: 1024 files (5368709120 bytes) moved
Finished (Mon May 30 03:52:00 UTC 2026)
`
	r := strings.NewReader(fixture)
	start, finish, files, bytes := parseMoverLog(r)

	// Expect the second (most-recent) run.
	if start.Day() != 30 {
		t.Errorf("expected start day=30, got %d", start.Day())
	}
	if finish.Day() != 30 {
		t.Errorf("expected finish day=30, got %d", finish.Day())
	}
	if files != 1024 {
		t.Errorf("expected files=1024, got %d", files)
	}
	if bytes != 5368709120 {
		t.Errorf("expected bytes=5368709120, got %d", bytes)
	}
}

// ---------------------------------------------------------------------------
// MoverCollector publish / dedupe tests (mirrors docker_update_test.go style)
// ---------------------------------------------------------------------------

func TestMoverCollector_PublishesAndDedupes(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicMoverUpdate.Name)
	defer hub.Unsub(sub)

	result := &dto.MoverStatus{
		Active:            false,
		LastRunFinish:     "2026-05-30T03:52:00Z",
		LastRunFilesMoved: 1024,
		LastRunBytesMoved: 5368709120,
		Timestamp:         time.Now(),
	}

	c := NewMoverCollector(&domain.Context{Hub: hub})
	c.CheckFn = func() (*dto.MoverStatus, error) { return result, nil }

	// First collect → must publish.
	c.Collect()
	select {
	case msg := <-sub:
		got, ok := msg.(*dto.MoverStatus)
		if !ok || got.LastRunFilesMoved != 1024 {
			t.Fatalf("unexpected first publish: %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("expected first publish, got none")
	}

	// Second collect with identical result → must NOT publish (dedupe).
	c.Collect()
	select {
	case msg := <-sub:
		t.Fatalf("expected no re-publish on unchanged result, got %#v", msg)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestMoverCollector_NilCheckFnIsSafe(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicMoverUpdate.Name)
	defer hub.Unsub(sub)

	c := NewMoverCollector(&domain.Context{Hub: hub})
	c.CheckFn = nil
	c.Collect() // must not panic and must not publish

	select {
	case msg := <-sub:
		t.Fatalf("expected no publish when CheckFn is nil, got %#v", msg)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestMoverCollector_RepublishesOnChange(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicMoverUpdate.Name)
	defer hub.Unsub(sub)

	c := NewMoverCollector(&domain.Context{Hub: hub})

	c.CheckFn = func() (*dto.MoverStatus, error) {
		return &dto.MoverStatus{LastRunFinish: "2026-05-30T03:52:00Z", LastRunFilesMoved: 100, Timestamp: time.Now()}, nil
	}
	c.Collect()
	select {
	case <-sub: // drain first publish
	case <-time.After(time.Second):
		t.Fatal("expected first publish, got none")
	}

	// Change file count → signature changes → must republish.
	c.CheckFn = func() (*dto.MoverStatus, error) {
		return &dto.MoverStatus{LastRunFinish: "2026-05-30T03:52:00Z", LastRunFilesMoved: 200, Timestamp: time.Now()}, nil
	}
	c.Collect()
	select {
	case <-sub: // success
	case <-time.After(time.Second):
		t.Fatal("expected republish after signature change, got none")
	}
}

func TestMoverCollector_CheckErrorNoPublish(t *testing.T) {
	hub := domain.NewEventBus(16)
	sub := hub.Sub(constants.TopicMoverUpdate.Name)
	defer hub.Unsub(sub)

	c := NewMoverCollector(&domain.Context{Hub: hub})
	c.CheckFn = func() (*dto.MoverStatus, error) { return nil, fmt.Errorf("boom") }
	c.Collect()

	select {
	case <-sub:
		t.Fatal("expected no publish on check error")
	case <-time.After(150 * time.Millisecond):
	}
}
