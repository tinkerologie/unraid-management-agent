package collectors

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// SettingsCollector collects extended settings information from Unraid configuration files
type SettingsCollector struct{}

// NewSettingsCollector creates a new settings collector
func NewSettingsCollector() *SettingsCollector {
	return &SettingsCollector{}
}

// GetDiskSettingsExtended reads extended disk settings including temperature thresholds
// This addresses Issue #45: Expose disk temperature thresholds from Unraid settings
func (c *SettingsCollector) GetDiskSettingsExtended() (*dto.DiskSettingsExtended, error) {
	settings := &dto.DiskSettingsExtended{
		// Set defaults matching Unraid's defaults
		HDDTempWarning:      45,
		HDDTempCritical:     55,
		SSDTempWarning:      60,
		SSDTempCritical:     70,
		WarningUtilization:  70,
		CriticalUtilization: 90,
		Timestamp:           time.Now(),
	}

	// Read disk.cfg for basic settings
	if err := c.parseDiskCfg(settings); err != nil {
		logger.Debug("Settings: Could not read disk.cfg: %v", err)
	}

	// Read dynamix.cfg for temperature thresholds
	if err := c.parseDynamixCfg(settings); err != nil {
		logger.Debug("Settings: Could not read dynamix.cfg: %v", err)
	}

	return settings, nil
}

// parseDiskCfg reads /boot/config/disk.cfg
func (c *SettingsCollector) parseDiskCfg(settings *dto.DiskSettingsExtended) error {
	file, err := os.Open(constants.DiskCfg)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

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

	return scanner.Err()
}

// parseDynamixCfg reads /boot/config/plugins/dynamix/dynamix.cfg for temperature thresholds
func (c *SettingsCollector) parseDynamixCfg(settings *dto.DiskSettingsExtended) error {
	file, err := os.Open(constants.DynamixCfg)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

	scanner := bufio.NewScanner(file)
	inDisplaySection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check for section headers
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.Trim(line, "[]")
			inDisplaySection = (section == "display")
			continue
		}

		if !inDisplaySection {
			continue
		}

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
		case "hot":
			// HDD warning temperature
			if temp, err := strconv.Atoi(value); err == nil {
				settings.HDDTempWarning = temp
			}
		case "max":
			// HDD critical temperature
			if temp, err := strconv.Atoi(value); err == nil {
				settings.HDDTempCritical = temp
			}
		case "hotssd":
			// SSD warning temperature
			if temp, err := strconv.Atoi(value); err == nil {
				settings.SSDTempWarning = temp
			}
		case "maxssd":
			// SSD critical temperature
			if temp, err := strconv.Atoi(value); err == nil {
				settings.SSDTempCritical = temp
			}
		case "warning":
			// Disk utilization warning
			if pct, err := strconv.Atoi(value); err == nil {
				settings.WarningUtilization = pct
			}
		case "critical":
			// Disk utilization critical
			if pct, err := strconv.Atoi(value); err == nil {
				settings.CriticalUtilization = pct
			}
		}
	}

	return scanner.Err()
}

// GetMoverSettings reads mover configuration and status
// This addresses Issue #48: Expose mover schedule and status via API
func (c *SettingsCollector) GetMoverSettings() (*dto.MoverSettings, error) {
	settings := &dto.MoverSettings{
		Timestamp: time.Now(),
	}

	// Read var.ini for mover status
	if err := c.parseMoverFromVarIni(settings); err != nil {
		logger.Debug("Settings: Could not read mover status from var.ini: %v", err)
	}

	return settings, nil
}

// parseMoverFromVarIni reads mover settings from /var/local/emhttp/var.ini
func (c *SettingsCollector) parseMoverFromVarIni(settings *dto.MoverSettings) error {
	file, err := os.Open(constants.VarIni)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

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
		case "shareMoverActive":
			settings.Active = (value == "yes" || value == "true" || value == "1")
		case "shareMoverSchedule":
			settings.Schedule = value
		case "shareMoverLogging":
			settings.Logging = (value == "yes" || value == "true" || value == "1")
		case "shareCacheFloor":
			if floor, err := strconv.Atoi(value); err == nil {
				settings.CacheFloor = floor
			}
		}
	}

	return scanner.Err()
}

