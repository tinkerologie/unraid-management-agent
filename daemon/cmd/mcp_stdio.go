// Package cmd provides command implementations for the Unraid Management Agent.
package cmd

import (
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services"
)

// MCPStdio represents the mcp-stdio command that runs the MCP server over stdin/stdout.
// This is the preferred transport for local AI clients (e.g., Claude Desktop, Cursor)
// running directly on the Unraid server.
//
// Usage in Claude Desktop config:
//
//	{
//	  "mcpServers": {
//	    "unraid": {
//	      "command": "/usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent",
//	      "args": ["mcp-stdio"]
//	    }
//	  }
//	}
type MCPStdio struct{}

// Run executes the mcp-stdio command by starting collectors and running MCP over STDIO.
func (m *MCPStdio) Run(ctx *domain.Context) error {
	return services.CreateOrchestrator(ctx).RunMCPStdio()
}
