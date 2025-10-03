package collectors

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ruaandeysel/unraid-management-agent/daemon/dto"
	"github.com/ruaandeysel/unraid-management-agent/daemon/logger"
)

// ConfigCollector collects configuration data
type ConfigCollector struct{}

// NewConfigCollector creates a new config collector
func NewConfigCollector() *ConfigCollector {
	return &ConfigCollector{}
}

// GetShareConfig reads share configuration from /boot/config/shares/{name}.cfg
func (c *ConfigCollector) GetShareConfig(shareName string) (*dto.ShareConfig, error) {
	configPath := fmt.Sprintf("/boot/config/shares/%s.cfg", shareName)
	logger.Debug("Config: Reading share config from %s", configPath)

	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("share config not found: %s", shareName)
		}
		return nil, fmt.Errorf("failed to open share config: %w", err)
	}
	defer file.Close()

	config := &dto.ShareConfig{
		Name:      shareName,
		Timestamp: time.Now(),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"`)

		switch key {
		case "shareComment":
			config.Comment = value
		case "shareAllocator":
			config.Allocator = value
		case "shareFloor":
			config.Floor = value
		case "shareSplitLevel":
			config.SplitLevel = value
		case "shareInclude":
			if value != "" {
				config.IncludeDisks = strings.Split(value, ",")
			}
		case "shareExclude":
			if value != "" {
				config.ExcludeDisks = strings.Split(value, ",")
			}
		case "shareUseCache":
			config.UseCache = value
		case "shareExport":
			config.Export = value
		case "shareSecurity":
			config.Security = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading share config: %w", err)
	}

	return config, nil
}

// GetNetworkConfig reads network configuration from /boot/config/network.cfg
func (c *ConfigCollector) GetNetworkConfig(interfaceName string) (*dto.NetworkConfig, error) {
	configPath := "/boot/config/network.cfg"
	logger.Debug("Config: Reading network config from %s", configPath)

	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("network config not found")
		}
		return nil, fmt.Errorf("failed to open network config: %w", err)
	}
	defer file.Close()

	config := &dto.NetworkConfig{
		Interface: interfaceName,
		Timestamp: time.Now(),
	}

	scanner := bufio.NewScanner(file)
	inSection := false
	currentInterface := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentInterface = strings.Trim(line, "[]")
			inSection = (currentInterface == interfaceName)
			continue
		}

		if !inSection {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"`)

		switch key {
		case "TYPE":
			config.Type = value
		case "IPADDR":
			config.IPAddress = value
		case "NETMASK":
			config.Netmask = value
		case "GATEWAY":
			config.Gateway = value
		case "BONDING_MODE":
			config.BondingMode = value
		case "BONDING_SLAVES":
			if value != "" {
				config.BondSlaves = strings.Split(value, " ")
			}
		case "BRIDGE_MEMBERS":
			if value != "" {
				config.BridgeMembers = strings.Split(value, " ")
			}
		case "VLAN_ID":
			if vlanID, err := strconv.Atoi(value); err == nil {
				config.VLANID = vlanID
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading network config: %w", err)
	}

	if config.Type == "" {
		return nil, fmt.Errorf("interface not found: %s", interfaceName)
	}

	return config, nil
}

// GetSystemSettings reads system settings from /boot/config/ident.cfg
func (c *ConfigCollector) GetSystemSettings() (*dto.SystemSettings, error) {
	configPath := "/boot/config/ident.cfg"
	logger.Debug("Config: Reading system settings from %s", configPath)

	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("system config not found")
		}
		return nil, fmt.Errorf("failed to open system config: %w", err)
	}
	defer file.Close()

	settings := &dto.SystemSettings{
		Timestamp: time.Now(),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"`)

		switch key {
		case "NAME":
			settings.ServerName = value
		case "COMMENT":
			settings.Description = value
		case "MODEL":
			settings.Model = value
		case "TIMEZONE":
			settings.Timezone = value
		case "DATE_FORMAT":
			settings.DateFormat = value
		case "TIME_FORMAT":
			settings.TimeFormat = value
		case "SECURITY":
			settings.SecurityMode = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading system config: %w", err)
	}

	return settings, nil
}

