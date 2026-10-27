// Package main is the entry point for the Unraid Management Agent.
// It provides a REST API and WebSocket interface for monitoring and controlling Unraid systems.
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/alecthomas/kong"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/tinkerologie/unraid-management-agent/daemon/cmd"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"github.com/tinkerologie/unraid-management-agent/daemon/platform"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/collectors"
)

// Version is the application version, set at build time via ldflags.
var Version = "dev"

// validCollectorNames contains all valid collector names for validation
var validCollectorNames = map[string]bool{
	"system":          true,
	"array":           true,
	"disk":            true,
	"docker":          true,
	"vm":              true,
	"ups":             true,
	"nut":             true,
	"gpu":             true,
	"shares":          true,
	"network":         true,
	"hardware":        true,
	"zfs":             true,
	"notification":    true,
	"registration":    true,
	"unassigned":      true,
	"fancontrol":      true,
	"tuning":          true,
	"docker_update":   true,
	"docker_networks": true,
	"plugin_update":   true,
	"os_update":       true,
	"mover":           true,

	"storage_topology": true,
	"nic":              true,
	"psu":              true,
}

var cli struct {
	LogsDir     string `default:"/var/log" help:"directory to store logs"`
	Port        int    `default:"8043" help:"HTTP server port"`
	BindAddress string `default:"" env:"BIND_ADDRESS" help:"IP address to bind the HTTP server to (empty = all interfaces)"`
	Debug       bool   `default:"false" help:"enable debug mode with stdout logging"`
	LogLevel    string `default:"info" help:"log level: debug, info, warning, error"`

	// Read-only mode - blocks all state-changing MCP tools (REST API unaffected)
	ReadOnly bool `default:"false" env:"READ_ONLY" help:"block all state-changing MCP tools so AI agents can only consume data"`

	// Per-tool MCP access policy - comma-separated list of tool=policy entries (e.g. array_action=hidden,system_reboot=ask)
	ToolPolicy string `default:"" env:"TOOL_POLICY" help:"comma-separated list of tool=policy entries (e.g. array_action=hidden,system_reboot=ask)"`

	// CORS
	CORSOrigin string `default:"*" env:"CORS_ORIGIN" help:"Access-Control-Allow-Origin value (default: *)"`

	// API token - when set, requests must present "Authorization: Bearer <token>",
	// except the health endpoint and the Swagger UI. Prefer API_TOKEN or the config
	// file over the flag: command-line arguments are visible in the process list.
	APIToken string `default:"" env:"API_TOKEN" help:"require bearer-token auth on the HTTP API and /mcp; the health endpoint and Swagger UI stay open (empty = unauthenticated reads, network writes refused). Prefer the API_TOKEN env var or config file: flags are visible in the process list"`

	// AllowUnauthenticatedWrites restores tokenless network write access. Without
	// it, an empty API token leaves REST and HTTP MCP read-only (#245).
	AllowUnauthenticatedWrites bool `default:"false" env:"ALLOW_UNAUTHENTICATED_WRITES" help:"allow state-changing REST requests and MCP tools over the network when API_TOKEN is empty (not recommended)"`

	// WANIPLookup asks external IP services for the WAN IP when network access
	// URLs are requested (#252).
	WANIPLookup bool `name:"wan-ip-lookup" default:"true" env:"WAN_IP_LOOKUP" help:"ask api.ipify.org, ifconfig.me and icanhazip.com for the WAN IP when network access URLs are requested (false: use only Unraid's configured WANIP)"`

	// VMGuestAgent lists VMs to read through the QEMU guest agent (GPU, disk
	// and filesystem stats, custom sensors).
	VMGuestAgent string `name:"vm-guest-agent" default:"" env:"VM_GUEST_AGENT" help:"comma-separated VM names to read through the QEMU guest agent (GPU, disk and filesystem stats); overrides vm_guest_agent.vms in config.yml"`

	// MCPConnectSecret enables an optional secret-bearing MCP path (/mcp/<secret>)
	// for URL-only MCP clients (ha-mcp style onboarding) without exposing full REST API access.
	MCPConnectSecret string `default:"" env:"MCP_CONNECT_SECRET" help:"optional URL-safe secret (>=32 chars) enabling the /mcp/<secret> connect URL without bearer headers"`

	// TLS: serve HTTPS (incl. the /mcp endpoint) when both files are provided
	TLSCertFile string `default:"" env:"TLS_CERT_FILE" help:"path to a PEM TLS certificate file (enables HTTPS when set with --tls-key-file)"`
	TLSKeyFile  string `default:"" env:"TLS_KEY_FILE" help:"path to a PEM TLS private key file (enables HTTPS when set with --tls-cert-file)"`

	// Low power mode - multiplies all intervals for resource-constrained systems
	LowPowerMode bool `default:"false" env:"UNRAID_LOW_POWER" help:"enable low power mode (4x longer intervals for old/slow hardware)"`

	// Collector disable flag (alternative to setting interval=0)
	DisableCollectors string `default:"" env:"UNRAID_DISABLE_COLLECTORS" help:"comma-separated list of collectors to disable (e.g., gpu,ups,zfs)"`

	// Hardware-gated collectors in automatic mode: they run only while their
	// hardware is detected (#261). Set by the plugin for intervals set to "auto".
	AutoCollectors string `default:"" env:"UNRAID_AUTO_COLLECTORS" help:"comma-separated hardware-gated collectors in automatic mode (psu,nic,storage_topology,gpu,ups,nut,zfs): run at 300s only while their hardware is detected"`

	// Collection mode (#280): dynamic lets the agent decide when collectors
	// run; manual uses the configured intervals.
	CollectionMode string `default:"manual" enum:"dynamic,manual" env:"COLLECTION_MODE" help:"collection mode: dynamic (event-driven and demand-aware, INTERVAL_* unused) or manual (configured intervals)"`

	// MQTT Configuration
	MQTTEnabled            bool   `default:"false" env:"MQTT_ENABLED" help:"enable MQTT publishing"`
	MQTTBroker             string `default:"" env:"MQTT_BROKER" help:"MQTT broker hostname or IP"`
	MQTTPort               int    `default:"1883" env:"MQTT_PORT" help:"MQTT broker port"`
	MQTTUsername           string `default:"" env:"MQTT_USERNAME" help:"MQTT username"`
	MQTTPassword           string `default:"" env:"MQTT_PASSWORD" help:"MQTT password"`
	MQTTClientID           string `default:"unraid-management-agent" env:"MQTT_CLIENT_ID" help:"MQTT client ID; the default is sent as unraid-management-agent-<hostname>"`
	MQTTTopicPrefix        string `default:"unraid" env:"MQTT_TOPIC_PREFIX" help:"MQTT topic prefix"`
	MQTTUseTLS             bool   `default:"false" env:"MQTT_USE_TLS" help:"use TLS for MQTT connection"`
	MQTTInsecureSkipVerify bool   `default:"false" env:"MQTT_INSECURE_SKIP_VERIFY" help:"skip TLS certificate verification"`
	MQTTQoS                int    `default:"0" env:"MQTT_QOS" help:"MQTT QoS level (0, 1, or 2)"`
	MQTTRetain             bool   `default:"true" env:"MQTT_RETAIN" help:"retain MQTT messages"`
	MQTTHomeAssistant      bool   `default:"false" env:"MQTT_HOME_ASSISTANT" help:"enable Home Assistant MQTT discovery"`
	MQTTHAPrefix           string `default:"homeassistant" env:"MQTT_HA_PREFIX" help:"Home Assistant discovery prefix"`

	// Discovery (zeroconf/mDNS) Configuration
	DiscoveryEnabled     bool   `default:"true" env:"DISCOVERY_ENABLED" help:"advertise the agent on the local network via mDNS for auto-discovery"`
	DiscoveryServiceName string `default:"" env:"DISCOVERY_SERVICE_NAME" help:"override the advertised mDNS instance name (default: system hostname)"`

	// Collection intervals (overridable via environment variables)
	// Use 0 to disable a collector completely
	// Maximum interval: 86400 seconds (24 hours)
	IntervalSystem         int  `default:"15" env:"INTERVAL_SYSTEM" help:"system metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalArray          int  `default:"60" env:"INTERVAL_ARRAY" help:"array metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalDisk           int  `default:"300" env:"INTERVAL_DISK" help:"disk metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalDocker         int  `default:"30" env:"INTERVAL_DOCKER" help:"docker metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalVM             int  `default:"60" env:"INTERVAL_VM" help:"VM metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalUPS            int  `default:"60" env:"INTERVAL_UPS" help:"UPS metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalNUT            int  `default:"0" env:"INTERVAL_NUT" help:"NUT plugin metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalGPU            int  `default:"60" env:"INTERVAL_GPU" help:"GPU metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalShares         int  `default:"60" env:"INTERVAL_SHARES" help:"shares metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalNetwork        int  `default:"60" env:"INTERVAL_NETWORK" help:"network metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalHardware       int  `default:"600" env:"INTERVAL_HARDWARE" help:"hardware metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalZFS            int  `default:"300" env:"INTERVAL_ZFS" help:"ZFS metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalNotification   int  `default:"30" env:"INTERVAL_NOTIFICATION" help:"notification interval (seconds, 0=disabled, max 86400)"`
	IntervalRegistration   int  `default:"600" env:"INTERVAL_REGISTRATION" help:"registration interval (seconds, 0=disabled, max 86400)"`
	IntervalUnassigned     int  `default:"60" env:"INTERVAL_UNASSIGNED" help:"unassigned devices interval (seconds, 0=disabled, max 86400)"`
	IntervalFanControl     int  `default:"60" env:"INTERVAL_FAN_CONTROL" help:"fan control interval (seconds, 0=disabled, max 86400)"`
	IntervalTuning         int  `default:"120" env:"INTERVAL_TUNING" help:"tuning parameters interval (seconds, 0=disabled, max 86400)"`
	IntervalDockerUpdate   int  `default:"21600" env:"INTERVAL_DOCKER_UPDATE" help:"container update check interval (seconds, 0=disabled, max 86400)"`
	IntervalDockerNetworks int  `default:"60" env:"INTERVAL_DOCKER_NETWORKS" help:"docker networks listing interval (seconds, 0=disabled, max 86400)"`
	IntervalPluginUpdate   int  `default:"3600" env:"INTERVAL_PLUGIN_UPDATE" help:"plugin update check interval (seconds, 0=disabled, max 86400)"`
	IntervalOSUpdate       int  `default:"86400" env:"INTERVAL_OS_UPDATE" help:"OS update availability check interval (seconds, 0=disabled, max 86400)"`
	IntervalMover          int  `default:"30" env:"INTERVAL_MOVER" help:"mover status collection interval (seconds, 0=disabled, max 86400)"`
	DockerUpdateNotify     bool `default:"false" env:"DOCKER_UPDATE_NOTIFY" help:"raise an Unraid notification when new container updates become available"`

	IntervalPSU             int `default:"60" env:"INTERVAL_PSU" help:"PSU telemetry interval (seconds, 0=disabled, max 86400); monitors detected supported USB PSUs"`
	IntervalStorageTopology int `default:"0" env:"INTERVAL_STORAGE_TOPOLOGY" help:"SAS storage topology (storcli/sg_ses) collection interval (seconds, 0=disabled (default), max 86400; 300 recommended when enabled)"`
	IntervalNIC             int `default:"0" env:"INTERVAL_NIC" help:"Network card (temperatures, optic diagnostics) collection interval (seconds, 0=disabled (default), max 86400; 60 recommended when enabled)"`

	Boot        cmd.Boot        `cmd:"" default:"1" help:"start the management agent"`
	MCPStdio    cmd.MCPStdio    `cmd:"mcp-stdio" help:"run MCP server over stdin/stdout for local AI clients"`
	Diagnostics cmd.Diagnostics `cmd:"diagnostics" help:"collect and download diagnostic bundle"`
}

