package collectors

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// syncBuffer is a concurrency-safe log sink. The watchdog logs its stall warning
// and goroutine dump from a separate goroutine while the test reads the captured
// output, so the buffer must guard against concurrent Write/Read.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// captureLogs redirects the standard logger to a synchronized buffer at debug
// level for the duration of a test, restoring the previous state afterwards.
func captureLogs(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prevLevel := logger.GetLevel()
	log.SetOutput(buf)
	logger.SetLevel(logger.LevelDebug)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		logger.SetLevel(prevLevel)
	})
	return buf
}

func TestCollectWithWatchdog_FastCycleNoStall(t *testing.T) {
	buf := captureLogs(t)

	ran := false
	collectWithWatchdog(context.Background(), "Test", 30*time.Second, func() { ran = true })

	if !ran {
		t.Fatal("collect function was not invoked")
	}

	out := buf.String()
	if !strings.Contains(out, "Test: collect cycle starting") {
		t.Errorf("expected cycle-start log, got: %q", out)
	}
	if !strings.Contains(out, "collect cycle finished in") {
		t.Errorf("expected fast-finish log, got: %q", out)
	}
	if strings.Contains(out, "was stalled") || strings.Contains(out, "dumping goroutine stacks") {
		t.Errorf("watchdog should not fire for a fast cycle, got: %q", out)
	}
}

func TestCollectWithWatchdog_SlowCycleDumpsStacks(t *testing.T) {
	buf := captureLogs(t)

	// Use the threshold-based core with a threshold far below the cycle duration
	// so the watchdog reliably fires (the interval-based wrapper clamps to a
	// 30s minimum, which is impractical for a unit test).
	runCollectWithWatchdog(context.Background(), "Test", 20*time.Millisecond, func() {
		time.Sleep(150 * time.Millisecond)
	})

	// The finish log is written synchronously when collectWithWatchdog returns.
	out := buf.String()
	if !strings.Contains(out, "was stalled") {
		t.Errorf("expected stalled-finish log, got: %q", out)
	}

	// The stall warning and goroutine dump are emitted from the watchdog
	// goroutine; it fires at the threshold (20ms), well before the 150ms cycle
	// ends, so it is present by now. Poll briefly to absorb scheduler latency.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "dumping goroutine stacks") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	out = buf.String()
	if !strings.Contains(out, "likely stalled; dumping goroutine stacks") {
		t.Errorf("expected stall warning, got: %q", out)
	}
	// The dump must contain a real goroutine trace, not just the header.
	if !strings.Contains(out, "goroutine ") {
		t.Errorf("expected goroutine stack dump content, got: %q", out)
	}
}

func TestWatchdogThreshold(t *testing.T) {
	cases := []struct {
		name     string
		interval time.Duration
		want     time.Duration
	}{
		{"below floor clamps up", 5 * time.Second, watchdogFloor},
		{"at floor", watchdogFloor, watchdogFloor},
		{"between floor and ceil passes through", 60 * time.Second, 60 * time.Second},
		{"at ceil", watchdogCeil, watchdogCeil},
		{"above ceil clamps down", 6 * time.Hour, watchdogCeil},
		{"zero clamps to floor", 0, watchdogFloor},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := watchdogThreshold(tc.interval); got != tc.want {
				t.Errorf("watchdogThreshold(%v) = %v, want %v", tc.interval, got, tc.want)
			}
		})
	}
}

func TestCollectWithWatchdog_RecoversPanic(t *testing.T) {
	captureLogs(t)

	// collectWithWatchdog does not itself recover; it must let the panic
	// propagate (the collector's own deferred recover handles it) while still
	// stopping the watchdog via its deferred close. Verify the panic surfaces
	// and no goroutine is left blocked.
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic to propagate through collectWithWatchdog")
		}
	}()

	collectWithWatchdog(context.Background(), "Test", 30*time.Second, func() {
		panic("boom")
	})
}
