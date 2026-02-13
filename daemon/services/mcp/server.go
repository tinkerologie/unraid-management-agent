// Package mcp provides a Model Context Protocol (MCP) server implementation for the Unraid Management Agent.
// It exposes Unraid system monitoring and control capabilities to AI agents via the standardized MCP protocol.
//
// Uses the official MCP Go SDK (github.com/modelcontextprotocol/go-sdk) implementing protocol version 2025-06-18.
// Supports two transports:
//   - Streamable HTTP: for remote connections (Claude, ChatGPT, Cursor, Copilot, Codex, Windsurf, Gemini, etc.)
//   - STDIO: for local connections on the Unraid server (newline-delimited JSON over stdin/stdout)
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/controllers"
)

// CacheProvider defines the interface for accessing cached data from the API server.
type CacheProvider interface {
	GetSystemCache() *dto.SystemInfo
	GetArrayCache() *dto.ArrayStatus
	GetDisksCache() []dto.DiskInfo
	GetSharesCache() []dto.ShareInfo
	GetDockerCache() []dto.ContainerInfo
	GetVMsCache() []dto.VMInfo
	GetUPSCache() *dto.UPSStatus
	GetGPUCache() []*dto.GPUMetrics
	GetNetworkCache() []dto.NetworkInfo
	GetHardwareCache() *dto.HardwareInfo
	GetRegistrationCache() *dto.Registration
	GetNotificationsCache() *dto.NotificationList
	GetZFSPoolsCache() []dto.ZFSPool
	GetZFSDatasetsCache() []dto.ZFSDataset
	GetZFSSnapshotsCache() []dto.ZFSSnapshot
	GetZFSARCStatsCache() *dto.ZFSARCStats
	GetUnassignedCache() *dto.UnassignedDeviceList
	GetNUTCache() *dto.NUTResponse
	GetParityHistoryCache() *dto.ParityCheckHistory
	// Logs
	ListLogFiles() []dto.LogFile
	GetLogContent(path, lines, start string) (*dto.LogFileContent, error)
	// Collectors
	GetCollectorsStatus() dto.CollectorsStatusResponse
	GetCollectorStatus(name string) (*dto.CollectorStatus, error)
	EnableCollector(name string) error
	DisableCollector(name string) error
	UpdateCollectorInterval(name string, interval int) error
	// Settings
	GetSystemSettings() *dto.SystemSettings
	GetDockerSettings() *dto.DockerSettings
	GetVMSettings() *dto.VMSettings
	GetDiskSettings() *dto.DiskSettings
	GetShareConfig(name string) *dto.ShareConfig
	// Network and Health
	GetNetworkAccessURLs() *dto.NetworkAccessURLs
	GetHealthStatus() map[string]interface{}
}

// ptr returns a pointer to the given bool value. Used for optional ToolAnnotations fields.
func ptr(b bool) *bool { return &b }

// Server represents the MCP server that exposes Unraid capabilities to AI agents.
type Server struct {
	ctx           *domain.Context
	mcpServer     *mcp.Server
	httpHandler   *mcp.StreamableHTTPHandler
	cacheProvider CacheProvider
}

// NewServer creates a new MCP server instance.
func NewServer(ctx *domain.Context, cacheProvider CacheProvider) *Server {
	return &Server{
		ctx:           ctx,
		cacheProvider: cacheProvider,
	}
}

// Initialize sets up the MCP server with all tools, resources, and prompts.
func (s *Server) Initialize() error {
	s.mcpServer = mcp.NewServer(
		&mcp.Implementation{
			Name:    "unraid-management-agent",
			Version: s.ctx.Version,
		},
		&mcp.ServerOptions{
			Instructions: "Unraid server management agent providing system monitoring, Docker container control, " +
				"VM management, array operations, and comprehensive diagnostics via MCP tools.",
		},
	)

	// Register all tools, resources, and prompts
	s.registerMonitoringTools()
	s.registerControlTools()
	s.registerResources()
	s.registerPrompts()

	// Create the Streamable HTTP handler (implements MCP 2025-06-18 transport)
	s.httpHandler = mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return s.mcpServer },
		nil,
	)

	logger.Info("MCP server initialized with official SDK (protocol 2025-06-18), tools, resources, and prompts")
	return nil
}

// GetHTTPHandler returns the Streamable HTTP handler for the MCP endpoint.
// This single handler supports POST, GET, DELETE, and OPTIONS on the MCP endpoint,
// conforming to the MCP 2025-06-18 Streamable HTTP transport specification.
func (s *Server) GetHTTPHandler() http.Handler {
	if s.httpHandler == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "MCP server not initialized", http.StatusInternalServerError)
		})
	}
	return s.httpHandler
}

// GetMCPServer returns the underlying MCP server instance.
// Returns nil if Initialize() has not been called.
func (s *Server) GetMCPServer() *mcp.Server {
	return s.mcpServer
}

// RunSTDIO runs the MCP server over stdin/stdout using newline-delimited JSON.
// This is the preferred transport for local AI clients (e.g., Claude Desktop) running
// directly on the Unraid server, as it requires no network overhead or authentication.
// The method blocks until the context is cancelled or the STDIO connection is closed.
func (s *Server) RunSTDIO(ctx context.Context) error {
	if s.mcpServer == nil {
		return fmt.Errorf("MCP server not initialized")
	}
	logger.Info("MCP STDIO transport starting (stdin/stdout)")
	return s.mcpServer.Run(ctx, &mcp.StdioTransport{})
}

