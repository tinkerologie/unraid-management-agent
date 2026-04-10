package diagnostics

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const (
	defaultAgentLogLines = 1000
	defaultSysLogLines   = 500
)

// BundleService collects and aggregates diagnostic information.
type BundleService struct {
	ctx *domain.Context
}

// NewBundleService creates a new diagnostic bundle service.
func NewBundleService(ctx *domain.Context) *BundleService {
	return &BundleService{ctx: ctx}
}

// CollectDiagnostics gathers all diagnostic information into a bundle.
func (s *BundleService) CollectDiagnostics(ctx context.Context) (*dto.DiagnosticBundle, error) {
	hostname, _ := os.Hostname()

	bundle := &dto.DiagnosticBundle{
		Metadata:      s.collectMetadata(hostname),
		SystemState:   s.collectSystemState(),
		ArrayStatus:   s.collectArrayStatus(),
		Containers:    s.collectContainers(),
		VMs:           s.collectVMs(),
		Network:       s.collectNetwork(),
		Logs:          s.collectLogs(ctx),
		Configuration: s.collectConfiguration(),
	}

	return bundle, nil
}

func (s *BundleService) collectMetadata(hostname string) dto.BundleMetadata {
	meta := dto.BundleMetadata{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		AgentVersion: s.ctx.Version,
		Hostname:     hostname,
	}

	// Read kernel version from /proc/version
	if data, err := os.ReadFile("/proc/version"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 3 {
			meta.KernelVersion = parts[2]
		}
	}

	// Read Unraid version from /etc/unraid-version
	if data, err := os.ReadFile("/etc/unraid-version"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "version=") {
				meta.UnraidVersion = strings.Trim(strings.TrimPrefix(line, "version="), "\"")
				break
			}
		}
	}

	return meta
}

func (s *BundleService) collectSystemState() dto.BundleSystemState {
	state := dto.BundleSystemState{}

	// Read uptime
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			state.Uptime = fields[0] + "s"
		}
	}

	// Read memory info
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		memInfo := parseMemInfo(string(data))
		totalKB := memInfo["MemTotal"]
		freeKB := memInfo["MemFree"] + memInfo["Buffers"] + memInfo["Cached"]
		usedKB := totalKB - freeKB
		if totalKB > 0 {
			state.RAMUsage = float64(usedKB) / float64(totalKB) * 100.0
			state.RAMTotalMB = float64(totalKB) / 1024.0
			state.RAMUsedMB = float64(usedKB) / 1024.0
		}
	}

	return state
}

// parseMemInfo parses /proc/meminfo into a key-value map of kB values.
func parseMemInfo(data string) map[string]int64 {
	result := make(map[string]int64)
	for _, line := range strings.Split(data, "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			key := strings.TrimSuffix(parts[0], ":")
			var val int64
			if _, err := fmt.Sscanf(parts[1], "%d", &val); err == nil {
				result[key] = val
			}
		}
	}
	return result
}

func (s *BundleService) collectArrayStatus() dto.BundleArrayStatus {
	status := dto.BundleArrayStatus{
		State: "unknown",
	}

	// Try reading array state from var.ini
	if data, err := os.ReadFile("/var/local/emhttp/var.ini"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "mdState=") {
				status.State = strings.Trim(strings.TrimPrefix(line, "mdState="), "\"")
			}
			if strings.HasPrefix(line, "mdNumDisks=") {
				if _, err := fmt.Sscanf(strings.TrimPrefix(line, "mdNumDisks="), "%d", &status.TotalDisks); err != nil {
					logger.Debug("failed to parse mdNumDisks: %v", err)
				}
			}
		}
	}

	return status
}

func (s *BundleService) collectContainers() []dto.BundleContainer {
	// Docker info is collected from the Docker SDK at runtime.
	// In CLI diagnostics mode (without running collectors), we try docker ps.
	output, err := lib.ExecCommandOutput("docker", "ps", "-a", "--format", "{{.Names}}\t{{.Image}}\t{{.State}}\t{{.Status}}")
	if err != nil {
		logger.Debug("failed to collect docker containers: %v", err)
		return nil
	}

	var containers []dto.BundleContainer
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		c := dto.BundleContainer{}
		if len(parts) > 0 {
			c.Name = parts[0]
		}
		if len(parts) > 1 {
			c.Image = parts[1]
		}
		if len(parts) > 2 {
			c.State = parts[2]
		}
		if len(parts) > 3 {
			c.Status = parts[3]
		}
		containers = append(containers, c)
	}
	return containers
}

