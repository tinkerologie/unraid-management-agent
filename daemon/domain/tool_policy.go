package domain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const (
	// DefaultToolPolicyConfigDir is the standard directory for MCP tool policy on Unraid.
	DefaultToolPolicyConfigDir = "/boot/config/plugins/unraid-management-agent"
	// ToolPolicyConfigFile is the filename where tool access policy is stored.
	ToolPolicyConfigFile = "tool_policy.json"
)

// ToolPolicyStore manages per-tool MCP access policies and the catalog of registered tools.
type ToolPolicyStore struct {
	mu           sync.RWMutex
	filePath     string
	policies     map[string]ToolPolicyValue
	catalog      map[string]dto.MCPToolCatalogItem
	catalogOrder []string
}

// NewToolPolicyStore creates a new ToolPolicyStore.
func NewToolPolicyStore(configDir string, initial map[string]ToolPolicyValue) *ToolPolicyStore {
	if configDir == "" {
		configDir = DefaultToolPolicyConfigDir
	}
	p := make(map[string]ToolPolicyValue)
	for k, v := range initial {
		if v != "" && v != PolicyDefault {
			p[k] = v
		}
	}
	return &ToolPolicyStore{
		filePath: filepath.Join(configDir, ToolPolicyConfigFile),
		policies: p,
		catalog:  make(map[string]dto.MCPToolCatalogItem),
	}
}

// Load reads saved tool access policies from JSON storage.
func (s *ToolPolicyStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading tool policy file: %w", err)
	}

	var saved map[string]string
	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("parsing tool policy file: %w", err)
	}

	loaded := make(map[string]ToolPolicyValue, len(saved))
	for k, v := range saved {
		if err := ValidateToolPolicyValue(v); err != nil {
			logger.Warning("ToolPolicyStore: invalid policy %q for tool %q in %s, ignoring: %v", v, k, s.filePath, err)
			continue
		}
		val := ToolPolicyValue(v)
		if val != "" && val != PolicyDefault {
			loaded[k] = val
		}
	}
	s.policies = loaded
	return nil
}

// Save writes current non-default policies to JSON storage.
func (s *ToolPolicyStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(s.filePath), 0o750); err != nil {
		return fmt.Errorf("creating directory for tool policy: %w", err)
	}

	toSave := make(map[string]string)
	for k, v := range s.policies {
		if v != "" && v != PolicyDefault {
			toSave[k] = string(v)
		}
	}

	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling tool policy: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0o600); err != nil {
		return fmt.Errorf("writing tool policy file: %w", err)
	}
	return nil
}

// RegisterTool registers an MCP tool into the catalog.
func (s *ToolPolicyStore) RegisterTool(name, description string, readOnly, destructive bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.catalog[name]; !exists {
		s.catalogOrder = append(s.catalogOrder, name)
	}
	s.catalog[name] = dto.MCPToolCatalogItem{
		Name:        name,
		Category:    CategorizeMCPTool(name),
		Description: description,
		ReadOnly:    readOnly,
		Destructive: destructive,
	}
}

// IsValidTool reports whether a tool with the given name is registered in the catalog.
func (s *ToolPolicyStore) IsValidTool(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.catalog[name]
	return exists
}

// GetPolicy returns the configured policy for a tool, or PolicyDefault.
func (s *ToolPolicyStore) GetPolicy(name string) ToolPolicyValue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.policies[name]; ok && v != "" {
		return v
	}
	return PolicyDefault
}

// GetEffectivePolicy calculates the effective policy for a tool given global ReadOnly mode.
// Precedence:
//  1. Hidden tool is always hidden.
//  2. If the tool is read-only (readOnlyHint: true), it is never blocked or gated (effective policy default, unless hidden).
//  3. If global readOnly is true, all state-changing tools are read_only.
//  4. Configured per-tool policy (read_only, allow, ask).
//  5. Default (current behavior).
func (s *ToolPolicyStore) GetEffectivePolicy(name string, globalReadOnly bool) ToolPolicyValue {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg := s.policies[name]
	if cfg == PolicyHidden {
		return PolicyHidden
	}

	item, exists := s.catalog[name]
	if exists && item.ReadOnly {
		// Read-only tools are never blocked or gated.
		return PolicyDefault
	}

	if globalReadOnly {
		return PolicyReadOnly
	}

	if cfg != "" && cfg != PolicyDefault {
		return cfg
	}
	return PolicyDefault
}

// GetAll returns a copy of all configured non-default policies for registered catalog tools.
// If the catalog is empty (before tools are registered), it returns all non-default policies.
func (s *ToolPolicyStore) GetAll() map[string]ToolPolicyValue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make(map[string]ToolPolicyValue, len(s.policies))
	for k, v := range s.policies {
		if len(s.catalog) > 0 {
			if _, ok := s.catalog[k]; !ok {
				continue
			}
		}
		if v != "" && v != PolicyDefault {
			res[k] = v
		}
	}
	return res
}

