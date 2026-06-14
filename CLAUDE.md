# CLAUDE.md

> **Read [`AGENTS.md`](./AGENTS.md) first** — it is the single source of truth for this project.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:

- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:

- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:

- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:

- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:

```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

## Claude-Specific Instructions

- **Use Context7** — automatically use Context7 MCP tools to get library documentation without explicit prompting
- **Sequential Thinking** — reason step-by-step internally before answering, keeping reasoning hidden unless requested

## Quick Reference

```bash
make deps && make local       # Setup and build
make test                     # Run all tests
make pre-commit-run           # Lint + security checks
make swagger                  # Regenerate Swagger docs

# Deploy to Unraid hardware (Ansible — preferred)
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml
```

## Key Paths

| Path                              | Purpose                                           |
| --------------------------------- | ------------------------------------------------- |
| `daemon/services/orchestrator.go` | Application lifecycle (init order is critical)    |
| `daemon/services/collectors/`     | Data collection goroutines                        |
| `daemon/services/api/`            | REST handlers, WebSocket hub, cache               |
| `daemon/services/controllers/`    | Control operations (Docker/VM/Array)              |
| `daemon/services/mcp/`            | MCP server for AI agents                          |
| `daemon/lib/validation.go`        | Input validation functions                        |
| `daemon/constants/const.go`       | System paths, intervals, binary locations         |
| `skills/unraid-management-agent/` | Agent Skill (MCP/REST usage) — reference material |
| `docs/integrations/`              | AI/automation guides (mcp, claude, chatgpt, mqtt) |

## Path-Specific Instructions

The `.github/instructions/` directory contains context-aware instructions auto-applied by GitHub Copilot based on file globs. These are useful reference for any AI agent:

- `go.instructions.md` — Go style, error handling, imports
- `collectors.instructions.md` — Collector pattern, panic recovery
- `api-handlers.instructions.md` — Cache mutex, response helpers
- `controllers.instructions.md` — Validate-execute-return pattern
- `mcp.instructions.md` — MCP tool registration
- `dto.instructions.md` — Struct conventions, JSON tags
- `tests.instructions.md` — Table-driven tests, security cases
- `yaml-markdown.instructions.md` — YAML/markdown formatting

## Reusable Prompts

The `.github/prompts/` directory contains step-by-step task guides:

- `Add Collector.prompt.md`
- `Add REST Endpoint.prompt.md`
- `Add MCP Tool.prompt.md`
- `Add Controller.prompt.md`
- `Debug Collector Issue.prompt.md`
- `Add WebSocket Event.prompt.md`