// registerMonitoringTools registers all read-only monitoring tools.
func (s *Server) registerMonitoringTools() {
	// System information tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_system_info",
		Description: "Get comprehensive Unraid system information including hostname, CPU usage, RAM usage, temperatures, and uptime",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		info := s.cacheProvider.GetSystemCache()
		if info == nil {
			return textResult("System information not available yet"), nil, nil
		}
		return jsonResult(info)
	})

	// Array status tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_array_status",
		Description: "Get Unraid array status including state, capacity, parity information, and disk assignments",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		status := s.cacheProvider.GetArrayCache()
		if status == nil {
			return textResult("Array status not available yet"), nil, nil
		}
		return jsonResult(status)
	})

	// List all disks tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_disks",
		Description: "List all disks in the Unraid server including array disks, cache, and unassigned devices with their health status",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPDiskArgs) (*mcp.CallToolResult, any, error) {
		disks := s.cacheProvider.GetDisksCache()
		if disks == nil {
			return textResult("Disk information not available yet"), nil, nil
		}
		return jsonResult(disks)
	})

	// Get specific disk info tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_disk_info",
		Description: "Get detailed information about a specific disk including SMART data and health status",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPDiskArgs) (*mcp.CallToolResult, any, error) {
		disks := s.cacheProvider.GetDisksCache()
		if disks == nil {
			return textResult("Disk information not available yet"), nil, nil
		}
		for _, disk := range disks {
			if disk.Device == args.DiskID || disk.Name == args.DiskID || disk.ID == args.DiskID {
				return jsonResult(disk)
			}
		}
		return textResult(fmt.Sprintf("Disk '%s' not found", args.DiskID)), nil, nil
	})

	// List shares tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_shares",
		Description: "List all network shares configured on the Unraid server with their settings and usage",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		shares := s.cacheProvider.GetSharesCache()
		if shares == nil {
			return textResult("Share information not available yet"), nil, nil
		}
		return jsonResult(shares)
	})

	// List Docker containers tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_containers",
		Description: "List all Docker containers on the Unraid server with their status, resource usage, and configuration",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPContainerListArgs) (*mcp.CallToolResult, any, error) {
		containers := s.cacheProvider.GetDockerCache()
		if containers == nil {
			return textResult("Container information not available yet"), nil, nil
		}

		// Filter by state if specified
		if args.State != "" && args.State != "all" {
			filtered := make([]dto.ContainerInfo, 0)
			for _, c := range containers {
				if (args.State == "running" && c.State == "running") ||
					(args.State == "stopped" && c.State != "running") {
					filtered = append(filtered, c)
				}
			}
			return jsonResult(filtered)
		}
		return jsonResult(containers)
	})

	// Get specific container info tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_container_info",
		Description: "Get detailed information about a specific Docker container",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPContainerArgs) (*mcp.CallToolResult, any, error) {
		containers := s.cacheProvider.GetDockerCache()
		if containers == nil {
			return textResult("Container information not available yet"), nil, nil
		}
		for _, c := range containers {
			if c.ID == args.ContainerID || c.Name == args.ContainerID {
				return jsonResult(c)
			}
		}
		return textResult(fmt.Sprintf("Container '%s' not found", args.ContainerID)), nil, nil
	})

	// List VMs tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_vms",
		Description: "List all virtual machines on the Unraid server with their status and configuration",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPVMListArgs) (*mcp.CallToolResult, any, error) {
		vms := s.cacheProvider.GetVMsCache()
		if vms == nil {
			return textResult("VM information not available yet"), nil, nil
		}

		// Filter by state if specified
		if args.State != "" && args.State != "all" {
			filtered := make([]dto.VMInfo, 0)
			for _, vm := range vms {
				isRunning := vm.State == "running"
				if (args.State == "running" && isRunning) ||
					(args.State == "stopped" && !isRunning) {
					filtered = append(filtered, vm)
				}
			}
			return jsonResult(filtered)
		}
		return jsonResult(vms)
	})

	// Get specific VM info tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_vm_info",
		Description: "Get detailed information about a specific virtual machine",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPVMArgs) (*mcp.CallToolResult, any, error) {
		vms := s.cacheProvider.GetVMsCache()
		if vms == nil {
			return textResult("VM information not available yet"), nil, nil
		}
		for _, vm := range vms {
			if vm.Name == args.VMName || vm.ID == args.VMName {
				return jsonResult(vm)
			}
		}
		return textResult(fmt.Sprintf("VM '%s' not found", args.VMName)), nil, nil
	})

	// UPS status tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_ups_status",
		Description: "Get UPS (Uninterruptible Power Supply) status including battery level, load, and runtime",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		ups := s.cacheProvider.GetUPSCache()
		if ups == nil {
			return textResult("UPS not configured or information not available"), nil, nil
		}
		return jsonResult(ups)
	})

	// GPU metrics tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_gpu_metrics",
		Description: "Get GPU metrics including utilization, temperature, and memory usage for all GPUs",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		gpus := s.cacheProvider.GetGPUCache()
		if len(gpus) == 0 {
			return textResult("No GPUs detected or GPU information not available"), nil, nil
		}
		return jsonResult(gpus)
	})

	// Network interfaces tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_network_info",
		Description: "Get network interface information including IP addresses, speeds, and traffic statistics",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		network := s.cacheProvider.GetNetworkCache()
		if network == nil {
			return textResult("Network information not available yet"), nil, nil
		}
		return jsonResult(network)
	})

	// Hardware info tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_hardware_info",
		Description: "Get detailed hardware information including motherboard, CPU, and memory details from DMI/SMBIOS",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		hardware := s.cacheProvider.GetHardwareCache()
		if hardware == nil {
			return textResult("Hardware information not available yet"), nil, nil
		}
		return jsonResult(hardware)
	})

	// Registration/License info tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_registration",
		Description: "Get Unraid license/registration information including license type and key status",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		reg := s.cacheProvider.GetRegistrationCache()
		if reg == nil {
			return textResult("Registration information not available yet"), nil, nil
		}
		return jsonResult(reg)
	})

	// Notifications tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_notifications",
		Description: "Get system notifications including alerts, warnings, and informational messages",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPNotificationArgs) (*mcp.CallToolResult, any, error) {
		notifications := s.cacheProvider.GetNotificationsCache()
		if notifications == nil {
			return textResult("Notifications not available yet"), nil, nil
		}
		return jsonResult(notifications)
	})

	// ZFS pools tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_zfs_pools",
		Description: "Get ZFS pool information including health status, capacity, and configuration",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPZFSPoolArgs) (*mcp.CallToolResult, any, error) {
		pools := s.cacheProvider.GetZFSPoolsCache()
		if len(pools) == 0 {
			return textResult("No ZFS pools configured or ZFS information not available"), nil, nil
		}

		// Return specific pool if name provided
		if args.PoolName != "" {
			for _, pool := range pools {
				if pool.Name == args.PoolName {
					return jsonResult(pool)
				}
			}
			return textResult(fmt.Sprintf("ZFS pool '%s' not found", args.PoolName)), nil, nil
		}
		return jsonResult(pools)
	})

	// ZFS datasets tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_zfs_datasets",
		Description: "Get ZFS dataset information including snapshots, quotas, and usage",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		datasets := s.cacheProvider.GetZFSDatasetsCache()
		if len(datasets) == 0 {
			return textResult("No ZFS datasets found or ZFS information not available"), nil, nil
		}
		return jsonResult(datasets)
	})

	// ZFS snapshots tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_zfs_snapshots",
		Description: "Get ZFS snapshot information for all pools and datasets",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		snapshots := s.cacheProvider.GetZFSSnapshotsCache()
		if len(snapshots) == 0 {
			return textResult("No ZFS snapshots found or ZFS information not available"), nil, nil
		}
		return jsonResult(snapshots)
	})

	// ZFS ARC stats tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_zfs_arc_stats",
		Description: "Get ZFS ARC (Adaptive Replacement Cache) statistics including hit ratio and memory usage",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		arcStats := s.cacheProvider.GetZFSARCStatsCache()
		if arcStats == nil {
			return textResult("ZFS ARC statistics not available"), nil, nil
		}
		return jsonResult(arcStats)
	})

	// Unassigned devices tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_unassigned_devices",
		Description: "Get information about unassigned (non-array) devices including USB drives and unassigned disks",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		unassigned := s.cacheProvider.GetUnassignedCache()
		if unassigned == nil || len(unassigned.Devices) == 0 {
			return textResult("No unassigned devices found or Unassigned Devices plugin not installed"), nil, nil
		}
		return jsonResult(unassigned)
	})

	// NUT (Network UPS Tools) status tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_nut_status",
		Description: "Get detailed NUT (Network UPS Tools) status including all UPS variables and metrics",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		nut := s.cacheProvider.GetNUTCache()
		if nut == nil {
			return textResult("NUT not configured or NUT information not available"), nil, nil
		}
		return jsonResult(nut)
	})

	// User scripts list tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_user_scripts",
		Description: "List all available user scripts from the User Scripts plugin",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		scripts, err := controllers.ListUserScripts()
		if err != nil {
			return textResult(fmt.Sprintf("Failed to list user scripts: %v", err)), nil, nil
		}
		if len(scripts) == 0 {
			return textResult("No user scripts found or User Scripts plugin not installed"), nil, nil
		}
		return jsonResult(scripts)
	})

	// Parity history tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_parity_history",
		Description: "Get parity check history including past check dates, durations, speeds, and error counts",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		history := s.cacheProvider.GetParityHistoryCache()
		if history == nil || len(history.Records) == 0 {
			return textResult("No parity check history available"), nil, nil
		}
		return jsonResult(history)
	})

	// List log files tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_log_files",
		Description: "List all available log files on the Unraid server including system logs, Docker logs, and plugin logs",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		logs := s.cacheProvider.ListLogFiles()
		if len(logs) == 0 {
			return textResult("No log files found"), nil, nil
		}
		return jsonResult(logs)
	})

	// Get log content tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_log_content",
		Description: "Retrieve content from a specific log file with optional line limits. Returns the last N lines (tail behavior) or specific range.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPLogArgs) (*mcp.CallToolResult, any, error) {
		if args.LogFile == "" {
			return textResult("log_file is required"), nil, nil
		}

		// Default to 100 lines, max 1000
		lines := args.Lines
		if lines <= 0 {
			lines = 100
		}
		if lines > 1000 {
			lines = 1000
		}

		content, err := s.cacheProvider.GetLogContent(args.LogFile, fmt.Sprintf("%d", lines), "")
		if err != nil {
			return textResult(fmt.Sprintf("Failed to read log: %v", err)), nil, nil
		}
		return jsonResult(content)
	})

	// Get syslog tool (convenience wrapper)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_syslog",
		Description: "Get the system log (syslog) - convenient shortcut for viewing system messages",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPLogArgs) (*mcp.CallToolResult, any, error) {
		lines := args.Lines
		if lines <= 0 {
			lines = 100
		}
		if lines > 1000 {
			lines = 1000
		}

		content, err := s.cacheProvider.GetLogContent("/var/log/syslog", fmt.Sprintf("%d", lines), "")
		if err != nil {
			return textResult(fmt.Sprintf("Failed to read syslog: %v", err)), nil, nil
		}
		return jsonResult(content)
	})

	// Get Docker log tool (convenience wrapper)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_docker_log",
		Description: "Get the Docker daemon log - useful for diagnosing container issues",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPLogArgs) (*mcp.CallToolResult, any, error) {
		lines := args.Lines
		if lines <= 0 {
			lines = 100
		}
		if lines > 1000 {
			lines = 1000
		}

		content, err := s.cacheProvider.GetLogContent("/var/log/docker.log", fmt.Sprintf("%d", lines), "")
		if err != nil {
			return textResult(fmt.Sprintf("Failed to read Docker log: %v", err)), nil, nil
		}
		return jsonResult(content)
	})

	// List collectors status tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_collectors",
		Description: "List all data collectors with their status, intervals, and runtime information",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		status := s.cacheProvider.GetCollectorsStatus()
		return jsonResult(status)
	})

	// Get specific collector status tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_collector_status",
		Description: "Get detailed status of a specific collector including enabled state, interval, last run time, and error count",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPCollectorArgs) (*mcp.CallToolResult, any, error) {
		if args.CollectorName == "" {
			return textResult("collector_name is required"), nil, nil
		}

		status, err := s.cacheProvider.GetCollectorStatus(args.CollectorName)
		if err != nil {
			return textResult(fmt.Sprintf("Failed to get collector status: %v", err)), nil, nil
		}
		return jsonResult(status)
	})

	// Get system settings tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_system_settings",
		Description: "Get system configuration settings including server name, timezone, security mode, and date/time formats",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		settings := s.cacheProvider.GetSystemSettings()
		if settings == nil {
			return textResult("System settings not available"), nil, nil
		}
		return jsonResult(settings)
	})

	// Get Docker settings tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_docker_settings",
		Description: "Get Docker configuration settings including enabled state, image path, and network configuration",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		settings := s.cacheProvider.GetDockerSettings()
		if settings == nil {
			return textResult("Docker settings not available"), nil, nil
		}
		return jsonResult(settings)
	})

	// Get VM settings tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_vm_settings",
		Description: "Get VM Manager configuration settings including enabled state, PCI/USB passthrough devices",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		settings := s.cacheProvider.GetVMSettings()
		if settings == nil {
			return textResult("VM settings not available"), nil, nil
		}
		return jsonResult(settings)
	})

	// Get disk settings tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_disk_settings",
		Description: "Get disk configuration settings including spindown delay, auto-start, spinup groups, and default filesystem",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		settings := s.cacheProvider.GetDiskSettings()
		if settings == nil {
			return textResult("Disk settings not available"), nil, nil
		}
		return jsonResult(settings)
	})

	// Get share config tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_share_config",
		Description: "Get detailed share configuration including allocation method, cache settings, disk inclusion/exclusion, and export settings",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPShareArgs) (*mcp.CallToolResult, any, error) {
		if args.ShareName == "" {
			return textResult("share_name is required"), nil, nil
		}
		config := s.cacheProvider.GetShareConfig(args.ShareName)
		if config == nil {
			return textResult(fmt.Sprintf("Share config not found: %s", args.ShareName)), nil, nil
		}
		return jsonResult(config)
	})

	// Get network access URLs tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_network_access_urls",
		Description: "Get all available methods to access the Unraid server including LAN, WAN, WireGuard, mDNS, and IPv6 addresses",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		urls := s.cacheProvider.GetNetworkAccessURLs()
		if urls == nil {
			return textResult("Network access URLs not available"), nil, nil
		}
		return jsonResult(urls)
	})

	// Get health status tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_health_status",
		Description: "Get a quick health check summary of the Unraid server including API status, uptime, and basic connectivity",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		health := s.cacheProvider.GetHealthStatus()
		return jsonResult(health)
	})

	// Get notifications overview tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_notifications_overview",
		Description: "Get a summary of notification counts by type (unread/archive) and importance level (alert/warning/info)",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		notifications := s.cacheProvider.GetNotificationsCache()
		if notifications == nil {
			return textResult("Notifications not available"), nil, nil
		}
		return jsonResult(notifications.Overview)
	})

	// Search containers tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "search_containers",
		Description: "Search Docker containers by name, image, or state. Returns matching containers.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPSearchArgs) (*mcp.CallToolResult, any, error) {
		if args.Query == "" {
			return textResult("query is required"), nil, nil
		}
		containers := s.cacheProvider.GetDockerCache()
		if containers == nil {
			return textResult("No containers available"), nil, nil
		}

		query := strings.ToLower(args.Query)
		var matches []dto.ContainerInfo
		for _, c := range containers {
			if strings.Contains(strings.ToLower(c.Name), query) ||
				strings.Contains(strings.ToLower(c.Image), query) ||
				strings.Contains(strings.ToLower(c.State), query) {
				matches = append(matches, c)
			}
		}

		if len(matches) == 0 {
			return textResult(fmt.Sprintf("No containers matching '%s' found", args.Query)), nil, nil
		}
		return jsonResult(map[string]interface{}{
			"query":   args.Query,
			"count":   len(matches),
			"results": matches,
		})
	})

	// Search VMs tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "search_vms",
		Description: "Search virtual machines by name or state. Returns matching VMs.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPSearchArgs) (*mcp.CallToolResult, any, error) {
		if args.Query == "" {
			return textResult("query is required"), nil, nil
		}
		vms := s.cacheProvider.GetVMsCache()
		if vms == nil {
			return textResult("No VMs available"), nil, nil
		}

		query := strings.ToLower(args.Query)
		var matches []dto.VMInfo
		for _, vm := range vms {
			if strings.Contains(strings.ToLower(vm.Name), query) ||
				strings.Contains(strings.ToLower(vm.State), query) {
				matches = append(matches, vm)
			}
		}

		if len(matches) == 0 {
			return textResult(fmt.Sprintf("No VMs matching '%s' found", args.Query)), nil, nil
		}
		return jsonResult(map[string]interface{}{
			"query":   args.Query,
			"count":   len(matches),
			"results": matches,
		})
	})

	// Get diagnostic summary tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_diagnostic_summary",
		Description: "Get a comprehensive diagnostic summary including system health, array status, recent alerts, disk health, and resource usage",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		summary := make(map[string]interface{})

		// System info
		if sysInfo := s.cacheProvider.GetSystemCache(); sysInfo != nil {
			summary["system"] = map[string]interface{}{
				"hostname":    sysInfo.Hostname,
				"uptime_days": sysInfo.Uptime / 86400,
				"cpu_usage":   sysInfo.CPUUsage,
				"ram_usage":   sysInfo.RAMUsage,
				"cpu_temp":    sysInfo.CPUTemp,
			}
		}

		// Array status
		if arrayStatus := s.cacheProvider.GetArrayCache(); arrayStatus != nil {
			summary["array"] = map[string]interface{}{
				"state":               arrayStatus.State,
				"parity_valid":        arrayStatus.ParityValid,
				"used_percent":        arrayStatus.UsedPercent,
				"parity_check_status": arrayStatus.ParityCheckStatus,
			}
		}

		// Disk issues
		disks := s.cacheProvider.GetDisksCache()
		var diskIssues []map[string]interface{}
		for _, disk := range disks {
			if disk.Temperature > 50 || disk.Status != "PASSED" {
				diskIssues = append(diskIssues, map[string]interface{}{
					"id":          disk.ID,
					"name":        disk.Name,
					"temperature": disk.Temperature,
					"status":      disk.Status,
				})
			}
		}
		summary["disk_issues"] = diskIssues
		summary["disk_issues_count"] = len(diskIssues)

		// Notifications summary
		if notifications := s.cacheProvider.GetNotificationsCache(); notifications != nil {
			summary["notifications"] = notifications.Overview
		}

		// Docker container issues (non-running when should be)
		containers := s.cacheProvider.GetDockerCache()
		var stoppedContainers []string
		for _, c := range containers {
			if c.State == "exited" {
				stoppedContainers = append(stoppedContainers, c.Name)
			}
		}
		summary["stopped_containers"] = stoppedContainers
		summary["stopped_containers_count"] = len(stoppedContainers)

		return jsonResult(summary)
	})

	logger.Debug("MCP monitoring tools registered (40 read-only tools with annotations)")
}