// Replace replaces all configured policies with the provided map.
func (s *ToolPolicyStore) Replace(newPolicies map[string]ToolPolicyValue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies = make(map[string]ToolPolicyValue, len(newPolicies))
	for k, v := range newPolicies {
		if v != "" && v != PolicyDefault {
			s.policies[k] = v
		}
	}
}

// GetCatalog returns the full list of tools with their configured and effective policies.
func (s *ToolPolicyStore) GetCatalog(globalReadOnly bool) []dto.MCPToolCatalogItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]dto.MCPToolCatalogItem, 0, len(s.catalogOrder))
	for _, name := range s.catalogOrder {
		item := s.catalog[name]
		cfg := s.policies[name]
		if cfg == "" {
			cfg = PolicyDefault
		}
		item.ConfiguredPolicy = string(cfg)

		// Calculate effective policy inline with lock held
		if cfg == PolicyHidden {
			item.EffectivePolicy = string(PolicyHidden)
		} else if item.ReadOnly {
			item.EffectivePolicy = string(PolicyDefault)
		} else if globalReadOnly {
			item.EffectivePolicy = string(PolicyReadOnly)
		} else if cfg != "" && cfg != PolicyDefault {
			item.EffectivePolicy = string(cfg)
		} else {
			item.EffectivePolicy = string(PolicyDefault)
		}

		items = append(items, item)
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Category != items[j].Category {
			return items[i].Category < items[j].Category
		}
		return items[i].Name < items[j].Name
	})

	return items
}

// CategorizeMCPTool assigns a logical category string to an MCP tool.
func CategorizeMCPTool(name string) string {
	switch {
	case strings.HasPrefix(name, "container_") || strings.Contains(name, "container") || strings.Contains(name, "docker"):
		return "docker"
	case strings.HasPrefix(name, "vm_") || strings.Contains(name, "vm"):
		return "vm"
	case strings.HasPrefix(name, "array_") || strings.Contains(name, "parity"):
		return "array"
	case strings.HasPrefix(name, "disk_") || strings.Contains(name, "disk") || strings.Contains(name, "smart_"):
		return "disk"
	case strings.HasPrefix(name, "fan_") || strings.Contains(name, "fan"):
		return "fancontrol"
	case strings.HasPrefix(name, "cpu_") || strings.Contains(name, "governor") || strings.Contains(name, "turbo"):
		return "cpu"
	case strings.HasPrefix(name, "tuning_") || strings.Contains(name, "sysctl") || strings.Contains(name, "inotify") || strings.Contains(name, "dirty_"):
		return "tuning"
	case strings.HasPrefix(name, "alert_") || strings.Contains(name, "alert"):
		return "alerting"
	case strings.HasPrefix(name, "watchdog_") || strings.Contains(name, "probe") || strings.Contains(name, "health_check"):
		return "watchdog"
	case strings.HasPrefix(name, "agent_") || strings.Contains(name, "agent"):
		return "agent"
	case strings.HasPrefix(name, "zfs_") || strings.Contains(name, "zpool") || strings.Contains(name, "dataset") || strings.Contains(name, "snapshot"):
		return "zfs"
	case strings.HasPrefix(name, "network_") || strings.Contains(name, "interface"):
		return "network"
	case strings.HasPrefix(name, "ups_") || strings.HasPrefix(name, "nut_"):
		return "ups"
	case strings.HasPrefix(name, "gpu_"):
		return "gpu"
	case strings.HasPrefix(name, "notification_") || strings.Contains(name, "notify"):
		return "notification"
	case strings.HasPrefix(name, "remote_share_") || strings.Contains(name, "unassigned"):
		return "unassigned"
	case strings.HasPrefix(name, "share_") || strings.Contains(name, "share"):
		return "shares"
	case strings.HasPrefix(name, "mover_") || strings.Contains(name, "mover"):
		return "mover"
	case strings.HasPrefix(name, "plugin_"):
		return "plugin"
	case strings.HasPrefix(name, "log_") || strings.Contains(name, "syslog"):
		return "logs"
	case strings.Contains(name, "remediation") || strings.Contains(name, "runbook"):
		return "remediation"
	case strings.HasPrefix(name, "system_") || strings.Contains(name, "hardware") || strings.Contains(name, "registration"):
		return "system"
	default:
		return "system"
	}
}

// ValidateToolPolicyValue validates that a policy string is one of the supported values.
func ValidateToolPolicyValue(val string) error {
	switch ToolPolicyValue(val) {
	case PolicyDefault, PolicyHidden, PolicyReadOnly, PolicyAllow, PolicyAsk, "":
		return nil
	default:
		return fmt.Errorf("invalid tool policy value %q (must be one of: %s, %s, %s, %s, %s)",
			val, PolicyDefault, PolicyHidden, PolicyReadOnly, PolicyAllow, PolicyAsk)
	}
}