// cleanupOldLogs removes old rotated log files from previous versions
// This is needed because lumberjack's MaxBackups only prevents new backups,
// it doesn't clean up existing ones from before the setting was changed
func cleanupOldLogs(logsDir, baseName string) {
	pattern := filepath.Join(logsDir, baseName+"-*.log")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return
	}
	for _, f := range files {
		_ = os.Remove(f)
	}
}

func main() {
	parser, ctx, err := parseCLI(os.Args[1:])
	parser.FatalIfErrorf(err)

	// Detect STDIO mode — stdout is reserved for MCP JSON-RPC
	isStdio := ctx.Command() == "mcp-stdio"

	// Load config file (defaults; CLI/env override)
	fileCfg, err := domain.LoadConfigFile(domain.DefaultConfigPath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "WARNING: Failed to load config file: %v\n", err)
	}
	applyFileConfig(fileCfg)

	// Set log level based on CLI flag
	switch strings.ToLower(cli.LogLevel) {
	case "debug":
		logger.SetLevel(logger.LevelDebug)
	case "info":
		logger.SetLevel(logger.LevelInfo)
	case "warning", "warn":
		logger.SetLevel(logger.LevelWarning)
	case "error":
		logger.SetLevel(logger.LevelError)
	default:
		logger.SetLevel(logger.LevelInfo)
	}

	// Set up logging
	if isStdio {
		// STDIO mode: stdout is reserved for MCP JSON-RPC protocol.
		// Log to file + stderr so MCP communication is not corrupted.
		cleanupOldLogs(cli.LogsDir, "unraid-management-agent")

		fileLogger := &lumberjack.Logger{
			Filename:   filepath.Join(cli.LogsDir, "unraid-management-agent.log"),
			MaxSize:    5,
			MaxBackups: 1,
			MaxAge:     1,
			Compress:   false,
		}
		multiWriter := io.MultiWriter(fileLogger, os.Stderr)
		log.SetOutput(multiWriter)
	} else if cli.Debug {
		// Debug mode: direct stdout/stderr with no buffering
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		logger.SetLevel(logger.LevelDebug)
		log.Println("Debug mode enabled - logging to stdout")
	} else {
		// Clean up old rotated log files from previous versions
		cleanupOldLogs(cli.LogsDir, "unraid-management-agent")

		// Production mode: log rotation with 5MB max size
		// MaxAge=1 ensures old backup files are deleted after 1 day
		fileLogger := &lumberjack.Logger{
			Filename:   filepath.Join(cli.LogsDir, "unraid-management-agent.log"),
			MaxSize:    5,     // 5 MB max file size
			MaxBackups: 1,     // Keep only 1 backup file
			MaxAge:     1,     // Delete backups older than 1 day
			Compress:   false, // No compression
		}
		// Write to both file and stdout
		multiWriter := io.MultiWriter(fileLogger, os.Stdout)
		log.SetOutput(multiWriter)
	}

	log.Printf("Starting Unraid Management Agent v%s (log level: %s)", Version, cli.LogLevel)

	// Validate the bind address. Fall back to all interfaces rather than
	// refusing to start, so a stale config value (e.g. after a VLAN change)
	// can never make the agent unreachable.
	if cli.BindAddress != "" {
		if err := lib.ValidateBindAddress(cli.BindAddress); err != nil {
			logger.Warning("%v; falling back to all interfaces", err)
			cli.BindAddress = ""
		} else {
			logger.Info("HTTP server will bind to %s only", cli.BindAddress)
		}
	}

	// Validate TLS configuration. A bad or half-configured cert/key pair falls
	// back to plain HTTP rather than refusing to start, mirroring the bind
	// address behaviour, so a stale cert path can never make the agent
	// unreachable. ValidateTLSConfig treats an empty pair as "TLS disabled".
	if err := lib.ValidateTLSConfig(cli.TLSCertFile, cli.TLSKeyFile); err != nil {
		logger.Warning("TLS is configured but invalid (%v); falling back to plain HTTP — the server will NOT be encrypted. Fix the certificate/key paths or remove the TLS settings.", err)
		cli.TLSCertFile = ""
		cli.TLSKeyFile = ""
	} else if cli.TLSCertFile != "" && cli.TLSKeyFile != "" {
		logger.Info("HTTPS enabled (certificate: %s)", cli.TLSCertFile)
	}

	// Validate the API token. Unlike the TLS fallback above, an invalid token is
	// a hard error: silently continuing without authentication would leave the
	// API open on a server the operator believes is protected. Availability is
	// the right trade for TLS, but not for a credential.
	if err := lib.ValidateAPIToken(cli.APIToken); err != nil {
		logger.Error("Invalid API token configuration: %v", err)
		os.Exit(1)
	}
	if cli.APIToken != "" {
		logger.Info("API authentication enabled: requests require a bearer token (health endpoint and Swagger UI remain open)")
	}

	// Validate the optional MCP connect secret. An invalid secret disables the
	// secret connect URL and logs a warning without exposing the secret value,
	// while keeping the agent and standard /mcp endpoint available.
	if err := lib.ValidateMCPConnectSecret(cli.MCPConnectSecret); err != nil {
		logger.Warning("Invalid MCP connect secret configuration (%v); disabling /mcp/<secret> connect URL", err)
		cli.MCPConnectSecret = ""
	} else if cli.MCPConnectSecret != "" {
		if cli.APIToken == "" {
			logger.Warning("MCP connect secret is configured, but API_TOKEN is empty: /mcp and REST endpoints remain readable without authentication")
		} else {
			logger.Info("MCP connect URL enabled (/mcp/<redacted>)")
		}
	}

	if cli.ReadOnly {
		logger.Info("Read-only mode enabled: all state-changing MCP tools are blocked")
	}

	// Parse disabled collectors from CLI/env and create a map
	disabledCollectors := make(map[string]bool)
	if cli.DisableCollectors != "" {
		for name := range strings.SplitSeq(cli.DisableCollectors, ",") {
			name = strings.TrimSpace(strings.ToLower(name))
			if name == "" {
				continue
			}
			if name == "system" {
				log.Printf("WARNING: Cannot disable system collector (always required), ignoring")
				continue
			}
			if !validCollectorNames[name] {
				log.Printf("WARNING: Unknown collector name '%s' in disable list, ignoring", name)
				continue
			}
			disabledCollectors[name] = true
			log.Printf("Collector '%s' disabled via UNRAID_DISABLE_COLLECTORS", name)
		}
	}

	autoCollectors := resolveAutoCollectors(cli.AutoCollectors, disabledCollectors, map[string]int{
		"nic":              cli.IntervalNIC,
		"psu":              cli.IntervalPSU,
		"storage_topology": cli.IntervalStorageTopology,
		"gpu":              cli.IntervalGPU,
		"ups":              cli.IntervalUPS,
		"nut":              cli.IntervalNUT,
		"zfs":              cli.IntervalZFS,
	}, fileConfigIntervals)
	gatedCollectors := hardwareGatedCollectors(disabledCollectors)

	// Helper function to get interval (returns 0 if collector is disabled)
	// In low power mode, intervals are multiplied by 4 for reduced CPU usage
	getInterval := func(name string, cliInterval int) int {
		if disabledCollectors[name] {
			return 0
		}
		if cli.LowPowerMode && cliInterval > 0 {
			return cliInterval * 4
		}
		return cliInterval
	}

	if cli.LowPowerMode {
		log.Printf("Low power mode enabled - all intervals multiplied by 4x")
	}
	log.Printf("Collection mode: %s", cli.CollectionMode)

	// Create diagnostic logger for structured JSON logging
	diagLogger := logger.NewDiagnosticLogger(
		filepath.Join(cli.LogsDir, "diagnostic.jsonl"),
		"unraid-management-agent",
	)

	// Parse per-tool MCP access policies
	toolPolicy := make(map[string]domain.ToolPolicyValue)
	if fileCfg != nil && fileCfg.ToolPolicy != nil {
		for k, v := range fileCfg.ToolPolicy {
			if err := lib.ValidateToolPolicyValue(v); err != nil {
				log.Printf("WARNING: Invalid tool policy %q for tool %q in config.yml, falling back to default: %v", v, k, err)
				continue
			}
			if v != "" && v != "default" {
				toolPolicy[k] = domain.ToolPolicyValue(v)
			}
		}
	}
	if cli.ToolPolicy != "" {
		for k, v := range parseToolPolicyCSV(cli.ToolPolicy) {
			if v == domain.PolicyDefault {
				delete(toolPolicy, k)
			} else {
				toolPolicy[k] = v
			}
		}
	}

	// Create application context with intervals from CLI/env
	appCtx := &domain.Context{
		Version:          Version,
		Port:             cli.Port,
		BindAddress:      cli.BindAddress,
		CORSOrigin:       cli.CORSOrigin,
		APIToken:         cli.APIToken,
		MCPConnectSecret: cli.MCPConnectSecret,
		ReadOnly:         cli.ReadOnly,
		TLSCertFile:      cli.TLSCertFile,
		TLSKeyFile:       cli.TLSKeyFile,
		ToolPolicy:       toolPolicy,
		VMGuest:          buildVMGuestConfig(cli.VMGuestAgent, vmGuestFileConfig(fileCfg)),
		Hub:              domain.NewEventBus(1024), // Buffer size for event bus
		Platform:         platform.NewRegistry(),
		MQTTConfig: domain.MQTTConfig{
			Enabled:             cli.MQTTEnabled,
			Broker:              cli.MQTTBroker,
			Port:                cli.MQTTPort,
			Username:            cli.MQTTUsername,
			Password:            cli.MQTTPassword,
			ClientID:            cli.MQTTClientID,
			UseTLS:              cli.MQTTUseTLS,
			InsecureSkipVerify:  cli.MQTTInsecureSkipVerify,
			TopicPrefix:         cli.MQTTTopicPrefix,
			QoS:                 cli.MQTTQoS,
			RetainMessages:      cli.MQTTRetain,
			HomeAssistantMode:   cli.MQTTHomeAssistant,
			HomeAssistantPrefix: cli.MQTTHAPrefix,
			DiscoveryEnabled:    cli.MQTTHomeAssistant, // Enable discovery when HA mode is enabled
		},
		DiscoveryConfig: domain.DiscoveryConfig{
			Enabled:     cli.DiscoveryEnabled,
			ServiceName: cli.DiscoveryServiceName,
		},
		DiagnosticLogger:   diagLogger,
		LogsDir:            cli.LogsDir,
		DockerUpdateNotify: cli.DockerUpdateNotify,
		GatedCollectors:    gatedCollectors,
		AutoCollectors:     autoCollectors,
		DisabledCollectors: disabledCollectors,
		CollectionMode:     cli.CollectionMode,
		LowPowerMode:       cli.LowPowerMode,
		Intervals: domain.Intervals{
			System:         getInterval("system", cli.IntervalSystem),
			Array:          getInterval("array", cli.IntervalArray),
			Disk:           getInterval("disk", cli.IntervalDisk),
			Docker:         getInterval("docker", cli.IntervalDocker),
			VM:             getInterval("vm", cli.IntervalVM),
			UPS:            getInterval("ups", cli.IntervalUPS),
			NUT:            getInterval("nut", cli.IntervalNUT),
			GPU:            getInterval("gpu", cli.IntervalGPU),
			Shares:         getInterval("shares", cli.IntervalShares),
			Network:        getInterval("network", cli.IntervalNetwork),
			Hardware:       getInterval("hardware", cli.IntervalHardware),
			ZFS:            getInterval("zfs", cli.IntervalZFS),
			Notification:   getInterval("notification", cli.IntervalNotification),
			Registration:   getInterval("registration", cli.IntervalRegistration),
			Unassigned:     getInterval("unassigned", cli.IntervalUnassigned),
			FanControl:     getInterval("fancontrol", cli.IntervalFanControl),
			Tuning:         getInterval("tuning", cli.IntervalTuning),
			DockerUpdate:   getInterval("docker_update", cli.IntervalDockerUpdate),
			DockerNetworks: getInterval("docker_networks", cli.IntervalDockerNetworks),
			PluginUpdate:   getInterval("plugin_update", cli.IntervalPluginUpdate),
			OSUpdate:       getInterval("os_update", cli.IntervalOSUpdate),
			Mover:          getInterval("mover", cli.IntervalMover),

			StorageTopology: getInterval("storage_topology", cli.IntervalStorageTopology),
			NIC:             getInterval("nic", cli.IntervalNIC),
			PSU:             getInterval("psu", cli.IntervalPSU),
		},
	}
	applyWriteAccess(appCtx, cli.APIToken, cli.AllowUnauthenticatedWrites)
	applyWANIPLookup(appCtx, cli.WANIPLookup)

	// Warn about insecure MQTT TLS configuration
	if appCtx.MQTTConfig.InsecureSkipVerify && appCtx.MQTTConfig.Enabled {
		logger.Warning("MQTT TLS certificate verification is disabled (InsecureSkipVerify). This is insecure and should only be used for testing.")
	}

	// Run the boot command
	err = ctx.Run(appCtx)
	ctx.FatalIfErrorf(err)
}

