// Package mcp provides a Model Context Protocol (MCP) server implementation for the Unraid Management Agent.
// It exposes Unraid system monitoring and control capabilities to AI agents via the standardized MCP protocol.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	mcp "github.com/metoro-io/mcp-golang"

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

// TransportType represents the type of MCP transport to use.
type TransportType string

const (
	// TransportHTTP is the standard HTTP transport (recommended for remote AI agents).
	TransportHTTP TransportType = "http"
	// TransportSSE is the Server-Sent Events transport (ideal for real-time notifications).
	TransportSSE TransportType = "sse"
	// TransportStdio is the stdio transport (for local AI client integrations like Claude Desktop).
	TransportStdio TransportType = "stdio"
)

// Server represents the MCP server that exposes Unraid capabilities to AI agents.
type Server struct {
	ctx            *domain.Context
	mcpServer      *mcp.Server
	transport      *StdHTTPTransport
	sseTransport   *SSETransport
	stdioTransport *StdioTransport
	transportType  TransportType
	cacheProvider  CacheProvider
	mu             sync.RWMutex
}

// NewServer creates a new MCP server instance with HTTP transport (default).
func NewServer(ctx *domain.Context, cacheProvider CacheProvider) *Server {
	return &Server{
		ctx:           ctx,
		cacheProvider: cacheProvider,
		transportType: TransportHTTP,
	}
}

// NewServerWithTransport creates a new MCP server instance with the specified transport type.
func NewServerWithTransport(ctx *domain.Context, cacheProvider CacheProvider, transportType TransportType) *Server {
	return &Server{
		ctx:           ctx,
		cacheProvider: cacheProvider,
		transportType: transportType,
	}
}

// Initialize sets up the MCP server with all tools and resources.
func (s *Server) Initialize() error {
	return s.InitializeWithTransport(s.transportType, nil, nil)
}

// InitializeWithTransport sets up the MCP server with the specified transport.
// For stdio transport, provide reader and writer (e.g., os.Stdin and os.Stdout).
func (s *Server) InitializeWithTransport(transportType TransportType, reader, writer interface{}) error {
	s.transportType = transportType

	switch transportType {
	case TransportHTTP:
		s.transport = NewStdHTTPTransport()
		s.mcpServer = mcp.NewServer(s.transport,
			mcp.WithName("unraid-management-agent"),
			mcp.WithVersion(s.ctx.Config.Version),
		)
	case TransportSSE:
		s.sseTransport = NewSSETransport()
		s.mcpServer = mcp.NewServer(s.sseTransport,
			mcp.WithName("unraid-management-agent"),
			mcp.WithVersion(s.ctx.Config.Version),
		)
	case TransportStdio:
		if reader == nil || writer == nil {
			return fmt.Errorf("stdio transport requires reader and writer")
		}
		r, ok := reader.(io.Reader)
		if !ok {
			return fmt.Errorf("reader must implement io.Reader")
		}
		w, ok := writer.(io.Writer)
		if !ok {
			return fmt.Errorf("writer must implement io.Writer")
		}
		s.stdioTransport = NewStdioTransport(r, w)
		s.mcpServer = mcp.NewServer(s.stdioTransport,
			mcp.WithName("unraid-management-agent"),
			mcp.WithVersion(s.ctx.Config.Version),
		)
	default:
		return fmt.Errorf("unsupported transport type: %s", transportType)
	}

	// Register all tools
	if err := s.registerMonitoringTools(); err != nil {
		return fmt.Errorf("failed to register monitoring tools: %w", err)
	}

	if err := s.registerControlTools(); err != nil {
		return fmt.Errorf("failed to register control tools: %w", err)
	}

	if err := s.registerResources(); err != nil {
		return fmt.Errorf("failed to register resources: %w", err)
	}

	if err := s.registerPrompts(); err != nil {
		return fmt.Errorf("failed to register prompts: %w", err)
	}

	// Start the MCP server to set up protocol handlers and connect transport
	// This registers handlers for initialize, tools/list, tools/call, etc.
	// For HTTP/SSE transports, this doesn't block - it just sets up the routing
	if err := s.mcpServer.Serve(); err != nil {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	logger.Info("MCP server initialized with %s transport, tools, resources, and prompts", transportType)
	return nil
}

// GetHandler returns the HTTP handler for the MCP endpoint (HTTP transport only).
func (s *Server) GetHandler() http.HandlerFunc {
	if s.transport == nil {
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "HTTP transport not configured", http.StatusInternalServerError)
		}
	}
	return http.HandlerFunc(s.transport.Handler())
}

// GetSSEHandler returns the SSE handler for Server-Sent Events connections.
func (s *Server) GetSSEHandler() http.HandlerFunc {
	if s.sseTransport == nil {
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "SSE transport not configured", http.StatusInternalServerError)
		}
	}
	return s.sseTransport.SSEHandler()
}

// GetSSEPostHandler returns the POST handler for SSE transport message submission.
func (s *Server) GetSSEPostHandler() http.HandlerFunc {
	if s.sseTransport == nil {
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "SSE transport not configured", http.StatusInternalServerError)
		}
	}
	return s.sseTransport.PostHandler()
}

// StartStdioTransport starts the stdio transport (blocks until done).
// This should be called in a goroutine if you want non-blocking operation.
func (s *Server) StartStdioTransport(ctx context.Context) error {
	if s.stdioTransport == nil {
		return fmt.Errorf("stdio transport not configured")
	}
	return s.stdioTransport.Start(ctx)
}

