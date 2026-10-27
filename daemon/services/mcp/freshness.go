package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/api"
)

// freshener refreshes stale collector data before a read (api.Server in
// Dynamic collection mode, #280).
type freshener interface {
	EnsureFresh(ctx context.Context, recordDemand bool, collectors ...string) api.Freshness
}

// readCollectors maps each read tool, prompt and resource that serves cached
// collector data to those collectors. TestReadCollectorsCoverReadTools makes
// every new read tool choose between this table and the test's toolsWithoutCollectors.
var readCollectors = map[string][]string{
	// Tools
	"get_system_info":            {"system"},
	"get_temperatures":           {"system"},
	"get_array_status":           {"array"},
	"list_disks":                 {"disk"},
	"get_disk_info":              {"disk"},
	"list_shares":                {"shares"},
	"list_containers":            {"docker"},
	"get_container_info":         {"docker"},
	"search_containers":          {"docker"},
	"get_docker_stats":           {"docker"},
	"list_docker_networks":       {"docker_networks"},
	"list_vms":                   {"vm"},
	"get_vm_info":                {"vm"},
	"search_vms":                 {"vm"},
	"get_ups_status":             {"ups"},
	"get_nut_status":             {"nut"},
	"get_gpu_metrics":            {"gpu"},
	"get_network_info":           {"network"},
	"get_psu_status":             {"psu"},
	"get_network_cards":          {"nic"},
	"get_hardware_info":          {"hardware"},
	"get_registration":           {"registration"},
	"get_notifications":          {"notification"},
	"get_notifications_overview": {"notification"},
	"get_zfs_pools":              {"zfs"},
	"get_zfs_datasets":           {"zfs:zfs_datasets_update"},
	"get_zfs_snapshots":          {"zfs:zfs_snapshots_update"},
	"get_zfs_arc_stats":          {"zfs:zfs_arc_stats_update"},
	"get_unassigned_devices":     {"unassigned"},
	"get_remote_shares":          {"unassigned"},
	"get_os_update":              {"os_update"},
	"get_mover_status":           {"mover"},
	"get_storage_topology":       {"storage_topology"},
	"check_plugin_updates":       {"plugin_update"},
	"get_fan_status":             {"fancontrol"},
	"get_tuning_status":          {"tuning"},
	"get_diagnostic_summary":     {"system", "array", "disk", "docker", "notification"},
	"system_health_report":       {"array", "disk", "docker"},
	"find_root_cause":            {"system", "array", "disk", "docker"},
	"get_health_status":          {"system", "array", "disk", "docker", "vm"},
	// Prompts
	"diagnose_disk_health":       {"disk"},
	"diagnose_performance_issue": {"system", "array", "docker", "vm"},
	"suggest_maintenance":        {"system", "array", "disk", "notification"},
	"explain_array_state":        {"array", "disk"},
	"system_overview":            {"system", "array", "docker", "vm"},
	// Resources
	"system-info":       {"system"},
	"array-status":      {"array"},
	"docker-containers": {"docker"},
	"virtual-machines":  {"vm"},
	"disk-status":       {"disk"},
}

// ensureFresh refreshes the collectors behind a read tool, prompt or
// resource and records the read as demand.
func (s *Server) ensureFresh(ctx context.Context, name string) {
	names, ok := readCollectors[name]
	if !ok {
		return
	}
	if f, ok := s.cacheProvider.(freshener); ok {
		f.EnsureFresh(ctx, true, names...)
	}
}

// addPrompt registers a prompt whose handler first refreshes stale data.
func (s *Server) addPrompt(p *mcp.Prompt, handler mcp.PromptHandler) {
	s.mcpServer.AddPrompt(p, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		s.ensureFresh(ctx, p.Name)
		return handler(ctx, req)
	})
}

// addResource registers a resource whose handler first refreshes stale data.
func (s *Server) addResource(r *mcp.Resource, handler mcp.ResourceHandler) {
	s.mcpServer.AddResource(r, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		s.ensureFresh(ctx, r.Name)
		return handler(ctx, req)
	})
}
