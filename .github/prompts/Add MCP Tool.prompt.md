---
description: Step-by-step guide for adding a new MCP tool for AI agent integration
tools: ["editor", "terminal"]
---

# Add a New MCP Tool

Follow these steps to add a new tool to the MCP (Model Context Protocol) server.

## Step 1: Identify the Tool

Decide what the tool should do:

- **Monitoring tool:** Returns system data (read-only)
- **Control tool:** Performs an action (start/stop/restart, etc.)

## Step 2: Define the Tool

In `daemon/services/mcp/server.go`, register the new tool following existing patterns.

### Tool Registration Pattern

Look at existing tools in `server.go` for the registration pattern. Each tool needs:

- A unique tool name (snake_case)
- A clear description of what it does
- Input schema (if it accepts parameters)
- Handler function

## Step 3: Implement the Handler

The handler should:

1. Parse and validate input parameters
2. Call existing collector caches or controller methods
3. Return a structured JSON response

### For monitoring tools:

Access cached data through the API server's cache fields.

### For control tools:

Use existing controllers in `daemon/services/controllers/`. Always validate input before executing.

## Step 4: Test

- Add tests in `daemon/services/mcp/` test files
- Test valid inputs, invalid inputs, and edge cases

## Step 5: Document

- Update MCP documentation in `docs/integrations/mcp.md`
- Update `CHANGELOG.md`
