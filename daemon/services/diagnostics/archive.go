package diagnostics

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// CreateArchive creates a ZIP archive from a diagnostic bundle.
// The archive filename follows the pattern: unraid-diagnostics-{hostname}-{timestamp}.zip
func CreateArchive(bundle *dto.DiagnosticBundle, outputDir string) (string, error) {
	timestamp := time.Now().UTC().Format("20060102-150405")
	hostname := bundle.Metadata.Hostname
	if hostname == "" {
		hostname = "unknown"
	}

	filename := fmt.Sprintf("unraid-diagnostics-%s-%s.zip", hostname, timestamp)
	outputPath := filepath.Join(outputDir, filename)

	file, err := os.Create(outputPath) // #nosec G304 -- outputPath is derived from user-supplied outputDir via CLI flag
	if err != nil {
		return "", fmt.Errorf("creating archive file: %w", err)
	}
	defer func() { _ = file.Close() }()

	w := zip.NewWriter(file)
	defer func() { _ = w.Close() }()

	// Write metadata.json
	if err := writeJSON(w, "metadata.json", bundle.Metadata); err != nil {
		return "", fmt.Errorf("writing metadata: %w", err)
	}

	// Write system_state.json
	if err := writeJSON(w, "system_state.json", bundle.SystemState); err != nil {
		return "", fmt.Errorf("writing system state: %w", err)
	}

	// Write array_status.json
	if err := writeJSON(w, "array_status.json", bundle.ArrayStatus); err != nil {
		return "", fmt.Errorf("writing array status: %w", err)
	}

	// Write containers.json
	if err := writeJSON(w, "containers.json", bundle.Containers); err != nil {
		return "", fmt.Errorf("writing containers: %w", err)
	}

	// Write vms.json
	if err := writeJSON(w, "vms.json", bundle.VMs); err != nil {
		return "", fmt.Errorf("writing VMs: %w", err)
	}

	// Write network.json
	if err := writeJSON(w, "network.json", bundle.Network); err != nil {
		return "", fmt.Errorf("writing network: %w", err)
	}

	// Write config/configuration.json
	if err := writeJSON(w, "config/configuration.json", bundle.Configuration); err != nil {
		return "", fmt.Errorf("writing configuration: %w", err)
	}

	// Write logs/diagnostic.jsonl
	if len(bundle.Logs.DiagnosticEntries) > 0 {
		if err := writeJSONLines(w, "logs/diagnostic.jsonl", bundle.Logs.DiagnosticEntries); err != nil {
			return "", fmt.Errorf("writing diagnostic logs: %w", err)
		}
	}

	// Write logs/agent.log
	if len(bundle.Logs.AgentLog) > 0 {
		if err := writeTextLines(w, "logs/agent.log", bundle.Logs.AgentLog); err != nil {
			return "", fmt.Errorf("writing agent log: %w", err)
		}
	}

	// Write logs/syslog.log
	if len(bundle.Logs.SysLog) > 0 {
		if err := writeTextLines(w, "logs/syslog.log", bundle.Logs.SysLog); err != nil {
			return "", fmt.Errorf("writing syslog: %w", err)
		}
	}

	return outputPath, nil
}

func writeJSON(w *zip.Writer, name string, data any) error {
	f, err := w.Create(name)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func writeJSONLines(w *zip.Writer, name string, entries []dto.DiagnosticLogEntry) error {
	f, err := w.Create(name)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			return err
		}
	}
	return nil
}

func writeTextLines(w *zip.Writer, name string, lines []string) error {
	f, err := w.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write([]byte(strings.Join(lines, "\n")))
	return err
}