// applyWriteAccess copies the write opt-out into the application context and
// logs the warning for an empty API token (#245).
func applyWriteAccess(appCtx *domain.Context, apiToken string, allowUnauthenticatedWrites bool) {
	appCtx.AllowUnauthenticatedWrites = allowUnauthenticatedWrites
	if msg := writeAccessWarning(apiToken, allowUnauthenticatedWrites); msg != "" {
		logger.Warning("%s", msg)
	}
}

// applyWANIPLookup copies the WAN IP lookup setting into the application
// context and logs when the external lookup is off (#252).
func applyWANIPLookup(appCtx *domain.Context, enabled bool) {
	appCtx.WANIPLookup = enabled
	if !enabled {
		logger.Info("WAN IP lookup disabled: network access URLs use only Unraid's configured WANIP")
	}
}

// writeAccessWarning returns the startup warning for an empty API token, or ""
// when a token is set (#245).
func writeAccessWarning(apiToken string, allowUnauthenticatedWrites bool) string {
	if strings.TrimSpace(apiToken) != "" {
		return ""
	}
	if allowUnauthenticatedWrites {
		return "API_TOKEN is empty and ALLOW_UNAUTHENTICATED_WRITES is enabled: anyone on the network can control this server. Set an API token and disable this option"
	}
	return "API_TOKEN is empty: network write access is disabled (REST and HTTP MCP are read-only) until an API token is set"
}

