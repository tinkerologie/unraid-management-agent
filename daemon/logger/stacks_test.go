package logger

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestStackBuf(t *testing.T) {
	s := stackBuf()
	if s == "" {
		t.Fatal("stackBuf returned empty string")
	}
	if !strings.Contains(s, "goroutine") {
		t.Errorf("stackBuf output does not look like a stack trace: %q", s)
	}
}

func TestAllGoroutineStacks(t *testing.T) {
	s := AllGoroutineStacks()
	if s == "" {
		t.Fatal("AllGoroutineStacks returned empty string")
	}
	if !strings.Contains(s, "goroutine") {
		t.Errorf("AllGoroutineStacks output does not look like a stack dump: %q", s)
	}
	const (
		maxOutput = 512 << 10
		marker    = "\n... (truncated)"
	)
	// The output is capped at maxOutput; a truncated dump must carry the marker
	// and must never exceed maxOutput plus the marker length.
	if len(s) > maxOutput {
		if !strings.HasSuffix(s, marker) {
			t.Errorf("output exceeds cap without truncation marker (len=%d)", len(s))
		}
		if len(s) != maxOutput+len(marker) {
			t.Errorf("truncated output len=%d, want exactly %d", len(s), maxOutput+len(marker))
		}
	}
}

func TestLogPanicWithStack(t *testing.T) {
	// Save and restore global logger state so this test is isolated.
	originalLevel := GetLevel()
	originalOut := log.Writer()
	originalFlags := log.Flags()
	t.Cleanup(func() {
		SetLevel(originalLevel)
		log.SetOutput(originalOut)
		log.SetFlags(originalFlags)
	})

	SetLevel(LevelError)

	t.Run("writes prefix, value and stack", func(t *testing.T) {
		var buf bytes.Buffer
		log.SetOutput(&buf)
		log.SetFlags(0)

		LogPanicWithStack("collector", "boom")

		out := buf.String()
		if !strings.Contains(out, "collector") {
			t.Errorf("output missing prefix: %q", out)
		}
		if !strings.Contains(out, "boom") {
			t.Errorf("output missing recovered value: %q", out)
		}
		if !strings.Contains(out, "goroutine") {
			t.Errorf("output missing stack trace: %q", out)
		}
	})

	t.Run("does not panic for varied values", func(t *testing.T) {
		var buf bytes.Buffer
		log.SetOutput(&buf)
		log.SetFlags(0)
		// Should not panic for any recovered value.
		LogPanicWithStack("test", nil)
		LogPanicWithStack("test", struct{ Code int }{Code: 500})
	})
}