// registerControlTools registers tools that can modify system state.
func (s *Server) registerControlTools() {
	// Container control tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "container_action",
		Description: "Perform an action on a Docker container (start, stop, restart, pause, unpause). Use with caution.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(true),
			IdempotentHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPContainerActionArgs) (*mcp.CallToolResult, any, error) {
		logger.Info("MCP: Container action '%s' requested for '%s'", args.Action, args.ContainerID)

		dockerCtrl := controllers.NewDockerController()
		var err error

		switch args.Action {
		case "start":
			err = dockerCtrl.Start(args.ContainerID)
		case "stop":
			err = dockerCtrl.Stop(args.ContainerID)
		case "restart":
			err = dockerCtrl.Restart(args.ContainerID)
		case "pause":
			err = dockerCtrl.Pause(args.ContainerID)
		case "unpause":
			err = dockerCtrl.Unpause(args.ContainerID)
		default:
			return textResult(fmt.Sprintf("Unknown action: %s", args.Action)), nil, nil
		}

		if err != nil {
			logger.Error("MCP: Container action failed: %v", err)
			return textResult(fmt.Sprintf("Failed to %s container: %v", args.Action, err)), nil, nil
		}

		return textResult(fmt.Sprintf("Successfully executed '%s' on container '%s'", args.Action, args.ContainerID)), nil, nil
	})

	// VM control tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "vm_action",
		Description: "Perform an action on a virtual machine (start, stop, restart, pause, resume, hibernate, force-stop). Use with caution.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(true),
			IdempotentHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPVMActionArgs) (*mcp.CallToolResult, any, error) {
		logger.Info("MCP: VM action '%s' requested for '%s'", args.Action, args.VMName)

		vmCtrl := controllers.NewVMController()
		var err error

		switch args.Action {
		case "start":
			err = vmCtrl.Start(args.VMName)
		case "stop":
			err = vmCtrl.Stop(args.VMName)
		case "restart":
			err = vmCtrl.Restart(args.VMName)
		case "pause":
			err = vmCtrl.Pause(args.VMName)
		case "resume":
			err = vmCtrl.Resume(args.VMName)
		case "hibernate":
			err = vmCtrl.Hibernate(args.VMName)
		case "force-stop":
			err = vmCtrl.ForceStop(args.VMName)
		default:
			return textResult(fmt.Sprintf("Unknown action: %s", args.Action)), nil, nil
		}

		if err != nil {
			logger.Error("MCP: VM action failed: %v", err)
			return textResult(fmt.Sprintf("Failed to %s VM: %v", args.Action, err)), nil, nil
		}

		return textResult(fmt.Sprintf("Successfully executed '%s' on VM '%s'", args.Action, args.VMName)), nil, nil
	})

	// Array control tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "array_action",
		Description: "Start or stop the Unraid array. CAUTION: Stopping the array will make all data inaccessible. Requires confirmation.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(true),
			IdempotentHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPArrayActionArgs) (*mcp.CallToolResult, any, error) {
		if !args.Confirm {
			return textResult("Action not confirmed. Set 'confirm' to true to execute this action."), nil, nil
		}

		logger.Info("MCP: Array action '%s' requested (confirmed)", args.Action)

		arrayCtrl := controllers.NewArrayController(s.ctx)
		var err error

		switch args.Action {
		case "start":
			err = arrayCtrl.StartArray()
		case "stop":
			err = arrayCtrl.StopArray()
		default:
			return textResult(fmt.Sprintf("Unknown action: %s", args.Action)), nil, nil
		}

		if err != nil {
			logger.Error("MCP: Array action failed: %v", err)
			return textResult(fmt.Sprintf("Failed to %s array: %v", args.Action, err)), nil, nil
		}

		return textResult(fmt.Sprintf("Successfully executed '%s' on array", args.Action)), nil, nil
	})

	// Parity check tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "parity_check_action",
		Description: "Start a parity check operation on the Unraid array",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(false),
			IdempotentHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPParityCheckArgs) (*mcp.CallToolResult, any, error) {
		logger.Info("MCP: Parity check action requested (correcting=%v)", args.Correcting)

		arrayCtrl := controllers.NewArrayController(s.ctx)
		err := arrayCtrl.StartParityCheck(args.Correcting)

		if err != nil {
			logger.Error("MCP: Parity check action failed: %v", err)
			return textResult(fmt.Sprintf("Failed to start parity check: %v", err)), nil, nil
		}

		checkType := "non-correcting"
		if args.Correcting {
			checkType = "correcting"
		}
		return textResult(fmt.Sprintf("Successfully started %s parity check", checkType)), nil, nil
	})

	// System reboot tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "system_reboot",
		Description: "Reboot the Unraid server. CAUTION: This will restart the entire system. Requires confirmation.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(true),
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPSystemActionArgs) (*mcp.CallToolResult, any, error) {
		if !args.Confirm {
			return textResult("Reboot not confirmed. Set 'confirm' to true to execute this action."), nil, nil
		}

		logger.Info("MCP: System reboot requested (confirmed)")

		systemCtrl := controllers.NewSystemController(s.ctx)
		err := systemCtrl.Reboot()

		if err != nil {
			logger.Error("MCP: System reboot failed: %v", err)
			return textResult(fmt.Sprintf("Failed to initiate reboot: %v", err)), nil, nil
		}

		return textResult("System reboot initiated. The server will restart shortly."), nil, nil
	})

	// System shutdown tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "system_shutdown",
		Description: "Shutdown the Unraid server. CAUTION: This will power off the entire system. Requires confirmation.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(true),
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPSystemActionArgs) (*mcp.CallToolResult, any, error) {
		if !args.Confirm {
			return textResult("Shutdown not confirmed. Set 'confirm' to true to execute this action."), nil, nil
		}

		logger.Info("MCP: System shutdown requested (confirmed)")

		systemCtrl := controllers.NewSystemController(s.ctx)
		err := systemCtrl.Shutdown()

		if err != nil {
			logger.Error("MCP: System shutdown failed: %v", err)
			return textResult(fmt.Sprintf("Failed to initiate shutdown: %v", err)), nil, nil
		}

		return textResult("System shutdown initiated. The server will power off shortly."), nil, nil
	})

	// Parity check stop tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "parity_check_stop",
		Description: "Stop a running parity check operation",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(false),
			IdempotentHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		logger.Info("MCP: Parity check stop requested")

		arrayCtrl := controllers.NewArrayController(s.ctx)
		err := arrayCtrl.StopParityCheck()

		if err != nil {
			logger.Error("MCP: Parity check stop failed: %v", err)
			return textResult(fmt.Sprintf("Failed to stop parity check: %v", err)), nil, nil
		}

		return textResult("Parity check stopped successfully"), nil, nil
	})

	// Parity check pause tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "parity_check_pause",
		Description: "Pause a running parity check operation",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(false),
			IdempotentHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		logger.Info("MCP: Parity check pause requested")

		arrayCtrl := controllers.NewArrayController(s.ctx)
		err := arrayCtrl.PauseParityCheck()

		if err != nil {
			logger.Error("MCP: Parity check pause failed: %v", err)
			return textResult(fmt.Sprintf("Failed to pause parity check: %v", err)), nil, nil
		}

		return textResult("Parity check paused successfully"), nil, nil
	})

	// Parity check resume tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "parity_check_resume",
		Description: "Resume a paused parity check operation",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(false),
			IdempotentHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
		logger.Info("MCP: Parity check resume requested")

		arrayCtrl := controllers.NewArrayController(s.ctx)
		err := arrayCtrl.ResumeParityCheck()

		if err != nil {
			logger.Error("MCP: Parity check resume failed: %v", err)
			return textResult(fmt.Sprintf("Failed to resume parity check: %v", err)), nil, nil
		}

		return textResult("Parity check resumed successfully"), nil, nil
	})

	// Disk spin down tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "disk_spin_down",
		Description: "Spin down a specific disk to save power. The disk will spin up automatically when accessed.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(false),
			IdempotentHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPDiskArgs) (*mcp.CallToolResult, any, error) {
		if args.DiskID == "" {
			return textResult("disk_id is required"), nil, nil
		}

		logger.Info("MCP: Disk spin down requested for '%s'", args.DiskID)

		arrayCtrl := controllers.NewArrayController(s.ctx)
		err := arrayCtrl.SpinDownDisk(args.DiskID)

		if err != nil {
			logger.Error("MCP: Disk spin down failed: %v", err)
			return textResult(fmt.Sprintf("Failed to spin down disk: %v", err)), nil, nil
		}

		return textResult(fmt.Sprintf("Disk '%s' spin down initiated", args.DiskID)), nil, nil
	})

	// Disk spin up tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "disk_spin_up",
		Description: "Spin up a specific disk that is in standby mode",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(false),
			IdempotentHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPDiskArgs) (*mcp.CallToolResult, any, error) {
		if args.DiskID == "" {
			return textResult("disk_id is required"), nil, nil
		}

		logger.Info("MCP: Disk spin up requested for '%s'", args.DiskID)

		arrayCtrl := controllers.NewArrayController(s.ctx)
		err := arrayCtrl.SpinUpDisk(args.DiskID)

		if err != nil {
			logger.Error("MCP: Disk spin up failed: %v", err)
			return textResult(fmt.Sprintf("Failed to spin up disk: %v", err)), nil, nil
		}

		return textResult(fmt.Sprintf("Disk '%s' spin up initiated", args.DiskID)), nil, nil
	})

	// Execute user script tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "execute_user_script",
		Description: "Execute a user script from the User Scripts plugin. Requires confirmation for safety.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(true),
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPUserScriptArgs) (*mcp.CallToolResult, any, error) {
		if !args.Confirm {
			return textResult("Script execution not confirmed. Set 'confirm' to true to execute."), nil, nil
		}

		if args.ScriptName == "" {
			return textResult("script_name is required"), nil, nil
		}

		logger.Info("MCP: User script execution requested for '%s' (confirmed)", args.ScriptName)

		response, err := controllers.ExecuteUserScript(args.ScriptName, false, true)
		if err != nil {
			logger.Error("MCP: User script execution failed: %v", err)
			return textResult(fmt.Sprintf("Failed to execute script: %v", err)), nil, nil
		}

		return jsonResult(response)
	})

	// Collector control tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "collector_action",
		Description: "Enable or disable a data collector at runtime. Note: some collectors like 'system' are required and cannot be disabled.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(false),
			IdempotentHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPCollectorControlArgs) (*mcp.CallToolResult, any, error) {
		if args.CollectorName == "" {
			return textResult("collector_name is required"), nil, nil
		}

		logger.Info("MCP: Collector action '%s' requested for '%s'", args.Action, args.CollectorName)

		var err error
		switch args.Action {
		case "enable":
			err = s.cacheProvider.EnableCollector(args.CollectorName)
		case "disable":
			err = s.cacheProvider.DisableCollector(args.CollectorName)
		default:
			return textResult(fmt.Sprintf("Unknown action: %s. Use 'enable' or 'disable'", args.Action)), nil, nil
		}

		if err != nil {
			logger.Error("MCP: Collector action failed: %v", err)
			return textResult(fmt.Sprintf("Failed to %s collector: %v", args.Action, err)), nil, nil
		}

		status, _ := s.cacheProvider.GetCollectorStatus(args.CollectorName)
		return jsonResult(map[string]interface{}{
			"success":   true,
			"message":   fmt.Sprintf("Collector '%s' %sd successfully", args.CollectorName, args.Action),
			"collector": status,
		})
	})

	// Update collector interval tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "update_collector_interval",
		Description: "Update the collection interval for a specific collector. Interval must be between 5 and 3600 seconds.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(false),
			IdempotentHint:  true,
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args dto.MCPCollectorIntervalArgs) (*mcp.CallToolResult, any, error) {
		if args.CollectorName == "" {
			return textResult("collector_name is required"), nil, nil
		}

		if args.Interval < 5 || args.Interval > 3600 {
			return textResult("interval must be between 5 and 3600 seconds"), nil, nil
		}

		logger.Info("MCP: Updating collector '%s' interval to %d seconds", args.CollectorName, args.Interval)

		err := s.cacheProvider.UpdateCollectorInterval(args.CollectorName, args.Interval)
		if err != nil {
			logger.Error("MCP: Collector interval update failed: %v", err)
			return textResult(fmt.Sprintf("Failed to update interval: %v", err)), nil, nil
		}

		status, _ := s.cacheProvider.GetCollectorStatus(args.CollectorName)
		return jsonResult(map[string]interface{}{
			"success":   true,
			"message":   fmt.Sprintf("Collector '%s' interval updated to %d seconds", args.CollectorName, args.Interval),
			"collector": status,
		})
	})

	logger.Debug("MCP control tools registered (14 tools with safety annotations)")
}