// GetDockerSettings reads Docker settings from /boot/config/docker.cfg
func (c *ConfigCollector) GetDockerSettings() (*dto.DockerSettings, error) {
	configPath := "/boot/config/docker.cfg"
	logger.Debug("Config: Reading Docker settings from %s", configPath)

	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &dto.DockerSettings{
				Enabled:   false,
				Timestamp: time.Now(),
			}, nil
		}
		return nil, fmt.Errorf("failed to open Docker config: %w", err)
	}
	defer file.Close()

	settings := &dto.DockerSettings{
		Timestamp: time.Now(),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"`)

		switch key {
		case "DOCKER_ENABLED":
			settings.Enabled = (value == "yes" || value == "true" || value == "1")
		case "DOCKER_IMAGE_FILE":
			settings.ImagePath = value
		case "DOCKER_DEFAULT_NETWORK":
			settings.DefaultNetwork = value
		case "DOCKER_CUSTOM_NETWORKS":
			if value != "" {
				settings.CustomNetworks = strings.Split(value, ",")
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading Docker config: %w", err)
	}

	return settings, nil
}

// GetVMSettings reads VM settings from /boot/config/domain.cfg
func (c *ConfigCollector) GetVMSettings() (*dto.VMSettings, error) {
	configPath := "/boot/config/domain.cfg"
	logger.Debug("Config: Reading VM settings from %s", configPath)

	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &dto.VMSettings{
				Enabled:   false,
				Timestamp: time.Now(),
			}, nil
		}
		return nil, fmt.Errorf("failed to open VM config: %w", err)
	}
	defer file.Close()

	settings := &dto.VMSettings{
		DefaultSettings: make(map[string]string),
		Timestamp:       time.Now(),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"`)

		switch key {
		case "SERVICE":
			settings.Enabled = (value == "enable" || value == "enabled")
		case "PCI_DEVICES":
			if value != "" {
				settings.PCIDevices = strings.Split(value, ",")
			}
		case "USB_DEVICES":
			if value != "" {
				settings.USBDevices = strings.Split(value, ",")
			}
		default:
			// Store other settings in default settings map
			settings.DefaultSettings[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading VM config: %w", err)
	}

	return settings, nil
}

// UpdateShareConfig writes share configuration to /boot/config/shares/{name}.cfg
func (c *ConfigCollector) UpdateShareConfig(config *dto.ShareConfig) error {
	configPath := fmt.Sprintf("/boot/config/shares/%s.cfg", config.Name)
	logger.Info("Config: Writing share config to %s", configPath)

	// Create backup
	backupPath := configPath + ".bak"
	if _, err := os.Stat(configPath); err == nil {
		if err := os.Rename(configPath, backupPath); err != nil {
			logger.Error("Config: Failed to create backup: %v", err)
		}
	}

	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("failed to create share config: %w", err)
	}
	defer file.Close()

	// Write configuration
	if config.Comment != "" {
		fmt.Fprintf(file, "shareComment=\"%s\"\n", config.Comment)
	}
	if config.Allocator != "" {
		fmt.Fprintf(file, "shareAllocator=\"%s\"\n", config.Allocator)
	}
	if config.Floor != "" {
		fmt.Fprintf(file, "shareFloor=\"%s\"\n", config.Floor)
	}
	if config.SplitLevel != "" {
		fmt.Fprintf(file, "shareSplitLevel=\"%s\"\n", config.SplitLevel)
	}
	if len(config.IncludeDisks) > 0 {
		fmt.Fprintf(file, "shareInclude=\"%s\"\n", strings.Join(config.IncludeDisks, ","))
	}
	if len(config.ExcludeDisks) > 0 {
		fmt.Fprintf(file, "shareExclude=\"%s\"\n", strings.Join(config.ExcludeDisks, ","))
	}
	if config.UseCache != "" {
		fmt.Fprintf(file, "shareUseCache=\"%s\"\n", config.UseCache)
	}
	if config.Export != "" {
		fmt.Fprintf(file, "shareExport=\"%s\"\n", config.Export)
	}
	if config.Security != "" {
		fmt.Fprintf(file, "shareSecurity=\"%s\"\n", config.Security)
	}

	logger.Info("Config: Share config written successfully")
	return nil
}

// UpdateSystemSettings writes system settings to /boot/config/ident.cfg
func (c *ConfigCollector) UpdateSystemSettings(settings *dto.SystemSettings) error {
	configPath := "/boot/config/ident.cfg"
	logger.Info("Config: Writing system settings to %s", configPath)

	// Create backup
	backupPath := configPath + ".bak"
	if _, err := os.Stat(configPath); err == nil {
		if err := os.Rename(configPath, backupPath); err != nil {
			logger.Error("Config: Failed to create backup: %v", err)
		}
	}

	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("failed to create system config: %w", err)
	}
	defer file.Close()

	// Write configuration
	if settings.ServerName != "" {
		fmt.Fprintf(file, "NAME=\"%s\"\n", settings.ServerName)
	}
	if settings.Description != "" {
		fmt.Fprintf(file, "COMMENT=\"%s\"\n", settings.Description)
	}
	if settings.Model != "" {
		fmt.Fprintf(file, "MODEL=\"%s\"\n", settings.Model)
	}
	if settings.Timezone != "" {
		fmt.Fprintf(file, "TIMEZONE=\"%s\"\n", settings.Timezone)
	}
	if settings.DateFormat != "" {
		fmt.Fprintf(file, "DATE_FORMAT=\"%s\"\n", settings.DateFormat)
	}
	if settings.TimeFormat != "" {
		fmt.Fprintf(file, "TIME_FORMAT=\"%s\"\n", settings.TimeFormat)
	}
	if settings.SecurityMode != "" {
		fmt.Fprintf(file, "SECURITY=\"%s\"\n", settings.SecurityMode)
	}

	logger.Info("Config: System settings written successfully")
	return nil
}

// GetDiskSettings reads disk settings from /boot/config/disk.cfg
func (c *ConfigCollector) GetDiskSettings() (*dto.DiskSettings, error) {
	configPath := "/boot/config/disk.cfg"
	logger.Debug("Config: Reading disk settings from %s", configPath)

	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("disk config not found")
		}
		return nil, fmt.Errorf("failed to open disk config: %w", err)
	}
	defer file.Close()

	settings := &dto.DiskSettings{
		Timestamp: time.Now(),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"`)

		switch key {
		case "spindownDelay":
			if delay, err := strconv.Atoi(value); err == nil {
				settings.SpindownDelay = delay
			}
		case "startArray":
			settings.StartArray = (value == "yes" || value == "true" || value == "1")
		case "spinupGroups":
			settings.SpinupGroups = (value == "yes" || value == "true" || value == "1")
		case "shutdownTimeout":
			if timeout, err := strconv.Atoi(value); err == nil {
				settings.ShutdownTimeout = timeout
			}
		case "defaultFsType":
			settings.DefaultFsType = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading disk config: %w", err)
	}

	return settings, nil
}