// GetServiceStatus reads Docker and VM service enabled status
// This addresses Issue #49: Expose Docker/VM service enabled status via API
func (c *SettingsCollector) GetServiceStatus() (*dto.ServiceStatus, error) {
	status := &dto.ServiceStatus{
		Timestamp: time.Now(),
	}

	// Read Docker settings
	if err := c.parseDockerEnabled(status); err != nil {
		logger.Debug("Settings: Could not read docker.cfg: %v", err)
	}

	// Read VM Manager settings
	if err := c.parseVMEnabled(status); err != nil {
		logger.Debug("Settings: Could not read domain.cfg: %v", err)
	}

	return status, nil
}

// parseDockerEnabled reads Docker enabled status from /boot/config/docker.cfg
func (c *SettingsCollector) parseDockerEnabled(status *dto.ServiceStatus) error {
	file, err := os.Open(constants.DockerCfg)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

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
			status.DockerEnabled = (value == "yes" || value == "true" || value == "1")
		case "DOCKER_AUTOSTART":
			status.DockerAutostart = (value == "yes" || value == "true" || value == "1")
		}
	}

	// Docker autostart defaults to true if enabled
	if status.DockerEnabled && !status.DockerAutostart {
		status.DockerAutostart = true
	}

	return scanner.Err()
}

// parseVMEnabled reads VM Manager enabled status from /boot/config/domain.cfg
func (c *SettingsCollector) parseVMEnabled(status *dto.ServiceStatus) error {
	file, err := os.Open(constants.DomainCfg)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

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
			status.VMManagerEnabled = (value == "enable" || value == "enabled" || value == "yes")
		case "DISABLE":
			// DISABLE="yes" means VM manager is disabled
			if value == "yes" || value == "true" || value == "1" {
				status.VMManagerEnabled = false
			}
		}
	}

	return scanner.Err()
}

// GetParitySchedule reads parity check schedule configuration
// This addresses Issue #47: Expose parity check schedule and last results via API
func (c *SettingsCollector) GetParitySchedule() (*dto.ParitySchedule, error) {
	schedule := &dto.ParitySchedule{
		Mode:       "manual",
		Correcting: true,
		Timestamp:  time.Now(),
	}

	// Read dynamix.cfg [parity] section
	if err := c.parseParityScheduleFromDynamix(schedule); err != nil {
		logger.Debug("Settings: Could not read parity schedule from dynamix.cfg: %v", err)
	}

	// Read parity-check.cron for pause/resume times
	if err := c.parseParityCheckCron(schedule); err != nil {
		logger.Debug("Settings: Could not read parity-check.cron: %v", err)
	}

	return schedule, nil
}

// parseParityScheduleFromDynamix reads parity schedule from dynamix.cfg [parity] section
func (c *SettingsCollector) parseParityScheduleFromDynamix(schedule *dto.ParitySchedule) error {
	file, err := os.Open(constants.DynamixCfg)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

	scanner := bufio.NewScanner(file)
	inParitySection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check for section headers
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.Trim(line, "[]")
			inParitySection = (section == "parity")
			continue
		}

		if !inParitySection {
			continue
		}

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
		case "mode":
			// Mode: 0=disabled, 1=daily, 2=weekly, 3=monthly, 4=yearly, 5=custom
			switch value {
			case "0":
				schedule.Mode = "disabled"
			case "1":
				schedule.Mode = "daily"
			case "2":
				schedule.Mode = "weekly"
			case "3":
				schedule.Mode = "monthly"
			case "4":
				schedule.Mode = "yearly"
			case "5":
				schedule.Mode = "custom"
			default:
				schedule.Mode = "manual"
			}
		case "day":
			if day, err := strconv.Atoi(value); err == nil {
				schedule.Day = day
			}
		case "hour":
			// Hour can be "0 0" format (minute hour)
			parts := strings.Fields(value)
			if len(parts) >= 2 {
				if hour, err := strconv.Atoi(parts[1]); err == nil {
					schedule.Hour = hour
				}
			} else if len(parts) == 1 {
				if hour, err := strconv.Atoi(parts[0]); err == nil {
					schedule.Hour = hour
				}
			}
		case "dotm":
			if dotm, err := strconv.Atoi(value); err == nil {
				schedule.DayOfMonth = dotm
			}
		case "frequency":
			if freq, err := strconv.Atoi(value); err == nil {
				schedule.Frequency = freq
			}
		case "duration":
			if dur, err := strconv.Atoi(value); err == nil {
				schedule.Duration = dur
			}
		case "cumulative":
			schedule.Cumulative = (value == "1" || value == "yes" || value == "true")
		case "write":
			// write="" means non-correcting, write="CORRECT" means correcting
			schedule.Correcting = (value == "CORRECT" || value == "")
		}
	}

	return scanner.Err()
}

