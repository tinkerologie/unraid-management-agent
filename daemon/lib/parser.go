// Package lib provides utility functions for parsing, validation, and shell command execution.
package lib

import (
	"fmt"

	"github.com/vaughan0/go-ini"
)

// ParseINIFile parses an INI file and returns a map
func ParseINIFile(path string) (map[string]string, error) {
	file, err := ini.LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse INI file %s: %w", path, err)
	}

	result := make(map[string]string)
	for key, value := range file[""] {
		result[key] = value
	}

	return result, nil
}

// GetINIValue gets a value from INI file with default
func GetINIValue(iniData map[string]string, key string, defaultValue string) string {
	if value, ok := iniData[key]; ok {
		return value
	}
	return defaultValue
}
