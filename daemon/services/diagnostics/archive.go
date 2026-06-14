package diagnostics

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

var hostnameCleanRe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// ArchiveFilename returns the standard archive filename for a bundle, following
// the pattern unraid-diagnostics-{hostname}-{timestamp}.zip. The hostname is
// sanitised so it is always safe to use in a filename and HTTP header.
func ArchiveFilename(bundle *dto.DiagnosticBundle) string {
	timestamp := time.Now().UTC().Format("20060102-150405")
	hostname := bundle.Metadata.Hostname
	if hostname == "" {
		hostname = "unknown"
	} else {
		hostname = hostnameCleanRe.ReplaceAllString(hostname, "_")
		if hostname == "" {
			hostname = "unknown"
		}
	}
	return fmt.Sprintf("unraid-diagnostics-%s-%s.zip", hostname, timestamp)
}

// WriteArchive writes a diagnostic bundle as a ZIP stream to w. It is shared by
// the CLI (which writes to a file via CreateArchive) and the HTTP download
// handler (which streams to a buffer/response), so the archive contents stay
// identical regardless of how diagnostics are produced.
func WriteArchive(w io.Writer, bundle *dto.DiagnosticBundle) error {
	zw := zip.NewWriter(w)

	if err := writeJSON(zw, "metadata.json", bundle.Metadata); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}
	if err := writeJSON(zw, "system_state.json", bundle.SystemState); err != nil {
		return fmt.Errorf("writing system state: %w", err)
	}
	if err := writeJSON(zw, "array_status.json", bundle.ArrayStatus); err != nil {
		return fmt.Errorf("writing array status: %w", err)
	}
	if err := writeJSON(zw, "containers.json", bundle.Containers); err != nil {
		return fmt.Errorf("writing containers: %w", err)
	}
	if err := writeJSON(zw, "vms.json", bundle.VMs); err != nil {
		return fmt.Errorf("writing VMs: %w", err)
	}
	if err := writeJSON(zw, "network.json", bundle.Network); err != nil {
		return fmt.Errorf("writing network: %w", err)
	}
	if err := writeJSON(zw, "config/configuration.json", bundle.Configuration); err != nil {
		return fmt.Errorf("writing configuration: %w", err)
	}
	if len(bundle.Logs.DiagnosticEntries) > 0 {
		if err := writeJSONLines(zw, "logs/diagnostic.jsonl", bundle.Logs.DiagnosticEntries); err != nil {
			return fmt.Errorf("writing diagnostic logs: %w", err)
		}
	}
	if len(bundle.Logs.AgentLog) > 0 {
		if err := writeTextLines(zw, "logs/agent.log", bundle.Logs.AgentLog); err != nil {
			return fmt.Errorf("writing agent log: %w", err)
		}
	}
	if len(bundle.Logs.SysLog) > 0 {
		if err := writeTextLines(zw, "logs/syslog.log", bundle.Logs.SysLog); err != nil {
			return fmt.Errorf("writing syslog: %w", err)
		}
	}

	// Close flushes the central directory; must be checked for write errors.
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finalizing archive: %w", err)
	}
	return nil
}

// CreateArchive creates a ZIP archive file from a diagnostic bundle in
// outputDir and returns its path. Used by the CLI diagnostics command.
func CreateArchive(bundle *dto.DiagnosticBundle, outputDir string) (string, error) {
	outputPath := filepath.Join(outputDir, ArchiveFilename(bundle))

	file, err := os.Create(outputPath) // #nosec G304 -- outputPath is derived from user-supplied outputDir via CLI flag
	if err != nil {
		return "", fmt.Errorf("creating archive file: %w", err)
	}
	archiveOK := false
	defer func() {
		_ = file.Close()
		if !archiveOK {
			_ = os.Remove(outputPath)
		}
	}()

	if err := WriteArchive(file, bundle); err != nil {
		return "", err
	}

	// Close file and check for write errors before declaring success.
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("closing archive file: %w", err)
	}

	archiveOK = true
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