// BroadcastSSE sends an event to all connected SSE clients.
func (s *Server) BroadcastSSE(event string, data interface{}) error {
	if s.sseTransport == nil {
		return fmt.Errorf("SSE transport not configured")
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}
	s.sseTransport.Broadcast(event, string(jsonData))
	return nil
}

// GetTransportType returns the current transport type.
func (s *Server) GetTransportType() TransportType {
	return s.transportType
}

// GetSSEClientCount returns the number of connected SSE clients.
func (s *Server) GetSSEClientCount() int {
	if s.sseTransport == nil {
		return 0
	}
	return s.sseTransport.ClientCount()
}

// registerMonitoringTools registers all read-only monitoring tools.
func (s *Server) registerMonitoringTools() error {
	// System information tool
	if err := s.mcpServer.RegisterTool("get_system_info",
		"Get comprehensive Unraid system information including hostname, CPU usage, RAM usage, temperatures, and uptime",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			info := s.cacheProvider.GetSystemCache()
			if info == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("System information not available yet")), nil
			}
			return s.jsonResponse(info)
		}); err != nil {
		return err
	}

	// Array status tool
	if err := s.mcpServer.RegisterTool("get_array_status",
		"Get Unraid array status including state, capacity, parity information, and disk assignments",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			status := s.cacheProvider.GetArrayCache()
			if status == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Array status not available yet")), nil
			}
			return s.jsonResponse(status)
		}); err != nil {
		return err
	}

	// List all disks tool
	if err := s.mcpServer.RegisterTool("list_disks",
		"List all disks in the Unraid server including array disks, cache, and unassigned devices with their health status",
		func(args dto.MCPDiskArgs) (*mcp.ToolResponse, error) {
			disks := s.cacheProvider.GetDisksCache()
			if disks == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Disk information not available yet")), nil
			}
			return s.jsonResponse(disks)
		}); err != nil {
		return err
	}

	// Get specific disk info tool
	if err := s.mcpServer.RegisterTool("get_disk_info",
		"Get detailed information about a specific disk including SMART data and health status",
		func(args dto.MCPDiskArgs) (*mcp.ToolResponse, error) {
			disks := s.cacheProvider.GetDisksCache()
			if disks == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Disk information not available yet")), nil
			}
			for _, disk := range disks {
				if disk.Device == args.DiskID || disk.Name == args.DiskID || disk.ID == args.DiskID {
					return s.jsonResponse(disk)
				}
			}
			return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Disk '%s' not found", args.DiskID))), nil
		}); err != nil {
		return err
	}

	// List shares tool
	if err := s.mcpServer.RegisterTool("list_shares",
		"List all network shares configured on the Unraid server with their settings and usage",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			shares := s.cacheProvider.GetSharesCache()
			if shares == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Share information not available yet")), nil
			}
			return s.jsonResponse(shares)
		}); err != nil {
		return err
	}

	// List Docker containers tool
	if err := s.mcpServer.RegisterTool("list_containers",
		"List all Docker containers on the Unraid server with their status, resource usage, and configuration",
		func(args dto.MCPContainerListArgs) (*mcp.ToolResponse, error) {
			containers := s.cacheProvider.GetDockerCache()
			if containers == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Container information not available yet")), nil
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
				return s.jsonResponse(filtered)
			}
			return s.jsonResponse(containers)
		}); err != nil {
		return err
	}

	// Get specific container info tool
	if err := s.mcpServer.RegisterTool("get_container_info",
		"Get detailed information about a specific Docker container",
		func(args dto.MCPContainerArgs) (*mcp.ToolResponse, error) {
			containers := s.cacheProvider.GetDockerCache()
			if containers == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Container information not available yet")), nil
			}
			for _, c := range containers {
				if c.ID == args.ContainerID || c.Name == args.ContainerID {
					return s.jsonResponse(c)
				}
			}
			return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Container '%s' not found", args.ContainerID))), nil
		}); err != nil {
		return err
	}

	// List VMs tool
	if err := s.mcpServer.RegisterTool("list_vms",
		"List all virtual machines on the Unraid server with their status and configuration",
		func(args dto.MCPVMListArgs) (*mcp.ToolResponse, error) {
			vms := s.cacheProvider.GetVMsCache()
			if vms == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("VM information not available yet")), nil
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
				return s.jsonResponse(filtered)
			}
			return s.jsonResponse(vms)
		}); err != nil {
		return err
	}

	// Get specific VM info tool
	if err := s.mcpServer.RegisterTool("get_vm_info",
		"Get detailed information about a specific virtual machine",
		func(args dto.MCPVMArgs) (*mcp.ToolResponse, error) {
			vms := s.cacheProvider.GetVMsCache()
			if vms == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("VM information not available yet")), nil
			}
			for _, vm := range vms {
				if vm.Name == args.VMName || vm.ID == args.VMName {
					return s.jsonResponse(vm)
				}
			}
			return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("VM '%s' not found", args.VMName))), nil
		}); err != nil {
		return err
	}

	// UPS status tool
	if err := s.mcpServer.RegisterTool("get_ups_status",
		"Get UPS (Uninterruptible Power Supply) status including battery level, load, and runtime",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			ups := s.cacheProvider.GetUPSCache()
			if ups == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("UPS not configured or information not available")), nil
			}
			return s.jsonResponse(ups)
		}); err != nil {
		return err
	}

	// GPU metrics tool
	if err := s.mcpServer.RegisterTool("get_gpu_metrics",
		"Get GPU metrics including utilization, temperature, and memory usage for all GPUs",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			gpus := s.cacheProvider.GetGPUCache()
			if gpus == nil || len(gpus) == 0 {
				return mcp.NewToolResponse(mcp.NewTextContent("No GPUs detected or GPU information not available")), nil
			}
			return s.jsonResponse(gpus)
		}); err != nil {
		return err
	}

	// Network interfaces tool
	if err := s.mcpServer.RegisterTool("get_network_info",
		"Get network interface information including IP addresses, speeds, and traffic statistics",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			network := s.cacheProvider.GetNetworkCache()
			if network == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Network information not available yet")), nil
			}
			return s.jsonResponse(network)
		}); err != nil {
		return err
	}

	// Hardware info tool
	if err := s.mcpServer.RegisterTool("get_hardware_info",
		"Get detailed hardware information including motherboard, CPU, and memory details from DMI/SMBIOS",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			hardware := s.cacheProvider.GetHardwareCache()
			if hardware == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Hardware information not available yet")), nil
			}
			return s.jsonResponse(hardware)
		}); err != nil {
		return err
	}

	// Registration/License info tool
	if err := s.mcpServer.RegisterTool("get_registration",
		"Get Unraid license/registration information including license type and key status",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			reg := s.cacheProvider.GetRegistrationCache()
			if reg == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Registration information not available yet")), nil
			}
			return s.jsonResponse(reg)
		}); err != nil {
		return err
	}

	// Notifications tool
	if err := s.mcpServer.RegisterTool("get_notifications",
		"Get system notifications including alerts, warnings, and informational messages",
		func(args dto.MCPNotificationArgs) (*mcp.ToolResponse, error) {
			notifications := s.cacheProvider.GetNotificationsCache()
			if notifications == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Notifications not available yet")), nil
			}
			return s.jsonResponse(notifications)
		}); err != nil {
		return err
	}

	// ZFS pools tool
	if err := s.mcpServer.RegisterTool("get_zfs_pools",
		"Get ZFS pool information including health status, capacity, and configuration",
		func(args dto.MCPZFSPoolArgs) (*mcp.ToolResponse, error) {
			pools := s.cacheProvider.GetZFSPoolsCache()
			if pools == nil || len(pools) == 0 {
				return mcp.NewToolResponse(mcp.NewTextContent("No ZFS pools configured or ZFS information not available")), nil
			}

			// Return specific pool if name provided
			if args.PoolName != "" {
				for _, pool := range pools {
					if pool.Name == args.PoolName {
						return s.jsonResponse(pool)
					}
				}
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("ZFS pool '%s' not found", args.PoolName))), nil
			}
			return s.jsonResponse(pools)
		}); err != nil {
		return err
	}

	// ZFS datasets tool
	if err := s.mcpServer.RegisterTool("get_zfs_datasets",
		"Get ZFS dataset information including snapshots, quotas, and usage",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			datasets := s.cacheProvider.GetZFSDatasetsCache()
			if datasets == nil || len(datasets) == 0 {
				return mcp.NewToolResponse(mcp.NewTextContent("No ZFS datasets found or ZFS information not available")), nil
			}
			return s.jsonResponse(datasets)
		}); err != nil {
		return err
	}

	// ZFS snapshots tool
	if err := s.mcpServer.RegisterTool("get_zfs_snapshots",
		"Get ZFS snapshot information for all pools and datasets",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			snapshots := s.cacheProvider.GetZFSSnapshotsCache()
			if snapshots == nil || len(snapshots) == 0 {
				return mcp.NewToolResponse(mcp.NewTextContent("No ZFS snapshots found or ZFS information not available")), nil
			}
			return s.jsonResponse(snapshots)
		}); err != nil {
		return err
	}

	// ZFS ARC stats tool
	if err := s.mcpServer.RegisterTool("get_zfs_arc_stats",
		"Get ZFS ARC (Adaptive Replacement Cache) statistics including hit ratio and memory usage",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			arcStats := s.cacheProvider.GetZFSARCStatsCache()
			if arcStats == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("ZFS ARC statistics not available")), nil
			}
			return s.jsonResponse(arcStats)
		}); err != nil {
		return err
	}

	// Unassigned devices tool
	if err := s.mcpServer.RegisterTool("get_unassigned_devices",
		"Get information about unassigned (non-array) devices including USB drives and unassigned disks",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			unassigned := s.cacheProvider.GetUnassignedCache()
			if unassigned == nil || len(unassigned.Devices) == 0 {
				return mcp.NewToolResponse(mcp.NewTextContent("No unassigned devices found or Unassigned Devices plugin not installed")), nil
			}
			return s.jsonResponse(unassigned)
		}); err != nil {
		return err
	}

	// NUT (Network UPS Tools) status tool
	if err := s.mcpServer.RegisterTool("get_nut_status",
		"Get detailed NUT (Network UPS Tools) status including all UPS variables and metrics",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			nut := s.cacheProvider.GetNUTCache()
			if nut == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("NUT not configured or NUT information not available")), nil
			}
			return s.jsonResponse(nut)
		}); err != nil {
		return err
	}

	// User scripts list tool
	if err := s.mcpServer.RegisterTool("list_user_scripts",
		"List all available user scripts from the User Scripts plugin",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			scripts, err := controllers.ListUserScripts()
			if err != nil {
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to list user scripts: %v", err))), nil
			}
			if len(scripts) == 0 {
				return mcp.NewToolResponse(mcp.NewTextContent("No user scripts found or User Scripts plugin not installed")), nil
			}
			return s.jsonResponse(scripts)
		}); err != nil {
		return err
	}

	// Parity history tool
	if err := s.mcpServer.RegisterTool("get_parity_history",
		"Get parity check history including past check dates, durations, speeds, and error counts",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			history := s.cacheProvider.GetParityHistoryCache()
			if history == nil || len(history.Records) == 0 {
				return mcp.NewToolResponse(mcp.NewTextContent("No parity check history available")), nil
			}
			return s.jsonResponse(history)
		}); err != nil {
		return err
	}

	// List log files tool
	if err := s.mcpServer.RegisterTool("list_log_files",
		"List all available log files on the Unraid server including system logs, Docker logs, and plugin logs",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			logs := s.cacheProvider.ListLogFiles()
			if len(logs) == 0 {
				return mcp.NewToolResponse(mcp.NewTextContent("No log files found")), nil
			}
			return s.jsonResponse(logs)
		}); err != nil {
		return err
	}

	// Get log content tool
	if err := s.mcpServer.RegisterTool("get_log_content",
		"Retrieve content from a specific log file with optional line limits. Returns the last N lines (tail behavior) or specific range.",
		func(args dto.MCPLogArgs) (*mcp.ToolResponse, error) {
			if args.LogFile == "" {
				return mcp.NewToolResponse(mcp.NewTextContent("log_file is required")), nil
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
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to read log: %v", err))), nil
			}
			return s.jsonResponse(content)
		}); err != nil {
		return err
	}

	// Get syslog tool (convenience wrapper)
	if err := s.mcpServer.RegisterTool("get_syslog",
		"Get the system log (syslog) - convenient shortcut for viewing system messages",
		func(args dto.MCPLogArgs) (*mcp.ToolResponse, error) {
			lines := args.Lines
			if lines <= 0 {
				lines = 100
			}
			if lines > 1000 {
				lines = 1000
			}

			content, err := s.cacheProvider.GetLogContent("/var/log/syslog", fmt.Sprintf("%d", lines), "")
			if err != nil {
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to read syslog: %v", err))), nil
			}
			return s.jsonResponse(content)
		}); err != nil {
		return err
	}

	// Get Docker log tool (convenience wrapper)
	if err := s.mcpServer.RegisterTool("get_docker_log",
		"Get the Docker daemon log - useful for diagnosing container issues",
		func(args dto.MCPLogArgs) (*mcp.ToolResponse, error) {
			lines := args.Lines
			if lines <= 0 {
				lines = 100
			}
			if lines > 1000 {
				lines = 1000
			}

			content, err := s.cacheProvider.GetLogContent("/var/log/docker.log", fmt.Sprintf("%d", lines), "")
			if err != nil {
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to read Docker log: %v", err))), nil
			}
			return s.jsonResponse(content)
		}); err != nil {
		return err
	}

	// List collectors status tool
	if err := s.mcpServer.RegisterTool("list_collectors",
		"List all data collectors with their status, intervals, and runtime information",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			status := s.cacheProvider.GetCollectorsStatus()
			return s.jsonResponse(status)
		}); err != nil {
		return err
	}

	// Get specific collector status tool
	if err := s.mcpServer.RegisterTool("get_collector_status",
		"Get detailed status of a specific collector including enabled state, interval, last run time, and error count",
		func(args dto.MCPCollectorArgs) (*mcp.ToolResponse, error) {
			if args.CollectorName == "" {
				return mcp.NewToolResponse(mcp.NewTextContent("collector_name is required")), nil
			}

			status, err := s.cacheProvider.GetCollectorStatus(args.CollectorName)
			if err != nil {
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to get collector status: %v", err))), nil
			}
			return s.jsonResponse(status)
		}); err != nil {
		return err
	}

	// Get system settings tool
	if err := s.mcpServer.RegisterTool("get_system_settings",
		"Get system configuration settings including server name, timezone, security mode, and date/time formats",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			settings := s.cacheProvider.GetSystemSettings()
			if settings == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("System settings not available")), nil
			}
			return s.jsonResponse(settings)
		}); err != nil {
		return err
	}

	// Get Docker settings tool
	if err := s.mcpServer.RegisterTool("get_docker_settings",
		"Get Docker configuration settings including enabled state, image path, and network configuration",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			settings := s.cacheProvider.GetDockerSettings()
			if settings == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Docker settings not available")), nil
			}
			return s.jsonResponse(settings)
		}); err != nil {
		return err
	}

	// Get VM settings tool
	if err := s.mcpServer.RegisterTool("get_vm_settings",
		"Get VM Manager configuration settings including enabled state, PCI/USB passthrough devices",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			settings := s.cacheProvider.GetVMSettings()
			if settings == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("VM settings not available")), nil
			}
			return s.jsonResponse(settings)
		}); err != nil {
		return err
	}

	// Get disk settings tool
	if err := s.mcpServer.RegisterTool("get_disk_settings",
		"Get disk configuration settings including spindown delay, auto-start, spinup groups, and default filesystem",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			settings := s.cacheProvider.GetDiskSettings()
			if settings == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Disk settings not available")), nil
			}
			return s.jsonResponse(settings)
		}); err != nil {
		return err
	}

	// Get share config tool
	if err := s.mcpServer.RegisterTool("get_share_config",
		"Get detailed share configuration including allocation method, cache settings, disk inclusion/exclusion, and export settings",
		func(args dto.MCPShareArgs) (*mcp.ToolResponse, error) {
			if args.ShareName == "" {
				return mcp.NewToolResponse(mcp.NewTextContent("share_name is required")), nil
			}
			config := s.cacheProvider.GetShareConfig(args.ShareName)
			if config == nil {
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Share config not found: %s", args.ShareName))), nil
			}
			return s.jsonResponse(config)
		}); err != nil {
		return err
	}

	// Get network access URLs tool
	if err := s.mcpServer.RegisterTool("get_network_access_urls",
		"Get all available methods to access the Unraid server including LAN, WAN, WireGuard, mDNS, and IPv6 addresses",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			urls := s.cacheProvider.GetNetworkAccessURLs()
			if urls == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Network access URLs not available")), nil
			}
			return s.jsonResponse(urls)
		}); err != nil {
		return err
	}

	// Get health status tool
	if err := s.mcpServer.RegisterTool("get_health_status",
		"Get a quick health check summary of the Unraid server including API status, uptime, and basic connectivity",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			health := s.cacheProvider.GetHealthStatus()
			return s.jsonResponse(health)
		}); err != nil {
		return err
	}

	// Get notifications overview tool
	if err := s.mcpServer.RegisterTool("get_notifications_overview",
		"Get a summary of notification counts by type (unread/archive) and importance level (alert/warning/info)",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			notifications := s.cacheProvider.GetNotificationsCache()
			if notifications == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("Notifications not available")), nil
			}
			return s.jsonResponse(notifications.Overview)
		}); err != nil {
		return err
	}

	// Search containers tool
	if err := s.mcpServer.RegisterTool("search_containers",
		"Search Docker containers by name, image, or state. Returns matching containers.",
		func(args dto.MCPSearchArgs) (*mcp.ToolResponse, error) {
			if args.Query == "" {
				return mcp.NewToolResponse(mcp.NewTextContent("query is required")), nil
			}
			containers := s.cacheProvider.GetDockerCache()
			if containers == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("No containers available")), nil
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
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("No containers matching '%s' found", args.Query))), nil
			}
			return s.jsonResponse(map[string]interface{}{
				"query":   args.Query,
				"count":   len(matches),
				"results": matches,
			})
		}); err != nil {
		return err
	}

	// Search VMs tool
	if err := s.mcpServer.RegisterTool("search_vms",
		"Search virtual machines by name or state. Returns matching VMs.",
		func(args dto.MCPSearchArgs) (*mcp.ToolResponse, error) {
			if args.Query == "" {
				return mcp.NewToolResponse(mcp.NewTextContent("query is required")), nil
			}
			vms := s.cacheProvider.GetVMsCache()
			if vms == nil {
				return mcp.NewToolResponse(mcp.NewTextContent("No VMs available")), nil
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
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("No VMs matching '%s' found", args.Query))), nil
			}
			return s.jsonResponse(map[string]interface{}{
				"query":   args.Query,
				"count":   len(matches),
				"results": matches,
			})
		}); err != nil {
		return err
	}

	// Get diagnostic summary tool
	if err := s.mcpServer.RegisterTool("get_diagnostic_summary",
		"Get a comprehensive diagnostic summary including system health, array status, recent alerts, disk health, and resource usage",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
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

			return s.jsonResponse(summary)
		}); err != nil {
		return err
	}

	logger.Debug("MCP monitoring tools registered")
	return nil
}