// parseParityCheckCron reads pause/resume times from parity-check.cron
func (c *SettingsCollector) parseParityCheckCron(schedule *dto.ParitySchedule) error {
	file, err := os.Open(constants.ParityCheckCron)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

	scanner := bufio.NewScanner(file)
	pauseRe := regexp.MustCompile(`^(\d+)\s+(\d+)\s+.*pause`)
	resumeRe := regexp.MustCompile(`^(\d+)\s+(\d+)\s+.*resume`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if matches := pauseRe.FindStringSubmatch(line); len(matches) == 3 {
			if hour, err := strconv.Atoi(matches[2]); err == nil {
				schedule.PauseHour = hour
			}
		}

		if matches := resumeRe.FindStringSubmatch(line); len(matches) == 3 {
			if hour, err := strconv.Atoi(matches[2]); err == nil {
				schedule.ResumeHour = hour
			}
		}
	}

	return scanner.Err()
}

// GetPluginList reads the list of installed plugins
// This addresses Issue #52: Expose installed plugins list via API
func (c *SettingsCollector) GetPluginList() (*dto.PluginList, error) {
	pluginList := &dto.PluginList{
		Plugins:   []dto.PluginInfo{},
		Timestamp: time.Now(),
	}

	// Get list of installed plugins from /boot/config/plugins/*.plg
	installedPlugins := make(map[string]dto.PluginInfo)

	files, err := filepath.Glob(filepath.Join(constants.PluginsConfigDir, "*.plg"))
	if err != nil {
		return pluginList, fmt.Errorf("failed to list plugins: %w", err)
	}

	for _, file := range files {
		plugin, err := c.parsePluginFile(file)
		if err != nil {
			logger.Debug("Settings: Could not parse plugin file %s: %v", file, err)
			continue
		}
		plugin.Enabled = true
		installedPlugins[plugin.Name] = plugin
	}

	// Check for updates in /tmp/plugins/*.plg
	updateFiles, err := filepath.Glob(filepath.Join(constants.PluginsTempDir, "*.plg"))
	if err == nil {
		for _, file := range updateFiles {
			updatePlugin, err := c.parsePluginFile(file)
			if err != nil {
				continue
			}

			// Check if we have this plugin installed
			if installed, ok := installedPlugins[updatePlugin.Name]; ok {
				if updatePlugin.Version != installed.Version {
					installed.UpdateAvailable = true
					installed.LatestVersion = updatePlugin.Version
					installedPlugins[updatePlugin.Name] = installed
					pluginList.UpdatesAvailable++
				}
			}
		}
	}

	// Convert map to slice
	for _, plugin := range installedPlugins {
		pluginList.Plugins = append(pluginList.Plugins, plugin)
	}

	pluginList.TotalCount = len(pluginList.Plugins)
	return pluginList, nil
}

// parsePluginFile parses a .plg XML file to extract plugin info
func (c *SettingsCollector) parsePluginFile(path string) (dto.PluginInfo, error) {
	plugin := dto.PluginInfo{}

	file, err := os.Open(path) //nolint:gosec // G304: path is from trusted plugin directory
	if err != nil {
		return plugin, err
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

	// Extract plugin name from filename
	basename := filepath.Base(path)
	plugin.Name = strings.TrimSuffix(basename, ".plg")

	scanner := bufio.NewScanner(file)
	entityRe := regexp.MustCompile(`<!ENTITY\s+(\w+)\s+"([^"]*)"`)

	for scanner.Scan() {
		line := scanner.Text()

		// Parse XML entities
		if matches := entityRe.FindStringSubmatch(line); len(matches) == 3 {
			key := matches[1]
			value := matches[2]

			switch key {
			case "name":
				plugin.Name = value
			case "version":
				plugin.Version = value
			case "author":
				plugin.Author = value
			case "pluginURL":
				plugin.URL = value
			case "support":
				plugin.SupportURL = value
			case "icon":
				plugin.Icon = value
			}
		}

		// Stop after DOCTYPE section (entities are defined there)
		if strings.Contains(line, "]>") {
			break
		}
	}

	return plugin, scanner.Err()
}

// GetUpdateStatus checks for Unraid OS and plugin updates
// This addresses Issue #50: Expose plugin and Unraid update availability via API
func (c *SettingsCollector) GetUpdateStatus() (*dto.UpdateStatus, error) {
	status := &dto.UpdateStatus{
		Timestamp: time.Now(),
	}

	// Get current Unraid version from var.ini
	if err := c.parseUnraidVersion(status); err != nil {
		logger.Debug("Settings: Could not read Unraid version: %v", err)
	}

	// Get plugin list with update info
	pluginList, err := c.GetPluginList()
	if err == nil {
		status.TotalPlugins = pluginList.TotalCount
		status.PluginUpdatesCount = pluginList.UpdatesAvailable

		// Collect plugins with updates
		for _, plugin := range pluginList.Plugins {
			if plugin.UpdateAvailable {
				status.PluginsWithUpdates = append(status.PluginsWithUpdates, plugin)
			}
		}
	}

	return status, nil
}

// parseUnraidVersion reads Unraid version from var.ini
func (c *SettingsCollector) parseUnraidVersion(status *dto.UpdateStatus) error {
	file, err := os.Open(constants.VarIni)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "version=") {
			status.CurrentVersion = strings.Trim(strings.TrimPrefix(line, "version="), `"`)
			break
		}
	}

	return scanner.Err()
}

