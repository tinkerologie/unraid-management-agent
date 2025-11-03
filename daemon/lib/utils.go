package lib

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadFile reads entire file contents
func ReadFile(path string) (string, error) {
	//nolint:gosec // G304: Path is from trusted sources (system files, config files), not user input
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", path, err)
	}
	return string(data), nil
}

// ReadLines reads a file and returns lines
func ReadLines(path string) ([]string, error) {
	content, err := ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(content, "\n"), nil
}

// ParseFloat safely parses a float from string
func ParseFloat(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

// ParseInt safely parses an integer from string
func ParseInt(s string) int {
	i, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return i
}

// ParseUint64 safely parses uint64 from string
func ParseUint64(s string) uint64 {
	i, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return i
}

// Round rounds a float to nearest integer
func Round(f float64) int {
	if f < 0 {
		return int(f - 0.5)
	}
	return int(f + 0.5)
}

// RoundFloat rounds a float to n decimal places
func RoundFloat(f float64, decimals int) float64 {
	multiplier := math.Pow(10, float64(decimals))
	return math.Round(f*multiplier) / multiplier
}

// ParseKeyValue parses "key=value" format
func ParseKeyValue(line string) (string, string) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", ""
	}
	key := strings.TrimSpace(parts[0])
	value := strings.Trim(strings.TrimSpace(parts[1]), "\"")
	return key, value
}

// ParseKeyValueMap parses multiple key=value lines into a map
func ParseKeyValueMap(lines []string) map[string]string {
	result := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value := ParseKeyValue(line)
		if key != "" {
			result[key] = value
		}
	}
	return result
}

// BytesToGB converts bytes to gigabytes
func BytesToGB(bytes uint64) float64 {
	return float64(bytes) / 1024 / 1024 / 1024
}

// BytesToMB converts bytes to megabytes
func BytesToMB(bytes uint64) float64 {
	return float64(bytes) / 1024 / 1024
}

// GBToBytes converts gigabytes to bytes
func GBToBytes(gb float64) uint64 {
	return uint64(gb * 1024 * 1024 * 1024)
}

// MBToBytes converts megabytes to bytes
func MBToBytes(mb float64) uint64 {
	return uint64(mb * 1024 * 1024)
}

// KBToBytes converts kilobytes to bytes
func KBToBytes(kb float64) uint64 {
	return uint64(kb * 1024)
}