// explicitFlags holds the addresses of CLI fields set on the command line or
// through a non-empty environment variable. applyFileConfig leaves them alone,
// so precedence is CLI flag > env var > config file > default (#257).
var explicitFlags map[uintptr]bool

// recordExplicitFlags fills explicitFlags from a parse. An empty environment
// variable does not count: the plugin's start script exports empty values
// (e.g. API_TOKEN=""), which must not discard a config file setting.
func recordExplicitFlags(ctx *kong.Context) {
	explicitFlags = make(map[uintptr]bool)
	onCommandLine := make(map[*kong.Flag]bool)
	for _, p := range ctx.Path {
		if p.Flag != nil {
			onCommandLine[p.Flag] = true
		}
	}
	for _, f := range ctx.Flags() {
		explicit := onCommandLine[f]
		for _, env := range f.Envs {
			if v, ok := os.LookupEnv(env); ok && v != "" {
				explicit = true
			}
		}
		if explicit {
			// Kong binds flags to fields of the struct it was given a
			// pointer to, so every target is addressable.
			explicitFlags[f.Target.Addr().Pointer()] = true
		}
	}
}

// parseCLI parses args into cli, treating empty environment variables as unset
// and recording which flags were set explicitly (#257).
func parseCLI(args []string, options ...kong.Option) (*kong.Kong, *kong.Context, error) {
	parser := kong.Must(&cli, options...)
	dropEmptyFlagEnv(parser.Model)
	ctx, err := parser.Parse(args)
	if err == nil {
		recordExplicitFlags(ctx)
	}
	return parser, ctx, err
}

