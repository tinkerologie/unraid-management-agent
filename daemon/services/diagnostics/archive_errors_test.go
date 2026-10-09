package diagnostics

import (
	"archive/zip"
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// failingWriter returns an error after allowing remaining bytes through.
type failingWriter struct {
	remaining int
}

func (f *failingWriter) Write(p []byte) (int, error) {
	if f.remaining <= 0 {
		return 0, errors.New("write failed")
	}
	if len(p) > f.remaining {
		n := f.remaining
		f.remaining = 0
		return n, errors.New("write failed")
	}
	f.remaining -= len(p)
	return len(p), nil
}

func TestWriteArchive_FailingWriter(t *testing.T) {
	bundle := &dto.DiagnosticBundle{
		Metadata: dto.BundleMetadata{Hostname: "h"},
	}
	// Writer fails immediately so the first entry cannot be flushed.
	w := &failingWriter{remaining: 0}
	if err := WriteArchive(w, bundle); err == nil {
		t.Fatal("expected error from failing writer, got nil")
	}
}

func TestWriteArchive_UnsupportedJSONValue(t *testing.T) {
	bundle := &dto.DiagnosticBundle{
		Metadata:    dto.BundleMetadata{Hostname: "h"},
		SystemState: dto.BundleSystemState{CPUUsage: math.NaN()},
	}
	var buf bytes.Buffer
	err := WriteArchive(&buf, bundle)
	if err == nil {
		t.Fatal("expected error for NaN value, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("system state")) {
		t.Errorf("error = %v, want it to mention system state", err)
	}
}

func TestWriteArchive_OmitsEmptyLogs(t *testing.T) {
	bundle := &dto.DiagnosticBundle{
		Metadata: dto.BundleMetadata{Hostname: "h"},
		// All log slices empty => log entries must be omitted.
	}
	var buf bytes.Buffer
	if err := WriteArchive(&buf, bundle); err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("invalid zip: %v", err)
	}
	for _, f := range reader.File {
		switch f.Name {
		case "logs/diagnostic.jsonl", "logs/agent.log", "logs/syslog.log":
			t.Errorf("empty log file %q should be omitted", f.Name)
		}
	}
}

func TestWriteArchive_IncludesDiagnosticEntries(t *testing.T) {
	bundle := &dto.DiagnosticBundle{
		Metadata: dto.BundleMetadata{Hostname: "h"},
		Logs: dto.BundleLogs{
			DiagnosticEntries: []dto.DiagnosticLogEntry{
				{Timestamp: "2026-04-08T00:00:00Z", Level: "INFO", Message: "m", Service: "s"},
			},
		},
	}
	var buf bytes.Buffer
	if err := WriteArchive(&buf, bundle); err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("invalid zip: %v", err)
	}
	found := false
	for _, f := range reader.File {
		if f.Name == "logs/diagnostic.jsonl" {
			found = true
		}
	}
	if !found {
		t.Error("expected logs/diagnostic.jsonl to be present")
	}
}

func TestCreateArchive_NonexistentOutputDir(t *testing.T) {
	bundle := &dto.DiagnosticBundle{
		Metadata: dto.BundleMetadata{Hostname: "h"},
	}
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := CreateArchive(bundle, missingDir)
	if err == nil {
		t.Fatal("expected error for nonexistent output directory, got nil")
	}
}

func TestCreateArchive_RemovesPartialFileOnFailure(t *testing.T) {
	// A NaN value makes WriteArchive fail after the output file is created,
	// so CreateArchive must remove the partially written file.
	bundle := &dto.DiagnosticBundle{
		Metadata:    dto.BundleMetadata{Hostname: "partial"},
		SystemState: dto.BundleSystemState{CPUUsage: math.NaN()},
	}
	dir := t.TempDir()
	_, err := CreateArchive(bundle, dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("read dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Errorf("expected output dir to be empty after failure, found %d entries", len(entries))
	}
}
