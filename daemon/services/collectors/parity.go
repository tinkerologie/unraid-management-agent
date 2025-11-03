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

	var records []dto.ParityCheckRecord
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
// Format examples:
// Parity-Check|2024-11-30, 00:30:26 (Saturday)|10 TB|1 day, 4 hr, 1 min, 28 sec|99.1 MB/s|OK|1348756140
// Parity-Sync|2025-05-04, 07:55:41 (Sunday)|16 TB|9 min, 3 sec|Unavailable|Canceled|0
func (c *ParityCollector) parseLine(line string) (dto.ParityCheckRecord, error) {
	parts := strings.Split(line, "|")
	if len(parts) < 7 {
		return dto.ParityCheckRecord{}, fmt.Errorf("invalid line format: expected 7 parts, got %d", len(parts))
	}

	record := dto.ParityCheckRecord{
		Action: strings.TrimSpace(parts[0]),
	}

	// Parse date (format: "2024-11-30, 00:30:26 (Saturday)")
	dateStr := strings.TrimSpace(parts[1])
	// Remove day of week in parentheses
	if idx := strings.Index(dateStr, "("); idx > 0 {
		dateStr = strings.TrimSpace(dateStr[:idx])
	}

	date, err := time.Parse("2006-01-02, 15:04:05", dateStr)
	if err != nil {
		return dto.ParityCheckRecord{}, fmt.Errorf("failed to parse date '%s': %w", dateStr, err)
	}
	record.Date = date

	// Parse size (format: "10 TB" or "16 TB")
	sizeStr := strings.TrimSpace(parts[2])
	size, err := c.parseSize(sizeStr)
	if err != nil {
		logger.Debug("Parity: Failed to parse size '%s': %v", sizeStr, err)
		record.Size = 0
	} else {
		record.Size = size
	}

	// Parse duration (format: "1 day, 4 hr, 1 min, 28 sec" or "9 min, 3 sec")
	durationStr := strings.TrimSpace(parts[3])
	duration, err := c.parseDuration(durationStr)
	if err != nil {
		logger.Debug("Parity: Failed to parse duration '%s': %v", durationStr, err)
		record.Duration = 0
	} else {
		record.Duration = duration
	}

	// Parse speed (format: "99.1 MB/s" or "Unavailable")
	speedStr := strings.TrimSpace(parts[4])
	if speedStr == "Unavailable" || speedStr == "" {
		record.Speed = 0
	} else {
		speed, err := c.parseSpeed(speedStr)
		if err != nil {
			logger.Debug("Parity: Failed to parse speed '%s': %v", speedStr, err)
			record.Speed = 0
		} else {
			record.Speed = speed
		}
	}

	// Parse status (format: "OK", "Canceled", or error count like "3572342875")
	statusStr := strings.TrimSpace(parts[5])
	record.Status = statusStr
	if statusStr != "OK" && statusStr != "Canceled" {
		// Try to parse as error count
		if errors, err := strconv.ParseInt(statusStr, 10, 64); err == nil {
			record.Errors = errors
			record.Status = fmt.Sprintf("%d errors", errors)
		}
	}

	// Parse errors (last field - error count)
	errorsStr := strings.TrimSpace(parts[6])
	if errors, err := strconv.ParseInt(errorsStr, 10, 64); err == nil {
		record.Errors = errors
	}

	return record, nil
}

// parseSize converts size string like "10 TB" to bytes
func (c *ParityCollector) parseSize(sizeStr string) (uint64, error) {
	parts := strings.Fields(sizeStr)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid size format: %s", sizeStr)
	}

	value, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size value: %s", parts[0])
	}

	unit := strings.ToUpper(parts[1])
	var multiplier uint64

	switch unit {
	case "B":
		multiplier = 1
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	case "TB":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown size unit: %s", unit)
	}

	return uint64(value * float64(multiplier)), nil
}

// parseDuration converts duration string like "1 day, 4 hr, 1 min, 28 sec" to seconds
func (c *ParityCollector) parseDuration(durationStr string) (int64, error) {
	var totalSeconds int64

	parts := strings.Split(durationStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		fields := strings.Fields(part)
		if len(fields) != 2 {
			continue
		}

		value, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}

		unit := strings.ToLower(fields[1])
		switch {
		case strings.HasPrefix(unit, "day"):
			totalSeconds += value * 86400
		case strings.HasPrefix(unit, "hr") || strings.HasPrefix(unit, "hour"):
			totalSeconds += value * 3600
		case strings.HasPrefix(unit, "min"):
			totalSeconds += value * 60
		case strings.HasPrefix(unit, "sec"):
			totalSeconds += value
		}
	}

	return totalSeconds, nil
}

// parseSpeed converts speed string like "99.1 MB/s" to MB/s
func (c *ParityCollector) parseSpeed(speedStr string) (float64, error) {
	// Remove " MB/s" suffix
	speedStr = strings.TrimSuffix(speedStr, " MB/s")
	speedStr = strings.TrimSpace(speedStr)

	speed, err := strconv.ParseFloat(speedStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid speed value: %s", speedStr)
	}

	return speed, nil
}