// dropEmptyFlagEnv unsets environment variables bound to flags that are set
// but empty, so an empty value means "not set": Kong would otherwise reject
// e.g. READ_ONLY="" as an invalid bool and the agent would not start (#257).
func dropEmptyFlagEnv(app *kong.Application) {
	for _, group := range app.AllFlags(true) {
		for _, f := range group {
			for _, env := range f.Envs {
				if v, ok := os.LookupEnv(env); ok && v == "" {
					_ = os.Unsetenv(env)
				}
			}
		}
	}
}

// isExplicitFlag reports whether the CLI field dst was set explicitly.
func isExplicitFlag(dst any) bool {
	return explicitFlags[reflect.ValueOf(dst).Pointer()]
}

// Limits for the vm_guest_agent settings.
const (
	vmGuestDefaultInterval = 30
	vmGuestMinInterval     = 10
	vmGuestMinSensorSecs   = 5
)

var vmGuestKeyRegex = regexp.MustCompile(`[^a-z0-9_]`)

var vmGuestSensorNameRegex = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// vmGuestFileConfig returns the vm_guest_agent section of config.yml, or nil.
func vmGuestFileConfig(cfg *domain.FileConfig) *domain.FileConfigVMGuest {
	if cfg == nil {
		return nil
	}
	return cfg.VMGuestAgent
}

