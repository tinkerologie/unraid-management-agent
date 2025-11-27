<!--
Sync Impact Report:
- Version: Initial → 1.0.0
- Ratification Date: 2025-11-27 (today)
- Last Amended: 2025-11-27 (today)
- Added Sections: All (initial creation)
- Modified Principles: None (initial creation)
- Removed Sections: None

Templates Status:
✅ plan-template.md - Constitution Check section aligns with principles
✅ spec-template.md - Requirements section aligns with security and testing principles
✅ tasks-template.md - Task organization aligns with testing and phase principles
⚠️ No command templates found in .specify/templates/commands/ - skipped

Follow-up TODOs: None
-->

# Unraid Management Agent Constitution

## Core Principles

### I. Reliability Over Features

**Rule**: Stability is paramount - this runs on production Unraid servers.

- Panic recovery is MANDATORY in all goroutines
- Graceful degradation when hardware/features unavailable
- NEVER crash the agent due to collector failures
- All goroutines MUST respect context cancellation for coordinated shutdown

**Rationale**: Users depend on this agent running on live production servers. A crash or hang can impact monitoring and control operations. Defensive programming and recovery mechanisms ensure the agent remains operational even when individual components fail.

### II. Security First

**Rule**: All user inputs MUST be validated before use.

- Protection against OWASP Top 10 vulnerabilities (especially CWE-22 path traversal, command injection)
- Use safe command execution wrappers (`lib.ExecuteShellCommand`)
- NO direct string interpolation of user input into shell commands
- Whitelist validation, NEVER blacklist
- Validate at use point, not just at UI

**Rationale**: The agent provides control operations (Docker, VM, Array) that execute system commands. Unvalidated input could lead to arbitrary command execution or unauthorized file access, compromising the entire Unraid server.

### III. Event-Driven Architecture

**Rule**: Components MUST be decoupled via PubSub pattern.

- Collectors publish, API server subscribes
- Real-time data flow to WebSocket clients
- STRICT initialization order: subscriptions BEFORE publishers (API subscriptions start before collectors)
- NO direct coupling between collectors and API handlers

**Rationale**: Event-driven architecture enables independent collector development, prevents tight coupling, supports real-time WebSocket broadcasting, and allows collectors to fail independently without cascading failures.

### IV. Thread Safety

**Rule**: All shared state MUST be protected by appropriate mutexes.

- Read-Write locks (`sync.RWMutex`) for cache access
- Context cancellation for coordinated shutdown
- NO data races tolerated (verify with `go test -race`)
- Proper lock ordering to prevent deadlocks

**Rationale**: Multiple goroutines (collectors, API handlers, WebSocket hub) access shared state concurrently. Unprotected access leads to data corruption, race conditions, and unpredictable behavior.

### V. Simplicity and Maintainability

**Rule**: Avoid over-engineering - build what's needed now.

- Clear separation of concerns (collectors, controllers, API, DTOs)
- Consistent patterns across similar components
- Self-documenting code preferred over excessive comments
- One primary concept per file
- NO premature abstractions or hypothetical requirements

**Rationale**: As a community plugin, maintainability is critical. Simple, consistent patterns make it easier for contributors to understand, modify, and extend the codebase. Over-engineering adds complexity without proven benefit.

## Code Organization

### Layered Architecture

**Strict layer separation MUST be maintained:**

- `dto/` - Data structures ONLY, NO logic
- `services/collectors/` - Data gathering, publish to event bus
- `services/controllers/` - Execute operations, return results
- `services/api/` - HTTP/WebSocket serving, cache management
- `lib/` - Shared utilities, NO business logic
- `domain/` - Core types (Context, Config)

### File Naming Conventions

- One primary concept per file
- Test files alongside implementation: `foo.go` → `foo_test.go`
- Security tests separate: `foo_security_test.go`
- Clear, descriptive names reflecting purpose

## Testing Requirements

### Mandatory Tests

The following MUST have tests:

- ALL input validation functions
- ALL security-sensitive operations
- ALL control operations (Docker/VM/Array)
- Error paths, not just happy paths

### Test Style

- Table-driven tests for multiple cases
- Mock external dependencies (filesystem, commands)
- Clear test names describing scenarios
- Use `make test-coverage` to verify coverage
- NO tests for trivial getters/setters

## API Design Standards

### REST Principles

- GET for reads, POST for actions
- Proper HTTP status codes: 200 (success), 400 (client error), 404 (not found), 500 (server error)
- Consistent JSON response format across all endpoints
- Cache-first for monitoring endpoints (respond from in-memory cache)

### WebSocket Requirements

- Broadcast ALL collector events to connected clients
- Client-agnostic event format (use DTOs)
- Connection health monitoring (ping/pong)
- Graceful client disconnect handling

## Error Handling Standards

### Collectors

- Log errors but CONTINUE operation
- Return partial data when possible
- NEVER panic - use recovery if needed
- Respect context cancellation

### API Handlers

- Return meaningful error messages
- Log errors with context (endpoint, parameters)
- 500 for server errors, 400 for client errors
- NO sensitive data in error responses

### Controllers