// registerControlTools registers tools that can modify system state.
func (s *Server) registerControlTools() error {
	// Container control tool
	if err := s.mcpServer.RegisterTool("container_action",
		"Perform an action on a Docker container (start, stop, restart, pause, unpause). Use with caution.",
		func(args dto.MCPContainerActionArgs) (*mcp.ToolResponse, error) {
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
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Unknown action: %s", args.Action))), nil
			}

			if err != nil {
				logger.Error("MCP: Container action failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to %s container: %v", args.Action, err))), nil
			}

			return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Successfully executed '%s' on container '%s'", args.Action, args.ContainerID))), nil
		}); err != nil {
		return err
	}

	// VM control tool
	if err := s.mcpServer.RegisterTool("vm_action",
		"Perform an action on a virtual machine (start, stop, restart, pause, resume, hibernate, force-stop). Use with caution.",
		func(args dto.MCPVMActionArgs) (*mcp.ToolResponse, error) {
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
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Unknown action: %s", args.Action))), nil
			}

			if err != nil {
				logger.Error("MCP: VM action failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to %s VM: %v", args.Action, err))), nil
			}

			return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Successfully executed '%s' on VM '%s'", args.Action, args.VMName))), nil
		}); err != nil {
		return err
	}

	// Array control tool
	if err := s.mcpServer.RegisterTool("array_action",
		"Start or stop the Unraid array. CAUTION: Stopping the array will make all data inaccessible. Requires confirmation.",
		func(args dto.MCPArrayActionArgs) (*mcp.ToolResponse, error) {
			if !args.Confirm {
				return mcp.NewToolResponse(mcp.NewTextContent("Action not confirmed. Set 'confirm' to true to execute this action.")), nil
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
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Unknown action: %s", args.Action))), nil
			}

			if err != nil {
				logger.Error("MCP: Array action failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to %s array: %v", args.Action, err))), nil
			}

			return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Successfully executed '%s' on array", args.Action))), nil
		}); err != nil {
		return err
	}

	// Parity check tool
	if err := s.mcpServer.RegisterTool("parity_check_action",
		"Start a parity check operation on the Unraid array",
		func(args dto.MCPParityCheckArgs) (*mcp.ToolResponse, error) {
			logger.Info("MCP: Parity check action requested (correcting=%v)", args.Correcting)

			arrayCtrl := controllers.NewArrayController(s.ctx)
			err := arrayCtrl.StartParityCheck(args.Correcting)

			if err != nil {
				logger.Error("MCP: Parity check action failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to start parity check: %v", err))), nil
			}

			checkType := "non-correcting"
			if args.Correcting {
				checkType = "correcting"
			}
			return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Successfully started %s parity check", checkType))), nil
		}); err != nil {
		return err
	}

	// System reboot tool
	if err := s.mcpServer.RegisterTool("system_reboot",
		"Reboot the Unraid server. CAUTION: This will restart the entire system. Requires confirmation.",
		func(args dto.MCPSystemActionArgs) (*mcp.ToolResponse, error) {
			if !args.Confirm {
				return mcp.NewToolResponse(mcp.NewTextContent("Reboot not confirmed. Set 'confirm' to true to execute this action.")), nil
			}

			logger.Info("MCP: System reboot requested (confirmed)")

			systemCtrl := controllers.NewSystemController(s.ctx)
			err := systemCtrl.Reboot()

			if err != nil {
				logger.Error("MCP: System reboot failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to initiate reboot: %v", err))), nil
			}

			return mcp.NewToolResponse(mcp.NewTextContent("System reboot initiated. The server will restart shortly.")), nil
		}); err != nil {
		return err
	}

	// System shutdown tool
	if err := s.mcpServer.RegisterTool("system_shutdown",
		"Shutdown the Unraid server. CAUTION: This will power off the entire system. Requires confirmation.",
		func(args dto.MCPSystemActionArgs) (*mcp.ToolResponse, error) {
			if !args.Confirm {
				return mcp.NewToolResponse(mcp.NewTextContent("Shutdown not confirmed. Set 'confirm' to true to execute this action.")), nil
			}

			logger.Info("MCP: System shutdown requested (confirmed)")

			systemCtrl := controllers.NewSystemController(s.ctx)
			err := systemCtrl.Shutdown()

			if err != nil {
				logger.Error("MCP: System shutdown failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to initiate shutdown: %v", err))), nil
			}

			return mcp.NewToolResponse(mcp.NewTextContent("System shutdown initiated. The server will power off shortly.")), nil
		}); err != nil {
		return err
	}

	// Parity check stop tool
	if err := s.mcpServer.RegisterTool("parity_check_stop",
		"Stop a running parity check operation",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			logger.Info("MCP: Parity check stop requested")

			arrayCtrl := controllers.NewArrayController(s.ctx)
			err := arrayCtrl.StopParityCheck()

			if err != nil {
				logger.Error("MCP: Parity check stop failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to stop parity check: %v", err))), nil
			}

			return mcp.NewToolResponse(mcp.NewTextContent("Parity check stopped successfully")), nil
		}); err != nil {
		return err
	}

	// Parity check pause tool
	if err := s.mcpServer.RegisterTool("parity_check_pause",
		"Pause a running parity check operation",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			logger.Info("MCP: Parity check pause requested")

			arrayCtrl := controllers.NewArrayController(s.ctx)
			err := arrayCtrl.PauseParityCheck()

			if err != nil {
				logger.Error("MCP: Parity check pause failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to pause parity check: %v", err))), nil
			}

			return mcp.NewToolResponse(mcp.NewTextContent("Parity check paused successfully")), nil
		}); err != nil {
		return err
	}

	// Parity check resume tool
	if err := s.mcpServer.RegisterTool("parity_check_resume",
		"Resume a paused parity check operation",
		func(args dto.MCPEmptyArgs) (*mcp.ToolResponse, error) {
			logger.Info("MCP: Parity check resume requested")

			arrayCtrl := controllers.NewArrayController(s.ctx)
			err := arrayCtrl.ResumeParityCheck()

			if err != nil {
				logger.Error("MCP: Parity check resume failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to resume parity check: %v", err))), nil
			}

			return mcp.NewToolResponse(mcp.NewTextContent("Parity check resumed successfully")), nil
		}); err != nil {
		return err
	}

	// Disk spin down tool
	if err := s.mcpServer.RegisterTool("disk_spin_down",
		"Spin down a specific disk to save power. The disk will spin up automatically when accessed.",
		func(args dto.MCPDiskArgs) (*mcp.ToolResponse, error) {
			if args.DiskID == "" {
				return mcp.NewToolResponse(mcp.NewTextContent("disk_id is required")), nil
			}

			logger.Info("MCP: Disk spin down requested for '%s'", args.DiskID)

			arrayCtrl := controllers.NewArrayController(s.ctx)
			err := arrayCtrl.SpinDownDisk(args.DiskID)

			if err != nil {
				logger.Error("MCP: Disk spin down failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to spin down disk: %v", err))), nil
			}

			return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Disk '%s' spin down initiated", args.DiskID))), nil
		}); err != nil {
		return err
	}

	// Disk spin up tool
	if err := s.mcpServer.RegisterTool("disk_spin_up",
		"Spin up a specific disk that is in standby mode",
		func(args dto.MCPDiskArgs) (*mcp.ToolResponse, error) {
			if args.DiskID == "" {
				return mcp.NewToolResponse(mcp.NewTextContent("disk_id is required")), nil
			}

			logger.Info("MCP: Disk spin up requested for '%s'", args.DiskID)

			arrayCtrl := controllers.NewArrayController(s.ctx)
			err := arrayCtrl.SpinUpDisk(args.DiskID)

			if err != nil {
				logger.Error("MCP: Disk spin up failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to spin up disk: %v", err))), nil
			}

			return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Disk '%s' spin up initiated", args.DiskID))), nil
		}); err != nil {
		return err
	}

	// Execute user script tool
	if err := s.mcpServer.RegisterTool("execute_user_script",
		"Execute a user script from the User Scripts plugin. Requires confirmation for safety.",
		func(args dto.MCPUserScriptArgs) (*mcp.ToolResponse, error) {
			if !args.Confirm {
				return mcp.NewToolResponse(mcp.NewTextContent("Script execution not confirmed. Set 'confirm' to true to execute.")), nil
			}

			if args.ScriptName == "" {
				return mcp.NewToolResponse(mcp.NewTextContent("script_name is required")), nil
			}

			logger.Info("MCP: User script execution requested for '%s' (confirmed)", args.ScriptName)

			response, err := controllers.ExecuteUserScript(args.ScriptName, false, true)
			if err != nil {
				logger.Error("MCP: User script execution failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to execute script: %v", err))), nil
			}

			return s.jsonResponse(response)
		}); err != nil {
		return err
	}

	// Collector control tool
	if err := s.mcpServer.RegisterTool("collector_action",
		"Enable or disable a data collector at runtime. Note: some collectors like 'system' are required and cannot be disabled.",
		func(args dto.MCPCollectorControlArgs) (*mcp.ToolResponse, error) {
			if args.CollectorName == "" {
				return mcp.NewToolResponse(mcp.NewTextContent("collector_name is required")), nil
			}

			logger.Info("MCP: Collector action '%s' requested for '%s'", args.Action, args.CollectorName)

			var err error
			switch args.Action {
			case "enable":
				err = s.cacheProvider.EnableCollector(args.CollectorName)
			case "disable":
				err = s.cacheProvider.DisableCollector(args.CollectorName)
			default:
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Unknown action: %s. Use 'enable' or 'disable'", args.Action))), nil
			}

			if err != nil {
				logger.Error("MCP: Collector action failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to %s collector: %v", args.Action, err))), nil
			}

			status, _ := s.cacheProvider.GetCollectorStatus(args.CollectorName)
			return s.jsonResponse(map[string]interface{}{
				"success":   true,
				"message":   fmt.Sprintf("Collector '%s' %sd successfully", args.CollectorName, args.Action),
				"collector": status,
			})
		}); err != nil {
		return err
	}

	// Update collector interval tool
	if err := s.mcpServer.RegisterTool("update_collector_interval",
		"Update the collection interval for a specific collector. Interval must be between 5 and 3600 seconds.",
		func(args dto.MCPCollectorIntervalArgs) (*mcp.ToolResponse, error) {
			if args.CollectorName == "" {
				return mcp.NewToolResponse(mcp.NewTextContent("collector_name is required")), nil
			}

			if args.Interval < 5 || args.Interval > 3600 {
				return mcp.NewToolResponse(mcp.NewTextContent("interval must be between 5 and 3600 seconds")), nil
			}

			logger.Info("MCP: Updating collector '%s' interval to %d seconds", args.CollectorName, args.Interval)

			err := s.cacheProvider.UpdateCollectorInterval(args.CollectorName, args.Interval)
			if err != nil {
				logger.Error("MCP: Collector interval update failed: %v", err)
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Failed to update interval: %v", err))), nil
			}

			status, _ := s.cacheProvider.GetCollectorStatus(args.CollectorName)
			return s.jsonResponse(map[string]interface{}{
				"success":   true,
				"message":   fmt.Sprintf("Collector '%s' interval updated to %d seconds", args.CollectorName, args.Interval),
				"collector": status,
			})
		}); err != nil {
		return err
	}

	logger.Debug("MCP control tools registered")
	return nil
}