// buildVMGuestConfig merges the --vm-guest-agent flag with the vm_guest_agent
// section of config.yml. The flag, when set, replaces the file's VM list.
// Invalid VM names and sensors are logged and skipped.
func buildVMGuestConfig(cliVMs string, file *domain.FileConfigVMGuest) domain.VMGuestConfig {
	cfg := domain.VMGuestConfig{Interval: vmGuestDefaultInterval}
	var names []string
	if file != nil {
		names = file.VMs
		if file.Interval != nil {
			cfg.Interval = max(*file.Interval, vmGuestMinInterval)
		}
	}
	if cliVMs != "" {
		names = strings.Split(cliVMs, ",")
	}

	known := make(map[string]bool)
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || known[name] {
			continue
		}
		if err := lib.ValidateVMName(name); err != nil {
			logger.Warning("vm_guest_agent: skipping VM %q: %v", name, err)
			continue
		}
		known[name] = true
		cfg.VMs = append(cfg.VMs, name)
	}
	if file == nil {
		return cfg
	}

	seen := make(map[string]bool)
	for _, s := range file.Sensors {
		s.VM = strings.TrimSpace(s.VM)
		key := s.VM + "/" + s.Name
		switch {
		case !known[s.VM]:
			logger.Warning("vm_guest_agent: skipping sensor %q: VM %q is not in the VM list", s.Name, s.VM)
		case !vmGuestSensorNameRegex.MatchString(s.Name):
			logger.Warning("vm_guest_agent: skipping sensor %q: name must be 1-64 lowercase letters, digits or underscores", s.Name)
		case seen[key]:
			logger.Warning("vm_guest_agent: skipping duplicate sensor %q for VM %q", s.Name, s.VM)
		case len(s.Command) == 0 || s.Command[0] == "":
			logger.Warning("vm_guest_agent: skipping sensor %q: command is empty", s.Name)
		case len(s.Attributes) > 0 && !s.JSON:
			logger.Warning("vm_guest_agent: skipping sensor %q: attributes need json: true", s.Name)
		case !validAttributes(s.Attributes):
			logger.Warning("vm_guest_agent: skipping sensor %q: attributes must map distinct non-empty keys to other keys that are not attributes themselves", s.Name)
		default:
			seen[key] = true
			attributes, _ := normalizeSensorAttributes(s.Attributes)
			interval := s.Interval
			if interval == 0 {
				interval = vmGuestDefaultInterval
			}
			cfg.Sensors = append(cfg.Sensors, domain.VMGuestSensorConfig{
				VM:         s.VM,
				Name:       s.Name,
				Command:    s.Command,
				Interval:   max(interval, vmGuestMinSensorSecs),
				JSON:       s.JSON,
				Attributes: attributes,
			})
		}
	}
	cfg.Sensors = dropAmbiguousJSONSensors(cfg.Sensors)
	return cfg
}

