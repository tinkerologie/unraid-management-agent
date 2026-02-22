---
description: Step-by-step guide for adding a new control operation
tools: ["editor", "terminal"]
---

# Add a New Controller

Follow these steps to add a new control operation (Docker/VM/Array style).

## Step 1: Create the Controller

Create a new file in `daemon/services/controllers/` (or add to an existing one).

Follow the validate-execute-return pattern:

```go
package controllers

import (
    "unraid-management-agent/daemon/constants"
    "unraid-management-agent/daemon/lib"
)

type MyController struct{}

func NewMyController() *MyController {
    return &MyController{}
}

func (c *MyController) PerformAction(input string) error {
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

## Step 2: Add Input Validation

If the existing `lib.Validate*()` functions don't cover your input type, add a new validation function in `daemon/lib/validation.go`. Include:

- Length checks
- Character allowlists (not denylists)
- Path traversal protection
- Null byte rejection

## Step 3: Wire Up to API

1. Add the controller to the API server's dependencies
2. Create a handler in `daemon/services/api/handlers.go`
3. Register the route in `setupRoutes()`

## Step 4: Test

- Add tests for the controller in `*_test.go`
- Add security test cases: command injection, path traversal, empty input
- Add handler tests for the API endpoint

## Step 5: Document

- Update `CHANGELOG.md`
- Update API documentation
- Run `make swagger` if Swagger annotations were added