// registerResources registers MCP resources for real-time data access.
func (s *Server) registerResources() error {
	// System resource
	if err := s.mcpServer.RegisterResource(
		"unraid://system",
		"system-info",
		"Real-time Unraid system information",
		"application/json",
		func() (*mcp.ResourceResponse, error) {
			info := s.cacheProvider.GetSystemCache()
			if info == nil {
				return mcp.NewResourceResponse(mcp.NewTextEmbeddedResource(
					"unraid://system",
					`{"error": "System information not available"}`,
					"application/json",
				)), nil
			}
			data, _ := json.Marshal(info)
			return mcp.NewResourceResponse(mcp.NewTextEmbeddedResource(
				"unraid://system",
				string(data),
				"application/json",
			)), nil
		}); err != nil {
		return err
	}

	// Array resource
	if err := s.mcpServer.RegisterResource(
		"unraid://array",
		"array-status",
		"Real-time Unraid array status",
		"application/json",
		func() (*mcp.ResourceResponse, error) {
			status := s.cacheProvider.GetArrayCache()
			if status == nil {
				return mcp.NewResourceResponse(mcp.NewTextEmbeddedResource(
					"unraid://array",
					`{"error": "Array status not available"}`,
					"application/json",
				)), nil
			}
			data, _ := json.Marshal(status)
			return mcp.NewResourceResponse(mcp.NewTextEmbeddedResource(
				"unraid://array",
				string(data),
				"application/json",
			)), nil
		}); err != nil {
		return err
	}

	// Containers resource
	if err := s.mcpServer.RegisterResource(
		"unraid://containers",
		"docker-containers",
		"Real-time Docker container list and status",
		"application/json",
		func() (*mcp.ResourceResponse, error) {
			containers := s.cacheProvider.GetDockerCache()
			if containers == nil {
				return mcp.NewResourceResponse(mcp.NewTextEmbeddedResource(
					"unraid://containers",
					`{"error": "Container information not available"}`,
					"application/json",
				)), nil
			}
			data, _ := json.Marshal(containers)
			return mcp.NewResourceResponse(mcp.NewTextEmbeddedResource(
				"unraid://containers",
				string(data),
				"application/json",
			)), nil
		}); err != nil {
		return err
	}

	// VMs resource
	if err := s.mcpServer.RegisterResource(
		"unraid://vms",
		"virtual-machines",
		"Real-time virtual machine list and status",
		"application/json",
		func() (*mcp.ResourceResponse, error) {
			vms := s.cacheProvider.GetVMsCache()
			if vms == nil {
				return mcp.NewResourceResponse(mcp.NewTextEmbeddedResource(
					"unraid://vms",
					`{"error": "VM information not available"}`,
					"application/json",
				)), nil
			}
			data, _ := json.Marshal(vms)
			return mcp.NewResourceResponse(mcp.NewTextEmbeddedResource(
				"unraid://vms",
				string(data),
				"application/json",
			)), nil
		}); err != nil {
		return err
	}

	// Disks resource
	if err := s.mcpServer.RegisterResource(
		"unraid://disks",
		"disk-status",
		"Real-time disk information and health status",
		"application/json",
		func() (*mcp.ResourceResponse, error) {
			disks := s.cacheProvider.GetDisksCache()
			if disks == nil {
				return mcp.NewResourceResponse(mcp.NewTextEmbeddedResource(
					"unraid://disks",
					`{"error": "Disk information not available"}`,
					"application/json",
				)), nil
			}
			data, _ := json.Marshal(disks)
			return mcp.NewResourceResponse(mcp.NewTextEmbeddedResource(
				"unraid://disks",
				string(data),
				"application/json",
			)), nil
		}); err != nil {
		return err
	}

	logger.Debug("MCP resources registered")
	return nil
}