func validAttributes(attributes map[string]string) bool {
	_, ok := normalizeSensorAttributes(attributes)
	return ok
}

// normalizeSensorAttributes normalises attribute keys and targets the way
// expandJSONSensor normalises JSON keys (lowercase, other characters to an
// underscore). It reports false for an empty name, two keys that normalise to
// the same name, or a target that is itself an attribute key.
func normalizeSensorAttributes(attributes map[string]string) (map[string]string, bool) {
	if len(attributes) == 0 {
		return nil, true
	}
	norm := make(map[string]string, len(attributes))
	for k, target := range attributes {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(target) == "" {
			return nil, false
		}
		nk := vmGuestKeyRegex.ReplaceAllString(strings.ToLower(k), "_")
		if _, dup := norm[nk]; dup {
			return nil, false
		}
		norm[nk] = vmGuestKeyRegex.ReplaceAllString(strings.ToLower(target), "_")
	}
	for _, target := range norm {
		if _, isAttr := norm[target]; isAttr {
			return nil, false
		}
	}
	return norm, true
}

// dropAmbiguousJSONSensors skips a sensor whose name starts with the name of a
// JSON sensor on the same VM followed by an underscore. The JSON sensor's keys
// could produce the same name (gpu with key core_temp, gpu_core with key temp),
// and which sensor owned it would depend on which command finished first.
func dropAmbiguousJSONSensors(sensors []domain.VMGuestSensorConfig) []domain.VMGuestSensorConfig {
	kept := sensors[:0:0]
outer:
	for _, s := range sensors {
		for _, j := range sensors {
			if j.JSON && j.VM == s.VM && strings.HasPrefix(s.Name, j.Name+"_") {
				logger.Warning("vm_guest_agent: skipping sensor %q: its name starts with JSON sensor %q and an underscore, so their names could clash", s.Name, j.Name)
				continue outer
			}
		}
		kept = append(kept, s)
	}
	return kept
}

// applyFileConfig merges config file values into the CLI struct.
// Only fields not explicitly set via CLI/env are overridden.
// Kong sets fields to their declared defaults before parsing, so file config
// values are applied after kong.Parse to fill in non-defaulted values.
// In practice this means file config acts as a "second default layer":
// CLI flag > env var > config file > struct default.
func applyFileConfig(cfg *domain.FileConfig) {
	if cfg == nil {
		return
	}

	setInt := func(dst *int, src *int) {
		if src != nil && !isExplicitFlag(dst) {
			*dst = *src
		}
	}
	setStr := func(dst *string, src *string) {
		if src != nil && !isExplicitFlag(dst) {
			*dst = *src
		}
	}
	setBool := func(dst *bool, src *bool) {
		if src != nil && !isExplicitFlag(dst) {
			*dst = *src
		}
	}

	// Server settings
	setInt(&cli.Port, cfg.Port)
	setStr(&cli.BindAddress, cfg.BindAddress)
	setStr(&cli.LogLevel, cfg.LogLevel)
	setStr(&cli.LogsDir, cfg.LogsDir)
	setBool(&cli.Debug, cfg.Debug)
	setBool(&cli.ReadOnly, cfg.ReadOnly)
	setBool(&cli.LowPowerMode, cfg.LowPowerMode)
	setStr(&cli.DisableCollectors, cfg.DisableCollectors)
	setStr(&cli.AutoCollectors, cfg.AutoCollectors)
	if m := cfg.CollectionMode; m != nil && !isExplicitFlag(&cli.CollectionMode) {
		if *m == domain.CollectionModeDynamic || *m == domain.CollectionModeManual {
			cli.CollectionMode = *m
		} else {
			log.Printf("WARNING: Invalid collection_mode %q in config.yml (want dynamic or manual), ignoring", *m)
		}
	}
	setStr(&cli.CORSOrigin, cfg.CORSOrigin)
	setStr(&cli.APIToken, cfg.APIToken)
	setStr(&cli.MCPConnectSecret, cfg.MCPConnectSecret)
	setBool(&cli.AllowUnauthenticatedWrites, cfg.AllowUnauthenticatedWrites)
	setBool(&cli.WANIPLookup, cfg.WANIPLookup)
	setStr(&cli.TLSCertFile, cfg.TLSCertFile)
	setStr(&cli.TLSKeyFile, cfg.TLSKeyFile)

	// MQTT
	if m := cfg.MQTT; m != nil {
		setBool(&cli.MQTTEnabled, m.Enabled)
		setStr(&cli.MQTTBroker, m.Broker)
		setInt(&cli.MQTTPort, m.Port)
		setStr(&cli.MQTTUsername, m.Username)
		setStr(&cli.MQTTPassword, m.Password)
		setStr(&cli.MQTTClientID, m.ClientID)
		setStr(&cli.MQTTTopicPrefix, m.TopicPrefix)
		setBool(&cli.MQTTUseTLS, m.UseTLS)
		setBool(&cli.MQTTInsecureSkipVerify, m.InsecureSkipVerify)
		setInt(&cli.MQTTQoS, m.QoS)
		setBool(&cli.MQTTRetain, m.Retain)
		setBool(&cli.MQTTHomeAssistant, m.HomeAssistant)
		setStr(&cli.MQTTHAPrefix, m.HAPrefix)
	}

	// Discovery (zeroconf/mDNS)
	if d := cfg.Discovery; d != nil {
		setBool(&cli.DiscoveryEnabled, d.Enabled)
		setStr(&cli.DiscoveryServiceName, d.ServiceName)
	}

	// Intervals. Remember which ones the file set: an explicit interval for a
	// hardware-gated collector takes it out of automatic mode.
	if iv := cfg.Intervals; iv != nil {
		setInterval := func(name string, dst *int, src *int) {
			if src != nil && !isExplicitFlag(dst) {
				*dst = *src
				fileConfigIntervals[name] = true
			}
		}
		setInterval("system", &cli.IntervalSystem, iv.System)
		setInterval("array", &cli.IntervalArray, iv.Array)
		setInterval("disk", &cli.IntervalDisk, iv.Disk)
		setInterval("docker", &cli.IntervalDocker, iv.Docker)
		setInterval("vm", &cli.IntervalVM, iv.VM)
		setInterval("ups", &cli.IntervalUPS, iv.UPS)
		setInterval("nut", &cli.IntervalNUT, iv.NUT)
		setInterval("gpu", &cli.IntervalGPU, iv.GPU)
		setInterval("shares", &cli.IntervalShares, iv.Shares)
		setInterval("network", &cli.IntervalNetwork, iv.Network)
		setInterval("hardware", &cli.IntervalHardware, iv.Hardware)
		setInterval("zfs", &cli.IntervalZFS, iv.ZFS)
		setInterval("notification", &cli.IntervalNotification, iv.Notification)
		setInterval("registration", &cli.IntervalRegistration, iv.Registration)
		setInterval("unassigned", &cli.IntervalUnassigned, iv.Unassigned)
		setInterval("fancontrol", &cli.IntervalFanControl, iv.FanControl)
		setInterval("tuning", &cli.IntervalTuning, iv.Tuning)
		setInterval("docker_update", &cli.IntervalDockerUpdate, iv.DockerUpdate)
		setInterval("docker_networks", &cli.IntervalDockerNetworks, iv.DockerNetworks)
		setInterval("plugin_update", &cli.IntervalPluginUpdate, iv.PluginUpdate)
		setInterval("os_update", &cli.IntervalOSUpdate, iv.OSUpdate)
		setInterval("mover", &cli.IntervalMover, iv.Mover)
		setInterval("storage_topology", &cli.IntervalStorageTopology, iv.StorageTopology)
		setInterval("nic", &cli.IntervalNIC, iv.NIC)
		setInterval("psu", &cli.IntervalPSU, iv.PSU)
	}
}

