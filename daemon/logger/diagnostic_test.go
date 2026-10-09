package logger

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// readEntries reads and decodes every JSONL entry from the given diagnostic log file.
func readEntries(t *testing.T, path string) []dto.DiagnosticLogEntry {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // path is a test-controlled temp file
	if err != nil {
		t.Fatalf("open log file: %v", err)
	}
	defer func() { _ = f.Close() }()

	var entries []dto.DiagnosticLogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry dto.DiagnosticLogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode JSONL line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan log file: %v", err)
	}
	return entries
}

func TestDiagnosticLoggerLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "diag.log")

	dl := NewDiagnosticLogger(logPath, "test-service")
	if dl == nil {
		t.Fatal("NewDiagnosticLogger returned nil")
	}

	ctx := context.WithValue(context.Background(), CorrelationContextKey, "corr-123")
	before := time.Now().UTC().Add(-time.Second)

	dl.Log(ctx, "INFO", "hello", map[string]any{"key": "value"})

	if err := dl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries := readEntries(t, logPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Level != "INFO" {
		t.Errorf("Level = %q, want INFO", e.Level)
	}
	if e.Message != "hello" {
		t.Errorf("Message = %q, want hello", e.Message)
	}
	if e.CorrelationID != "corr-123" {
		t.Errorf("CorrelationID = %q, want corr-123", e.CorrelationID)
	}
	if e.Service != "test-service" {
		t.Errorf("Service = %q, want test-service", e.Service)
	}
	if e.Host == "" {
		t.Error("Host should not be empty")
	}
	if got := e.Context["key"]; got != "value" {
		t.Errorf("Context[key] = %v, want value", got)
	}

	ts, err := time.Parse(time.RFC3339, e.Timestamp)
	if err != nil {
		t.Fatalf("timestamp %q not RFC3339: %v", e.Timestamp, err)
	}
	if ts.Location() != time.UTC {
		t.Errorf("timestamp location = %v, want UTC", ts.Location())
	}
	if ts.Before(before) {
		t.Errorf("timestamp %v is before test start %v", ts, before)
	}
}

func TestDiagnosticLoggerLevels(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "levels.log")

	dl := NewDiagnosticLogger(logPath, "svc")
	ctx := context.Background()

	dl.Error(ctx, "err-msg", nil)
	dl.Warn(ctx, "warn-msg", nil)
	dl.Info(ctx, "info-msg", nil)
	dl.Debug(ctx, "debug-msg", nil)

	if err := dl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries := readEntries(t, logPath)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	want := []struct {
		level string
		msg   string
	}{
		{"ERROR", "err-msg"},
		{"WARN", "warn-msg"},
		{"INFO", "info-msg"},
		{"DEBUG", "debug-msg"},
	}
	for i, w := range want {
		if entries[i].Level != w.level {
			t.Errorf("entry %d Level = %q, want %q", i, entries[i].Level, w.level)
		}
		if entries[i].Message != w.msg {
			t.Errorf("entry %d Message = %q, want %q", i, entries[i].Message, w.msg)
		}
		if entries[i].CorrelationID != "" {
			t.Errorf("entry %d CorrelationID = %q, want empty", i, entries[i].CorrelationID)
		}
	}
}

func TestDiagnosticLoggerNilContext(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nilctx.log")

	dl := NewDiagnosticLogger(logPath, "svc")
	// A nil context variable exercises the nil guard in correlationIDFromContext
	// without tripping vet's literal-nil-context check.
	var ctx context.Context
	dl.Log(ctx, "INFO", "no-ctx", nil)
	if err := dl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries := readEntries(t, logPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].CorrelationID != "" {
		t.Errorf("CorrelationID = %q, want empty", entries[0].CorrelationID)
	}
}

func TestDiagnosticLoggerContextWithoutCorrelationID(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "noncorr.log")

	dl := NewDiagnosticLogger(logPath, "svc")
	// Context carries an unrelated, non-string value under the key.
	ctx := context.WithValue(context.Background(), CorrelationContextKey, 42)
	dl.Info(ctx, "msg", nil)
	if err := dl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries := readEntries(t, logPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].CorrelationID != "" {
		t.Errorf("CorrelationID = %q, want empty", entries[0].CorrelationID)
	}
}

func TestDiagnosticLoggerConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "concurrent.log")

	dl := NewDiagnosticLogger(logPath, "svc")

	const goroutines = 10
	const perGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(g int) {
			defer wg.Done()
			ctx := context.Background()
			for i := range perGoroutine {
				// Each goroutine owns its own fields map.
				dl.Info(ctx, "concurrent", map[string]any{"g": g, "i": i})
			}
		}(g)
	}
	wg.Wait()

	if err := dl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries := readEntries(t, logPath)
	if len(entries) != goroutines*perGoroutine {
		t.Fatalf("expected %d entries, got %d", goroutines*perGoroutine, len(entries))
	}

	// Verify every (g, i) pair appears exactly once, so a missing write masked
	// by a duplicate cannot slip through on the total count alone.
	seen := make(map[[2]int]int, goroutines*perGoroutine)
	for _, e := range entries {
		gv, gok := e.Context["g"].(float64)
		iv, iok := e.Context["i"].(float64)
		if !gok || !iok {
			t.Fatalf("entry missing numeric g/i context: %v", e.Context)
		}
		seen[[2]int{int(gv), int(iv)}]++
	}
	for g := range goroutines {
		for i := range perGoroutine {
			if n := seen[[2]int{g, i}]; n != 1 {
				t.Errorf("pair (g=%d, i=%d) appeared %d times, want 1", g, i, n)
			}
		}
	}
}
