package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/memory"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/remediation"
)

// AgentConfigFile is the on-disk agent configuration filename.
const AgentConfigFile = "agent_config.json"

// APIKeyEnv is the environment variable that supplies the LLM API key.
const APIKeyEnv = "UMA_AGENT_API_KEY" // #nosec G101 -- env var name, not a credential

// LoadConfig reads agent_config.json, falling back to safe defaults. Empty dir uses DefaultConfigDir.
func LoadConfig(configDir string) dto.AgentConfig {
	if configDir == "" {
		configDir = DefaultConfigDir
	}
	cfg := dto.DefaultAgentConfig()
	// #nosec G304 -- config path is operator-controlled, under the plugin config dir
	data, err := os.ReadFile(filepath.Join(configDir, AgentConfigFile))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.Warning("Agent: failed to read config file, using defaults: %v", err)
		}
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		logger.Warning("Agent: invalid config file, using defaults: %v", err)
		return dto.DefaultAgentConfig()
	}
	return cfg
}

// BuildService assembles the agent service from config. Returns (nil, nil) when disabled.
// The API key is sourced from the environment (never persisted).
func BuildService(cfg dto.AgentConfig, configDir string, state tools.StateProvider, docker tools.DockerActor, bc Broadcaster) (*Service, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if key := os.Getenv(APIKeyEnv); key != "" {
		cfg.APIKey = key
	}

	var provider llm.Provider
	switch cfg.Provider {
	case "anthropic":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("agent enabled but %s is not set", APIKeyEnv)
		}
		provider = llm.NewAnthropicProvider(cfg.APIKey, cfg.Model, cfg.Endpoint)
	case "openai", "openrouter", "gemini":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("agent enabled but %s is not set", APIKeyEnv)
		}
		endpoint := cfg.Endpoint
		if endpoint == "" {
			switch cfg.Provider {
			case "openrouter":
				endpoint = "https://openrouter.ai/api/v1/chat/completions"
			case "gemini":
				endpoint = "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"
			default:
				endpoint = "https://api.openai.com/v1/chat/completions"
			}
		}
		provider = llm.NewOpenAIProvider(cfg.APIKey, cfg.Model, endpoint)
	default:
		return nil, fmt.Errorf("unsupported agent provider %q", cfg.Provider)
	}

	store := NewStore(configDir)
	if err := store.Load(); err != nil {
		logger.Warning("Agent: failed to load sessions: %v", err)
	}
	mem := memory.NewStore(configDir, cfg.MaxIncidents)
	if err := mem.Load(); err != nil {
		logger.Warning("Agent: failed to load memory: %v", err)
	}
	reg := tools.BuildDefault(state, docker)
	svc := NewService(cfg, provider, reg, store, mem, bc)
	rbStore := remediation.NewRunbookStore(configDir)
	if err := rbStore.Load(); err != nil {
		logger.Warning("Agent: failed to load runbooks: %v", err)
	}
	svc.SetRunbookStore(rbStore)
	svc.RegisterLearningTools(reg)
	return svc, nil
}