// fileConfigIntervals records the collectors whose interval came from config.yml.
var fileConfigIntervals = map[string]bool{}

// resolveAutoCollectors parses the automatic-mode collector list (#261). A name
// must be a hardware-gated collector. A collector stays out of automatic mode
// when it is in the disabled list, when its interval came from config.yml, or
// when its configured interval is not 0: an explicit interval is used instead
// of the auto interval while the hardware is present.
func resolveAutoCollectors(csv string, disabled map[string]bool, intervals map[string]int, fromFile map[string]bool) map[string]bool {
	auto := make(map[string]bool)
	for name := range strings.SplitSeq(csv, ",") {
		name = strings.TrimSpace(strings.ToLower(name))
		switch {
		case name == "":
			continue
		case !isHardwareGated(name):
			logger.Warning("'%s' in UNRAID_AUTO_COLLECTORS is not a hardware-gated collector, ignoring", name)
		case disabled[name]:
			logger.Warning("Collector '%s' is in both UNRAID_DISABLE_COLLECTORS and UNRAID_AUTO_COLLECTORS; it stays disabled", name)
		case fromFile[name]:
			logger.Info("Collector '%s' has an interval in config.yml; automatic mode off", name)
		case intervals[name] != 0:
			logger.Info("Collector '%s' has an explicit interval (%ds); automatic mode off", name, intervals[name])
		default:
			auto[name] = true
		}
	}
	return auto
}

// hardwareGatedCollectors returns the collectors the hardware policy controls:
// every hardware-gated collector except those in the disabled list. Absent
// hardware turns them off whatever their configured interval (#261).
func hardwareGatedCollectors(disabled map[string]bool) map[string]bool {
	gated := make(map[string]bool)
	for _, class := range collectors.HardwareClasses {
		if class.Collector != "" && !disabled[class.Collector] {
			gated[class.Collector] = true
		}
	}
	return gated
}

// isHardwareGated reports whether name is a collector gated by a hardware class.
func isHardwareGated(name string) bool {
	_, ok := collectors.HardwareClassForCollector(name)
	return ok
}

// parseToolPolicyCSV parses a comma-separated tool=policy string into a map of ToolPolicyValue.
func parseToolPolicyCSV(csv string) map[string]domain.ToolPolicyValue {
	result := make(map[string]domain.ToolPolicyValue)
	if strings.TrimSpace(csv) == "" {
		return result
	}
	for part := range strings.SplitSeq(csv, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" {
			continue
		}
		if err := lib.ValidateToolPolicyValue(v); err != nil {
			log.Printf("WARNING: Invalid tool policy %q for tool %q in TOOL_POLICY, falling back to default: %v", v, k, err)
			continue
		}
		if v != "" {
			result[k] = domain.ToolPolicyValue(v)
		}
	}
	return result
}
