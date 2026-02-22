---
applyTo: "**/*_test.go"
---

# Testing Instructions

Reference: [`AGENTS.md`](../../AGENTS.md) for full project context.

## Pattern: Table-Driven Tests

```go
func TestValidateInput(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"valid input", "abc123", false},
        {"empty input", "", true},
        {"path traversal", "../etc/passwd", true},
        {"null bytes", "test\x00evil", true},
        {"command injection", "test; rm -rf /", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateInput(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateInput(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
            }
        })
    }
}
```

## Security Test Cases

Always include these in validation tests:

- Path traversal: `../`, `..\\`, `/etc/passwd`
- Null bytes: `\x00`
- Command injection: `; rm -rf /`, `$(cmd)`, `` `cmd` ``
- Empty/blank input
- Excessively long input

## Conventions

- Tests are located alongside source files (`*_test.go`)
- Use `daemon/lib/testutil/` for shared test utilities
- Mock file system access and external commands
- Run with race detection: `make test` (includes `-race`)

## Commands

```bash
make test                                           # All tests
make test-coverage                                  # Coverage report
go test -v ./daemon/services/api/handlers_test.go   # Specific test
```
