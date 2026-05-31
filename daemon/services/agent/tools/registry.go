// Package tools is the agent's action space: a registry of risk-tiered tools.
package tools

import (
	"context"
	"sort"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
)

// Tool is one risk-tiered action the agent can take.
type Tool struct {
	Name        string
	Description string
	Schema      []byte // JSON Schema for arguments
	RiskTier    dto.RiskTier
	Invoke      func(ctx context.Context, argsJSON string) (string, error)
}

// Registry holds the available tools by name.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

// Register adds or replaces a tool.
func (r *Registry) Register(t Tool) { r.tools[t.Name] = t }

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) { t, ok := r.tools[name]; return t, ok }

// Schemas returns the LLM-facing schema for every tool, name-sorted for determinism.
func (r *Registry) Schemas() []llm.ToolSchema {
	out := make([]llm.ToolSchema, 0, len(r.tools))
	for _, t := range r.tools {
		schema := t.Schema
		if len(schema) == 0 {
			schema = []byte(`{"type":"object","properties":{}}`)
		}
		out = append(out, llm.ToolSchema{Name: t.Name, Description: t.Description, Schema: schema})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