- Validate before execution
- Return structured error responses
- Log ALL control operations (start/stop Docker, VM, Array)
- Rollback state on failures when possible

## Logging Standards

### When to Log

- **Info**: Startup, shutdown, configuration changes
- **Success**: Successful control operations
- **Warning**: Degraded operation, missing features (GPU, UPS unavailable)
- **Error**: Failed operations, unexpected conditions
- **Debug**: Detailed flow (debug mode only)

### What NOT to Log

- Sensitive data (passwords, tokens, credentials)
- High-frequency routine operations (every collector run)
- Redundant information

## Performance Expectations

### Collector Intervals

- **Critical data** (system, array): 5-10s
- **Standard data** (docker, vm, network): 10-15s
- **Expensive data** (disks, SMART): 30s+
- **Rarely-changing data** (hardware): 5min

### Response Times

- **Monitoring endpoints**: <50ms (cached)
- **Control operations**: <5s (command execution)
- **Configuration reads**: <100ms

## Backward Compatibility

### Breaking Changes MUST Be Avoided

- Maintain existing API endpoint contracts
- Add new fields, DON'T remove existing ones
- Deprecate before removal
- Version API if breaking changes absolutely necessary

### Safe Changes (Allowed)

- New endpoints
- New optional fields
- Performance improvements
- Bug fixes
- Internal refactoring

## Hardware Compatibility

### Assume Diversity

The plugin MUST gracefully handle:

- Different CPUs, GPUs, disk controllers
- Various UPS models and monitoring tools
- Multiple network interface types
- DMI/SMBIOS structure variations

### Defensive Parsing

- Fallback for unknown output formats
- Graceful handling of missing commands
- Default values for unavailable metrics
- Log warnings for unsupported hardware, but continue operation

## Security Posture

### Input Validation

- Whitelist validation, NEVER blacklist
- Validate before use, not just at UI
- Context-specific validation (paths, names, IDs)
- Use validation functions from `lib/validation.go`

### Command Execution

- Use library wrappers (`lib.ExecuteShellCommand`)
- Validate ALL parameters
- Log ALL control operations
- Principle of least privilege

## Code Quality Gates

### Before Committing

- `make test` MUST pass
- NO new linter warnings
- Security validation if touching user inputs
- Follows existing patterns in codebase

### Code Review Focus

- Security implications
- Thread safety (mutex usage, race conditions)
- Error handling completeness
- Test coverage for new code

## Non-Negotiables

These rules MUST NEVER be violated:

1. **Initialization order**: API subscriptions start BEFORE collectors
2. **Mutex discipline**: ALL cache access MUST be protected
3. **Input validation**: ALL user inputs MUST be validated before use
4. **Panic recovery**: ALL goroutines MUST have recovery
5. **Context respect**: ALL goroutines MUST honor context cancellation
6. **Semantic versioning**: Date-based releases (YYYY.MM.DD format)
7. **Linux/amd64 target**: Build for Unraid platform
8. **No external network dependencies**: Self-contained operation

## Decision Framework

When faced with implementation choices, prioritize in this order:

1. **Security** - Is it safe?
2. **Reliability** - Will it fail gracefully?
3. **Simplicity** - Is it the simplest solution?
4. **Performance** - Is it fast enough?
5. **Features** - Does it add value?

If a feature sacrifices security or reliability, it MUST be rejected or redesigned.

## Community Plugin Expectations

- This is **unofficial** - NEVER misrepresent as Unraid official product
- Complement, don't replace official APIs
- Document limitations clearly
- Accept that hardware variations WILL cause issues
- Community-driven fixes and enhancements welcomed

## Governance

### Constitution Authority

This constitution supersedes all other practices and documentation. When conflicts arise:

1. Constitution principles take precedence
2. CLAUDE.md provides implementation guidance
3. README.md documents user-facing features
4. Individual code comments provide local context

### Amendment Procedure

Changes to this constitution require:

1. Documentation of the proposed change and rationale
2. Review for impact on existing code and principles
3. Update to dependent templates (plan, spec, tasks)
4. Version increment per semantic versioning rules
5. Update of `LAST_AMENDED_DATE`

### Version Increment Rules

- **MAJOR** (X.0.0): Backward incompatible governance/principle removals or redefinitions
- **MINOR** (x.Y.0): New principle/section added or materially expanded guidance
- **PATCH** (x.y.Z): Clarifications, wording, typo fixes, non-semantic refinements

### Compliance Review

All PRs and code reviews MUST verify compliance with:

- Core Principles (I-V)
- Non-Negotiables (1-8)
- Security Posture
- Testing Requirements

Complexity that violates principles MUST be justified in PR description with:

- Why the violation is necessary
- What simpler alternatives were considered
- Why those alternatives are insufficient

### Runtime Development Guidance

For day-to-day development guidance, refer to `CLAUDE.md`. This file provides:

- Common patterns (adding collectors, endpoints, controllers)
- Architecture details and data flow
- Testing conventions
- Unraid integration specifics

**Version**: 1.0.0 | **Ratified**: 2025-11-27 | **Last Amended**: 2025-11-27
