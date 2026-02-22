---
applyTo: "daemon/services/controllers/**/*.go"
---

# Controller Instructions

Reference: [`AGENTS.md`](../../AGENTS.md) for full project context.

## Pattern: Validate-Execute-Return

Every controller operation follows this pattern:

```go
func (c *Controller) Action(input string) error {
    // 1. Validate input
    if err := lib.ValidateInput(input); err != nil {
        return err
    }
    // 2. Execute operation
    _, err := lib.ExecCommand(constants.SomeBin, "action", input)
    // 3. Return result
    return err
}
```

## Shell Command Safety

- **Always** use `lib.ExecCommand()` or `lib.ExecCommandOutput()` from `daemon/lib/shell.go`
- **Never** use `exec.Command` directly
- **Never** interpolate user input into shell command strings

## Input Validation

Use the appropriate validation function from `daemon/lib/validation.go`:

| Function | Use For |
|----------|---------|
| `ValidateContainerID()` | Docker container IDs |
| `ValidateVMName()` | VM names |
| `ValidateShareName()` | Share names |
| `ValidateConfigPath()` | File paths |
| `ValidateNotificationFilename()` | Notification filenames |

## Existing Controllers

- `docker.go` — Container start/stop/restart/pause/unpause
- `vm.go` — VM start/stop/restart/pause/resume/hibernate
- `array.go` — Array start/stop, parity check
- `notification.go` — Notification create/archive/delete
- `plugin.go` — Plugin management
- `process.go` — Process management
- `service.go` — Service management
