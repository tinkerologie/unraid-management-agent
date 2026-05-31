package llm

import (
	"context"
	"fmt"
	"sync"
)

// MockProvider replays a fixed script of responses for deterministic tests.
type MockProvider struct {
	mu       sync.Mutex
	script   []*ChatResponse
	idx      int
	requests []ChatRequest
}

// NewMockProvider builds a mock that returns the given responses in order.
func NewMockProvider(responses ...*ChatResponse) *MockProvider {
	return &MockProvider{script: responses}
}

// Name identifies the provider.
func (m *MockProvider) Name() string { return "mock" }

// Chat returns the next scripted response and records the request.
func (m *MockProvider) Chat(_ context.Context, req ChatRequest) (*ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
	if m.idx >= len(m.script) {
		return nil, fmt.Errorf("mock provider exhausted after %d responses", len(m.script))
	}
	resp := m.script[m.idx]
	m.idx++
	return resp, nil
}

// Requests returns all requests received so far.
func (m *MockProvider) Requests() []ChatRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ChatRequest, len(m.requests))
	copy(out, m.requests)
	return out
}
