package lib

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Docker container IDs are either 12 or 64 hexadecimal characters
	containerIDShortRegex = regexp.MustCompile(`^[a-f0-9]{12}$`)
	containerIDFullRegex  = regexp.MustCompile(`^[a-f0-9]{64}$`)

	// VM names: alphanumeric, spaces, hyphens, underscores, dots (max 253 chars)
	// Based on common VM naming practices (allows spaces for user-friendly names)
	vmNameRegex = regexp.MustCompile(`^[a-zA-Z0-9 _.-]{1,253}$`)

	// Disk IDs: common Linux disk naming patterns
	// Examples: sda, sdb1, nvme0n1, nvme0n1p1, md0, loop0
	diskIDRegex = regexp.MustCompile(`^(sd[a-z]|nvme[0-9]+n[0-9]+|md[0-9]+|loop[0-9]+)(p?[0-9]+)?$`)

	// Share names: alphanumeric, hyphens, underscores (max 255 chars)
	// Must not contain path separators or parent directory references
	shareNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,255}$`)

	// User script names: alphanumeric, hyphens, underscores, dots (max 255 chars)
	// Must not contain path separators or parent directory references
	userScriptNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,255}$`)

	// Container references: either hex IDs (12/64 chars) or names (alphanumeric, hyphens, underscores, dots, slashes)
	containerNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,254}$`)

	// Plugin names: alphanumeric, hyphens, underscores, dots (max 255 chars)
	pluginNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,255}$`)

	// Service names: lowercase alphanumeric and hyphens
	serviceNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

	// Snapshot names: alphanumeric, hyphens, underscores, dots (max 255 chars)
	snapshotNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,254}$`)
)

// ValidateContainerID validates a Docker container ID format
// Accepts both short (12 chars) and full (64 chars) hexadecimal IDs
func ValidateContainerID(id string) error {
	if id == "" {
		return fmt.Errorf("container ID cannot be empty")
	}

	// Convert to lowercase for validation
	id = strings.ToLower(id)

	// Check if it matches either short or full format
	if containerIDShortRegex.MatchString(id) || containerIDFullRegex.MatchString(id) {
		return nil
	}

	return fmt.Errorf("invalid container ID format: must be 12 or 64 hexadecimal characters")
}

// ValidateVMName validates a virtual machine name
// Allows alphanumeric characters, spaces, hyphens, underscores, and dots
// Maximum length: 253 characters (DNS hostname limit)
func ValidateVMName(name string) error {
	if name == "" {
		return fmt.Errorf("VM name cannot be empty")
	}

	if len(name) > 253 {
		return fmt.Errorf("VM name too long: maximum 253 characters, got %d", len(name))
	}

	if !vmNameRegex.MatchString(name) {
		return fmt.Errorf("invalid VM name format: must contain only alphanumeric characters, spaces, hyphens, underscores, and dots")
	}

	// Additional checks for common issues
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return fmt.Errorf("invalid VM name: cannot start or end with hyphen")
	}

	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return fmt.Errorf("invalid VM name: cannot start or end with dot")
	}

	return nil
}

// ValidateDiskID validates a disk identifier
// Supports common Linux disk naming patterns
func ValidateDiskID(id string) error {
	if id == "" {
		return fmt.Errorf("disk ID cannot be empty")
	}

	if !diskIDRegex.MatchString(id) {
		return fmt.Errorf("invalid disk ID format: must match Linux disk naming pattern (e.g., sda, nvme0n1, md0)")
	}

	return nil
}

// ValidateShareName validates an Unraid share name
// Prevents path traversal attacks by ensuring the name contains only safe characters
// and does not contain path separators or parent directory references
func ValidateShareName(name string) error {
	if name == "" {
		return fmt.Errorf("share name cannot be empty")
	}

	if len(name) > 255 {
		return fmt.Errorf("share name too long: maximum 255 characters, got %d", len(name))
	}

	// Check for parent directory references first (more specific check)
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid share name: cannot contain parent directory references")
	}

	// Check for path separators
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid share name: cannot contain path separators")
	}

	// Validate against regex pattern (alphanumeric, hyphens, underscores only)
	if !shareNameRegex.MatchString(name) {
		return fmt.Errorf("invalid share name format: must contain only alphanumeric characters, hyphens, and underscores")
	}

	// Additional checks for common issues
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return fmt.Errorf("invalid share name: cannot start or end with hyphen")
	}

	return nil
}

// ValidateNonEmpty validates that a string is not empty or whitespace-only
func ValidateNonEmpty(value, fieldName string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s cannot be empty", fieldName)
	}
	return nil
}

// ValidateMaxLength validates that a string does not exceed maximum length
func ValidateMaxLength(value, fieldName string, maxLength int) error {
	if len(value) > maxLength {
		return fmt.Errorf("%s too long: maximum %d characters, got %d", fieldName, maxLength, len(value))
	}
	return nil
}

// ValidateUserScriptName validates a user script name
// Prevents path traversal attacks by ensuring the name contains only safe characters
// and does not contain path separators or parent directory references
func ValidateUserScriptName(name string) error {
	if name == "" {
		return fmt.Errorf("user script name cannot be empty")
	}

	if len(name) > 255 {
		return fmt.Errorf("user script name too long: maximum 255 characters, got %d", len(name))
	}

	// Check for parent directory references first (more specific check)
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid user script name: cannot contain parent directory references")
	}

	// Check for path separators
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid user script name: cannot contain path separators")
	}

	// Check for absolute paths
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return fmt.Errorf("invalid user script name: cannot be an absolute path")
	}

	// Validate against regex pattern (alphanumeric, hyphens, underscores, dots only)
	if !userScriptNameRegex.MatchString(name) {
		return fmt.Errorf("invalid user script name format: must contain only alphanumeric characters, hyphens, underscores, and dots")
	}

	// Additional checks for common issues
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return fmt.Errorf("invalid user script name: cannot start or end with hyphen")
	}

	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return fmt.Errorf("invalid user script name: cannot start or end with dot")
	}

	return nil
}

// ValidateLogFilename validates a log filename
// Prevents path traversal attacks (CWE-22) by ensuring the filename contains only safe characters
// and does not contain path separators or parent directory references
func ValidateLogFilename(name string) bool {
	if name == "" {
		return false
	}

	if len(name) > 255 {
		return false
	}

	// Check for parent directory references (CWE-22 path traversal)
	if strings.Contains(name, "..") {
		return false
	}

	// Check for path separators (only allow forward slashes for plugin log paths like "plugin/file.log")
	if strings.Contains(name, "\\") {
		return false
	}

	// Check for absolute paths
	if strings.HasPrefix(name, "/") {
		return false
	}

	// Check for null bytes (CWE-158)
	if strings.Contains(name, "\x00") {
		return false
	}

	return true
}

// ValidateContainerRef validates a Docker container reference (ID or name).
// Accepts both short/full hex IDs and container names.
func ValidateContainerRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("container reference cannot be empty")
	}

	if len(ref) > 255 {
		return fmt.Errorf("container reference too long: maximum 255 characters")
	}

	// Check for path traversal and injection
	if strings.Contains(ref, "..") || strings.Contains(ref, "/") || strings.Contains(ref, "\\") {
		return fmt.Errorf("invalid container reference: cannot contain path separators or directory traversal")
	}

	if strings.Contains(ref, "\x00") {
		return fmt.Errorf("invalid container reference: cannot contain null bytes")
	}

	// Accept hex IDs (12 or 64 chars)
	refLower := strings.ToLower(ref)
	if containerIDShortRegex.MatchString(refLower) || containerIDFullRegex.MatchString(refLower) {
		return nil
	}

	// Accept container names
	if containerNameRegex.MatchString(ref) {
		return nil
	}

	return fmt.Errorf("invalid container reference: must be a 12/64 hex ID or a valid container name")
}

// ValidatePluginName validates an Unraid plugin name.
func ValidatePluginName(name string) error {
	if name == "" {
		return fmt.Errorf("plugin name cannot be empty")
	}

	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid plugin name: cannot contain path separators or directory traversal")
	}

	if !pluginNameRegex.MatchString(name) {
		return fmt.Errorf("invalid plugin name format: must contain only alphanumeric characters, hyphens, underscores, and dots")
	}

	return nil
}

// ValidateServiceName validates an Unraid service name.
func ValidateServiceName(name string) error {
	if name == "" {
		return fmt.Errorf("service name cannot be empty")
	}

	nameLower := strings.ToLower(name)
	if !serviceNameRegex.MatchString(nameLower) {
		return fmt.Errorf("invalid service name format: must be lowercase alphanumeric with hyphens")
	}

	return nil
}

// ValidateSnapshotName validates a VM snapshot name.
func ValidateSnapshotName(name string) error {
	if name == "" {
		return fmt.Errorf("snapshot name cannot be empty")
	}

	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid snapshot name: cannot contain path separators or directory traversal")
	}

	if !snapshotNameRegex.MatchString(name) {
		return fmt.Errorf("invalid snapshot name format: must start with alphanumeric and contain only alphanumeric, hyphens, underscores, and dots")
	}

	return nil
}
