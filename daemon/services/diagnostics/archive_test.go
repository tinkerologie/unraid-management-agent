package diagnostics

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestCreateArchive(t *testing.T) {
	bundle := &dto.DiagnosticBundle{
		Metadata: dto.BundleMetadata{
			Timestamp:    "2026-04-08T00:00:00Z",
			AgentVersion: "2026.04.00",
			Hostname:     "testhost",
		},
		SystemState: dto.BundleSystemState{
			CPUUsage:   12.5,
			RAMUsage:   45.0,
			RAMTotalMB: 16384,
			RAMUsedMB:  7372.8,
			Uptime:     "12345s",
		},
		ArrayStatus: dto.BundleArrayStatus{
			State:      "Started",
			TotalDisks: 4,
		},
		Containers: []dto.BundleContainer{
			{Name: "plex", Image: "plexinc/pms-docker", State: "running"},
		},
		VMs: []dto.BundleVM{
			{Name: "windows10", State: "running"},
		},
		Network: []dto.BundleNetwork{
			{Name: "eth0", IPAddr: "192.168.1.100"},
		},
		Logs: dto.BundleLogs{
			DiagnosticEntries: []dto.DiagnosticLogEntry{
				{Timestamp: "2026-04-08T00:00:00Z", Level: "INFO", Message: "test log", Service: "test"},
			},
			AgentLog: []string{"line1", "line2"},
			SysLog:   []string{"syslog1"},
		},
		Configuration: dto.BundleConfiguration{
			Port:    8043,
			Version: "2026.04.00",
			CollectorIntervals: map[string]int{
				"system": 15,
				"docker": 30,
			},
		},
	}

	tmpDir := t.TempDir()
	outputPath, err := CreateArchive(bundle, tmpDir)
	if err != nil {
		t.Fatalf("CreateArchive() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("archive file not found: %v", err)
	}

	// Verify filename format
	base := filepath.Base(outputPath)
	if !strings.Contains(base, "unraid-diagnostics-testhost-") || !strings.Contains(base, ".zip") {
		t.Errorf("unexpected filename: %s", base)
	}

	// Open and verify ZIP contents
	reader, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("failed to open ZIP: %v", err)
	}
	defer func() { _ = reader.Close() }()

	expectedFiles := map[string]bool{
		"metadata.json":             false,
		"system_state.json":         false,
		"array_status.json":         false,
		"containers.json":           false,
		"vms.json":                  false,
		"network.json":              false,
		"config/configuration.json": false,
		"logs/diagnostic.jsonl":     false,
		"logs/agent.log":            false,
		"logs/syslog.log":           false,
	}

	for _, f := range reader.File {
		if _, ok := expectedFiles[f.Name]; ok {
			expectedFiles[f.Name] = true
		}
	}

	for name, found := range expectedFiles {
		if !found {
			t.Errorf("expected file %q not found in archive", name)
		}
	}

	// Verify metadata.json is valid JSON
	for _, f := range reader.File {
		if f.Name == "metadata.json" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("failed to open metadata.json: %v", err)
			}
			var meta dto.BundleMetadata
			if err := json.NewDecoder(rc).Decode(&meta); err != nil {
				t.Errorf("metadata.json is not valid JSON: %v", err)
			}
			_ = rc.Close()
			if meta.Hostname != "testhost" {
				t.Errorf("metadata hostname = %q, want %q", meta.Hostname, "testhost")
			}
		}
	}
}

func TestCreateArchive_EmptyHostname(t *testing.T) {
	bundle := &dto.DiagnosticBundle{
		Metadata: dto.BundleMetadata{Timestamp: "2026-04-08T00:00:00Z"},
	}

	tmpDir := t.TempDir()
	outputPath, err := CreateArchive(bundle, tmpDir)
	if err != nil {
		t.Fatalf("CreateArchive() error = %v", err)
	}

	base := filepath.Base(outputPath)
	if !strings.Contains(base, "unraid-diagnostics-unknown-") {
		t.Errorf("expected 'unknown' hostname in filename, got: %s", base)
	}
}