// GetFlashDriveHealth reads USB flash drive health information
// This addresses Issue #51: Expose USB flash drive health via API
func (c *SettingsCollector) GetFlashDriveHealth() (*dto.FlashDriveHealth, error) {
	health := &dto.FlashDriveHealth{
		Device:    "/dev/sda",
		Timestamp: time.Now(),
	}

	// Read flash info from var.ini
	if err := c.parseFlashInfoFromVarIni(health); err != nil {
		logger.Debug("Settings: Could not read flash info from var.ini: %v", err)
	}

	// Get disk usage from /boot
	if err := c.getFlashDiskUsage(health); err != nil {
		logger.Debug("Settings: Could not get flash disk usage: %v", err)
	}

	return health, nil
}

// parseFlashInfoFromVarIni reads flash drive info from var.ini
func (c *SettingsCollector) parseFlashInfoFromVarIni(health *dto.FlashDriveHealth) error {
	file, err := os.Open(constants.VarIni)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

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
		case "flashGUID":
			health.GUID = value
		case "flashProduct":
			health.Model = value
		case "flashVendor":
			health.Vendor = value
		}
	}

	return scanner.Err()
}

// getFlashDiskUsage gets disk usage for /boot mount point
func (c *SettingsCollector) getFlashDiskUsage(health *dto.FlashDriveHealth) error {
	var stat os.FileInfo
	stat, err := os.Stat("/boot")
	if err != nil {
		return err
	}

	// Use syscall to get filesystem stats
	var fsstat syscall.Statfs_t
	if err := syscall.Statfs("/boot", &fsstat); err != nil {
		return err
	}

	_ = stat // Avoid unused variable warning

	//nolint:gosec // G115: Bsize is always positive on Linux systems
	health.SizeBytes = fsstat.Blocks * uint64(fsstat.Bsize)
	//nolint:gosec // G115: Bsize is always positive on Linux systems
	health.FreeBytes = fsstat.Bfree * uint64(fsstat.Bsize)
	health.UsedBytes = health.SizeBytes - health.FreeBytes

	if health.SizeBytes > 0 {
		health.UsagePercent = float64(health.UsedBytes) / float64(health.SizeBytes) * 100
	}

	return nil
}