func (s *BundleService) collectVMs() []dto.BundleVM {
	output, err := lib.ExecCommandOutput("virsh", "list", "--all", "--name")
	if err != nil {
		logger.Debug("failed to collect VMs: %v", err)
		return nil
	}

	var vms []dto.BundleVM
	for _, name := range strings.Split(strings.TrimSpace(output), "\n") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		state := "unknown"
		if stateOutput, err := lib.ExecCommandOutput("virsh", "domstate", name); err == nil {
			state = strings.TrimSpace(stateOutput)
		}
		vms = append(vms, dto.BundleVM{Name: name, State: state})
	}
	return vms
}

func (s *BundleService) collectNetwork() []dto.BundleNetwork {
	// Read network info from Unraid network.ini
	var networks []dto.BundleNetwork

	if data, err := os.ReadFile("/var/local/emhttp/network.ini"); err == nil {
		// Parse simple network entries
		var current dto.BundleNetwork
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "NAME=") {
				if current.Name != "" {
					networks = append(networks, current)
				}
				current = dto.BundleNetwork{Name: strings.Trim(strings.TrimPrefix(line, "NAME="), "\"")}
			}
			if strings.HasPrefix(line, "IPADDR:0=") {
				current.IPAddr = strings.Trim(strings.TrimPrefix(line, "IPADDR:0="), "\"")
			}
		}
		if current.Name != "" {
			networks = append(networks, current)
		}
	}

	return networks
}

func (s *BundleService) collectLogs(ctx context.Context) dto.BundleLogs {
	logs := dto.BundleLogs{}

	// Collect diagnostic log entries (structured JSON)
	diagPath := filepath.Join(s.ctx.LogsDir, "diagnostic.jsonl")
	// #nosec G304 -- path built from trusted LogsDir config
	if data, err := os.ReadFile(diagPath); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			var entry dto.DiagnosticLogEntry
			if err := json.Unmarshal([]byte(line), &entry); err == nil {
				// Redact sensitive data in log messages
				entry.Message = lib.Redact(entry.Message)
				if entry.Context != nil {
					entry.Context = lib.RedactMap(entry.Context)
				}
				logs.DiagnosticEntries = append(logs.DiagnosticEntries, entry)
			}
		}
	}

	// Collect agent log (last N lines, redacted)
	agentLogPath := filepath.Join(s.ctx.LogsDir, "unraid-management-agent.log")
	logs.AgentLog = readLastNLines(agentLogPath, defaultAgentLogLines)

	// Collect syslog (last N lines, redacted)
	logs.SysLog = readLastNLines("/var/log/syslog", defaultSysLogLines)

	return logs
}

// readLastNLines reads the last n lines from a file and redacts sensitive data.
func readLastNLines(path string, n int) []string {
	file, err := os.Open(path) // #nosec G304 -- path is always a hardcoded known log path
	if err != nil {
		return nil
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Debug("failed to close file %s: %v", path, err)
		}
	}()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, lib.Redact(scanner.Text()))
	}

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func (s *BundleService) collectConfiguration() dto.BundleConfiguration {
	config := dto.BundleConfiguration{
		Port:    s.ctx.Port,
		Version: s.ctx.Version,
		CollectorIntervals: map[string]int{
			"system":       s.ctx.Intervals.System,
			"array":        s.ctx.Intervals.Array,
			"disk":         s.ctx.Intervals.Disk,
			"docker":       s.ctx.Intervals.Docker,
			"vm":           s.ctx.Intervals.VM,
			"ups":          s.ctx.Intervals.UPS,
			"nut":          s.ctx.Intervals.NUT,
			"gpu":          s.ctx.Intervals.GPU,
			"shares":       s.ctx.Intervals.Shares,
			"network":      s.ctx.Intervals.Network,
			"hardware":     s.ctx.Intervals.Hardware,
			"zfs":          s.ctx.Intervals.ZFS,
			"notification": s.ctx.Intervals.Notification,
			"registration": s.ctx.Intervals.Registration,
			"unassigned":   s.ctx.Intervals.Unassigned,
			"fancontrol":   s.ctx.Intervals.FanControl,
			"tuning":       s.ctx.Intervals.Tuning,
		},
	}

	// Redact MQTT config
	mqttRedacted := lib.RedactStruct(s.ctx.MQTTConfig)
	if m, ok := mqttRedacted.(map[string]any); ok {
		config.MQTTConfig = m
	}

	return config
}
