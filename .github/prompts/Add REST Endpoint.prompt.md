---
description: Step-by-step guide for adding a new REST API endpoint
tools: ["editor", "terminal"]
---

# Add a New REST Endpoint

Follow these steps to add a new REST API endpoint.

## Step 1: Determine Endpoint Type

- **Read-only (GET):** Returns cached data from a collector
- **Control (POST):** Executes an action via a controller

## Step 2: Add Handler

In `daemon/services/api/handlers.go`:

### For GET (cached data):

```go
func (s *Server) handleMyResource(w http.ResponseWriter, _ *http.Request) {
    s.cacheMutex.RLock()
    data := s.myResourceCache
    s.cacheMutex.RUnlock()
    respondJSON(w, http.StatusOK, data)
}
```

### For POST (control operation):

```go
func (s *Server) handleMyAction(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id := vars["id"]

    // Validate input
    if err := lib.ValidateContainerID(id); err != nil {
        respondJSON(w, http.StatusBadRequest, dto.Response{
            Status:  "error",
            Message: err.Error(),
        })
        return
    }

    // Execute action via controller
    if err := s.controller.MyAction(id); err != nil {
        respondJSON(w, http.StatusInternalServerError, dto.Response{
            Status:  "error",
            Message: err.Error(),
        })
        return
    }

    respondJSON(w, http.StatusOK, dto.Response{
        Status:  "success",
        Message: "Action completed",
    })
}
```

## Step 3: Register Route

In `daemon/services/api/server.go` `setupRoutes()`:

```go
router.HandleFunc("/api/v1/myresource", s.handleMyResource).Methods("GET")
// or
router.HandleFunc("/api/v1/myresource/{id}/{action}", s.handleMyAction).Methods("POST")
```

## Step 4: Add Swagger Annotations

Add Swagger comments above the handler function, then run `make swagger`.

## Step 5: Test

- Add handler tests in `handlers_test.go` or a new test file
- Test both success and error cases
- Include security test cases for any user input

## Step 6: Document

- Update `CHANGELOG.md`
- Update API documentation in `docs/api/`
