---
applyTo: "daemon/services/mcp/**/*.go"
---

# MCP Server Instructions

Reference: [`AGENTS.md`](../../AGENTS.md) for full project context.

## Overview

The MCP (Model Context Protocol) server exposes 54+ tools at `POST /mcp` using Streamable HTTP transport (spec 2025-06-18). It enables AI agents to monitor and control the Unraid system.

## Key Files

- `server.go` — Tool registration and MCP server setup
- `transport.go` — HTTP transport for JSON-RPC requests

## Adding a New Tool

1. Define the tool handler function in `server.go`
2. Register the tool with appropriate name, description, and input schema
3. Follow existing tool patterns for input validation and response format
4. Update documentation in `docs/integrations/mcp.md`

## Patterns

- Tools receive JSON-RPC requests and return structured responses
- Input validation is required for all user-provided parameters
- Use existing controller/collector infrastructure where possible
- Tools should return consistent error formats

## Library

Uses `github.com/metoro-io/mcp-golang` for the MCP server implementation.
