---
applyTo: "**/*.go"
---

# Go Code Conventions

Reference: [`AGENTS.md`](../../AGENTS.md) for full project context.

## Style

- Standard Go: `gofmt` and `goimports` enforced
- Zero tolerance for linting errors (`golangci-lint`)
- PascalCase for exported names, camelCase for unexported
- Group imports: stdlib, external, internal (separated by blank lines)

## Error Handling

- Always wrap errors with context: `fmt.Errorf("doing X: %w", err)`
- Return errors up the call stack; don't swallow them silently
- Use `errors.Is()` / `errors.As()` for error checking
- Log at the point of handling, not at every intermediate return

## Context Propagation

- Pass `context.Context` as the first parameter to functions that need it
- Respect `ctx.Done()` in goroutines for graceful shutdown
- Never store context in a struct

## Security

- Use `lib.ExecCommand()` / `lib.ExecCommandOutput()` for shell commands — never `exec.Command`
- Validate all user input with `lib.Validate*()` functions from `daemon/lib/validation.go`
- No secrets in code; no hardcoded credentials

## Concurrency

- Use `sync.RWMutex` for shared cache: `RLock`/`RUnlock` for reads, `Lock`/`Unlock` for writes
- All goroutines must support context cancellation
- Collectors must wrap work in defer/recover for panic recovery