// GetNetworkServicesStatus collects status of all network services
// This provides visibility into NFS, SMB, FTP, SSH, Telnet, Avahi, WireGuard, etc.
func (c *SettingsCollector) GetNetworkServicesStatus() (*dto.NetworkServicesStatus, error) {
	status := &dto.NetworkServicesStatus{
		Timestamp: time.Now(),
	}

	// Parse configuration from var.ini and ident.cfg
	varIniSettings := c.parseVarIniForNetworkServices()
	identSettings := c.parseIdentCfgForNetworkServices()
	tipsSettings := c.parseTipsTweaksCfg()

	// Merge settings (var.ini takes precedence as it's runtime state)
	mergedSettings := c.mergeNetworkSettings(varIniSettings, identSettings, tipsSettings)

	// SMB (Samba)
	status.SMB = dto.NetworkServiceInfo{
		Name:        "SMB",
		Enabled:     mergedSettings["shareSMBEnabled"] == "yes",
		Running:     c.isServiceRunning("smbd"),
		Port:        445,
		Description: "Windows/SMB file sharing",
	}

	// NFS
	status.NFS = dto.NetworkServiceInfo{
		Name:        "NFS",
		Enabled:     mergedSettings["shareNFSEnabled"] == "yes",
		Running:     c.isServiceRunning("nfsd") || c.isServiceRunning("rpc.nfsd"),
		Port:        2049,
		Description: "NFS file sharing",
	}

	// AFP (Apple Filing Protocol via netatalk - usually via Avahi)
	status.AFP = dto.NetworkServiceInfo{
		Name:        "AFP",
		Enabled:     false, // AFP is deprecated, rarely used
		Running:     c.isServiceRunning("netatalk") || c.isServiceRunning("afpd"),
		Port:        548,
		Description: "Apple Filing Protocol (legacy)",
	}

	// FTP
	ftpEnabled := mergedSettings["FTP"] == "yes" || mergedSettings["FTP_TELNET"] == "yes"
	status.FTP = dto.NetworkServiceInfo{
		Name:        "FTP",
		Enabled:     ftpEnabled,
		Running:     c.isServiceRunning("vsftpd") || c.isServiceRunning("proftpd"),
		Port:        21,
		Description: "FTP file transfer",
	}

	// SSH
	sshPort := 22
	if port, err := strconv.Atoi(mergedSettings["PORTSSH"]); err == nil && port > 0 {
		sshPort = port
	}
	status.SSH = dto.NetworkServiceInfo{
		Name:        "SSH",
		Enabled:     mergedSettings["USE_SSH"] == "yes",
		Running:     c.isServiceRunning("sshd"),
		Port:        sshPort,
		Description: "Secure Shell remote access",
	}

	// Telnet
	telnetPort := 23
	if port, err := strconv.Atoi(mergedSettings["PORTTELNET"]); err == nil && port > 0 {
		telnetPort = port
	}
	status.Telnet = dto.NetworkServiceInfo{
		Name:        "Telnet",
		Enabled:     mergedSettings["USE_TELNET"] == "yes",
		Running:     c.isServiceRunning("telnetd") || c.isServiceRunning("in.telnetd"),
		Port:        telnetPort,
		Description: "Telnet remote access (insecure)",
	}

	// Avahi (mDNS/DNS-SD)
	status.Avahi = dto.NetworkServiceInfo{
		Name:        "Avahi",
		Enabled:     mergedSettings["shareAvahiEnabled"] == "yes",
		Running:     c.isServiceRunning("avahi-daemon"),
		Port:        5353,
		Description: "mDNS/DNS-SD service discovery",
	}

	// NetBIOS
	status.NetBIOS = dto.NetworkServiceInfo{
		Name:        "NetBIOS",
		Enabled:     mergedSettings["USE_NETBIOS"] == "yes",
		Running:     c.isServiceRunning("nmbd"),
		Port:        137,
		Description: "NetBIOS name service",
	}

	// WSD (Web Services Discovery)
	status.WSD = dto.NetworkServiceInfo{
		Name:        "WSD",
		Enabled:     mergedSettings["USE_WSD"] == "yes",
		Running:     c.isServiceRunning("wsdd") || c.isServiceRunning("wsdd2"),
		Port:        3702,
		Description: "Web Services Discovery for Windows",
	}

	// WireGuard VPN
	status.WireGuard = dto.NetworkServiceInfo{
		Name:        "WireGuard",
		Enabled:     c.isWireGuardConfigured(),
		Running:     c.isWireGuardRunning(),
		Port:        51820, // Default WireGuard port
		Description: "WireGuard VPN",
	}

	// UPnP
	status.UPNP = dto.NetworkServiceInfo{
		Name:        "UPnP",
		Enabled:     mergedSettings["USE_UPNP"] == "yes",
		Running:     c.isServiceRunning("upnpd") || c.isServiceRunning("miniupnpd"),
		Port:        1900,
		Description: "UPnP/IGD port forwarding",
	}

	// NTP
	status.NTP = dto.NetworkServiceInfo{
		Name:        "NTP",
		Enabled:     mergedSettings["USE_NTP"] == "yes",
		Running:     c.isServiceRunning("ntpd") || c.isServiceRunning("chronyd"),
		Port:        123,
		Description: "Network Time Protocol",
	}

	// Syslog Server (remote syslog receiver)
	status.SyslogServer = dto.NetworkServiceInfo{
		Name:        "Syslog",
		Enabled:     c.isSyslogServerEnabled(),
		Running:     c.isServiceRunning("rsyslogd"),
		Port:        514,
		Description: "Syslog daemon",
	}

	// Calculate totals
	services := []dto.NetworkServiceInfo{
		status.SMB, status.NFS, status.AFP, status.FTP,
		status.SSH, status.Telnet, status.Avahi, status.NetBIOS,
		status.WSD, status.WireGuard, status.UPNP, status.NTP, status.SyslogServer,
	}

	status.TotalServices = len(services)
	for _, svc := range services {
		if svc.Enabled {
			status.EnabledServices++
		}
		if svc.Running {
			status.RunningServices++
		}
	}

	return status, nil
}

