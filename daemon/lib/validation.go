package lib

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
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

	// Fan IDs: alphanumeric, underscores, hyphens (max 100 chars)
	// e.g. "hwmon0_fan1", "it8721_fan2", "ipmi_fan3"
	fanIDRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,99}$`)
)

// ValidateContainerID validates a Docker container ID format
// Accepts both short (12 chars) and full (64 chars) hexadecimal IDs
func ValidateContainerID(id string) error {
	if id == "" {
		return errors.New("container ID cannot be empty")
	}

	// Convert to lowercase for validation
	id = strings.ToLower(id)

	// Check if it matches either short or full format
	if containerIDShortRegex.MatchString(id) || containerIDFullRegex.MatchString(id) {
		return nil
	}

	return errors.New("invalid container ID format: must be 12 or 64 hexadecimal characters")
}

// ValidateVMName validates a virtual machine name
// Allows alphanumeric characters, spaces, hyphens, underscores, and dots
// Maximum length: 253 characters (DNS hostname limit)
func ValidateVMName(name string) error {
	if name == "" {
		return errors.New("VM name cannot be empty")
	}

	if len(name) > 253 {
		return fmt.Errorf("VM name too long: maximum 253 characters, got %d", len(name))
	}

	if !vmNameRegex.MatchString(name) {
		return errors.New("invalid VM name format: must contain only alphanumeric characters, spaces, hyphens, underscores, and dots")
	}

	// Additional checks for common issues
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return errors.New("invalid VM name: cannot start or end with hyphen")
	}

	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return errors.New("invalid VM name: cannot start or end with dot")
	}

	return nil
}

// ValidateDiskID validates a disk identifier
// Supports common Linux disk naming patterns
func ValidateDiskID(id string) error {
	if id == "" {
		return errors.New("disk ID cannot be empty")
	}

	if !diskIDRegex.MatchString(id) {
		return errors.New("invalid disk ID format: must match Linux disk naming pattern (e.g., sda, nvme0n1, md0)")
	}

	return nil
}

// ValidateShareName validates an Unraid share name
// Prevents path traversal attacks by ensuring the name contains only safe characters
// and does not contain path separators or parent directory references
func ValidateShareName(name string) error {
	if name == "" {
		return errors.New("share name cannot be empty")
	}

	if len(name) > 255 {
		return fmt.Errorf("share name too long: maximum 255 characters, got %d", len(name))
	}

	// Check for parent directory references first (more specific check)
	if strings.Contains(name, "..") {
		return errors.New("invalid share name: cannot contain parent directory references")
	}

	// Check for path separators
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return errors.New("invalid share name: cannot contain path separators")
	}

	// Check for null bytes (CWE-158)
	if strings.Contains(name, "\x00") {
		return errors.New("invalid share name: cannot contain null bytes")
	}

	// Validate against regex pattern (alphanumeric, hyphens, underscores only)
	if !shareNameRegex.MatchString(name) {
		return errors.New("invalid share name format: must contain only alphanumeric characters, hyphens, and underscores")
	}

	// Additional checks for common issues
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return errors.New("invalid share name: cannot start or end with hyphen")
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
		return errors.New("user script name cannot be empty")
	}

	if len(name) > 255 {
		return fmt.Errorf("user script name too long: maximum 255 characters, got %d", len(name))
	}

	// Check for parent directory references first (more specific check)
	if strings.Contains(name, "..") {
		return errors.New("invalid user script name: cannot contain parent directory references")
	}

	// Check for path separators
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return errors.New("invalid user script name: cannot contain path separators")
	}

	// Check for absolute paths
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return errors.New("invalid user script name: cannot be an absolute path")
	}

	// Check for null bytes (CWE-158)
	if strings.Contains(name, "\x00") {
		return errors.New("invalid user script name: cannot contain null bytes")
	}

	// Validate against regex pattern (alphanumeric, hyphens, underscores, dots only)
	if !userScriptNameRegex.MatchString(name) {
		return errors.New("invalid user script name format: must contain only alphanumeric characters, hyphens, underscores, and dots")
	}

	// Additional checks for common issues
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return errors.New("invalid user script name: cannot start or end with hyphen")
	}

	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return errors.New("invalid user script name: cannot start or end with dot")
	}

	return nil
}

// ValidateLogFilename validates a log filename
// Prevents path traversal attacks (CWE-22) by ensuring the filename contains only safe characters
// and does not contain path separators or parent directory references
func ValidateLogFilename(name string) error {
	if name == "" {
		return errors.New("log filename cannot be empty")
	}

	if len(name) > 255 {
		return errors.New("log filename too long: maximum 255 characters")
	}

	// Check for parent directory references (CWE-22 path traversal)
	if strings.Contains(name, "..") {
		return errors.New("invalid log filename: cannot contain parent directory references")
	}

	// Check for path separators (only allow forward slashes for plugin log paths like "plugin/file.log")
	if strings.Contains(name, "\\") {
		return errors.New("invalid log filename: cannot contain backslashes")
	}

	// Check for absolute paths
	if strings.HasPrefix(name, "/") {
		return errors.New("invalid log filename: cannot be an absolute path")
	}

	// Check for null bytes (CWE-158)
	if strings.Contains(name, "\x00") {
		return errors.New("invalid log filename: cannot contain null bytes")
	}

	return nil
}

// ValidateContainerRef validates a Docker container reference (ID or name).
// Accepts both short/full hex IDs and container names.
func ValidateContainerRef(ref string) error {
	if ref == "" {
		return errors.New("container reference cannot be empty")
	}

	if len(ref) > 255 {
		return errors.New("container reference too long: maximum 255 characters")
	}

	// Check for path traversal and injection
	if strings.Contains(ref, "..") || strings.Contains(ref, "/") || strings.Contains(ref, "\\") {
		return errors.New("invalid container reference: cannot contain path separators or directory traversal")
	}

	if strings.Contains(ref, "\x00") {
		return errors.New("invalid container reference: cannot contain null bytes")
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

	return errors.New("invalid container reference: must be a 12/64 hex ID or a valid container name")
}

// ValidatePluginName validates an Unraid plugin name.
func ValidatePluginName(name string) error {
	if name == "" {
		return errors.New("plugin name cannot be empty")
	}

	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return errors.New("invalid plugin name: cannot contain path separators or directory traversal")
	}

	// Check for null bytes (CWE-158)
	if strings.Contains(name, "\x00") {
		return errors.New("invalid plugin name: cannot contain null bytes")
	}

	if !pluginNameRegex.MatchString(name) {
		return errors.New("invalid plugin name format: must contain only alphanumeric characters, hyphens, underscores, and dots")
	}

	return nil
}

// ValidateServiceName validates an Unraid service name.
func ValidateServiceName(name string) error {
	if name == "" {
		return errors.New("service name cannot be empty")
	}

	nameLower := strings.ToLower(name)
	if !serviceNameRegex.MatchString(nameLower) {
		return errors.New("invalid service name format: must be lowercase alphanumeric with hyphens")
	}

	return nil
}

// ValidateSnapshotName validates a VM snapshot name.
func ValidateSnapshotName(name string) error {
	if name == "" {
		return errors.New("snapshot name cannot be empty")
	}

	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return errors.New("invalid snapshot name: cannot contain path separators or directory traversal")
	}

	// Check for null bytes (CWE-158)
	if strings.Contains(name, "\x00") {
		return errors.New("invalid snapshot name: cannot contain null bytes")
	}

	if !snapshotNameRegex.MatchString(name) {
		return errors.New("invalid snapshot name format: must start with alphanumeric and contain only alphanumeric, hyphens, underscores, and dots")
	}

	return nil
}

// ValidateFanID validates a fan device identifier.
func ValidateFanID(id string) error {
	if id == "" {
		return errors.New("fan ID cannot be empty")
	}

	if strings.Contains(id, "..") || strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return errors.New("invalid fan ID: cannot contain path separators or directory traversal")
	}

	if strings.Contains(id, "\x00") {
		return errors.New("invalid fan ID: cannot contain null bytes")
	}

	if !fanIDRegex.MatchString(id) {
		return errors.New("invalid fan ID format: must start with alphanumeric and contain only alphanumeric, hyphens, and underscores (max 100)")
	}

	return nil
}

// ValidatePWMPercent validates a PWM percentage value (0-100).
func ValidatePWMPercent(pct int) error {
	if pct < 0 || pct > 100 {
		return fmt.Errorf("PWM percent must be between 0 and 100, got %d", pct)
	}
	return nil
}

// ValidateFanControlMode validates a fan control mode string.
func ValidateFanControlMode(mode string) error {
	switch mode {
	case "automatic", "manual":
		return nil
	default:
		return fmt.Errorf("invalid fan control mode %q: must be 'automatic' or 'manual'", mode)
	}
}

// ValidateCPUGovernor validates a CPU scaling governor name against the set of
// governors actually supported by the running kernel.
func ValidateCPUGovernor(governor string) error {
	if governor == "" {
		return errors.New("governor cannot be empty")
	}

	available, err := ReadAvailableGovernors()
	if err != nil {
		return fmt.Errorf("cannot determine available governors: %w", err)
	}

	for _, g := range available {
		if g == governor {
			return nil
		}
	}
	return fmt.Errorf("invalid governor %q: available governors are %v", governor, available)
}

// hostnameRegex allows RFC-952/1123 hostnames: labels of alphanumerics and hyphens
// separated by dots, each label at most 63 chars, total at most 253 chars.
var hostnameRegex = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// ValidateHostOrIP validates that a string is a valid hostname or IP address.
// It rejects empty strings, strings starting with '-' (flag injection), and
// values that are not valid IPs or RFC-1123 hostnames.
func ValidateHostOrIP(host string) error {
	if host == "" {
		return errors.New("host cannot be empty")
	}
	if strings.HasPrefix(host, "-") {
		return fmt.Errorf("invalid host %q: must not start with a hyphen", host)
	}
	if net.ParseIP(host) != nil {
		return nil
	}
	if len(host) > 253 {
		return fmt.Errorf("invalid host %q: exceeds 253 characters", host)
	}
	if !hostnameRegex.MatchString(host) {
		return fmt.Errorf("invalid host %q: must be a valid hostname or IP address", host)
	}
	return nil
}

// ValidateFanTempSource validates a fan curve temperature source.
func ValidateFanTempSource(src dto.FanTempSource) error {
	switch src.Type {
	case dto.FanTempSourceHwmon:
		if err := validateHwmonSensorPath(src.SensorPath); err != nil {
			return err
		}
	case dto.FanTempSourceDrives:
		if len(src.DriveIDs) == 0 {
			return errors.New("drives source requires at least one drive ID")
		}
	default:
		return fmt.Errorf("invalid temperature source type: %q", src.Type)
	}
	// Fallback is optional, but if set it must be a valid hwmon path.
	if src.FallbackSensorPath != "" {
		if err := validateHwmonSensorPath(src.FallbackSensorPath); err != nil {
			return err
		}
	}
	return nil
}

// validateHwmonSensorPath ensures a sysfs path is under /sys/class/hwmon and
// free of directory traversal.
// This is a string-level guard: symlinks are resolved by the kernel at open
// time, so callers must still open the path with appropriate (read-only) access.
func validateHwmonSensorPath(path string) error {
	if path == "" {
		return errors.New("hwmon sensor path cannot be empty")
	}
	if len(path) > 4096 {
		return errors.New("hwmon sensor path too long")
	}
	if strings.Contains(path, "..") || strings.Contains(path, "\x00") {
		return errors.New("invalid hwmon sensor path: traversal or null byte")
	}
	if !strings.HasPrefix(path, "/sys/class/hwmon/") {
		return errors.New("hwmon sensor path must be under /sys/class/hwmon/")
	}
	return nil
}

// ValidateRemoteShareSource validates an Unassigned Devices remote-share source
// identifier as accepted by the rc.unassigned mount/umount commands. Valid forms
// are SMB ("//server/share") and NFS ("server:/export"). The value is passed to
// the mount script as a separate argument (never through a shell), so the checks
// here guard against malformed input and flag injection rather than shell
// metacharacters.
func ValidateRemoteShareSource(source string) error {
	if source == "" {
		return errors.New("remote share source cannot be empty")
	}
	if len(source) > 4096 {
		return fmt.Errorf("invalid remote share source: exceeds 4096 characters")
	}
	if strings.ContainsAny(source, "\x00\n\r") {
		return errors.New("invalid remote share source: must not contain control characters")
	}
	if strings.HasPrefix(source, "-") {
		return fmt.Errorf("invalid remote share source %q: must not start with a hyphen", source)
	}
	// Reject path-traversal segments. Note: shell metacharacters are NOT
	// rejected here because the source is always passed to the mount script as a
	// discrete exec argument (never via a shell), and characters such as '$' are
	// legitimate in SMB share names (e.g. hidden "share$").
	if strings.Contains(source, "..") {
		return fmt.Errorf("invalid remote share source %q: must not contain '..'", source)
	}

	switch {
	case strings.HasPrefix(source, "//"):
		// SMB: //server/share — both server and share must be present.
		rest := strings.TrimPrefix(source, "//")
		server, share, ok := strings.Cut(rest, "/")
		if !ok || server == "" || share == "" {
			return fmt.Errorf("invalid SMB remote share source %q: expected //server/share", source)
		}
	case strings.Contains(source, ":/"):
		// NFS: server:/export — both server and a rooted export must be present.
		server, export, _ := strings.Cut(source, ":/")
		if server == "" || export == "" {
			return fmt.Errorf("invalid NFS remote share source %q: expected server:/export", source)
		}
	default:
		return fmt.Errorf("invalid remote share source %q: must be an SMB (//server/share) or NFS (server:/export) source", source)
	}
	return nil
}