// registerResources registers MCP resources for real-time data access.
func (s *Server) registerResources() {
	// System resource
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "unraid://system",
		Name:        "system-info",
		Description: "Real-time Unraid system information",
		MIMEType:    "application/json",
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		info := s.cacheProvider.GetSystemCache()
		if info == nil {
			return resourceResult("unraid://system", `{"error": "System information not available"}`)
		}
		data, _ := json.Marshal(info)
		return resourceResult("unraid://system", string(data))
	})

	// Array resource
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "unraid://array",
		Name:        "array-status",
		Description: "Real-time Unraid array status",
		MIMEType:    "application/json",
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		status := s.cacheProvider.GetArrayCache()
		if status == nil {
			return resourceResult("unraid://array", `{"error": "Array status not available"}`)
		}
		data, _ := json.Marshal(status)
		return resourceResult("unraid://array", string(data))
	})

	// Containers resource
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "unraid://containers",
		Name:        "docker-containers",
		Description: "Real-time Docker container list and status",
		MIMEType:    "application/json",
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		containers := s.cacheProvider.GetDockerCache()
		if containers == nil {
			return resourceResult("unraid://containers", `{"error": "Container information not available"}`)
		}
		data, _ := json.Marshal(containers)
		return resourceResult("unraid://containers", string(data))
	})

	// VMs resource
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "unraid://vms",
		Name:        "virtual-machines",
		Description: "Real-time virtual machine list and status",
		MIMEType:    "application/json",
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		vms := s.cacheProvider.GetVMsCache()
		if vms == nil {
			return resourceResult("unraid://vms", `{"error": "VM information not available"}`)
		}
		data, _ := json.Marshal(vms)
		return resourceResult("unraid://vms", string(data))
	})

	// Disks resource
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "unraid://disks",
		Name:        "disk-status",
		Description: "Real-time disk information and health status",
		MIMEType:    "application/json",
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		disks := s.cacheProvider.GetDisksCache()
		if disks == nil {
			return resourceResult("unraid://disks", `{"error": "Disk information not available"}`)
		}
		data, _ := json.Marshal(disks)
		return resourceResult("unraid://disks", string(data))
	})

	logger.Debug("MCP resources registered (5 resources)")
}