// parseVarIniForNetworkServices parses network service settings from var.ini
func (c *SettingsCollector) parseVarIniForNetworkServices() map[string]string {
	settings := make(map[string]string)

	file, err := os.Open(constants.VarIni)
	if err != nil {
		logger.Debug("Settings: Could not open var.ini for network services: %v", err)
		return settings
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

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
		settings[key] = value
	}

	return settings
}

// parseIdentCfgForNetworkServices parses network service settings from ident.cfg
func (c *SettingsCollector) parseIdentCfgForNetworkServices() map[string]string {
	settings := make(map[string]string)

	file, err := os.Open(constants.IdentCfg)
	if err != nil {
		logger.Debug("Settings: Could not open ident.cfg for network services: %v", err)
		return settings
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

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
		settings[key] = value
	}

	return settings
}

// parseTipsTweaksCfg parses FTP settings from tips.and.tweaks.cfg
func (c *SettingsCollector) parseTipsTweaksCfg() map[string]string {
	settings := make(map[string]string)

	tipsConfig := "/boot/config/plugins/tips.and.tweaks/tips.and.tweaks.cfg"
	file, err := os.Open(tipsConfig)
	if err != nil {
		logger.Debug("Settings: Could not open tips.and.tweaks.cfg: %v", err)
		return settings
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

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
		settings[key] = value
	}

	return settings
}

// mergeNetworkSettings merges settings from multiple sources
func (c *SettingsCollector) mergeNetworkSettings(varIni, identCfg, tips map[string]string) map[string]string {
	merged := make(map[string]string)

	// Start with ident.cfg
	for k, v := range identCfg {
		merged[k] = v
	}

	// Overlay tips.and.tweaks.cfg
	for k, v := range tips {
		merged[k] = v
	}

	// Overlay var.ini (runtime state takes precedence)
	for k, v := range varIni {
		merged[k] = v
	}

	return merged
}

// isServiceRunning checks if a service is running by looking at /proc
func (c *SettingsCollector) isServiceRunning(processName string) bool {
	// Read /proc to find running processes
	procDir, err := os.Open("/proc")
	if err != nil {
		return false
	}
	defer procDir.Close() //nolint:errcheck

	entries, err := procDir.Readdirnames(-1)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		// Only look at numeric directories (PIDs)
		if _, err := strconv.Atoi(entry); err != nil {
			continue
		}

		commPath := filepath.Join("/proc", entry, "comm")
		commBytes, err := os.ReadFile(commPath) //nolint:gosec // G304: path is constructed from /proc
		if err != nil {
			continue
		}

		comm := strings.TrimSpace(string(commBytes))
		if comm == processName {
			return true
		}
	}

	return false
}

// isWireGuardConfigured checks if WireGuard is configured
func (c *SettingsCollector) isWireGuardConfigured() bool {
	// Check for WireGuard config files
	wgConfigDir := "/boot/config/wireguard"
	files, err := os.ReadDir(wgConfigDir)
	if err != nil {
		return false
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".conf") {
			return true
		}
	}

	return false
}

// isWireGuardRunning checks if any WireGuard tunnels are active
func (c *SettingsCollector) isWireGuardRunning() bool {
	// Check for wg interfaces
	interfaces, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return false
	}

	for _, iface := range interfaces {
		name := iface.Name()
		// WireGuard interfaces are typically named wg0, wg1, etc.
		if strings.HasPrefix(name, "wg") {
			return true
		}
	}

	return false
}

// isSyslogServerEnabled checks if syslog server is enabled to receive remote logs
func (c *SettingsCollector) isSyslogServerEnabled() bool {
	// Check rsyslog.conf for UDP/TCP listener
	rsyslogConf := "/etc/rsyslog.conf"
	content, err := os.ReadFile(rsyslogConf)
	if err != nil {
		return false
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}
		// Check for module load and server run directives
		if strings.Contains(line, "$ModLoad imudp") ||
			strings.Contains(line, "$ModLoad imtcp") ||
			strings.Contains(line, "module(load=\"imudp\"") ||
			strings.Contains(line, "module(load=\"imtcp\"") {
			return true
		}
	}

	return false
}