// registerPrompts registers MCP prompts for guided interactions.
func (s *Server) registerPrompts() error {
	// Disk health analysis prompt
	if err := s.mcpServer.RegisterPrompt(
		"analyze_disk_health",
		"Analyze the health status of all disks and provide recommendations",
		func(args dto.MCPEmptyArgs) (*mcp.PromptResponse, error) {
			disks := s.cacheProvider.GetDisksCache()
			if disks == nil {
				return mcp.NewPromptResponse("Disk health analysis",
					mcp.NewPromptMessage(
						mcp.NewTextContent("Disk information is not available. Please wait for the system to collect disk data."),
						mcp.RoleAssistant,
					),
				), nil
			}

			data, _ := json.MarshalIndent(disks, "", "  ")
			return mcp.NewPromptResponse("Disk health analysis",
				mcp.NewPromptMessage(
					mcp.NewTextContent(fmt.Sprintf(`Please analyze the following Unraid disk information and provide:
1. Overall health assessment for each disk
2. Any SMART warnings or concerns
3. Recommendations for maintenance or replacement
4. Temperature analysis

Disk Data:
%s`, string(data))),
					mcp.RoleUser,
				),
			), nil
		}); err != nil {
		return err
	}

	// System overview prompt
	if err := s.mcpServer.RegisterPrompt(
		"system_overview",
		"Get a comprehensive overview of the Unraid system status",
		func(args dto.MCPEmptyArgs) (*mcp.PromptResponse, error) {
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
			return mcp.NewPromptResponse("System overview",
				mcp.NewPromptMessage(
					mcp.NewTextContent(fmt.Sprintf(`Please provide a summary of the following Unraid system status:
1. System health and resource usage
2. Array status and capacity
3. Running services (containers and VMs)
4. Any issues or recommendations

System Data:
%s`, string(data))),
					mcp.RoleUser,
				),
			), nil
		}); err != nil {
		return err
	}

	// Troubleshooting prompt
	if err := s.mcpServer.RegisterPrompt(
		"troubleshoot_issue",
		"Help troubleshoot common Unraid issues",
		func(args dto.MCPEmptyArgs) (*mcp.PromptResponse, error) {
			return mcp.NewPromptResponse("Troubleshooting assistant",
				mcp.NewPromptMessage(
					mcp.NewTextContent(`I'm here to help troubleshoot your Unraid server. To get started, please describe:

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

Please describe your issue and I'll gather the relevant system information to help diagnose it.`),
					mcp.RoleAssistant,
				),
			), nil
		}); err != nil {
		return err
	}

	logger.Debug("MCP prompts registered")
	return nil
}

// jsonResponse creates a tool response with JSON-formatted content.
func (s *Server) jsonResponse(data interface{}) (*mcp.ToolResponse, error) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Error formatting response: %v", err))), nil
	}
	return mcp.NewToolResponse(mcp.NewTextContent(string(jsonData))), nil
}