// registerPrompts registers MCP prompts for guided interactions.
func (s *Server) registerPrompts() {
	// Disk health analysis prompt
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "analyze_disk_health",
		Description: "Analyze the health status of all disks and provide recommendations",
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		disks := s.cacheProvider.GetDisksCache()
		if disks == nil {
			return &mcp.GetPromptResult{
				Description: "Disk health analysis",
				Messages: []*mcp.PromptMessage{{
					Role:    "assistant",
					Content: &mcp.TextContent{Text: "Disk information is not available. Please wait for the system to collect disk data."},
				}},
			}, nil
		}

		data, _ := json.MarshalIndent(disks, "", "  ")
		return &mcp.GetPromptResult{
			Description: "Disk health analysis",
			Messages: []*mcp.PromptMessage{{
				Role: "user",
				Content: &mcp.TextContent{Text: fmt.Sprintf(`Please analyze the following Unraid disk information and provide:
1. Overall health assessment for each disk
2. Any SMART warnings or concerns
3. Recommendations for maintenance or replacement
4. Temperature analysis

Disk Data:
%s`, string(data))},
			}},
		}, nil
	})

	// System overview prompt
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "system_overview",
		Description: "Get a comprehensive overview of the Unraid system status",
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		system := s.cacheProvider.GetSystemCache()
		array := s.cacheProvider.GetArrayCache()
		containers := s.cacheProvider.GetDockerCache()
		vms := s.cacheProvider.GetVMsCache()

		containerCount := 0
		if containers != nil {
			containerCount = len(containers)
		}
		vmCount := 0
		if vms != nil {
			vmCount = len(vms)
		}

		overview := map[string]interface{}{
			"system":     system,
			"array":      array,
			"containers": containerCount,
			"vms":        vmCount,
			"timestamp":  time.Now(),
		}

		data, _ := json.MarshalIndent(overview, "", "  ")
		return &mcp.GetPromptResult{
			Description: "System overview",
			Messages: []*mcp.PromptMessage{{
				Role: "user",
				Content: &mcp.TextContent{Text: fmt.Sprintf(`Please provide a summary of the following Unraid system status:
1. System health and resource usage
2. Array status and capacity
3. Running services (containers and VMs)
4. Any issues or recommendations

System Data:
%s`, string(data))},
			}},
		}, nil
	})

	// Troubleshooting prompt
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "troubleshoot_issue",
		Description: "Help troubleshoot common Unraid issues",
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Troubleshooting assistant",
			Messages: []*mcp.PromptMessage{{
				Role: "assistant",
				Content: &mcp.TextContent{Text: `I'm here to help troubleshoot your Unraid server. To get started, please describe:

1. What issue are you experiencing?
2. When did it start?
3. Any recent changes to the system?

I can help with:
- Array issues (parity errors, disk problems)
- Docker container issues
- VM problems
- Network connectivity
- Performance issues
- Temperature/hardware concerns

Please describe your issue and I'll gather the relevant system information to help diagnose it.`},
			}},
		}, nil
	})

	logger.Debug("MCP prompts registered (3 prompts)")
}

// textResult creates a tool result with text content.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

// jsonResult creates a tool result with JSON-formatted text content.
func jsonResult(data interface{}) (*mcp.CallToolResult, any, error) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error formatting response: %v", err)}},
			IsError: true,
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(jsonData)}},
	}, nil, nil
}

// resourceResult creates a resource read result with text content.
func resourceResult(uri, text string) (*mcp.ReadResourceResult, error) {
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: "application/json",
			Text:     text,
		}},
	}, nil
}
