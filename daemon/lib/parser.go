// Package lib provides utility functions for parsing, validation, and shell command execution.
package lib

import (
	"fmt"

	"gopkg.in/ini.v1"
)

// ParseINIFile parses an INI file and returns a map
func ParseINIFile(path string) (map[string]string, error) {
	cfg, err := ini.Load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse INI file %s: %w", path, err)
	}

	result := make(map[string]string)
	// Get the default section (unnamed section)
	defaultSection := cfg.Section("")
	for _, key := range defaultSection.Keys() {
		result[key.Name()] = key.String()
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
