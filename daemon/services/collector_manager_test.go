package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

// mockCollector is a simple collector for testing
type mockCollector struct {
	started  bool
	interval time.Duration
	mu       sync.Mutex
}

func (m *mockCollector) Start(ctx context.Context, interval time.Duration) {
	m.mu.Lock()
	m.started = true
	m.interval = interval
	m.mu.Unlock()

	// Wait for context cancellation
	<-ctx.Done()

	m.mu.Lock()
	m.started = false
	m.mu.Unlock()
}

func createTestContext() *domain.Context {
	return &domain.Context{
		Hub: domain.NewEventBus(100),
		Intervals: domain.Intervals{
			System:       5,
			Array:        10,
			Disk:         30,
			Docker:       10,
			VM:           10,
			UPS:          0, // Disabled
			NUT:          0, // Disabled
			GPU:          10,
			Shares:       60,
			Network:      15,
			Hardware:     60,
			ZFS:          30,
			Notification: 30,
			Registration: 300,
			Unassigned:   60,
		},
	}
}

func TestNewCollectorManager(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	if cm == nil {
		t.Fatal("NewCollectorManager returned nil")
	}

	if cm.collectors == nil {
		t.Error("collectors map not initialized")
	}

	if cm.domainCtx != ctx {
		t.Error("domain context not set")
	}
}

func TestCollectorManager_Register(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	// Register a mock collector
	cm.Register("test", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 30, false)

	// Verify registration
	status, err := cm.GetStatus("test")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.Name != "test" {
		t.Errorf("expected name 'test', got '%s'", status.Name)
	}

	if status.Interval != 30 {
		t.Errorf("expected interval 30, got %d", status.Interval)
	}

	if status.Required {
		t.Error("expected required=false")
	}
}

func TestCollectorManager_RegisterRequired(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	// Register a required collector
	cm.Register("system", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 5, true)

	status, err := cm.GetStatus("system")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if !status.Required {
		t.Error("expected required=true for system collector")
	}
}

func TestCollectorManager_EnableDisable(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	// Register a collector that's initially disabled
	cm.Register("test", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 0, false)

	// Verify initially disabled
	status, _ := cm.GetStatus("test")
	if status.Enabled {
		t.Error("expected collector to be initially disabled")
	}

	// Enable the collector
	err := cm.EnableCollector("test")
	if err != nil {
		t.Fatalf("EnableCollector failed: %v", err)
	}

	// Give time for goroutine to start
	time.Sleep(50 * time.Millisecond)

	status, _ = cm.GetStatus("test")
	if !status.Enabled {
		t.Error("expected collector to be enabled")
	}
	if status.Status != "running" {
		t.Errorf("expected status 'running', got '%s'", status.Status)
	}

	// Disable the collector
	err = cm.DisableCollector("test")
	if err != nil {
		t.Fatalf("DisableCollector failed: %v", err)
	}

	// Give time for context cancellation
	time.Sleep(50 * time.Millisecond)

	status, _ = cm.GetStatus("test")
	if status.Enabled {
		t.Error("expected collector to be disabled")
	}
	if status.Status != "stopped" {
		t.Errorf("expected status 'stopped', got '%s'", status.Status)
	}
}

