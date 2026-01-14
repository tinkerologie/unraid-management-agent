package collectors

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const parityLogPath = "/boot/config/parity-checks.log"

// ParityCollector collects parity check history
type ParityCollector struct{}

// NewParityCollector creates a new parity collector
func NewParityCollector() *ParityCollector {
	return &ParityCollector{}
}

// GetParityHistory reads and parses the parity-checks.log file
func (c *ParityCollector) GetParityHistory() (*dto.ParityCheckHistory, error) {
	logger.Debug("Parity: Reading parity check history from %s", parityLogPath)

	file, err := os.Open(parityLogPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debug("Parity: Parity log file does not exist: %s", parityLogPath)
			return &dto.ParityCheckHistory{
				Records:   []dto.ParityCheckRecord{},
				Timestamp: time.Now(),
			}, nil
		}
		return nil, fmt.Errorf("failed to open parity log: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Debug("Error closing parity log file: %v", err)
		}
	}()

	records := []dto.ParityCheckRecord{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		record, err := c.parseLine(line)
		if err != nil {
			logger.Debug("Parity: Failed to parse line: %s - %v", line, err)
			continue
		}

		records = append(records, record)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading parity log: %w", err)
	}

	logger.Debug("Parity: Found %d parity check records", len(records))

	return &dto.ParityCheckHistory{
		Records:   records,
		Timestamp: time.Now(),
	}, nil
}

// parseLine parses a single line from parity-checks.log
// Supports multiple Unraid log formats across different versions:
//
// Format 1 (5 fields - older Unraid before ~2022):
//
//	2022 May 22 20:17:49|73068|54.8 MB/s|0|0
//	Fields: Date|Duration|Speed|ExitCode|Errors
//
// Format 2 (7 fields - standard format):
//
//	2024 Nov 30 00:30:26|100888|99128056|0|1348756140|check P|9766436812
//	Fields: Date|Duration|Speed(bytes/s)|ExitCode|Errors|Action|Size
//
// Format 3 (10 fields - with parity check plugin):
//
//	2024 Sep 9 02:23:50|18548|25.9 MB/s|0|0|check P Q|468850520|95023|2|Scheduled Non-Correcting Parity-Check
//	Fields: Date|Duration|Speed|ExitCode|Errors|Action|Size|ElapsedTime|Increments|Description
//
// Speed can be either raw bytes/second (integer) or human-readable format (e.g., "25.9 MB/s")
func (c *ParityCollector) parseLine(line string) (dto.ParityCheckRecord, error) {
	parts := strings.Split(line, "|")
	if len(parts) < 5 {
		return dto.ParityCheckRecord{}, fmt.Errorf("invalid line format: expected at least 5 parts, got %d", len(parts))
	}

	record := dto.ParityCheckRecord{}

	// Parse date (format: "2024 Nov 30 00:30:26" or "2025 Jan  2 06:25:17" with double space for single-digit days)
	dateStr := strings.TrimSpace(parts[0])
	// Normalize multiple spaces to single space for parsing
	for strings.Contains(dateStr, "  ") {
		dateStr = strings.ReplaceAll(dateStr, "  ", " ")
	}

	date, err := time.Parse("2006 Jan 2 15:04:05", dateStr)
	if err != nil {
		return dto.ParityCheckRecord{}, fmt.Errorf("failed to parse date '%s': %w", dateStr, err)
	}
	record.Date = date

	// Parse duration in seconds (field 1)
	durationStr := strings.TrimSpace(parts[1])
	if duration, err := strconv.ParseInt(durationStr, 10, 64); err == nil {
		record.Duration = duration
	}

	// Parse speed (field 2) - can be raw bytes/second or human-readable format
	speedStr := strings.TrimSpace(parts[2])
	record.Speed = c.parseSpeed(speedStr)

	// Parse exit code (field 3): 0=OK, -4=Canceled
	exitCodeStr := strings.TrimSpace(parts[3])
	exitCode, _ := strconv.ParseInt(exitCodeStr, 10, 64)

	// Parse error count (field 4)
	errorsStr := strings.TrimSpace(parts[4])
	if errors, err := strconv.ParseInt(errorsStr, 10, 64); err == nil {
		record.Errors = errors
	}

	// Handle different format lengths
	if len(parts) >= 7 {
		// Parse action (field 5): "check P"=Parity-Check, "check P Q"=Dual Parity-Check, "recon P"=Parity-Sync/Rebuild
		actionStr := strings.TrimSpace(parts[5])
		record.Action = c.parseAction(actionStr)

		// Parse size in bytes (field 6)
		sizeStr := strings.TrimSpace(parts[6])
		if size, err := strconv.ParseUint(sizeStr, 10, 64); err == nil {
			record.Size = size
		}
	} else {
		// 5-field format - no action or size, default to Parity-Check
		record.Action = "Parity-Check"
		record.Size = 0
	}

	// Determine status based on exit code and errors
	switch exitCode {
	case 0:
		if record.Errors > 0 {
			record.Status = fmt.Sprintf("%d errors", record.Errors)
		} else {
			record.Status = "OK"
		}
	case -4:
		record.Status = "Canceled"
	default:
		record.Status = fmt.Sprintf("Exit code %d", exitCode)
	}

	return record, nil
}

// parseSpeed parses speed from either raw bytes/second or human-readable format
// Examples: "99128056" (bytes/sec), "54.8 MB/s", "Unavailable"
func (c *ParityCollector) parseSpeed(speedStr string) float64 {
	speedStr = strings.TrimSpace(speedStr)

	// Handle unavailable/empty speed
	if speedStr == "" || strings.EqualFold(speedStr, "Unavailable") {
		return 0
	}

	// Try parsing as raw bytes/second first
	if speed, err := strconv.ParseFloat(speedStr, 64); err == nil {
		// It's a raw number - convert bytes/sec to MB/s
		return speed / (1024 * 1024)
	}

	// Try parsing human-readable format (e.g., "54.8 MB/s", "1.2 GB/s")
	speedStr = strings.TrimSuffix(speedStr, "/s")
	speedStr = strings.TrimSpace(speedStr)

	parts := strings.Fields(speedStr)
	if len(parts) != 2 {
		return 0
	}

	value, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}

	unit := strings.ToUpper(parts[1])
	switch unit {
	case "B":
		return value / (1024 * 1024)
	case "KB":
		return value / 1024
	case "MB":
		return value
	case "GB":
		return value * 1024
	case "TB":
		return value * 1024 * 1024
	default:
		return value // Assume MB/s if unit not recognized
	}
}

// parseAction converts action codes to human-readable format
func (c *ParityCollector) parseAction(actionStr string) string {
	switch {
	case strings.HasPrefix(actionStr, "check P Q"):
		return "Dual Parity-Check"
	case strings.HasPrefix(actionStr, "check"):
		return "Parity-Check"
	case strings.HasPrefix(actionStr, "recon P Q"):
		return "Dual Parity-Sync"
	case strings.HasPrefix(actionStr, "recon"):
		return "Parity-Sync"
	case strings.HasPrefix(actionStr, "clear"):
		return "Parity-Clear"
	default:
		return actionStr
	}
}