func TestCollectorManager_DisableRequired(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	// Register a required collector
	cm.Register("system", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 5, true)

	// Try to disable - should fail
	err := cm.DisableCollector("system")
	if err == nil {
		t.Error("expected error when disabling required collector")
	}

	if err.Error() != "cannot disable system collector (always required)" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCollectorManager_UnknownCollector(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	// Try to enable unknown collector
	err := cm.EnableCollector("unknown")
	if err == nil {
		t.Error("expected error for unknown collector")
	}

	// Try to disable unknown collector
	err = cm.DisableCollector("unknown")
	if err == nil {
		t.Error("expected error for unknown collector")
	}

	// Try to get status of unknown collector
	_, err = cm.GetStatus("unknown")
	if err == nil {
		t.Error("expected error for unknown collector")
	}

	// Try to update interval of unknown collector
	err = cm.UpdateInterval("unknown", 30)
	if err == nil {
		t.Error("expected error for unknown collector")
	}
}

func TestCollectorManager_UpdateInterval(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	// Register a collector
	cm.Register("test", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 30, false)

	// Update interval
	err := cm.UpdateInterval("test", 60)
	if err != nil {
		t.Fatalf("UpdateInterval failed: %v", err)
	}

	status, _ := cm.GetStatus("test")
	if status.Interval != 60 {
		t.Errorf("expected interval 60, got %d", status.Interval)
	}
}

func TestCollectorManager_UpdateIntervalBounds(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	cm.Register("test", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 30, false)

	// Try too small interval
	err := cm.UpdateInterval("test", 2)
	if err == nil {
		t.Error("expected error for interval < 5")
	}

	// Try too large interval
	err = cm.UpdateInterval("test", 4000)
	if err == nil {
		t.Error("expected error for interval > 3600")
	}

	// Valid interval bounds
	err = cm.UpdateInterval("test", 5)
	if err != nil {
		t.Errorf("expected no error for interval = 5: %v", err)
	}

	err = cm.UpdateInterval("test", 3600)
	if err != nil {
		t.Errorf("expected no error for interval = 3600: %v", err)
	}
}

func TestCollectorManager_GetAllStatus(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	// Register some collectors
	cm.Register("system", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 5, true)

	cm.Register("docker", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 10, false)

	cm.Register("gpu", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 0, false) // Disabled

	status := cm.GetAllStatus()

	if status.Total != 3 {
		t.Errorf("expected total 3, got %d", status.Total)
	}

	if status.EnabledCount != 2 {
		t.Errorf("expected enabled_count 2, got %d", status.EnabledCount)
	}

	if status.DisabledCount != 1 {
		t.Errorf("expected disabled_count 1, got %d", status.DisabledCount)
	}

	if status.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestCollectorManager_StartAll(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	// Register collectors with different states
	cm.Register("system", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 5, true)

	cm.Register("docker", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 10, false)

	cm.Register("gpu", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 0, false) // Disabled

	// Start all enabled collectors
	count := cm.StartAll()

	if count != 2 {
		t.Errorf("expected 2 collectors started, got %d", count)
	}

	// Give time for goroutines to start
	time.Sleep(50 * time.Millisecond)

	// Verify status
	systemStatus, _ := cm.GetStatus("system")
	if systemStatus.Status != "running" {
		t.Errorf("expected system status 'running', got '%s'", systemStatus.Status)
	}

	dockerStatus, _ := cm.GetStatus("docker")
	if dockerStatus.Status != "running" {
		t.Errorf("expected docker status 'running', got '%s'", dockerStatus.Status)
	}

	gpuStatus, _ := cm.GetStatus("gpu")
	if gpuStatus.Status == "running" {
		t.Error("expected gpu status to not be 'running'")
	}

	// Clean up
	cm.StopAll()
	time.Sleep(50 * time.Millisecond)
}

func TestCollectorManager_StopAll(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	// Register and start collectors
	cm.Register("system", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 5, true)

	cm.Register("docker", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 10, false)

	cm.StartAll()
	time.Sleep(50 * time.Millisecond)

	// Stop all
	cm.StopAll()
	time.Sleep(50 * time.Millisecond)

	// Verify all stopped
	systemStatus, _ := cm.GetStatus("system")
	if systemStatus.Status != "stopped" {
		t.Errorf("expected system status 'stopped', got '%s'", systemStatus.Status)
	}

	dockerStatus, _ := cm.GetStatus("docker")
	if dockerStatus.Status != "stopped" {
		t.Errorf("expected docker status 'stopped', got '%s'", dockerStatus.Status)
	}
}

func TestCollectorManager_GetCollectorNames(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	cm.Register("system", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 5, true)

	cm.Register("docker", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 10, false)

	names := cm.GetCollectorNames()

	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}

	// Check that both names are present (order may vary due to map iteration)
	hasSystem := false
	hasDocker := false
	for _, name := range names {
		if name == "system" {
			hasSystem = true
		}
		if name == "docker" {
			hasDocker = true
		}
	}

	if !hasSystem {
		t.Error("expected 'system' in names")
	}
	if !hasDocker {
		t.Error("expected 'docker' in names")
	}
}

func TestCollectorManager_IdempotentEnable(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	cm.Register("test", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 30, false)

	// Enable twice - should be idempotent
	err := cm.EnableCollector("test")
	if err != nil {
		t.Fatalf("First EnableCollector failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	err = cm.EnableCollector("test")
	if err != nil {
		t.Fatalf("Second EnableCollector failed: %v", err)
	}

	status, _ := cm.GetStatus("test")
	if status.Status != "running" {
		t.Errorf("expected status 'running', got '%s'", status.Status)
	}

	cm.StopAll()
}

func TestCollectorManager_IdempotentDisable(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	cm.Register("test", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 30, false)

	// Start first
	cm.EnableCollector("test")
	time.Sleep(50 * time.Millisecond)

	// Disable twice - should be idempotent
	err := cm.DisableCollector("test")
	if err != nil {
		t.Fatalf("First DisableCollector failed: %v", err)
	}

	err = cm.DisableCollector("test")
	if err != nil {
		t.Fatalf("Second DisableCollector failed: %v", err)
	}

	status, _ := cm.GetStatus("test")
	if status.Status != "stopped" {
		t.Errorf("expected status 'stopped', got '%s'", status.Status)
	}
}

func TestCollectorManager_DefaultInterval(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup

	cm := NewCollectorManager(ctx, &wg)

	// Register with 0 interval
	cm.Register("gpu", func(ctx *domain.Context) Collector {
		return &mockCollector{}
	}, 0, false)

	// Enable - should use default interval
	err := cm.EnableCollector("gpu")
	if err != nil {
		t.Fatalf("EnableCollector failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	status, _ := cm.GetStatus("gpu")
	if status.Interval <= 0 {
		t.Errorf("expected positive interval, got %d", status.Interval)
	}

	cm.StopAll()
}
